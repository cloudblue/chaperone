// test/load/targetserver/main.go
// Minimal HTTP echo server for load testing.
// Returns a fixed JSON response with minimal overhead so that
// load test measurements reflect proxy performance, not backend.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":9999", "listen address")
	flag.Parse()

	body := []byte(`{"status":"ok"}`)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})

	fmt.Printf("target server listening on %s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
