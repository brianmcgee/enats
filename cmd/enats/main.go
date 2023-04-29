package main

import (
	"github.com/alecthomas/kong"
	"github.com/brianmcgee/enats/internal/cmd"
)

func main() {
	ctx := kong.Parse(&cmd.Cli)
	ctx.FatalIfErrorf(cmd.BuildLogger())
	ctx.FatalIfErrorf(ctx.Run())
}
