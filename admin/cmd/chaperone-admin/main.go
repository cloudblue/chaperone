// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/cloudblue/chaperone/admin"
	"github.com/cloudblue/chaperone/admin/auth"
	"github.com/cloudblue/chaperone/admin/config"
	"github.com/cloudblue/chaperone/admin/metrics"
	"github.com/cloudblue/chaperone/admin/poller"
	"github.com/cloudblue/chaperone/admin/store"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		switch os.Args[1] {
		case "create-user":
			return runCreateUser(os.Args[2:])
		case "reset-password":
			return runResetPassword(os.Args[2:])
		case "serve":
			return runServer(os.Args[2:])
		default:
			return fmt.Errorf("unknown command %q (available: serve, create-user, reset-password)", os.Args[1])
		}
	}
	return runServer(os.Args[1:])
}

func runServer(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: chaperone-admin [command] [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  serve           Start the admin portal server (default)\n")
		fmt.Fprintf(os.Stderr, "  create-user     Create a new admin user\n")
		fmt.Fprintf(os.Stderr, "  reset-password  Reset a user's password\n")
		fmt.Fprintf(os.Stderr, "\nServer flags:\n")
		fs.PrintDefaults()
	}

	configPath := fs.String("config", "", "Path to config file (default: chaperone-admin.yaml)")
	showVersion := fs.Bool("version", false, "Print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("chaperone-admin %s (commit: %s, built: %s)\n", Version, GitCommit, BuildDate)
		return nil
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	configureLogging(cfg)

	slog.Info("starting chaperone-admin", "version", Version, "commit", GitCommit, "built", BuildDate)

	st, err := store.Open(context.Background(), cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer st.Close()

	collector := metrics.NewCollector(metrics.DefaultCapacity)

	srv, err := admin.NewServer(cfg, st, collector)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	// Start background goroutines.
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	p := poller.New(st, collector, cfg.Scraper.Interval.Unwrap(), cfg.Scraper.Timeout.Unwrap())
	go p.Run(bgCtx)
	go cleanupExpiredSessions(bgCtx, st)
	go sweepRateLimiter(bgCtx, srv)

	return serve(cfg.Server.Addr, srv)
}

func runCreateUser(args []string) error {
	fs := flag.NewFlagSet("create-user", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to config file")
	username := fs.String("username", "", "Username for the new user")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *username == "" {
		return fmt.Errorf("--username is required")
	}

	password, err := readPasswordConfirm("Password: ", "Confirm password: ")
	if err != nil {
		return err
	}

	svc, cleanup, err := openAuthService(*configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := svc.CreateUser(context.Background(), *username, password); err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	fmt.Fprintf(os.Stderr, "User %q created successfully.\n", *username)
	return nil
}

func runResetPassword(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to config file")
	username := fs.String("username", "", "Username to reset")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *username == "" {
		return fmt.Errorf("--username is required")
	}

	password, err := readPasswordConfirm("New password: ", "Confirm password: ")
	if err != nil {
		return err
	}

	svc, cleanup, err := openAuthService(*configPath)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := svc.ResetPassword(context.Background(), *username, password); err != nil {
		return fmt.Errorf("resetting password: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Password for %q has been reset. All existing sessions invalidated.\n", *username)
	return nil
}

func openAuthService(configPath string) (*auth.Service, func(), error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading configuration: %w", err)
	}

	st, err := store.Open(context.Background(), cfg.Database.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening database: %w", err)
	}

	svc := auth.NewService(st, cfg.Session.MaxAge.Unwrap(), cfg.Session.IdleTimeout.Unwrap())
	cleanup := func() {
		if err := st.Close(); err != nil {
			slog.Error("closing database", "error", err)
		}
	}
	return svc, cleanup, nil
}

func readPasswordConfirm(prompt, confirmPrompt string) (string, error) {
	password, err := readPassword(prompt)
	if err != nil {
		return "", err
	}
	confirm, err := readPassword(confirmPrompt)
	if err != nil {
		return "", err
	}
	if password != confirm {
		return "", fmt.Errorf("passwords do not match")
	}
	return password, nil
}

func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd())) // #nosec G115 -- stdin fd is always 0
	fmt.Fprintln(os.Stderr)                                // newline after hidden input
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return string(password), nil
}

func cleanupExpiredSessions(ctx context.Context, st *store.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := st.DeleteExpiredSessions(ctx)
			if err != nil {
				slog.Error("cleaning up expired sessions", "error", err)
			} else if n > 0 {
				slog.Info("cleaned up expired sessions", "count", n)
			}
		}
	}
}

func sweepRateLimiter(ctx context.Context, srv *admin.Server) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			srv.SweepRateLimiter()
		}
	}
}

func serve(addr string, srv *admin.Server) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server error: %w", err)
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down server: %w", err)
		}
		return nil
	}
}

func configureLogging(cfg *config.Config) {
	var level slog.Level
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Log.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
