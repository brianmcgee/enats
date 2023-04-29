package web3

import (
	"context"
	"encoding/json"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/juju/errors"
	"gitlab.com/pjrpc/pjrpc/v2"
	"gitlab.com/pjrpc/pjrpc/v2/client"
	"go.uber.org/zap"
	"golang.org/x/net/websocket"
	"strconv"
	"sync/atomic"
	"time"
)

type Session struct {
	log    *zap.Logger
	conn   *websocket.Conn
	client *client.AsyncClient

	clientId  string
	requestId *uint64

	newHeads chan *json.RawMessage

	ClientInfo *ClientInfo
}

func NewSession(log *zap.Logger, conn *websocket.Conn, clientId string) *Session {
	c := client.NewAsyncClient(conn)

	requestId := uint64(0)
	return &Session{
		log:       log,
		conn:      conn,
		client:    c,
		clientId:  clientId,
		requestId: &requestId,
		newHeads:  make(chan *json.RawMessage, 16),
	}
}

func (s *Session) Listen() error {
	// ensure channel is closed on exit
	defer close(s.newHeads)

	// log out any parse errors
	s.client.OnParseMessageError = func(err error) error {
		s.log.Error("Parse error", zap.Error(err))
		return err
	}

	// start listening for responses
	return s.client.Listen()
}

func (s *Session) Init() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error

	netVersion, err := s.netVersion(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to retrieve net version")
	}

	chainId, err := s.ethChainId(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to retrieve chain id")
	}

	clientVersion, err := s.web3ClientVersion(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to retrieve client version")
	}

	s.ClientInfo = &ClientInfo{
		NetworkId:     netVersion,
		ChainId:       chainId,
		ClientId:      s.clientId,
		ClientVersion: *clientVersion,
	}

	// if client id has not been provided, attempt to derive it from admin_nodeInfo
	if s.clientId == "" {
		nodeInfo, err := s.adminNodeInfo(ctx)
		if err != nil {
			return errors.Annotate(err, "failed to retrieve node info")
		}
		s.ClientInfo.ClientId = nodeInfo.Id
	}

	// set notification handler
	s.client.OnRequest = s.onNewHead

	// subscribe for new head notifications
	_, err = s.subscribeToNewHeads(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to subscribe to new heads")
	}

	return nil
}

func (s *Session) NewHeads() <-chan *json.RawMessage {
	return s.newHeads
}

func (s *Session) LoadBlock(number uint64) (*BlockHeader, error) {
	s.log.Debug("loading block", zap.Uint64("number", number))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	raw, err := s.BlockByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	var b *BlockHeader
	err = json.Unmarshal(raw, &b)
	return b, err
}

func (s *Session) nextId() string {
	return strconv.FormatUint(atomic.AddUint64(s.requestId, 1), 10)
}

func (s *Session) netVersion(ctx context.Context) (uint64, error) {
	var str string
	if err := s.client.Invoke(ctx, s.nextId(), "net_version", nil, &str); err != nil {
		return 0, err
	}
	return strconv.ParseUint(str, 10, 64)
}

func (s *Session) ethChainId(ctx context.Context) (uint64, error) {
	var hex string
	if err := s.client.Invoke(ctx, s.nextId(), "eth_chainId", nil, &hex); err != nil {
		return 0, err
	}
	return hexutil.DecodeUint64(hex)
}

func (s *Session) adminNodeInfo(ctx context.Context) (*NodeInfo, error) {
	var result NodeInfo
	if err := s.client.Invoke(ctx, s.nextId(), "admin_nodeInfo", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Session) web3ClientVersion(ctx context.Context) (*ClientVersion, error) {
	var result string
	if err := s.client.Invoke(ctx, s.nextId(), "web3_clientVersion", nil, &result); err != nil {
		return nil, err
	}
	clientVersion, err := ParseClientVersion(result)
	return &clientVersion, err
}

func (s *Session) BlockNumber(ctx context.Context) (uint64, error) {
	var result string
	if err := s.client.Invoke(ctx, s.nextId(), "eth_blockNumber", nil, &result); err != nil {
		return 0, err
	}
	return hexutil.DecodeUint64(result)
}

func (s *Session) BlockByNumber(ctx context.Context, number uint64) (json.RawMessage, error) {
	var result json.RawMessage
	if err := s.client.Invoke(ctx, s.nextId(), "eth_getBlockByNumber", []interface{}{hexutil.EncodeUint64(number), false}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Session) LatestBlock(ctx context.Context) (json.RawMessage, error) {
	var result json.RawMessage
	if err := s.client.Invoke(ctx, s.nextId(), "eth_getBlockByNumber", []interface{}{"latest", false}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Session) subscribe(ctx context.Context, params []interface{}) (result string, err error) {
	err = s.client.Invoke(ctx, s.nextId(), "eth_subscribe", params, &result)
	return result, err
}

func (s *Session) subscribeToNewHeads(ctx context.Context) (string, error) {
	return s.subscribe(ctx, []any{"newHeads"})
}

func (s *Session) onNewHead(req *pjrpc.Request) error {
	// check method
	if req.Method != MethodEthSubscription {
		return errors.Errorf("expected method '%s', found: '%s'", MethodEthSubscription, req.Method)
	}

	// unmarshal the notification payload
	var notification SubscriptionNotification
	if err := json.Unmarshal(req.Params, &notification); err != nil {
		return errors.Annotate(err, "failed to unmarshal notification")
	}

	// publish new head
	s.newHeads <- &notification.Result

	return nil
}

func (s *Session) InvokeRequest(ctx context.Context, req *pjrpc.Request, resp *json.RawMessage) error {
	s.log.Debug("invoking request", zap.String("id", req.GetID()), zap.String("method", req.Method))
	return s.Invoke(ctx, req.GetID(), req.Method, req.Params, resp)
}

func (s *Session) Invoke(ctx context.Context, id, method string, params, result interface{}) error {
	return s.client.Invoke(ctx, id, method, params, result)
}

func (s *Session) Shutdown() error {
	return s.client.Close()
}
