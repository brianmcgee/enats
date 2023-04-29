package sidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/brianmcgee/enats/pkg/web3"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"regexp"
	"strconv"
	"time"
)

func (s *Sidecar) processNewHeads(session *web3.Session) error {

	ci := session.ClientInfo

	subject := fmt.Sprintf("web3.newHeads.%d.%d", ci.NetworkId, ci.ChainId)

	regex := regexp.MustCompile(`.*"hash"\s*:\s*"(.*?)".*`)

	setSourceHeaders := func(msg *nats.Msg) {
		msg.Header.Add("networkId", strconv.FormatUint(ci.NetworkId, 10))
		msg.Header.Add("chainId", strconv.FormatUint(ci.ChainId, 10))
		msg.Header.Add("clientId", ci.ClientId)
		msg.Header.Add("clientVersion", ci.ClientVersion.String())
	}

	setMsgId := func(msg *nats.Msg) error {
		// set the msg id for de-duplication in JetStream
		matches := regex.FindSubmatch(msg.Data)
		if len(matches) != 2 {
			return errors.Errorf("unexpected number of hash matches: %d, expected %d", len(matches), 2)
		}
		msg.Header.Add(nats.MsgIdHdr, string(matches[1]))
		return nil
	}

	conn := s.nats

	// publish initial head

	msg := nats.NewMsg(subject)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initial, _ := session.LatestBlock(ctx)
	msg.Data = initial

	if err := setMsgId(msg); err != nil {
		return errors.Annotate(err, "failed to set msg id")
	}

	setSourceHeaders(msg)

	if err := conn.PublishMsg(msg); err != nil {
		// nats will reconnect and recover from certain issues, so we treat this as a possible transient error
		s.log.Error("failed to publish initial head", zap.Error(err))
	}

	// publish subsequent head updates
	for raw := range session.NewHeads() {
		msg := nats.NewMsg(subject)
		msg.Data = *raw

		if err := setMsgId(msg); err != nil {
			return errors.Annotate(err, "failed to set msg id")
		}

		setSourceHeaders(msg)

		if err := conn.PublishMsg(msg); err != nil {
			// nats will reconnect and recover from certain issues, so we treat this as a possible transient error
			s.log.Error("failed to publish new head", zap.Error(err))
		}

		var newHead web3.BlockHeader
		if err := json.Unmarshal(*raw, &newHead); err != nil {
			return errors.Annotate(err, "failed to unmarshal new head")
		}

		if err := s.handlers.Register(&newHead); err != nil {
			return errors.Annotate(err, "failed to register block rpc handlers")
		}
	}

	s.log.Debug("New heads channel has been closed")
	return nil
}
