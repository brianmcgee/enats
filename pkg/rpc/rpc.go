package rpc

import (
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
)

const (
	HeaderMethod = "method"
	HeaderErr    = "error"

	ErrMethodHeaderNotFound = errors.ConstError("method header not found")
)

func RespondWithError(msg *nats.Msg, err error) error {
	resp := nats.NewMsg(msg.Reply)
	resp.Header.Set(HeaderErr, err.Error())
	return resp.Respond([]byte{})
}
