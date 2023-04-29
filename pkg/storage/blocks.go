package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/brianmcgee/enats/pkg/web3"
	"github.com/juju/errors"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"sync/atomic"
)

const (
	ErrIntegrityCheckFailed = errors.ConstError("integrity check has failed")
)

type BlockIndex struct {
	Log       *zap.Logger
	Path      string
	Size      int
	ReOrgSize int
	ClientId  string
	Loader    func(number uint64) (*web3.BlockHeader, error)

	db *bolt.DB
}

type BlockIndexStats struct {
	Size           int
	EarliestNumber uint64
	EarliestHash   string
	LatestNumber   uint64
	LatestHash     string
}

func (b BlockIndexStats) String() string {
	return fmt.Sprintf("[count=%d] %s(%d) -> %s(%d)", b.Size, b.EarliestHash, b.EarliestNumber, b.LatestHash, b.LatestNumber)
}

func (bi *BlockIndex) Open() error {
	l := bi.Log.With(zap.String("path", bi.Path))

	l.Info("init")

	// open db
	db, err := bolt.Open(bi.Path, 0666, nil)
	if err != nil {
		return err
	}
	l.Debug("opened db")

	var stats *BlockIndexStats
	err = db.Update(func(tx *bolt.Tx) error {
		// create a bucket for tracking blocks by hash
		_, err = tx.CreateBucketIfNotExists([]byte(BucketNameBlocks))
		if err != nil {
			return errors.Annotate(err, "failed to create blocks bucket")
		}

		// create a bucket for tracking number -> hash
		_, err = tx.CreateBucketIfNotExists([]byte(BucketNameNumberIdx))
		if err != nil {
			return errors.Annotate(err, "failed to create numberIdx bucket")
		}

		// capture current stats
		stats, err = blockIndexStats(tx)
		return err
	})

	l.Info("init complete", zap.Any("stats", stats))

	// capture reference
	bi.db = db

	return nil
}

func (bi *BlockIndex) CheckIntegrity() (err error) {
	return bi.db.Update(func(tx *bolt.Tx) error {
		l := bi.Log
		cursor := bucketNumberIdx(tx).Cursor()

		var numberBytes, hashBytes []byte
		numberBytes, hashBytes = cursor.Last()

		if numberBytes == nil {
			// nothing to check
			return nil
		}

		// check in reverse order

		var checked = 0
		var deleted = 0

		for checked < bi.ReOrgSize && numberBytes != nil {

			number := mustDecodeNumber(numberBytes)
			block, err := bi.Loader(number)

			if err != nil {
				return errors.Annotate(err, "failed to load block")
			}

			if block == nil {
				l.Warn("block not found, deleting entry", zap.Uint64("number", number))
				if err = cursor.Delete(); err != nil {
					return err
				}
				deleted += 1
			} else if block.Hash != string(hashBytes) {
				l.Warn("block hash mismatch, deleting entry", zap.Uint64("number", number), zap.String("expected", string(hashBytes)), zap.String("actual", block.Hash))
				if err = cursor.Delete(); err != nil {
					return err
				}
				deleted += 1
			}

			checked += 1
			numberBytes, hashBytes = cursor.Prev()
		}

		if deleted == checked {
			return ErrIntegrityCheckFailed
		}

		return nil
	})
}

func (bi *BlockIndex) Stats() (stats *BlockIndexStats, err error) {
	err = bi.db.View(func(tx *bolt.Tx) error {
		stats, err = blockIndexStats(tx)
		return err
	})
	return stats, err
}

func blockIndexStats(tx *bolt.Tx) (*BlockIndexStats, error) {
	stats := &BlockIndexStats{}

	numberIdx := bucketNumberIdx(tx)
	cursor := numberIdx.Cursor()

	stats.Size = numberIdx.Stats().KeyN

	numberBytes, hashBytes := cursor.First()
	if numberBytes == nil {
		// no entries
		return stats, nil
	}

	stats.EarliestHash = string(hashBytes)
	stats.EarliestNumber = mustDecodeNumber(numberBytes)

	numberBytes, hashBytes = cursor.Last()
	stats.LatestHash = string(hashBytes)
	stats.LatestNumber = mustDecodeNumber(numberBytes)

	return stats, nil
}

func (bi *BlockIndex) ForEachBlock(fn func(block *web3.BlockHeader) error) error {
	return bi.db.View(func(tx *bolt.Tx) error {
		return bucketBlocks(tx).
			ForEach(func(_, blockBytes []byte) error {
				var block *web3.BlockHeader
				if err := json.Unmarshal(blockBytes, &block); err != nil {
					return err
				}
				return fn(block)
			})
	})
}

func (bi *BlockIndex) Put(block *web3.BlockHeader) (latest uint64, err error) {
	err = bi.db.Update(func(tx *bolt.Tx) error {
		err = putBlock(tx, block)
		if err != nil {
			return err
		}
		latestBlock, err := latestBlock(tx)
		if latestBlock != nil {
			latest, err = latestBlock.NumberUint64()
		}
		return err
	})
	return
}

func putBlock(tx *bolt.Tx, block *web3.BlockHeader) (err error) {
	blockBytes, err := json.Marshal(block)
	if err != nil {
		return errors.Annotate(err, "failed to marshal block")
	}

	// record block by hash
	if err := bucketBlocks(tx).Put([]byte(block.Hash), blockBytes); err != nil {
		return errors.Annotate(err, "failed to write byHash")
	}

	// record block hash for number, encoding it with BigEndian to ensure natural sort order
	numberIdx := bucketNumberIdx(tx)

	number := block.MustNumberUint64()
	numberBytes := mustEncodeNumber(number)

	if err := numberIdx.Put(numberBytes, []byte(block.Hash)); err != nil {
		return errors.Annotate(err, "failed to write to numberIdx")
	}

	return nil
}

func latestBlock(tx *bolt.Tx) (*web3.BlockHeader, error) {
	var block web3.BlockHeader

	cursor := bucketNumberIdx(tx).Cursor()

	_, hash := cursor.Last()
	if hash == nil {
		return nil, nil
	}

	blockBytes := bucketBlocks(tx).Get(hash)
	if blockBytes == nil {
		// if there's an entry in the number idx we should have the block by hash
		return nil, errors.Errorf("could not find block for number idx entry")
	}

	err := json.Unmarshal(blockBytes, &block)

	return &block, err
}

func (bi *BlockIndex) Prune(callback func(block *web3.BlockHeader)) (size int, removed int, err error) {
	l := bi.Log.With(zap.Int("maxSize", bi.Size))
	l.Info("pruning")
	err = bi.db.Update(func(tx *bolt.Tx) error {
		latest, err := latestBlock(tx)
		if latest == nil {
			return nil // nothing to do
		}

		removed, err = prune(l, tx, bi.Size, latest.MustNumberUint64(), callback)
		return err
	})

	var stats *BlockIndexStats
	stats, err = bi.Stats()

	if err != nil {
		return
	}
	size = stats.Size

	l.Info("pruning complete", zap.Int("size", stats.Size), zap.Int("removed", removed))
	return
}

func prune(
	log *zap.Logger,
	tx *bolt.Tx,
	size int,
	latest uint64,
	callback func(block *web3.BlockHeader),
) (int, error) {

	// determine cutoff
	var cutOff = int64(latest) - int64(size)
	if cutOff < 0 {
		cutOff = int64(0)
	}

	blocks := bucketBlocks(tx)
	cursor := bucketNumberIdx(tx).Cursor()

	var removed = 0
	var numberBytes, hashBytes []byte

	// fetch the first entry
	numberBytes, hashBytes = cursor.First()

	for {
		if numberBytes == nil {
			// no more entries
			break
		}

		number := mustDecodeNumber(numberBytes)

		log.Debug("examining entry",
			zap.Uint64("latest", latest),
			zap.Int64("cutOff", cutOff),
			zap.Uint64("number", number),
		)

		if number < uint64(cutOff) {
			// remove from numberIdx bucket
			if err := cursor.Delete(); err != nil {
				return removed, err
			}

			// check if there is a block entry
			blockBytes := blocks.Get(hashBytes)

			// only decode the block entry if a callback was provided
			if !(blockBytes == nil || callback == nil) {
				var block web3.BlockHeader
				if err := json.Unmarshal(blockBytes, &block); err != nil {
					return removed, errors.Annotate(err, "failed to unmarshal block")
				}
				callback(&block)
			}

			// delete the block entry
			if err := blocks.Delete(hashBytes); err != nil {
				return removed, errors.Annotate(err, "failed to delete block")
			}
			removed += 1
		} else {
			break
		}

		// iterate next
		numberBytes, hashBytes = cursor.Next()
	}

	return removed, nil
}

func (bi *BlockIndex) FillGaps(
	ctx context.Context,
	latestNumber uint64,
) error {

	var start = int64(latestNumber) - int64(bi.Size)
	if start < 0 {
		start = int64(0)
	}

	l := bi.Log.With(
		zap.Int64("start", start),
		zap.Uint64("end", latestNumber),
	)

	l.Info("filling gaps in history")

	fetched := atomic.Uint32{}

	blocks := make(chan *web3.BlockHeader, 128)
	g, ctx := errgroup.WithContext(ctx)

	writeBatch := func(blocks []*web3.BlockHeader) error {
		err := bi.db.Update(func(tx *bolt.Tx) error {
			for _, block := range blocks {
				if err := putBlock(tx, block); err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil {
			fetched.Add(uint32(len(blocks)))
		}
		return err
	}

	// process blocks
	g.Go(func() error {
		batchSize := 64

		var batch []*web3.BlockHeader
		for block := range blocks {
			batch = append(batch, block)
			if len(batch) == batchSize {
				if err := writeBatch(batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}

		// write final batch
		return writeBatch(batch)
	})

	// fetch blocks
	g.Go(func() error {
		for i := start; i <= int64(latestNumber); i++ {
			select {
			case <-ctx.Done():
				l.Warn("ctx cancelled")
				break
			default:
				err := bi.db.View(func(tx *bolt.Tx) error {

					numberIdx := bucketNumberIdx(tx)
					entry := numberIdx.Get(mustEncodeNumber(uint64(i)))

					if entry != nil {
						// we already have this block, do nothing
						return nil
					}
					block, err := bi.Loader(uint64(i))
					if err != nil {
						return err
					}
					blocks <- block
					return nil
				})

				if err != nil {
					return err
				}
			}
		}
		// indicate we are done fetching
		close(blocks)
		return nil
	})

	err := g.Wait()

	l.Info("filling gaps in history complete", zap.Uint32("fetched", fetched.Load()))

	return err
}

func (bi *BlockIndex) Close() error {
	return bi.db.Close()
}
