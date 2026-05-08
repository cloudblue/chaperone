// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package chaperone

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cloudblue/chaperone/internal/config"
	"github.com/cloudblue/chaperone/internal/observability"
)

// TestConfigureLogging_LogTargetAddrWarnings verifies that configureLogging
// emits an INFO when log_target_addr is set to "path" and a loud WARN when
// set to "full". These notify the operator that path/query data may now
// appear in logs (per LITE-34062).
func TestConfigureLogging_LogTargetAddrWarnings(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		wantLevel      string
		wantSubstring  string
		shouldNotMatch string
	}{
		{
			name:           "host: no warning",
			mode:           "host",
			wantLevel:      "",
			wantSubstring:  "",
			shouldNotMatch: "target_addr logging",
		},
		{
			name:          "path: informational INFO",
			mode:          "path",
			wantLevel:     `"level":"INFO"`,
			wantSubstring: "target_addr logging set to 'path'",
		},
		{
			name:          "full: loud WARN",
			mode:          "full",
			wantLevel:     `"level":"WARN"`,
			wantSubstring: "target_addr logging set to 'full'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			rc := &runConfig{logOutput: &buf}
			cfg := &config.Config{}
			cfg.Observability.LogLevel = "info"
			cfg.Observability.LogTargetAddr = observability.TargetAddrMode(tt.mode)

			configureLogging(rc, cfg)

			out := buf.String()
			if tt.shouldNotMatch != "" && strings.Contains(out, tt.shouldNotMatch) {
				t.Errorf("host mode must not emit a target_addr warning, got: %s", out)
			}
			if tt.wantSubstring != "" {
				if !strings.Contains(out, tt.wantSubstring) {
					t.Errorf("expected log to contain %q, got: %s", tt.wantSubstring, out)
				}
				if !strings.Contains(out, tt.wantLevel) {
					t.Errorf("expected level %q, got: %s", tt.wantLevel, out)
				}
				if !strings.Contains(out, `"env_var":"CHAPERONE_OBSERVABILITY_LOG_TARGET_ADDR"`) {
					t.Errorf("expected env_var attribute in log, got: %s", out)
				}
			}
		})
	}
}
