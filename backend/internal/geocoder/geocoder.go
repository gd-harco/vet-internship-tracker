package geocoder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vet-tracker/backend/internal/database"
)

const (
	datagouvURL  = "https://data.geopf.fr/geocodage/search"
	nominatimURL = "https://nominatim.openstreetmap.org/search"
	userAgent    = "Vet-Tracker/1.0 Guillaume d'Harcourt g.dharcourt@posteo.com"
)

var (
	errRateLimited = errors.New("rate limited by nominatim")
	errNotFound    = errors.New("no result for this address")
	errTooVague    = errors.New("result too vague for a clinic")
)

var vagueKinds = map[string]bool{
	"country": true,
	"state":   true,
	"region":  true,
	"county":  true,
}

type geocodeResponse struct {
	Features []struct {
		Geometry struct {
			// GeoJSON : [longitude, latitude]
			Coordinates [2]float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties struct {
			Type string `json:"type"`
		} `json:"properties"`
	} `json:"features"`
}

type geocodeResult struct {
	Lat  float64
	Lng  float64
	Kind string
}

// Run géocode toutes les cliniques sans coordonnées.
func Run(ctx context.Context, dbquery *database.Queries, logger *slog.Logger) error {
	clinics, err := dbquery.GetNoGeoClinics(ctx)
	if err != nil {
		return fmt.Errorf("get clinics without coordinates: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	var updated, notFound, skipped, failed int

	for _, clinic := range clinics {
		res, err := geocodeDataGouv(ctx, client, clinic)
		if err != nil {
			if errors.Is(err, errRateLimited) {
				logger.Error("rate limited, stopping", "clinicID", clinic.ID)
				failed++
				break
			}
			if errors.Is(err, errNotFound) || errors.Is(err, errTooVague) {
				logger.Warn("clinic not geocoded", "error", err, "clinicID", clinic.ID)
				notFound++
				continue
			}
			logger.Error("geocoding failed for clinic", "error", err, "clinicID", clinic.ID)
			failed++
			continue
		}

		data := database.AddGeoToClinicsParams{
			Lat: sql.NullFloat64{Float64: res.Lat, Valid: true},
			Lng: sql.NullFloat64{Float64: res.Lng, Valid: true},
			ID:  clinic.ID,
		}
		if err := dbquery.AddGeoToClinics(ctx, data); err != nil {
			logger.Error("error adding coordinates to clinic", "error", err, "clinicID", clinic.ID, "lat", res.Lat, "lng", res.Lng)
			failed++
			continue
		}
		logger.Info("clinic geocoded", "clinicID", clinic.ID, "kind", res.Kind)
		updated++
	}

	logger.Info("geocoding finished",
		"total", len(clinics),
		"updated", updated,
		"notFound", notFound,
		"skipped", skipped,
		"failed", failed,
	)
	return nil
}

// GeocodeClinic géocode une clinique et enregistre ses coordonnées.
func GeocodeClinic(ctx context.Context, dbquery *database.Queries, clinic database.Clinic) (lat, lng float64, err error) {
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := geocodeDataGouv(ctx, client, clinic)
	if err != nil {
		return 0, 0, fmt.Errorf("geocode clinic %d: %w", clinic.ID, err)
	}
	if err := dbquery.AddGeoToClinics(ctx, database.AddGeoToClinicsParams{
		Lat: sql.NullFloat64{Float64: res.Lat, Valid: true},
		Lng: sql.NullFloat64{Float64: res.Lng, Valid: true},
		ID:  clinic.ID,
	}); err != nil {
		return 0, 0, fmt.Errorf("save coordinates for clinic %d: %w", clinic.ID, err)
	}
	return res.Lat, res.Lng, nil
}

func geocodeNominatim(ctx context.Context, client *http.Client, clinic database.Clinic) (geocodeResult, error) {
	q := url.Values{}
	addParam(q, "street", clinic.AddressWay.String)
	addParam(q, "city", clinic.City.String)
	addParam(q, "postalcode", clinic.ZipCode.String)
	q.Set("country", "France")
	q.Set("format", "geojson")
	q.Set("limit", "1")

	feature, err := search(ctx, client, nominatimURL, q)
	if err != nil {
		return geocodeResult{}, err
	}
	if vagueKinds[feature.Properties.Type] {
		return geocodeResult{}, fmt.Errorf("%w: %s", errTooVague, feature.Properties.Type)
	}
	return toResult(feature), nil
}

func geocodeDataGouv(ctx context.Context, client *http.Client, clinic database.Clinic) (geocodeResult, error) {
	q := url.Values{}
	addParam(q, "q", clinic.AddressWay.String)
	addParam(q, "city", clinic.City.String)
	addParam(q, "postcode", clinic.ZipCode.String)
	q.Set("type", "housenumber")
	q.Set("returntruegeometry", "false")
	q.Set("index", "address")
	q.Set("limit", "1")
	q.Set("autocomplete", "0")

	feature, err := search(ctx, client, datagouvURL, q)
	if err != nil {
		return geocodeResult{}, err
	}
	return toResult(feature), nil
}

type geocodeFeature = struct {
	Geometry struct {
		Coordinates [2]float64 `json:"coordinates"`
	} `json:"geometry"`
	Properties struct {
		Type string `json:"type"`
	} `json:"properties"`
}

func search(ctx context.Context, client *http.Client, baseURL string, q url.Values) (geocodeFeature, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return geocodeFeature{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return geocodeFeature{}, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return geocodeFeature{}, errRateLimited
	default:
		return geocodeFeature{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var body geocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return geocodeFeature{}, fmt.Errorf("decode response: %w", err)
	}
	if len(body.Features) == 0 {
		return geocodeFeature{}, errNotFound
	}
	return body.Features[0], nil
}

func toResult(f geocodeFeature) geocodeResult {
	return geocodeResult{
		Lng:  f.Geometry.Coordinates[0],
		Lat:  f.Geometry.Coordinates[1],
		Kind: f.Properties.Type,
	}
}

func addParam(q url.Values, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		q.Set(key, v)
	}
}
