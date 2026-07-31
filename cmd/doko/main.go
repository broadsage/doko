// Package main is the entrypoint for the Doko BuildKit frontend CLI.
package main

import (
	"fmt"
	"os"

	"github.com/moby/buildkit/frontend/gateway/grpcclient"
	"github.com/moby/buildkit/util/appcontext"

	"github.com/broadsage/doko/internal/builder"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		showVersion()
		os.Exit(0)
	}

	if err := grpcclient.RunFromEnvironment(appcontext.Context(), builder.Build); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "doko: fatal error: %v\n", err)
		os.Exit(1)
	}
}
