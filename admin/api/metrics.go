// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"log/slog"
	"net/http"

	"github.com/cloudblue/chaperone/admin/metrics"
	"github.com/cloudblue/chaperone/admin/store"
)

// MetricsHandler serves computed metrics via the REST API.
type MetricsHandler struct {
	store     *store.Store
	collector *metrics.Collector
}

// NewMetricsHandler creates a handler backed by the given store and collector.
func NewMetricsHandler(st *store.Store, c *metrics.Collector) *MetricsHandler {
	return &MetricsHandler{store: st, collector: c}
}

// Register mounts metrics routes on the given mux.
func (h *MetricsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/metrics/fleet", h.fleet)
	mux.HandleFunc("GET /api/metrics/{id}", h.instance)
}

func (h *MetricsHandler) fleet(w http.ResponseWriter, r *http.Request) {
	instances, err := h.store.ListInstances(r.Context())
	if err != nil {
		slog.Error("listing instances for fleet metrics", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list instances")
		return
	}

	ids := make([]int64, len(instances))
	for i := range instances {
		ids[i] = instances[i].ID
	}

	fm := h.collector.GetFleetMetrics(ids)
	respondJSON(w, http.StatusOK, fm)
}

func (h *MetricsHandler) instance(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	im := h.collector.GetInstanceMetrics(id)
	if im == nil {
		respondError(w, http.StatusNotFound, "NO_METRICS", "No metric data available for this instance")
		return
	}

	respondJSON(w, http.StatusOK, im)
}
