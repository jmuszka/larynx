package main

import (
	"os"

	"github.com/jmuszka/larynx/internal/logging"
	"github.com/jmuszka/larynx/internal/server"
	"github.com/joho/godotenv"
)

func Loadenv() {
	if err := godotenv.Load(); err != nil {
		logging.New(logging.Config{Level: logging.LevelInfo}).Fatal("error loading .env file", "error", err)
	}
}

const version = "preview"

func main() {
	Loadenv()

	logger := logging.New(logging.Config{
		Level:    logging.ParseLevel(os.Getenv("LOG_LEVEL")),
		FilePath: os.Getenv("LOG_FILE"),
	})
	defer logger.Close()

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
		Logger:        logger,
	}

	s := server.New(cfg)
	logger.Info("listening", "addr", cfg.Addr)
	logger.Fatal("server stopped", "error", s.ListenAndServe())
}
