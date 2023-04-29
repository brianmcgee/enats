package client

import (
	"context"
	"fmt"
	"github.com/brianmcgee/enats/pkg/rpc"
	"github.com/brianmcgee/enats/pkg/tracking"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"gitlab.com/pjrpc/pjrpc/v2"
	"go.uber.org/zap"
)

const (
	ErrUnsupportedMethod = errors.ConstError("unsupported method")
)

type Handler = func(ctx context.Context, req *pjrpc.Request, resp *pjrpc.Response)

type Router struct {
	SubjectPrefix string

	conn *nats.Conn
	ht   *tracking.HeadTracker
	log  *zap.Logger

	handlers map[string]Handler
}

func (r *Router) Route(ctx context.Context, req *pjrpc.Request, resp *pjrpc.Response) {
	route := r.handlers[req.Method]
	if route == nil {
		resp.SetError(ErrUnsupportedMethod)
		return
	}
	route(ctx, req, resp)
}

func (r *Router) invoke(ctx context.Context, subject string, req *pjrpc.Request, resp *pjrpc.Response) {
	msg := nats.NewMsg(fmt.Sprintf("%s.%s", r.SubjectPrefix, subject))
	msg.Header.Set(rpc.HeaderMethod, req.Method)
	msg.Data = req.Params

	reply, err := r.conn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		resp.SetError(err)
		return
	}

	errResp := reply.Header.Get(rpc.HeaderErr)
	if errResp != "" {
		resp.SetError(errors.New(errResp))
		return
	}

	resp.Result = reply.Data
}

func NewRouter(conn *nats.Conn, ht *tracking.HeadTracker, subjectPrefix string, log *zap.Logger) Router {

	router := Router{
		SubjectPrefix: subjectPrefix,
		conn:          conn,
		ht:            ht,
		log:           log,
	}

	router.handlers = defaultRoutes(&router)

	return router
}
