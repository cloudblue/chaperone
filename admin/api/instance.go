// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudblue/chaperone/admin/auth"
	"github.com/cloudblue/chaperone/admin/poller"
	"github.com/cloudblue/chaperone/admin/store"
)

// InstanceHandler handles instance CRUD and test-connection endpoints.
type InstanceHandler struct {
	store  *store.Store
	client *http.Client
}

// NewInstanceHandler creates a handler with the given store and probe timeout.
func NewInstanceHandler(st *store.Store, probeTimeout time.Duration) *InstanceHandler {
	return &InstanceHandler{
		store:  st,
		client: &http.Client{Timeout: probeTimeout},
	}
}

// Register mounts instance routes on the given mux.
func (h *InstanceHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/instances", h.list)
	mux.HandleFunc("POST /api/instances", h.create)
	mux.HandleFunc("POST /api/instances/test", h.testConnection)
	mux.HandleFunc("GET /api/instances/{id}", h.get)
	mux.HandleFunc("PUT /api/instances/{id}", h.update)
	mux.HandleFunc("DELETE /api/instances/{id}", h.delete)
}

func (h *InstanceHandler) list(w http.ResponseWriter, r *http.Request) {
	instances, err := h.store.ListInstances(r.Context())
	if err != nil {
		slog.Error("listing instances", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list instances")
		return
	}
	if instances == nil {
		instances = []store.Instance{}
	}
	respondJSON(w, http.StatusOK, instances)
}

func (h *InstanceHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	inst, err := h.store.GetInstance(r.Context(), id)
	if errors.Is(err, store.ErrInstanceNotFound) {
		respondError(w, http.StatusNotFound, "INSTANCE_NOT_FOUND", fmt.Sprintf("No instance with ID %d", id))
		return
	}
	if err != nil {
		slog.Error("getting instance", "id", id, "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get instance")
		return
	}
	respondJSON(w, http.StatusOK, inst)
}

type instanceRequest struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (h *InstanceHandler) create(w http.ResponseWriter, r *http.Request) {
	var req instanceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validateInstanceRequest(w, &req) {
		return
	}

	inst, err := h.store.CreateInstance(r.Context(), req.Name, req.Address)
	if errors.Is(err, store.ErrDuplicateAddress) {
		respondError(w, http.StatusConflict, "DUPLICATE_ADDRESS",
			fmt.Sprintf("An instance with address %q is already registered", req.Address))
		return
	}
	if err != nil {
		slog.Error("creating instance", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create instance")
		return
	}

	h.audit(r, AuditActionInstanceCreate, &inst.ID,
		fmt.Sprintf("Created instance %q at %s", inst.Name, inst.Address))
	respondJSON(w, http.StatusCreated, inst)
}

func (h *InstanceHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req instanceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validateInstanceRequest(w, &req) {
		return
	}

	inst, err := h.store.UpdateInstance(r.Context(), id, req.Name, req.Address)
	if errors.Is(err, store.ErrInstanceNotFound) {
		respondError(w, http.StatusNotFound, "INSTANCE_NOT_FOUND", fmt.Sprintf("No instance with ID %d", id))
		return
	}
	if errors.Is(err, store.ErrDuplicateAddress) {
		respondError(w, http.StatusConflict, "DUPLICATE_ADDRESS",
			fmt.Sprintf("An instance with address %q is already registered", req.Address))
		return
	}
	if err != nil {
		slog.Error("updating instance", "id", id, "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update instance")
		return
	}

	h.audit(r, AuditActionInstanceUpdate, &inst.ID,
		fmt.Sprintf("Updated instance %q (address: %s)", inst.Name, inst.Address))
	respondJSON(w, http.StatusOK, inst)
}

func (h *InstanceHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	// Fetch instance name before deletion for the audit detail.
	inst, getErr := h.store.GetInstance(r.Context(), id)

	err := h.store.DeleteInstance(r.Context(), id)
	if errors.Is(err, store.ErrInstanceNotFound) {
		respondError(w, http.StatusNotFound, "INSTANCE_NOT_FOUND", fmt.Sprintf("No instance with ID %d", id))
		return
	}
	if err != nil {
		slog.Error("deleting instance", "id", id, "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete instance")
		return
	}

	detail := fmt.Sprintf("Deleted instance ID %d", id)
	if getErr == nil {
		detail = fmt.Sprintf("Deleted instance %q (%s)", inst.Name, inst.Address)
	}
	h.audit(r, AuditActionInstanceDelete, nil, detail)
	w.WriteHeader(http.StatusNoContent)
}

func (h *InstanceHandler) testConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	addr := strings.TrimSpace(req.Address)
	if addr == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "address is required")
		return
	}
	if err := validHostPort(addr); err != nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	result := poller.Probe(r.Context(), h.client, addr)
	respondJSON(w, http.StatusOK, result)
}

// parseID extracts and validates the {id} path parameter.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", fmt.Sprintf("Invalid instance ID: %q", raw))
		return 0, false
	}
	return id, true
}

// decodeJSON reads and decodes a JSON request body (max 1 MB).
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid JSON request body")
		return false
	}
	return true
}

func validateInstanceRequest(w http.ResponseWriter, req *instanceRequest) bool {
	req.Name = strings.TrimSpace(req.Name)
	req.Address = strings.TrimSpace(req.Address)

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return false
	}
	if req.Address == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "address is required")
		return false
	}
	if err := validHostPort(req.Address); err != nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return false
	}
	return true
}

var errInvalidHostPort = errors.New("address must be a valid host:port (e.g. 192.168.1.10:9090)")

func (h *InstanceHandler) audit(r *http.Request, action string, instanceID *int64, detail string) {
	user := auth.ContextUser(r.Context())
	if user == nil {
		return
	}
	if err := h.store.InsertAuditEntry(r.Context(), user.ID, action, instanceID, detail); err != nil {
		slog.Error("writing audit entry", "action", action, "error", err)
	}
}

func validHostPort(addr string) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return errInvalidHostPort
	}
	if host == "" {
		return errInvalidHostPort
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil || port == 0 {
		return errInvalidHostPort
	}
	return nil
}
