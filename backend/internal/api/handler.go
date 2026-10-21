package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"vet-tracker/backend/internal/database"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const pageSize = 12

type Handler struct {
	queries   *database.Queries
	templates *template.Template // nil in dev mode — re-parsed on every request
	devMode   bool
	logger    *slog.Logger
}

type IndexViewData struct {
	ClinicAndNotes []DisplayedClinic
	HasNext        bool
	NextPage       int
}

type NotesCountOOBData struct {
	ClinicID int64
	Count    int
}

type ClinicEditFormData struct {
	Clinic       database.Clinic
	ErrorMessage string
}

func NewHandler(queries *database.Queries, logger ...*slog.Logger) *Handler {
	devMode := os.Getenv("DEV") == "true"
	var appLogger *slog.Logger
	if len(logger) > 0 && logger[0] != nil {
		appLogger = logger[0]
	} else {
		appLogger = slog.Default()
	}

	h := &Handler{queries: queries, devMode: devMode, logger: appLogger}
	if !devMode {
		h.templates = template.Must(parseTemplates())
	}
	return h
}

func parseTemplates() (*template.Template, error) {
	patterns := []string{
		"internal/templates/layouts/*.gohtml",
		"internal/templates/pages/*.gohtml",
		"internal/templates/partials/*.gohtml",
	}
	var files []string
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	return template.ParseFiles(files...)
}

func (h *Handler) tmpl() *template.Template {
	if h.devMode {
		t, err := parseTemplates()
		if err != nil {
			panic(err)
		}
		return t
	}
	return h.templates
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	if h.devMode {
		r.Use(middleware.Logger)
	}

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	r.Get("/", h.handleIndex)
	r.Get("/partials/clinics", h.handleClinicsChunk)
	r.Get("/clinics/{clinicID}/edit", h.handleGetClinicEditForm)
	r.Get("/clinics/{clinicID}/view", h.handleGetClinicView)
	r.Get("/clinics/{clinicID}/card", h.handleGetClinicCard)
	r.Post("/clinics/manual", h.handleCreateManualClinic)
	r.Post("/clinics/{clinicID}/hide", h.handleHideClinic)
	r.Post("/clinics/{clinicID}/status", h.handleUpdateStatus)
	r.Put("/clinics/{clinicID}", h.handleUpdateClinic)
	r.Get("/map", h.handleGetMap)
	// r.Get("/map/marker", h.handleGetMarker)
	r.Get("/map/popup/{clinicID}", h.HandleGetPopup)
	r.Get("/card", h.handleCardView)
	r.Post("/clinics/{clinicID}/notes", h.handleAddNote)
	r.Delete("/clinics/{clinicID}/notes/{noteID}", h.handleDeleteNote)

	return r
}

type markerList struct {
	Markers []database.GetClinicMapMakerRow `json:"markers"`
}

func (h *Handler) HandleGetPopup(w http.ResponseWriter, r *http.Request) {
	clinicID, err := strconv.ParseInt(chi.URLParam(r, "clinicID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid clinic id", http.StatusBadRequest)
		return
	}
	data, err := h.queries.GetClinicPopup(r.Context(), clinicID)
	if err != nil {
		slog.Error("Database error", "error", err, "clinicID", clinicID)
		http.Error(w, "failed to load clinics", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err = json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("error when unmarshal", "errors", err, "data", data)
	}
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	clinics, hasNext, err := h.listClinicsPage(r.Context(), 1, pageSize)
	if err != nil {
		h.logger.Error("handleIndex list clinics", "error", err)
		http.Error(w, "failed to load clinics", http.StatusInternalServerError)
		return
	}
	data := IndexViewData{ClinicAndNotes: clinics, HasNext: hasNext, NextPage: 2}
	var buf bytes.Buffer
	if err := h.tmpl().ExecuteTemplate(&buf, "index", data); err != nil {
		h.logger.Error("handleIndex render", "error", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func (h *Handler) handleCardView(w http.ResponseWriter, r *http.Request) {
	clinics, hasNext, err := h.listClinicsPage(r.Context(), 1, pageSize)
	if err != nil {
		h.logger.Error("handleCardView list clinics", "error", err)
		http.Error(w, "failed to load clinics", http.StatusInternalServerError)
		return
	}
	data := IndexViewData{ClinicAndNotes: clinics, HasNext: hasNext, NextPage: 2}
	var buf bytes.Buffer
	if err := h.tmpl().ExecuteTemplate(&buf, "card_view", data); err != nil {
		h.logger.Error("handleCardView render", "error", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func (h *Handler) handleClinicsChunk(w http.ResponseWriter, r *http.Request) {
	page := parseIntOrDefault(r.URL.Query().Get("page"), 1)
	clinics, hasNext, err := h.listClinicsPage(r.Context(), page, pageSize)
	if err != nil {
		h.logger.Error("handleClinicsChunk list clinics", "page", page, "error", err)
		http.Error(w, "failed to load clinics", http.StatusInternalServerError)
		return
	}
	data := IndexViewData{ClinicAndNotes: clinics, HasNext: hasNext, NextPage: page + 1}
	var buffer bytes.Buffer
	if err := h.tmpl().ExecuteTemplate(&buffer, "clinic_page_chunk", data); err != nil {
		h.logger.Error("render clinic chunk", slog.Int("page", page), slog.Any("error", err))
		http.Error(w, "failed to render chunk", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buffer.WriteTo(w); err != nil {
		h.logger.Debug("handleClinicsChunk write", "page", page, "error", err)
	}
}

func (h *Handler) handleCreateManualClinic(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	addressWay := strings.TrimSpace(r.FormValue("address_way"))
	zipCode := strings.TrimSpace(r.FormValue("zip_code"))
	city := strings.TrimSpace(r.FormValue("city"))
	phone := strings.TrimSpace(r.FormValue("phone"))

	if name == "" || city == "" {
		http.Error(w, "name and city are required", http.StatusBadRequest)
		return
	}

	clinic, err := h.createManualClinic(r.Context(), name, phone, addressWay, zipCode, city)
	if err != nil {
		http.Error(w, "failed to create clinic", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "manual-clinic-created")
	if err := h.tmpl().ExecuteTemplate(w, "clinic_card", clinic); err != nil {
		http.Error(w, "failed to render clinic", http.StatusInternalServerError)
	}
}

func (h *Handler) handleGetClinicEditForm(w http.ResponseWriter, r *http.Request) {
	clinicID, err := strconv.ParseInt(chi.URLParam(r, "clinicID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid clinic id", http.StatusBadRequest)
		return
	}

	clinic, err := h.queries.GetClinicByID(r.Context(), clinicID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "clinic not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load clinic", http.StatusInternalServerError)
		return
	}

	h.renderClinicEditForm(w, clinic, "")
}

func (h *Handler) handleGetClinicView(w http.ResponseWriter, r *http.Request) {
	clinicID, err := strconv.ParseInt(chi.URLParam(r, "clinicID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid clinic id", http.StatusBadRequest)
		return
	}

	clinic, err := h.queries.GetClinicByID(r.Context(), clinicID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "clinic not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load clinic", http.StatusInternalServerError)
		return
	}

	if err := h.tmpl().ExecuteTemplate(w, "clinic_card_info", clinic); err != nil {
		http.Error(w, "failed to render clinic", http.StatusInternalServerError)
	}
}

func (h *Handler) handleGetClinicCard(w http.ResponseWriter, r *http.Request) {
	clinicID, err := strconv.ParseInt(chi.URLParam(r, "clinicID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid clinic id", http.StatusBadRequest)
		return
	}

	clinic, err := h.queries.GetClinicByID(r.Context(), clinicID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "clinic not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load clinic", http.StatusInternalServerError)
		return
	}

	notes, err := h.queries.ListNotesByClinicID(r.Context(), clinicID)
	if err != nil {
		http.Error(w, "failed to load notes", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := h.tmpl().ExecuteTemplate(&buf, "clinic_card", DisplayedClinic{Clinic: clinic, Notes: notes}); err != nil {
		h.logger.Error("handleGetClinicCard render", "error", err)
		http.Error(w, "failed to render clinic", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func (h *Handler) handleUpdateClinic(w http.ResponseWriter, r *http.Request) {
	clinicID, err := strconv.ParseInt(chi.URLParam(r, "clinicID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid clinic id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	params := database.UpdateClinicParams{
		Name:       strings.TrimSpace(r.FormValue("name")),
		AddressWay: sql.NullString{String: strings.TrimSpace(r.FormValue("address_way")), Valid: true},
		ZipCode:    sql.NullString{String: strings.TrimSpace(r.FormValue("zip_code")), Valid: true},
		City:       sql.NullString{String: strings.TrimSpace(r.FormValue("city")), Valid: true},
		Phone:      sql.NullString{String: strings.TrimSpace(r.FormValue("phone")), Valid: true},
		ID:         clinicID,
	}

	if params.Name == "" || params.City.String == "" {
		h.renderClinicEditForm(w, database.Clinic{
			ID:         clinicID,
			Name:       params.Name,
			Phone:      params.Phone,
			AddressWay: params.AddressWay,
			ZipCode:    params.ZipCode,
			City:       params.City,
		}, "Le nom et la ville sont obligatoires.")
		return
	}

	if _, err := h.queries.UpdateClinic(r.Context(), params); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "clinic not found", http.StatusNotFound)
			return
		}
		h.renderClinicEditForm(w, database.Clinic{
			ID:         clinicID,
			Name:       params.Name,
			Phone:      params.Phone,
			AddressWay: params.AddressWay,
			ZipCode:    params.ZipCode,
			City:       params.City,
		}, "Impossible d'enregistrer les modifications pour le moment.")
		return
	}

	clinic, err := h.queries.GetClinicByID(r.Context(), clinicID)
	if err != nil {
		http.Error(w, "failed to load clinic", http.StatusInternalServerError)
		return
	}

	if err := h.tmpl().ExecuteTemplate(w, "clinic_card_info", clinic); err != nil {
		http.Error(w, "failed to render clinic", http.StatusInternalServerError)
	}
}

func (h *Handler) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	clinicID, err := strconv.ParseInt(chi.URLParam(r, "clinicID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid clinic id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		http.Error(w, "missing status", http.StatusBadRequest)
		return
	}

	updatedData := database.UpdateTrackingStatusParams{
		TrackStatus: status,
		ID:          clinicID,
	}

	if _, err := h.queries.UpdateTrackingStatus(r.Context(), updatedData); err != nil {
		http.Error(w, "failed to update status", http.StatusInternalServerError)
		return
	}
	slog.Info("status updated", "clinicID", clinicID, "new_status", status)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleHideClinic(w http.ResponseWriter, r *http.Request) {
	clinicID, err := strconv.ParseInt(chi.URLParam(r, "clinicID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid clinic id", http.StatusBadRequest)
		return
	}

	if err := h.queries.HideClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "failed to hide clinic", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleAddNote(w http.ResponseWriter, r *http.Request) {
	clinicID, err := strconv.ParseInt(chi.URLParam(r, "clinicID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid clinic id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		http.Error(w, "missing content", http.StatusBadRequest)
		return
	}

	note, err := h.addNote(r.Context(), clinicID, content)
	if err != nil {
		http.Error(w, "failed to add note", http.StatusInternalServerError)
		return
	}

	count, err := h.countNotes(r.Context(), clinicID)
	if err != nil {
		http.Error(w, "failed to count notes", http.StatusInternalServerError)
		return
	}

	t := h.tmpl()
	if err := t.ExecuteTemplate(w, "note_item", note); err != nil {
		http.Error(w, "failed to render note", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "notes_count_oob", NotesCountOOBData{ClinicID: clinicID, Count: count}); err != nil {
		http.Error(w, "failed to render count", http.StatusInternalServerError)
	}
}

func (h *Handler) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	clinicID, err := strconv.ParseInt(chi.URLParam(r, "clinicID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid clinic id", http.StatusBadRequest)
		return
	}
	noteID, err := strconv.ParseInt(chi.URLParam(r, "noteID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid note id", http.StatusBadRequest)
		return
	}

	if err := h.deleteNote(r.Context(), noteID); err != nil {
		h.logger.Error("handleDeleteNote", "clinic_id", clinicID, "note_id", noteID, "error", err)
		http.Error(w, "failed to delete note", http.StatusInternalServerError)
		return
	}

	count, err := h.countNotes(r.Context(), clinicID)
	if err != nil {
		http.Error(w, "failed to count notes", http.StatusInternalServerError)
		return
	}

	// Réponse vide pour le swap outerHTML du <li> (suppression visuelle)
	// + fragment OOB pour mettre à jour le compteur
	if err := h.tmpl().ExecuteTemplate(w, "notes_count_oob", NotesCountOOBData{ClinicID: clinicID, Count: count}); err != nil {
		http.Error(w, "failed to render count", http.StatusInternalServerError)
	}
}

func parseIntOrDefault(v string, fallback int) int {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func (h *Handler) renderClinicEditForm(w http.ResponseWriter, clinic database.Clinic, errorMessage string) {
	if err := h.tmpl().ExecuteTemplate(w, "clinic_card_edit_form", ClinicEditFormData{
		Clinic:       clinic,
		ErrorMessage: errorMessage,
	}); err != nil {
		http.Error(w, "failed to render clinic form", http.StatusInternalServerError)
	}
}

func (h *Handler) handleGetMap(w http.ResponseWriter, r *http.Request) {
	data, err := h.queries.GetClinicMapMaker(r.Context())
	if err != nil {
		slog.Error("Database error", "error", err)
		http.Error(w, "failed to load clinics", http.StatusInternalServerError)
		return
	}
	payload := markerList{
		Markers: data,
	}
	h.tmpl().ExecuteTemplate(w, "map", payload)
}
