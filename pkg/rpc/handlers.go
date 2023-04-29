package rpc

import (
	"encoding/json"
	"fmt"
	"github.com/brianmcgee/enats/pkg/storage"
	"github.com/brianmcgee/enats/pkg/web3"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"github.com/puzpuzpuz/xsync/v2"
	"go.uber.org/zap"
)

type Option func(opts *Options) error

type Options struct {
	Concurrency int
	QueueName   string
}

func Concurrency(concurrency int) Option {
	return func(opts *Options) error {
		opts.Concurrency = concurrency
		return nil
	}
}

func QueueName(name string) Option {
	return func(opts *Options) error {
		opts.QueueName = name
		return nil
	}
}

func GetDefaultOptions() Options {
	return Options{
		Concurrency: 32,
		QueueName:   "enats-sidecar",
	}
}

type blockSubs struct {
	number uint64
	hash   string
	subs   []*nats.Subscription
}

type Handlers struct {
	Options Options

	log     *zap.Logger
	conn    *nats.Conn
	session *web3.Session

	endpointPrefix string
	clientSubject  string

	blockIndex *storage.BlockIndex

	globalSubs   []*nats.Subscription
	subsByNumber *xsync.MapOf[uint64, *blockSubs]

	queue   chan *nats.Msg
	handler func(msg *nats.Msg)
}

func (r *Handlers) Init() error {

	ci := r.session.ClientInfo
	prefix := fmt.Sprintf("%s.%d.%d", r.endpointPrefix, ci.NetworkId, ci.ChainId)

	l := r.log.With(zap.String("subjectPrefix", prefix))
	l.Info("init")

	// register chain wide broadcast responder
	broadcastSub, err := r.conn.ChanSubscribe(prefix, r.queue)
	if err != nil {
		return errors.Annotate(err, "failed to register broadcast handler")
	}
	l.Debug("registered broadcast handler", zap.String("subject", broadcastSub.Subject))

	// register client specific rpc handler
	r.clientSubject = fmt.Sprintf("%s.client.%s", prefix, ci.ClientId)
	clientSub, err := r.conn.ChanSubscribe(r.clientSubject, r.queue)
	if err != nil {
		return errors.Annotate(err, "failed to register client specific handler")
	}
	l.Debug("registered client specific handler", zap.String("subject", clientSub.Subject))

	// register info handler
	infoSub, err := r.conn.Subscribe(fmt.Sprintf("%s.info", prefix), func(msg *nats.Msg) {
		bytes, err := json.Marshal(r.session.ClientInfo)
		if err != nil {
			l.Error("failed to marshal client info", zap.Error(err))
		}
		if err := msg.Respond(bytes); err != nil {
			l.Error("failed to respond with client info", zap.Error(err))
		}
	})
	if err != nil {
		return errors.Annotate(err, "failed to register client info handler")
	}
	l.Debug("registered client info handler", zap.String("subject", infoSub.Subject))

	// capture for closing later
	r.globalSubs = []*nats.Subscription{broadcastSub, clientSub, infoSub}

	// register a handler for every block index entry
	var count = 0
	err = r.blockIndex.ForEachBlock(func(block *web3.BlockHeader) error {
		count += 1
		return r.registerWithNats(block)
	})

	if err == nil {
		l.Info("init complete", zap.Int("blockCount", count))
	}

	return err
}

func (r *Handlers) ClientSubject() string {
	return r.clientSubject
}

func (r *Handlers) Register(block *web3.BlockHeader) error {
	// add to block index and record latest number
	_, err := r.blockIndex.Put(block)
	if err != nil {
		return errors.Annotate(err, "failed to update block index")
	}

	// register rpc handlers with nats
	return r.registerWithNats(block)
}

func (r *Handlers) registerWithNats(block *web3.BlockHeader) error {

	ci := r.session.ClientInfo
	queueName := r.Options.QueueName

	number, err := block.NumberUint64()
	if err != nil {
		return err
	}

	l := r.log.With(zap.Uint64("number", number), zap.String("hash", block.Hash))

	prefix := fmt.Sprintf("%s.%d.%d", r.endpointPrefix, ci.NetworkId, ci.ChainId)

	// register hash based handler
	hashSub, err := r.conn.ChanQueueSubscribe(
		fmt.Sprintf("%s.hash.%s", prefix, block.Hash[2:]),
		queueName,
		r.queue,
	)
	if err != nil {
		return err
	}
	l.Debug("registered handler", zap.String("subject", hashSub.Subject))

	// register hash based handler
	numberSub, err := r.conn.ChanQueueSubscribe(
		fmt.Sprintf("%s.number.%s", prefix, block.Number[2:]),
		queueName,
		r.queue,
	)
	if err != nil {
		return err
	}
	l.Debug("registered handler", zap.String("subject", numberSub.Subject))

	// record subs
	entry := &blockSubs{
		number: number,
		hash:   block.Hash,
		subs:   []*nats.Subscription{hashSub, numberSub},
	}
	r.subsByNumber.Store(number, entry)

	return nil
}

func (r *Handlers) Prune() (err error) {
	l := r.log
	l.Info("pruning")

	var size, removed int
	size, removed, err = r.blockIndex.Prune(func(block *web3.BlockHeader) {
		// decode number
		number, err := block.NumberUint64()
		if err != nil {
			l.Error("failed to decode block number", zap.Any("block", block))
		}

		// load + delete block subs
		blockSubs, ok := r.subsByNumber.LoadAndDelete(number)
		if !ok {
			return // nothing to do
		}

		// drain block subs
		for _, sub := range blockSubs.subs {
			if err := sub.Drain(); err != nil {
				l.Error("failed to drain subscription", zap.String("subject", sub.Subject))
			} else {
				l.Debug("drained subscription", zap.String("subject", sub.Subject))
			}
		}
	})
	if err == nil {
		l.Info("pruning complete", zap.Int("size", size), zap.Int("removed", removed))
	} else {
		l.Error("pruning failed", zap.Error(err))
	}
	return
}

func (r *Handlers) Shutdown() error {
	l := r.log
	l.Info("close")

	for _, sub := range r.globalSubs {
		subject := zap.String("subject", sub.Subject)
		if err := sub.Drain(); err != nil {
			l.Error("failure whilst draining sub", subject, zap.Error(err))
		} else {
			l.Info("drained sub", subject)
		}
	}

	r.subsByNumber.Range(func(_ uint64, v *blockSubs) bool {
		for _, sub := range v.subs {
			subject := zap.String("subject", sub.Subject)
			if err := sub.Drain(); err != nil {
				r.log.Error("failure whilst draining sub", subject, zap.Error(err))
			} else {
				l.Debug("drained sub", subject)
			}
		}
		return true
	})

	close(r.queue)

	l.Info("close complete")
	return nil
}

func (r *Handlers) HandleMsgs() error {
	for i := 0; i < r.Options.Concurrency; i++ {
		go func(id int) {
			l := r.log.With(zap.Int("handlerId", id))
			for msg := range r.queue {
				r.handler(msg)
			}
			l.Debug("msg queue closed, stopping")
		}(i + 1)
	}
	return nil
}

func NewHandlers(
	log *zap.Logger,
	conn *nats.Conn,
	session *web3.Session,
	endpointPrefix string,
	blockIndex *storage.BlockIndex,
	handler func(msg *nats.Msg),
	options ...Option) (*Handlers, error) {
	// process options
	opts := GetDefaultOptions()
	for _, opt := range options {
		if err := opt(&opts); err != nil {
			return nil, err
		}
	}

	return &Handlers{
		Options:        opts,
		log:            log,
		conn:           conn,
		session:        session,
		blockIndex:     blockIndex,
		endpointPrefix: endpointPrefix,
		subsByNumber:   xsync.NewIntegerMapOf[uint64, *blockSubs](),
		queue:          make(chan *nats.Msg, opts.Concurrency),
		handler:        handler,
	}, nil
}
