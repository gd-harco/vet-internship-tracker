package api

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"vet-tracker/backend/internal/database"
	"vet-tracker/backend/internal/geocoder"
)

const DefaultTrackStatus = "not_contacted"

type DisplayedClinic struct {
	database.Clinic
	Notes []database.Note
}

func (h *Handler) listClinicsPage(ctx context.Context, page, pageSize int) ([]DisplayedClinic, bool, error) {
	h.logger.Debug("listClinicsPage", slog.Int("page", page), slog.Int("pageSize", pageSize))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	offset := (page - 1) * pageSize

	rows, err := h.queries.GetVisibleClinics(ctx, database.GetVisibleClinicsParams{
		Limit:  int64(pageSize + 1),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, false, err
	}

	hasNext := len(rows) > pageSize
	if hasNext {
		rows = rows[:pageSize]
	}

	clinics := make([]DisplayedClinic, 0, len(rows))
	for _, row := range rows {
		notes, err := h.queries.ListNotesByClinicID(ctx, row.ID)
		if err != nil {
			return nil, false, err
		}
		object := DisplayedClinic{row, notes}
		clinics = append(clinics, object)
	}

	return clinics, hasNext, nil
}

func (h *Handler) createManualClinic(ctx context.Context, name, phone, addressWay, zipCode, city string) (database.Clinic, error) {
	now := time.Now().UTC()
	externalID := fmt.Sprintf("manual-%d", now.UnixNano())
	res, err := h.queries.CreateClinic(ctx, database.CreateClinicParams{
		ExternalID:  externalID,
		Name:        name,
		Phone:       toNullString(phone),
		AddressWay:  toNullString(addressWay),
		ZipCode:     toNullString(zipCode),
		City:        toNullString(city),
		FetchedAt:   now,
		Source:      "manual",
		IsVisible:   true,
		TrackStatus: DefaultTrackStatus,
	})
	if err != nil {
		return database.Clinic{}, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return database.Clinic{}, err
	}
	clinic, err := h.queries.GetClinicByID(ctx, id)
	if err != nil {
		return database.Clinic{}, err
	}

	// A geocoding failure must not prevent the clinic creation.
	lat, lng, err := geocoder.GeocodeClinic(ctx, h.queries, clinic)
	if err != nil {
		h.logger.Warn("manual clinic not geocoded", "clinicID", clinic.ID, "error", err)
		return clinic, nil
	}
	clinic.Lat = sql.NullFloat64{Float64: lat, Valid: true}
	clinic.Lng = sql.NullFloat64{Float64: lng, Valid: true}
	return clinic, nil
}

func (h *Handler) addNote(ctx context.Context, clinicID int64, content string) (database.Note, error) {
	now := time.Now().UTC()
	res, err := h.queries.InsertNote(ctx, database.InsertNoteParams{ClinicID: clinicID, Content: content, CreatedAt: now})
	if err != nil {
		return database.Note{}, err
	}
	noteID, err := res.LastInsertId()
	if err != nil {
		return database.Note{}, err
	}
	return database.Note{ID: noteID, ClinicID: clinicID, Content: content, CreatedAt: now}, nil
}

func (h *Handler) deleteNote(ctx context.Context, noteID int64) error {
	_, err := h.queries.DeleteNote(ctx, noteID)
	return err
}

func (h *Handler) countNotes(ctx context.Context, clinicID int64) (int, error) {
	count, err := h.queries.CountNotesByClinicID(ctx, clinicID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func toNullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func fromNullString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func lastFetchedAt(ctx context.Context, queries *database.Queries) (time.Time, bool, error) {
	maxFetchedAt, err := queries.LastFetchedAt(ctx)
	if err != nil {
		return time.Time{}, false, err
	}
	if maxFetchedAt.IsZero() {
		return time.Time{}, false, nil
	}
	return maxFetchedAt, true, nil
}
