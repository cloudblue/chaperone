// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package poller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/cloudblue/chaperone/admin/metrics"
	"github.com/cloudblue/chaperone/admin/store"
)

const (
	failuresUntilUnreachable = 3
	maxJitter                = time.Second
)

// ProbeResult holds the outcome of a single proxy probe.
type ProbeResult struct {
	OK      bool   `json:"ok"`
	Health  string `json:"health,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Probe performs a one-off health and version check against a proxy admin port.
func Probe(ctx context.Context, client *http.Client, address string) ProbeResult {
	health, err := fetchHealth(ctx, client, address)
	if err != nil {
		return ProbeResult{OK: false, Error: friendlyError(err)}
	}

	version, err := fetchVersion(ctx, client, address)
	if err != nil {
		return ProbeResult{OK: false, Error: friendlyError(err)}
	}

	return ProbeResult{OK: true, Health: health, Version: version}
}

// Poller periodically polls registered proxy instances for health and version.
type Poller struct {
	store     *store.Store
	collector *metrics.Collector
	client    *http.Client
	interval  time.Duration
	timeout   time.Duration

	mu       sync.Mutex
	failures map[int64]int // instance ID → consecutive failure count
}

// New creates a Poller with the given configuration.
// If collector is non-nil, each successful poll also scrapes /metrics.
func New(st *store.Store, collector *metrics.Collector, interval, timeout time.Duration) *Poller {
	return &Poller{
		store:     st,
		collector: collector,
		client:    &http.Client{Timeout: timeout},
		interval:  interval,
		timeout:   timeout,
		failures:  make(map[int64]int),
	}
}

// Run starts the polling loop. It blocks until the context is cancelled.
func (p *Poller) Run(ctx context.Context) {
	slog.Info("poller started", "interval", p.interval, "timeout", p.timeout)

	// Run an immediate first poll, then tick on interval.
	p.pollAll(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("poller stopped")
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

func (p *Poller) pollAll(ctx context.Context) {
	instances, err := p.store.ListInstances(ctx)
	if err != nil {
		slog.Error("poller: listing instances", "error", err)
		return
	}
	// Prune failure counts and stale metric buffers.
	p.pruneFailures(instances)
	p.pruneCollector(instances)

	if len(instances) == 0 {
		return
	}

	type result struct {
		id      int64
		probe   ProbeResult
		metrics []byte // raw /metrics text, nil if unavailable
	}

	results := make(chan result, len(instances))
	var wg sync.WaitGroup

	for i := range instances {
		inst := &instances[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Jitter: ±1s random offset to spread scrapes.
			jitter := time.Duration(rand.Int64N(int64(2*maxJitter))) - maxJitter // #nosec G404 -- jitter doesn't need cryptographic randomness //nolint:gosec
			sleep(ctx, jitter)

			pr := Probe(ctx, p.client, inst.Address)
			var raw []byte
			if pr.OK && p.collector != nil {
				raw = fetchMetrics(ctx, p.client, inst.Address)
			}
			results <- result{id: inst.ID, probe: pr, metrics: raw}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	now := time.Now()
	for r := range results {
		p.applyResult(ctx, r.id, r.probe)
		if r.metrics != nil {
			if err := p.collector.RecordScrape(r.id, r.metrics, now); err != nil {
				slog.Warn("poller: parsing metrics", "id", r.id, "error", err)
			}
		}
	}
}

func (p *Poller) pruneFailures(active []store.Instance) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id := range p.failures {
		found := false
		for j := range active {
			if active[j].ID == id {
				found = true
				break
			}
		}
		if !found {
			delete(p.failures, id)
		}
	}
}

func (p *Poller) applyResult(ctx context.Context, id int64, pr ProbeResult) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if pr.OK {
		p.failures[id] = 0
		if err := p.store.SetInstanceHealthy(ctx, id, pr.Version); err != nil {
			slog.Error("poller: setting instance healthy", "id", id, "error", err)
		}
		return
	}

	p.failures[id]++
	count := p.failures[id]
	slog.Debug("poller: probe failed", "id", id, "consecutive_failures", count, "error", pr.Error)

	if count >= failuresUntilUnreachable {
		if err := p.store.SetInstanceUnreachable(ctx, id); err != nil {
			slog.Error("poller: setting instance unreachable", "id", id, "error", err)
		}
	}
}

// fetchHealth calls GET /_ops/health and returns the status field.
func fetchHealth(ctx context.Context, client *http.Client, address string) (string, error) {
	url := fmt.Sprintf("http://%s/_ops/health", address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("creating health request: %w", err)
	}

	resp, err := client.Do(req) // #nosec G704 -- address comes from admin-managed instance registry
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("health endpoint returned %d", resp.StatusCode)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding health response: %w", err)
	}
	return body.Status, nil
}

// fetchVersion calls GET /_ops/version and returns the version field.
func fetchVersion(ctx context.Context, client *http.Client, address string) (string, error) {
	url := fmt.Sprintf("http://%s/_ops/version", address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("creating version request: %w", err)
	}

	resp, err := client.Do(req) // #nosec G704 -- address comes from admin-managed instance registry
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version endpoint returned %d", resp.StatusCode)
	}

	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding version response: %w", err)
	}
	return body.Version, nil
}

func (p *Poller) pruneCollector(active []store.Instance) {
	if p.collector == nil {
		return
	}
	ids := make(map[int64]bool, len(active))
	for i := range active {
		ids[active[i].ID] = true
	}
	p.collector.Prune(ids)
}

// fetchMetrics calls GET /metrics on a proxy admin port and returns the raw body.
func fetchMetrics(ctx context.Context, client *http.Client, address string) []byte {
	url := fmt.Sprintf("http://%s/metrics", address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil
	}

	resp, err := client.Do(req) // #nosec G704 -- address comes from admin-managed instance registry
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return nil
	}
	return data
}

// friendlyError converts network errors into user-facing messages.
func friendlyError(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "Connection timed out. Check that the proxy is running and the address is correct."
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return fmt.Sprintf("Connection failed: %s. The proxy admin server may be bound to localhost only; check admin_addr in the proxy configuration.", opErr.Err)
	}

	return fmt.Sprintf("Connection failed: %s", err)
}

// sleep waits for the given duration or until the context is cancelled.
// Negative durations return immediately.
func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
