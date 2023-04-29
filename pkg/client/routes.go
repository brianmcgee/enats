package client

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/juju/errors"
	"gitlab.com/pjrpc/pjrpc/v2"
)

const (
	ErrEmptyParams        = errors.ConstError("empty params")
	ErrInsufficientParams = errors.ConstError("insufficient params")
	ErrHeadUnavailable    = errors.ConstError("latest head is not available")
)

func defaultRoutes(router *Router) map[string]Handler {
	result := make(map[string]Handler)

	result["eth_chainId"] = ethChainId(router)
	result["eth_estimateGas"] = latestBlockHandler(router)
	result["eth_feeHistory"] = latestBlockHandler(router)
	result["eth_gasPrice"] = latestBlockHandler(router)
	result["eth_getLogs"] = latestBlockHandler(router) // todo this can perhaps be smarter about routing

	result["eth_blockNumber"] = ethBlockNumber(router)

	result["eth_call"] = blockNumberHandler(1, router)
	result["eth_getBalance"] = blockNumberHandler(1, router)
	result["eth_getBlockByNumber"] = blockNumberHandler(0, router)
	result["eth_getBlockReceipts"] = blockNumberHandler(0, router)
	result["eth_getBlockTransactionCountByNumber"] = blockNumberHandler(0, router)
	result["eth_getCode"] = blockNumberHandler(1, router)
	result["eth_getProof"] = optionalBlockNumberHandler(2, router)
	result["eth_getStorageAt"] = blockNumberHandler(2, router)

	result["eth_getBlockByHash"] = blockHashHandler(0, router)
	result["eth_getBlockTransactionCountByHash"] = blockHashHandler(0, router)
	result["eth_getTransactionByBlockHashAndIndex"] = blockHashHandler(0, router)
	result["eth_getTransactionByBlockNumberAndIndex"] = blockNumberHandler(0, router)
	result["eth_getTransactionByHash"] = latestBlockHandler(router)
	result["eth_getTransactionCount"] = blockNumberHandler(1, router)
	result["eth_getTransactionReceipt"] = latestBlockHandler(router)
	result["eth_getUncleCountByBlockHash"] = blockHashHandler(0, router)
	result["eth_getUncleCountByBlockNumber"] = blockNumberHandler(0, router)
	result["eth_maxPriorityFeePerGas"] = latestBlockHandler(router)
	result["eth_sendRawTransaction"] = latestBlockHandler(router)
	result["eth_signTransaction"] = latestBlockHandler(router)

	return result
}

func latestBlockHandler(router *Router) Handler {
	return func(ctx context.Context, req *pjrpc.Request, resp *pjrpc.Response) {
		head := router.ht.Head()
		if head == nil {
			resp.SetError(ErrHeadUnavailable)
			return
		}

		// sans 0x prefix
		number := head.Number[2:]
		router.invoke(ctx, fmt.Sprintf("head.%s", number), req, resp)
	}
}

func blockNumberHandler(idx int, router *Router) Handler {
	return func(ctx context.Context, req *pjrpc.Request, resp *pjrpc.Response) {
		number, err := blockNumberForRequest(idx, req, router)
		if err != nil {
			resp.SetError(err)
			return
		}
		subject := fmt.Sprintf("number.%s", number)
		router.invoke(ctx, subject, req, resp)
	}
}

func optionalBlockNumberHandler(idx int, router *Router) Handler {
	return func(ctx context.Context, req *pjrpc.Request, resp *pjrpc.Response) {
		var handler Handler
		if len(req.Params) <= idx {
			// default to latest block
			handler = latestBlockHandler(router)
		} else {
			handler = blockNumberHandler(idx, router)
		}
		handler(ctx, req, resp)
	}
}

func ethChainId(router *Router) Handler {
	return func(_ context.Context, _ *pjrpc.Request, resp *pjrpc.Response) {
		resp.SetResult(hexutil.EncodeUint64(router.ht.ChainId))
	}
}

func ethBlockNumber(router *Router) Handler {
	return func(_ context.Context, _ *pjrpc.Request, resp *pjrpc.Response) {
		head := router.ht.Head()
		if head == nil {
			resp.SetError(ErrHeadUnavailable)
		} else {
			resp.SetResult(head.Number)
		}
	}
}

func blockHashHandler(idx int, router *Router) Handler {
	return func(ctx context.Context, req *pjrpc.Request, resp *pjrpc.Response) {
		number, err := blockHashForRequest(idx, req)
		if err != nil {
			resp.SetError(err)
			return
		}
		subject := fmt.Sprintf("hash.%s", number)
		router.invoke(ctx, subject, req, resp)
	}
}

func blockNumberForRequest(idx int, req *pjrpc.Request, router *Router) (string, error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return "", err
	}

	if idx > (len(params) - 1) {
		return "", ErrInsufficientParams
	}

	// todo improve validation
	// typically the first param
	number := params[idx]
	switch number {
	case "latest":
		head := router.ht.Head()
		if head == nil {
			return "", ErrHeadUnavailable
		}
		return head.Number[2:], nil
	case "earliest":
		fallthrough
	case "pending":
		fallthrough
	case "safe":
		fallthrough
	case "finalized":
		return "", errors.Errorf("unsupported value: %s", number)
	default:
		// assume it is hex
		return number.(string)[2:], nil
	}
}

func blockHashForRequest(idx int, req *pjrpc.Request) (string, error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return "", err
	}

	if idx > (len(params) - 1) {
		return "", ErrInsufficientParams
	}
	return (params[idx].(string))[2:], nil
}
