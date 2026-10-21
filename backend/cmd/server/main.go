package main

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"vet-tracker/backend/internal/api"
	"vet-tracker/backend/internal/database"
	"vet-tracker/backend/internal/fetcher"
	migrations "vet-tracker/backend/sql/migrations"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func main() {
	logger := initLogger()
	slog.SetDefault(logger)
	port := getenv("PORT", "8080")
	dbPath := getenv("DB_PATH", "./data.db")

	dbLink, err := sql.Open("sqlite", dbPath)
	if err != nil {
		logger.Error("failed to init db", "error", err)
		os.Exit(1)
	}
	defer dbLink.Close()
	if err := ensureSchema(dbLink); err != nil {
		logger.Error("failed to apply db schema", "error", err)
		os.Exit(1)
	}

	timeoutSeconds, err := strconv.Atoi(getenv("VET_API_TIMEOUT_SECONDS", "20"))
	if err != nil || timeoutSeconds <= 0 {
		timeoutSeconds = 20
	}
	apiClient := fetcher.NewClient(
		getenv("VET_API_BASE_URL", "https://extranet.veterinaire.fr"),
		time.Duration(timeoutSeconds)*time.Second,
	)

	queries := database.New(dbLink)
	defer dbLink.Close()
	h := api.NewHandler(queries, logger)
	logger.Info("vet-tracker listening", "port", port)
	go runScheduledFetches(logger, queries, apiClient)
	if err := http.ListenAndServe(":"+port, h.Routes()); err != nil {
		logger.Error("server stopped", "error", err)
	}
}

func initLogger() *slog.Logger {
	logPath := getenv("LOG_PATH", "./vet-tracker.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		panic(err)
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, logFile), &slog.HandlerOptions{AddSource: true, Level: slog.LevelInfo}))
}

func ensureSchema(dbLink *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.Up(dbLink, ".")
}

func toNullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
