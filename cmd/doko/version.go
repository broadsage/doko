package main

import (
	"fmt"
	"runtime"
)

// These variables are populated at build time using -ldflags.
var (
	version   = "v1.0.0-dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// showVersion outputs the version, commit SHA, and Go runtime details.
func showVersion() {
	fmt.Printf("Doko - BuildKit Image Orchestrator %s\n", version)
	fmt.Printf("Commit: %s\n", commit)
	fmt.Printf("BuildTime: %s\n", buildTime)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
