// test/load/targetserver/main.go
// Minimal HTTP echo server for load testing.
// Returns a fixed JSON response with minimal overhead so that
// load test measurements reflect proxy performance, not backend.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"time"
)

func main() {
	addr := flag.String("addr", ":9999", "listen address")
	flag.Parse()

	body := []byte(`{"status":"ok"}`)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})

	server := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	slog.Info("target server listening", "addr", *addr)
	if err := server.ListenAndServe(); err != nil {
		slog.Error("server failed", "error", err)
	}
}
