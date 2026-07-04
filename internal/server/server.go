package server

import (
	"encoding/json"
	"net/http"
)

type Server struct {
	*http.Server
}

func New(addr string) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handleHealth)

	return &Server{
		Server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"version": "2026.07.04",
	})
}
