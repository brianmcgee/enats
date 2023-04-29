package tracking

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/brianmcgee/enats/pkg/web3"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"sync/atomic"
	"time"
)

type HeadTracker struct {
	NetworkId uint64
	ChainId   uint64

	js  nats.JetStreamContext
	log *zap.Logger

	sub    *nats.Subscription
	latest atomic.Pointer[web3.BlockHeader]

	init   bool
	initCh chan interface{}
}

func NewHeadTracker(networkId uint64, chainId uint64, js nats.JetStreamContext, log *zap.Logger) HeadTracker {
	return HeadTracker{
		NetworkId: networkId,
		ChainId:   chainId,
		js:        js,
		log:       log,

		initCh: make(chan interface{}),
	}
}

func (ht *HeadTracker) Head() *web3.BlockHeader {
	return ht.latest.Load()
}

func (ht *HeadTracker) Init(ctx context.Context) error {
	l := ht.log
	l.Info("init")

	_, err := ht.js.AddStream(&nats.StreamConfig{
		Name:       "web3_newHeads",
		Subjects:   []string{"web3.newHeads.>"},
		Duplicates: 10 * time.Minute,
	})

	if err != nil {
		return errors.Annotate(err, "failed to initialise new heads stream")
	}

	startTime := time.Now().Add(-1 * time.Minute)
	subject := fmt.Sprintf("web3.newHeads.%d.%d", ht.NetworkId, ht.ChainId)
	sub, err := ht.js.Subscribe(subject, ht.onNewHead, nats.StartTime(startTime))

	ht.sub = sub

	select {
	case <-ctx.Done():
		l.Warn("init cancelled", zap.Error(ctx.Err()))
		return ctx.Err()
	case <-ht.initCh:
		l.Info("init complete")
		return nil
	}
}

func (ht *HeadTracker) Shutdown() error {
	if ht.sub != nil {
		return ht.sub.Drain()
	}
	return nil
}

func (ht *HeadTracker) onNewHead(msg *nats.Msg) {
	l := ht.log

	var head web3.BlockHeader
	if err := json.Unmarshal(msg.Data, &head); err != nil {
		l.Error("failed to unmarshal head", zap.Error(err))
		return
	}

	number, err := head.NumberUint64()
	if err != nil {
		l.Error("failed to decode block number", zap.Error(err))
		return
	}

	latest := ht.latest.Load()

	if latest == nil || latest.MustNumberUint64() < number {
		l.Debug("latest head update", zap.Any("head", head))
		ht.latest.Store(&head)

		// get js metadata
		meta, err := msg.Metadata()
		if err != nil {
			l.Error("failed to retrieve jet stream metadata from msg", zap.Error(err))
		}

		if meta.NumPending == 0 && !ht.init {
			// signal that we have caught up with the new heads subscription for the first time
			ht.initCh <- nil
			ht.init = true
		}
	}
}
