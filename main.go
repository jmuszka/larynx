package main

import (
	"log"
	"os"

	"github.com/jmuszka/larynx/internal/server"
	"github.com/joho/godotenv"
)

func Loadenv() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}
}

const version = "preview"

func main() {
	Loadenv()

	cfg := server.Config{
		Addr:          ":" + os.Getenv("PORT"),
		Neo4jUri:      os.Getenv("NEO4J_URI"),
		Neo4jUser:     os.Getenv("NEO4J_USER"),
		Neo4jPassword: os.Getenv("NEO4J_PASSWORD"),
		Version:       version,
		SqlitePath:    os.Getenv("SQLITE_PATH"),
		AIBaseURL:     os.Getenv("AI_BASE_URL"),
		AIKey:         os.Getenv("AI_API_KEY"),
		AIModel:       os.Getenv("AI_MODEL"),
	}

	s := server.New(cfg)
	log.Printf("Listening on %s", cfg.Addr)
	log.Fatal(s.ListenAndServe())
}
