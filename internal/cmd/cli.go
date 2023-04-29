package cmd

import "go.uber.org/zap"

var logger *zap.Logger

var Cli struct {
	Log struct {
		Level       string `enum:"debug,info,warn,error" env:"LOG_LEVEL" default:"warn" help:"Configure logging level."`
		Development bool   `env:"LOG_DEVELOPMENT" default:"false" help:"Configure development style log output."`
	} `embed:"" prefix:"log-"`

	NatsUrl   string `name:"nats-url" env:"NATS_URL" default:"ns://127.0.0.1:4222" help:"NATS server url."`
	ClientUrl string `name:"client-url" env:"CLIENT_URL" default:"ws://127.0.0.1:8545" help:"Websocket url for connecting to a eth client."`

	Request requestCmd `cmd:"" help:"Send a request"`
	Sidecar sidecarCmd `cmd:"" help:"Run a sidecar." default:"1"`
	Version versionCmd `cmd:"" help:"Show version information."`
}

func BuildLogger() error {
	// configure logging
	var config zap.Config
	if Cli.Log.Development {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}

	// set log level
	switch {
	case Cli.Log.Level == "debug":
		config.Level.SetLevel(zap.DebugLevel)
	case Cli.Log.Level == "info":
		config.Level.SetLevel(zap.InfoLevel)
	case Cli.Log.Level == "warn":
		config.Level.SetLevel(zap.WarnLevel)
	case Cli.Log.Level == "error":
		config.Level.SetLevel(zap.ErrorLevel)
	}

	var err error
	logger, err = config.Build()

	return err
}
