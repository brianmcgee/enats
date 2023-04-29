package sidecar

import (
	"context"
	"encoding/json"
	"github.com/brianmcgee/enats/pkg/rpc"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"time"
)

func (s *Sidecar) setMsgMetadata(msg *nats.Msg) {
	ci := s.web3.ClientInfo
	h := msg.Header
	h.Set("clientSubject", s.handlers.ClientSubject())
	h.Set("clientVersion", ci.ClientVersion.String())
}

func (s *Sidecar) handleRpcRequest(msg *nats.Msg) {
	log := s.log.With(
		zap.String("subject", msg.Subject),
		zap.String("reply", msg.Reply),
	)

	// extract method from header
	method := msg.Header.Get(rpc.HeaderMethod)
	if method == "" {
		_ = rpc.RespondWithError(msg, rpc.ErrMethodHeaderNotFound)
		return
	}

	// extract params from body
	var params []interface{}
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &params); err != nil {
			_ = rpc.RespondWithError(msg, errors.Annotate(err, "failed to unmarshal params"))
			return
		}
	}

	log.Debug("handling rpc request", zap.String("method", method))

	// todo make timeout configurable
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// invoke against the web3 session
	var result json.RawMessage
	err := s.web3.Invoke(ctx, msg.Reply, method, params, &result)
	if err != nil {
		log.Error("invocation failure", zap.Error(err))
		_ = rpc.RespondWithError(msg, err)
		return
	}

	// reply
	replyMsg := nats.NewMsg(msg.Reply)
	replyMsg.Data = result

	// set this client's subject as the reply if the requester wants to follow up with this same node
	replyMsg.Reply = s.handlers.ClientSubject()

	// set additional metadata
	s.setMsgMetadata(replyMsg)

	// respond
	if err = msg.RespondMsg(replyMsg); err != nil {
		log.Error("failed to respond to msg", zap.Error(err))
	}
}
