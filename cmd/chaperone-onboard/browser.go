// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// openBrowser opens the given URL in the user's default browser.
// On failure it returns an error; the caller should print the URL to stderr
// and continue waiting (not exit).
func openBrowser(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url) // #nosec G204 -- url is from CLI flags, not external input
	case "linux":
		cmd = exec.CommandContext(ctx, "xdg-open", url) // #nosec G204 -- url is from CLI flags, not external input
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", url) // #nosec G204 -- url is from CLI flags, not external input
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}

	return cmd.Start()
}
