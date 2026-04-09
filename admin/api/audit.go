// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloudblue/chaperone/admin/store"
)

// AuditHandler serves the audit log REST endpoint.
type AuditHandler struct {
	store *store.Store
}

// NewAuditHandler creates a handler for the audit log endpoint.
func NewAuditHandler(st *store.Store) *AuditHandler {
	return &AuditHandler{store: st}
}

// Register mounts audit routes on the given mux.
func (h *AuditHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/audit", h.list)
}

func (h *AuditHandler) list(w http.ResponseWriter, r *http.Request) {
	filter, err := parseAuditFilter(r.URL.Query())
	if err != nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	page, err := h.store.ListAuditEntries(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list audit entries")
		return
	}

	respondJSON(w, http.StatusOK, page)
}

func parseAuditFilter(q url.Values) (store.AuditFilter, error) {
	filter := store.AuditFilter{
		Action:  strings.TrimSpace(q.Get("action")),
		Query:   strings.TrimSpace(q.Get("q")),
		Page:    1,
		PerPage: 20,
	}

	if err := parseIDParam(q, "user", &filter.UserID); err != nil {
		return filter, err
	}
	if err := parseIDParam(q, "instance_id", &filter.InstanceID); err != nil {
		return filter, err
	}
	if err := parseTimeParam(q, "from", &filter.From); err != nil {
		return filter, err
	}
	if err := parseTimeParam(q, "to", &filter.To); err != nil {
		return filter, err
	}
	if err := parsePageParams(q, &filter.Page, &filter.PerPage); err != nil {
		return filter, err
	}

	return filter, nil
}

func parseIDParam(q url.Values, key string, dst **int64) error {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid %s: %q", key, v)
	}
	*dst = &id
	return nil
}

func parseTimeParam(q url.Values, key string, dst **time.Time) error {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return fmt.Errorf("invalid %s: %q (expected RFC 3339)", key, v)
	}
	*dst = &t
	return nil
}

func parsePageParams(q url.Values, page, perPage *int) error {
	if v := q.Get("page"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 {
			return fmt.Errorf("invalid page: %q", v)
		}
		*page = p
	}
	if v := q.Get("per_page"); v != "" {
		pp, err := strconv.Atoi(v)
		if err != nil || pp < 1 || pp > 100 {
			return fmt.Errorf("invalid per_page: %q (must be 1-100)", v)
		}
		*perPage = pp
	}
	return nil
}
