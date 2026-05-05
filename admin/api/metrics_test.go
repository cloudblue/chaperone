// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudblue/chaperone/admin/metrics"
)

func makeSnapshot(t time.Time, totalReq, errReq, active, panics float64) metrics.Snapshot {
	return metrics.Snapshot{
		Time: t,
		Vendors: map[string]*metrics.VendorSnapshot{
			"acme": {
				RequestsTotal:  totalReq,
				RequestsErrors: errReq,
				Duration: metrics.Histogram{
					Count: totalReq,
					Buckets: []metrics.Bucket{
						{UpperBound: 0.1, Count: totalReq * 0.5},
						{UpperBound: 0.5, Count: totalReq * 0.9},
						{UpperBound: 1.0, Count: totalReq},
					},
				},
			},
		},
		ActiveConnections: active,
		PanicsTotal:       panics,
	}
}

func TestMetricsHandler_Fleet_ReturnsAggregated(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	c := metrics.NewCollector(10)

	ctx := context.Background()
	inst, err := st.CreateInstance(ctx, "proxy-1", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	t0 := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	c.Record(inst.ID, makeSnapshot(t0, 1000, 50, 10, 1))
	c.Record(inst.ID, makeSnapshot(t0.Add(10*time.Second), 1100, 55, 12, 2))

	h := NewMetricsHandler(st, c)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/fleet", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var fm metrics.FleetMetrics
	if err := json.NewDecoder(rec.Body).Decode(&fm); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if fm.TotalRPS <= 0 {
		t.Errorf("TotalRPS = %v, want > 0", fm.TotalRPS)
	}
	if len(fm.Instances) != 1 {
		t.Errorf("Instances = %d, want 1", len(fm.Instances))
	}
}

func TestMetricsHandler_Instance_ReturnsMetrics(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	c := metrics.NewCollector(10)

	ctx := context.Background()
	inst, err := st.CreateInstance(ctx, "proxy-1", "10.0.0.1:9090")
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}

	t0 := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	c.Record(inst.ID, makeSnapshot(t0, 1000, 50, 10, 1))
	c.Record(inst.ID, makeSnapshot(t0.Add(10*time.Second), 1100, 55, 12, 2))

	h := NewMetricsHandler(st, c)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var im metrics.InstanceMetrics
	if err := json.NewDecoder(rec.Body).Decode(&im); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if im.RPS <= 0 {
		t.Errorf("RPS = %v, want > 0", im.RPS)
	}
	if im.DataPoints != 2 {
		t.Errorf("DataPoints = %d, want 2", im.DataPoints)
	}
}

func TestMetricsHandler_Instance_NoData_Returns404(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	c := metrics.NewCollector(10)

	h := NewMetricsHandler(st, c)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/99", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestMetricsHandler_Instance_InvalidID_Returns400(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	c := metrics.NewCollector(10)

	h := NewMetricsHandler(st, c)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestMetricsHandler_Fleet_EmptyFleet_ReturnsEmptyInstances(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	c := metrics.NewCollector(10)

	h := NewMetricsHandler(st, c)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/fleet", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var fm metrics.FleetMetrics
	if err := json.NewDecoder(rec.Body).Decode(&fm); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(fm.Instances) != 0 {
		t.Errorf("Instances = %d, want 0", len(fm.Instances))
	}
}
