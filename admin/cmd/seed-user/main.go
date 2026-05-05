// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

// seed-user is a test-only tool that creates a user in the admin portal
// database without interactive terminal input. Used by E2E tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/cloudblue/chaperone/admin/auth"
	"github.com/cloudblue/chaperone/admin/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dbPath := flag.String("db", "", "Path to SQLite database")
	username := flag.String("username", "", "Username to create")
	password := flag.String("password", "", "Password for the user")
	flag.Parse()

	if *dbPath == "" || *username == "" || *password == "" {
		return fmt.Errorf("usage: seed-user --db <path> --username <name> --password <pass>")
	}

	ctx := context.Background()

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer st.Close()

	svc := auth.NewService(st, 24*time.Hour, 2*time.Hour)

	if err := svc.CreateUser(ctx, *username, *password); err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	fmt.Printf("User %q created successfully\n", *username)
	return nil
}
