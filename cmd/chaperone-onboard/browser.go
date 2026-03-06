// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser opens the given URL in the user's default browser.
// On failure it returns an error; the caller should print the URL to stderr
// and continue waiting (not exit).
//
// The command is launched with Start (fire-and-forget) rather than Run
// because some implementations (e.g., xdg-open on certain Linux desktops)
// block until the browser process exits. We use context.Background() so
// the context never cancels — a real timeout would kill the browser itself.
func openBrowser(url string) error {
	ctx := context.Background()

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
