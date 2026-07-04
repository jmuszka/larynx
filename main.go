package main

import (
	"log"

	"github.com/jmuszka/larynx/internal/server"
)

func main() {
	addr := ":8080"

	s := server.New(addr)
	log.Printf("listening on %s", addr)
	log.Fatal(s.ListenAndServe())
}
