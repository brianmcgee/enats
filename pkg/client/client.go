package client

import (
	"context"
	"fmt"
	"github.com/brianmcgee/enats/pkg/tracking"
	"github.com/nats-io/nats.go"
	"gitlab.com/pjrpc/pjrpc/v2"
	"go.uber.org/zap"
	"sync/atomic"
)

type Client struct {
	NetworkId uint64
	ChainId   uint64

	conn *nats.Conn
	js   nats.JetStreamContext
	log  *zap.Logger

	ht *tracking.HeadTracker

	initCh   chan interface{}
	initFlag atomic.Bool

	router Router
}

func NewClient(networkId uint64, chainId uint64, conn *nats.Conn, js nats.JetStreamContext, log *zap.Logger) Client {
	ht := tracking.NewHeadTracker(networkId, chainId, js, log)
	return Client{
		NetworkId: networkId,
		ChainId:   chainId,
		conn:      conn,
		js:        js,
		log:       log,
		ht:        &ht,
		initCh:    make(chan interface{}, 1),
		router:    NewRouter(conn, &ht, fmt.Sprintf("web3.rpc.request.%d.%d", networkId, chainId), log),
	}
}

func (c *Client) Init(ctx context.Context) error {
	c.log.Info("init")
	err := c.ht.Init(ctx)
	if err == nil {
		c.log.Info("init complete")
	}
	return err
}

func (c *Client) Invoke(ctx context.Context, req *pjrpc.Request, resp *pjrpc.Response) error {
	resp.ID = req.ID
	c.router.Route(ctx, req, resp)
	return nil
}
