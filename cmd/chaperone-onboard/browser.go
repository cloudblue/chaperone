// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
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
// block until the browser process exits. A context timeout would kill the
// browser itself in that case, so we avoid it entirely.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) // #nosec G204 -- url is from CLI flags, not external input
	case "linux":
		cmd = exec.Command("xdg-open", url) // #nosec G204 -- url is from CLI flags, not external input
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url) // #nosec G204 -- url is from CLI flags, not external input
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}

	return cmd.Start()
}
