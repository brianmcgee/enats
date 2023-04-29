package cmd

import (
	"context"
	"encoding/json"
	"github.com/brianmcgee/enats/pkg/client"
	"github.com/nats-io/nats.go"
	"gitlab.com/pjrpc/pjrpc/v2"
)

type requestCmd struct {
	NetworkId uint64          `name:"network-id" env:"NETWORK_ID" default:"1" help:"Ethereum network id."`
	ChainId   uint64          `name:"chain-id" env:"CHAIN_ID" default:"1" help:"Ethereum chain id."`
	Id        string          `name:"id" default:"1" help:"Request id. If not specified one will be generated."`
	Method    string          `arg:"" help:"Request method."`
	Params    json.RawMessage `arg:"" default:"" help:"Json encoded params array."`
}

func (cmd *requestCmd) Run() error {

	conn, err := nats.Connect(Cli.NatsUrl, nats.CustomInboxPrefix("web3.rpc.response"))
	if err != nil {
		return err
	}

	js, err := conn.JetStream()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := client.NewClient(Cli.Request.NetworkId, Cli.Request.ChainId, conn, js, logger)
	if err = c.Init(ctx); err != nil {
		return err
	}

	req, err := pjrpc.NewRequest(Cli.Request.Id, Cli.Request.Method, Cli.Request.Params)
	if err != nil {
		return err
	}

	var resp pjrpc.Response
	err = c.Invoke(ctx, req, &resp)
	if err != nil {
		return err
	}

	println(string(resp.Result))
	return nil
}
