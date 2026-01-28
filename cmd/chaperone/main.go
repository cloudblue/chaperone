// Copyright 2024-2026 CloudBlue
// SPDX-License-Identifier: Apache-2.0

// Package main is the entry point for the Chaperone egress proxy.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/cloudblue/chaperone/internal/proxy"
	"github.com/cloudblue/chaperone/plugins/reference"
	"github.com/cloudblue/chaperone/sdk"
)

// Version information (set via ldflags during build)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	// Parse command line flags
	addr := flag.String("addr", ":8080", "Address to listen on")
	credFile := flag.String("credentials", "", "Path to credentials JSON file (optional)")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	// Show version and exit
	if *showVersion {
		fmt.Printf("Chaperone Egress Proxy\n")
		fmt.Printf("Version: %s\nCommit: %s\nBuilt: %s\n", Version, GitCommit, BuildDate)
		os.Exit(0)
	}

	// Configure logging
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("starting chaperone",
		"version", Version,
		"commit", GitCommit,
		"build_date", BuildDate,
	)

	// Configure plugin (optional)
	var plugin sdk.Plugin
	if *credFile != "" {
		plugin = reference.New(*credFile)
		slog.Info("loaded reference plugin", "credentials_file", *credFile)
	} else {
		slog.Warn("no credentials file specified, running without credential injection")
	}

	// Create and start server
	srv := proxy.NewServer(proxy.Config{
		Addr:    *addr,
		Plugin:  plugin,
		Version: Version,
	})

	slog.Info("server listening", "addr", *addr)
	if err := srv.Start(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
