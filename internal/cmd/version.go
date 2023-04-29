package cmd

import (
	"fmt"
	"github.com/brianmcgee/enats/internal/build"
)

type versionCmd struct {
}

func (cmd *versionCmd) Run() error {
	fmt.Printf("%s %s", build.Name, build.Version)
	return nil
}
