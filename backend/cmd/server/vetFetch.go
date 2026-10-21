package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"vet-tracker/backend/internal/api"
	"vet-tracker/backend/internal/database"
	"vet-tracker/backend/internal/fetcher"
	"vet-tracker/backend/internal/geocoder"
)

const (
	fetchDepartmentID = 70
	fetchLimit        = 15
	startupFetchAge   = 12 * time.Hour
)

func runScheduledFetches(logger *slog.Logger, queries *database.Queries, client *fetcher.Client) {
	ctx := context.Background()
	lastFetchedAt, hasLastFetch, err := lastFetchedAt(ctx, queries)
	if err != nil {
		logger.Error("startup fetch check failed", "error", err)
		return
	}
	if !hasLastFetch || time.Since(lastFetchedAt) > startupFetchAge {
		logger.Info("startup fetch started")
		imported, fetchErr := importDepartmentClinics(ctx, queries, client, fetchDepartmentID, fetchLimit)
		if fetchErr != nil {
			logger.Error("startup fetch failed", "error", fetchErr)
		} else {
			logger.Info("startup fetch complete", "imported", imported)
			runGeocoder(ctx, logger, queries)
		}
	} else {
		logger.Info("startup fetch skipped", "last_fetch_at", lastFetchedAt.Format(time.RFC3339))
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		logger.Info("scheduled fetch started")
		imported, err := importDepartmentClinics(ctx, queries, client, fetchDepartmentID, fetchLimit)
		if err != nil {
			logger.Warn("scheduled fetch failed", "error", err)
			continue
		}
		logger.Info("scheduled fetch complete", "imported", imported)
		runGeocoder(ctx, logger, queries)
	}
}

func runGeocoder(ctx context.Context, logger *slog.Logger, queries *database.Queries) {
	if err := geocoder.Run(ctx, queries, logger); err != nil {
		logger.Error("geocoding after fetch failed", "error", err)
	}
}

func lastFetchedAt(ctx context.Context, queries *database.Queries) (time.Time, bool, error) {
	maxFetchedAt, err := queries.LastFetchedAt(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	return maxFetchedAt, true, nil
}

func importDepartmentClinics(ctx context.Context, queries *database.Queries, client *fetcher.Client, departmentID int, limit int) (int, error) {
	dpes, err := client.FetchByDepartmentWithContext(ctx, departmentID, limit)
	if err != nil {
		return 0, err
	}

	fetchedAt := time.Now().UTC()
	imported := 0
	for _, dpe := range dpes {
		externalID := strings.TrimSpace(dpe.OrdinalNumber)
		if externalID == "" {
			continue
		}

		clinic := database.UpsertClinicParams{
			ExternalID:         externalID,
			Name:               strings.TrimSpace(dpe.Name),
			Phone:              toNullString(strings.TrimSpace(dpe.Phone)),
			AddressWay:         toNullString(strings.TrimSpace(dpe.Address.Way)),
			AddressAdditional1: toNullString(strings.TrimSpace(dpe.Address.AdditionalAddress1)),
			AddressAdditional2: toNullString(strings.TrimSpace(dpe.Address.AdditionalAddress2)),
			ZipCode:            toNullString(strings.TrimSpace(dpe.Address.ZipCode)),
			City:               toNullString(strings.TrimSpace(dpe.Address.City)),
			FetchedAt:          fetchedAt,
			Source:             "fetched",
			IsVisible:          true,
			TrackStatus:        api.DefaultTrackStatus,
		}
		if dpe.VetLastname != "" {
			clinic.VetLastname = sql.NullString{String: dpe.VetLastname, Valid: true}
		}
		if dpe.VetFirstname != "" {
			clinic.VetFirstname = sql.NullString{String: dpe.VetFirstname, Valid: true}
		}
		if dpe.HasVetOrdinal {
			clinic.VetOrdinalNumber = sql.NullInt64{Int64: dpe.VetOrdinalNumber, Valid: true}
		}

		if _, err := queries.UpsertClinic(ctx, clinic); err != nil {
			return 0, err
		}
		imported++
	}

	return imported, nil
}
