package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"

	"vet-tracker/backend/internal/database"
	"vet-tracker/backend/internal/geocoder"

	_ "modernc.org/sqlite"
)

func initLogger() (*slog.Logger, error) {
	logPath := getenv("LOG_PATH", "./geocoding.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	w := io.MultiWriter(os.Stderr, logFile)
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{AddSource: true, Level: slog.LevelInfo})), nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("geocoding failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	logger, err := initLogger()
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	slog.SetDefault(logger)

	dbLink, err := sql.Open("sqlite", getenv("DB_PATH", "./data.db"))
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer dbLink.Close()

	return geocoder.Run(context.Background(), database.New(dbLink), logger)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
