package main

import (
	"os"
	"strings"

	"github.com/jmuszka/larynx/internal/logging"
	"github.com/jmuszka/larynx/internal/server"
	"github.com/joho/godotenv"

	_ "github.com/jmuszka/larynx/docs"
)

func splitOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}

func Loadenv() {
	if err := godotenv.Load(); err != nil {
		logging.New(logging.Config{Level: logging.LevelInfo}).Fatal("error loading .env file", "error", err)
	}
}

const version = "preview"

// @title           Larynx API
// @version         preview
// @description     Larynx word etymology and language API.
// @BasePath        /api/v1
func main() {
	Loadenv()

	logger := logging.New(logging.Config{
		Level:    logging.ParseLevel(os.Getenv("LOG_LEVEL")),
		FilePath: os.Getenv("LOG_FILE"),
	})
	defer logger.Close()

	cfg := server.Config{
		Addr:           ":" + os.Getenv("PORT"),
		Neo4jUri:       os.Getenv("NEO4J_URI"),
		Neo4jUser:      os.Getenv("NEO4J_USER"),
		Neo4jPassword:  os.Getenv("NEO4J_PASSWORD"),
		Version:        version,
		SqlitePath:     os.Getenv("SQLITE_PATH"),
		AIBaseURL:      os.Getenv("AI_BASE_URL"),
		AIKey:          os.Getenv("AI_API_KEY"),
		AIModel:        os.Getenv("AI_MODEL"),
		Logger:         logger,
		AllowedOrigins: splitOrigins(os.Getenv("ALLOWED_ORIGINS")),
		DebugMode:      strings.ToLower(os.Getenv("DEBUG")) == "true",
	}

	s := server.New(cfg)
	logger.Info("listening", "addr", cfg.Addr)
	logger.Fatal("server stopped", "error", s.ListenAndServe())
}
