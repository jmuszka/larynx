package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jmuszka/larynx/internal/logging"
	"github.com/jmuszka/larynx/internal/server"
	"github.com/joho/godotenv"

	_ "github.com/jmuszka/larynx/docs"
)

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			values = append(values, p)
		}
	}
	return values
}

func Loadenv() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("error loading .env file: %v", err)
	}
}

const version = "preview"

// @title           Larynx API
// @version         preview
// @description     Larynx word etymology and language API.
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     Client bearer token. Enter the full value as: Bearer <token>
// @securityDefinitions.apikey AdminJWTAuth
// @in              header
// @name            X-Admin-JWT
// @description     Admin JWT for blog write endpoints (create/update/delete)
func main() {
	Loadenv()

	logger, err := logging.New(logging.Config{
		Level:    logging.ParseLevel(os.Getenv("LOG_LEVEL")),
		FilePath: os.Getenv("LOG_FILE"),
	})
	if err != nil {
		log.Fatalf("failed to initialize logging: %v", err)
	}
	defer logger.Close()

	adminJWTSecret := os.Getenv("ADMIN_JWT_SECRET")
	if adminJWTSecret == "" {
		logger.Fatal("ADMIN_JWT_SECRET is not set")
	}
	adminJWTSubject := os.Getenv("ADMIN_JWT_SUBJECT")
	if adminJWTSubject == "" {
		adminJWTSubject = "blog-admin"
	}

	cfg := server.Config{
		Addr:              ":" + os.Getenv("PORT"),
		Neo4jUri:          os.Getenv("NEO4J_URI"),
		Neo4jUser:         os.Getenv("NEO4J_USER"),
		Neo4jPassword:     os.Getenv("NEO4J_PASSWORD"),
		Version:           version,
		SqlitePath:        os.Getenv("SQLITE_PATH"),
		AIBaseURL:         os.Getenv("AI_BASE_URL"),
		AIKey:             os.Getenv("AI_API_KEY"),
		AIModel:           os.Getenv("AI_MODEL"),
		Logger:            logger,
		AllowedOrigins:    splitCSV(os.Getenv("ALLOWED_ORIGINS")),
		BearerTokens:      splitCSV(os.Getenv("BEARER_TOKENS")),
		AdminJWTSecret:    adminJWTSecret,
		AdminJWTSubject:   adminJWTSubject,
		DebugMode:         strings.ToLower(os.Getenv("DEBUG")) == "true",
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	s := server.New(cfg)

	go func() {
		logger.Info("listening", "addr", cfg.Addr)
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server stopped unexpectedly", "error", err)
		}
	}()

	// Wait for a shutdown signal, then shut down gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
