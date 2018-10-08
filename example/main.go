package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"
)

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte(fmt.Sprintf(`{"queue": {"length": %d}}`, size)))
}

var (
	size int
)

func main() {
	flag.IntVar(&size, "fake-queue-length", 10, "Fake queue length for fake metrics.")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", metricsHandler)

	server := &http.Server{
		Addr:        ":9090",
		Handler:     mux,
		ReadTimeout: 5 * time.Second,
	}

	server.ListenAndServe()
}
