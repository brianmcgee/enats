package cmd

import (
	"context"
	"github.com/brianmcgee/enats/pkg/sidecar"
	"github.com/ethereum/go-ethereum/log"
	"github.com/juju/errors"
	"os"
	"os/signal"
	"syscall"
)

type sidecarCmd struct {
	ClientId       string   `name:"client-id" env:"CLIENT_ID" help:"Unique id for this client. If not specified it will be derived from an 'admin_nodeInfo' API call."`
	DataDir        string   `name:"data-dir" env:"DATA_DIR" default:"./data" help:"Directory where the sidecar will store data about the client it is connected to.'"`
	RpcPrefix      string   `name:"rpc-prefix" env:"RPC_PREFIX" default:"web3.rpc.request" help:"Subject prefix for the JSON RPC handler."`
	RpcDenyList    []string `name:"rpc-deny-list" env:"RPC_DENY_LIST" default:"admin_.*" help:"A list of regex expressions used to determine which rpc calls to deny."`
	RpcHistorySize int      `name:"rpc-history-size" env:"RPC_HISTORY_SIZE" default:"128" help:"Number of blocks worth of data to make available."`
}

func (cmd *sidecarCmd) toOptions() ([]sidecar.Option, error) {
	return []sidecar.Option{
		sidecar.ClientUrl(Cli.ClientUrl),
		sidecar.NatsUrl(Cli.NatsUrl),
		sidecar.ClientId(cmd.ClientId),
		sidecar.NatsEndpointPrefix(cmd.RpcPrefix),
		sidecar.RpcDenyList(cmd.RpcDenyList),
		sidecar.RpcHistorySize(cmd.RpcHistorySize),
		sidecar.DataDir(cmd.DataDir),
	}, nil
}

func (cmd *sidecarCmd) Run() error {

	options, err := cmd.toOptions()
	if err != nil {
		return err
	}

	s, err := sidecar.NewSidecar(logger, options...)
	if err != nil {
		return errors.Annotate(err, "failed to create sidecar")
	}

	if err := s.Init(); err != nil {
		return errors.Annotate(err, "failed to initialise sidecar")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		log.Debug("listening for termination signals")
		c := make(chan os.Signal, 1) // we need to reserve to buffer size 1, so the notifier are not blocked
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		cancel()
	}()

	return s.Run(ctx)
}
