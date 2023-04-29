package sidecar

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/brianmcgee/enats/pkg/rpc"
	"github.com/brianmcgee/enats/pkg/storage"
	"github.com/brianmcgee/enats/pkg/web3"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"golang.org/x/net/websocket"
	"golang.org/x/sync/errgroup"
	"net/http"
	url "net/url"
	"os"
	"time"
)

const (
	DefaultNatsURL            = "ns://127.0.0.1:4222"
	DefaultNatsQueueName      = "enats"
	DefaultNatsEndpointPrefix = "web3.rpc.request"
	DefaultClientURL          = "ws://127.0.0.1:8545"
	DefaultDataDir            = "./data"
	DefaultRpcHistorySize     = 7200
)

var (
	DefaultRpcDenyList = []string{"admin_.*"}
)

type Option func(opts *Options) error

type Options struct {
	NatsUrl            string
	NatsQueueName      string
	NatsEndpointPrefix string

	ClientUrl string
	ClientId  string

	DataDir string

	RpcDenyList    []string
	RpcHistorySize int
}

func NatsUrl(url string) Option {
	return func(opts *Options) error {
		opts.NatsUrl = url
		return nil
	}
}

func NatsEndpointPrefix(prefix string) Option {
	return func(opts *Options) error {
		opts.NatsEndpointPrefix = prefix
		return nil
	}
}

func ClientId(id string) Option {
	return func(opts *Options) error {
		opts.ClientId = id
		return nil
	}
}

func ClientUrl(url string) Option {
	return func(opts *Options) error {
		opts.ClientUrl = url
		return nil
	}
}

func RpcDenyList(deny []string) Option {
	return func(opts *Options) error {
		opts.RpcDenyList = deny
		return nil
	}
}

func RpcHistorySize(size int) Option {
	return func(opts *Options) error {
		opts.RpcHistorySize = size
		return nil
	}
}

func DataDir(path string) Option {
	return func(opts *Options) error {
		opts.DataDir = path
		return nil
	}
}

func GetDefaultOptions() Options {
	return Options{
		NatsUrl:            DefaultNatsURL,
		NatsQueueName:      DefaultNatsQueueName,
		NatsEndpointPrefix: DefaultNatsEndpointPrefix,
		ClientUrl:          DefaultClientURL,
		RpcDenyList:        DefaultRpcDenyList,
		RpcHistorySize:     DefaultRpcHistorySize,
		DataDir:            DefaultDataDir,
	}
}

type Sidecar struct {
	Options Options

	log  *zap.Logger
	nats *nats.Conn
	web3 *web3.Session

	handlers *rpc.Handlers
	bi       *storage.BlockIndex

	subject string

	errGroup errgroup.Group
}

func NewSidecar(log *zap.Logger, options ...Option) (*Sidecar, error) {
	// process options
	opts := GetDefaultOptions()
	for _, opt := range options {
		if err := opt(&opts); err != nil {
			return nil, err
		}
	}

	return &Sidecar{Options: opts, log: log}, nil
}

func (s *Sidecar) Init() error {

	if err := os.MkdirAll(s.Options.DataDir, os.ModePerm); err != nil {
		return errors.Annotate(err, "failed to create data directory")
	}

	if err := s.connectNats(); err != nil {
		return errors.Annotate(err, "failed to connect to NATS")
	}
	s.log.Info("connected to nats", zap.String("url", s.Options.NatsUrl))
	return nil
}

func (s *Sidecar) Run(ctx context.Context) error {

	log := s.log.With(zap.String("url", s.Options.ClientUrl))

	var retryDelay time.Duration

	resetRetryDelay := func() {
		retryDelay = 1 * time.Second
	}

	waitForReconnect := func() {
		log.Info("waiting before next connection attempt", zap.Duration("delay", retryDelay))
		time.Sleep(retryDelay)
		// increase retry delay for next time, capping it at 30 seconds
		retryDelay = retryDelay * 2
		if retryDelay == (32 * time.Second) {
			retryDelay = 30 * time.Second
		}
	}

	// set initial delay
	resetRetryDelay()

	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if err != nil && err != context.Canceled {
				s.log.Error("sidecar stopped with error", zap.Error(err))
				return err
			} else {
				s.log.Info("stopped sidecar")
				return nil
			}
		default:
			session, err := s.connectWeb3()
			s.web3 = session

			if err != nil {
				log.Warn("failed to connect to web3", zap.Error(err))
				waitForReconnect()
				continue
			}

			resetRetryDelay()

			log.Info("connected to web3 client")

			if err := s.RunSession(ctx, session); err != context.Canceled {
				s.log.Error("session ended with error", zap.Error(err))
			}

			// cleanup
			if err := session.Shutdown(); err != nil {
				s.log.Error("error when stopping web3 session", zap.Error(err))
			}

			s.web3 = nil

			s.log.Info("disconnected from web3 client")

			// wait before reconnecting
			if ctx.Err() != context.Canceled {
				waitForReconnect()
			}
		}
	}
}

func (s *Sidecar) RunSession(ctx context.Context, session *web3.Session) error {
	sessionCtx, sessionCancel := context.WithCancelCause(ctx)
	defer sessionCancel(nil)

	errGroup, errGroupCtx := errgroup.WithContext(sessionCtx)

	// start processing responses
	errGroup.Go(session.Listen)

	// fetch metadata about the client and initialise any subscriptions
	if err := session.Init(); err != nil {
		return errors.Annotate(err, "failed to initialise web3 session")
	}

	clientInfo := s.web3.ClientInfo

	//

	blockIndex := storage.BlockIndex{
		Log:       s.log,
		Path:      fmt.Sprintf("%s/%s.db", s.Options.DataDir, clientInfo.ClientId),
		Loader:    session.LoadBlock,
		ClientId:  clientInfo.ClientId,
		Size:      s.Options.RpcHistorySize,
		ReOrgSize: 12,
	}

	// load block index
	err := blockIndex.Open()
	if err != nil {
		return errors.Annotate(err, "failed to initialise block index")
	}

	// pre-emptive prune
	_, _, err = blockIndex.Prune(nil)
	if err != nil {
		return errors.Annotate(err, "failed to prune block index")
	}

	// index integrity check
	if err = blockIndex.CheckIntegrity(); err != nil {
		return err
	}

	// fetch current latest block number
	blockNumber, err := s.web3.BlockNumber(context.Background())
	if err != nil {
		return errors.Annotate(err, "failed to load block number")
	}

	// fill any missing entries that might have occurred whilst being stopped
	if err := blockIndex.FillGaps(sessionCtx, blockNumber); err != nil {
		return err
	}

	handlers, err := rpc.NewHandlers(
		s.log,
		s.nats,
		session,
		s.Options.NatsEndpointPrefix,
		&blockIndex,
		s.handleRpcRequest,
	)

	if err != nil {
		return errors.Annotate(err, "failed to initialise rpc handlers")
	}

	if err := handlers.Init(); err != nil {
		return errors.Annotate(err, "failed to initialise rpc handlers")
	}

	s.handlers = handlers
	s.bi = &blockIndex

	if err != nil {
		return err
	}

	// start processing new heads
	errGroup.Go(func() error {
		err := s.processNewHeads(session)
		// if new heads channel was closed it indicates the connection to the client was lost
		sessionCancel(err)
		return err
	})

	// start processing rpc requests
	errGroup.Go(func() error {
		return handlers.HandleMsgs()
	})

	// prune rpc handlers periodically
	errGroup.Go(func() error {
		interval := 1 * time.Minute
		var timeout <-chan time.Time

		for {
			timeout = time.After(interval)
			select {
			case <-sessionCtx.Done():
				return nil
			case <-timeout:
				if err := handlers.Prune(); err != nil {
					return err
				}
			}
		}
	})

	// wait for shutdown or an error
	<-errGroupCtx.Done()

	// cleanup
	if err = handlers.Shutdown(); err != nil {
		s.log.Error("error shutting down handlers", zap.Error(err))
	}
	if err = blockIndex.Close(); err != nil {
		s.log.Error("error closing block index", zap.Error(err))
	}

	return errGroupCtx.Err()
}

func (s *Sidecar) connectNats() error {
	var err error

	conn, err := nats.Connect(s.Options.NatsUrl)
	if err != nil {
		return errors.Annotate(err, "failed to connect to NATS")
	}

	s.nats = conn

	return nil
}

func (s *Sidecar) connectWeb3() (*web3.Session, error) {
	URL, err := url.Parse(s.Options.ClientUrl)
	if err != nil {
		return nil, errors.Annotate(err, "failed to parse client url")
	}

	config, err := websocket.NewConfig(s.Options.ClientUrl, s.Options.ClientUrl)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create websocket config")
	}

	if URL.User != nil {
		auth := base64.StdEncoding.EncodeToString([]byte(URL.User.String()))
		config.Header = http.Header{
			"Authorization": {fmt.Sprintf("Basic %s", auth)},
		}
	}

	ws, err := websocket.DialConfig(config)
	if err != nil {
		return nil, errors.Annotate(err, "failed to dial websocket")
	}

	return web3.NewSession(s.log, ws, s.Options.ClientId), nil
}
