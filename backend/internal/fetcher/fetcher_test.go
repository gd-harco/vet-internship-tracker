package fetcher

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestFetchByDepartmentWithContext(t *testing.T) {
	t.Parallel()

	requestedPages := make([]int, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != searchEndpoint {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		query := r.URL.Query()
		page, err := parseIntQuery(query, "page")
		if err != nil {
			t.Fatalf("invalid page query: %v", err)
		}
		limit, err := parseIntQuery(query, "limit")
		if err != nil {
			t.Fatalf("invalid limit query: %v", err)
		}
		departmentID, err := parseIntQuery(query, "departmentId")
		if err != nil {
			t.Fatalf("invalid department query: %v", err)
		}
		if limit != 15 {
			t.Fatalf("unexpected limit: %d", limit)
		}
		if departmentID != 70 {
			t.Fatalf("unexpected department id: %d", departmentID)
		}

		requestedPages = append(requestedPages, page)

		w.Header().Set("Content-Type", "application/json")
		switch page {
		case 0:
			_, _ = w.Write([]byte(`{
				"pagination": {"page": 0, "total": 20, "limit": 15},
				"veterinaries": [
					{
						"ordinalNumber": 1001,
						"lastname": "DURAND",
						"firstname": "ALICE",
						"dpes": [
							{
								"name": "CLINIQUE A",
								"phone": "0102030405",
								"ordinalNumber": "503737-1",
								"address": {
									"way": "1 RUE A",
									"additionalAddress1": "",
									"additionalAddress2": "",
									"zipCode": "69000",
									"city": "ECULLY"
								}
							}
						]
					}
				]
			}`))
		case 1:
			_, _ = w.Write([]byte(`{
				"pagination": {"page": 1, "total": 20, "limit": 15},
				"veterinaries": [
					{
						"ordinalNumber": 1002,
						"lastname": "MARTIN",
						"firstname": "BOB",
						"dpes": [
							{
								"name": "CLINIQUE B",
								"phone": "0607080910",
								"ordinalNumber": "503738-1",
								"address": {
									"way": "2 RUE B",
									"additionalAddress1": "",
									"additionalAddress2": "",
									"zipCode": "69001",
									"city": "ECULLY"
								}
							}
						]
					}
				]
			}`))
		default:
			t.Fatalf("unexpected page requested: %d", page)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)
	dpes, err := client.FetchByDepartmentWithContext(t.Context(), 70, 15)
	if err != nil {
		t.Fatalf("FetchByDepartmentWithContext returned error: %v", err)
	}

	if len(dpes) != 2 {
		t.Fatalf("expected 2 dpes, got %d", len(dpes))
	}
	if dpes[0].OrdinalNumber != "503737-1" || dpes[0].VetLastname != "DURAND" || dpes[0].VetFirstname != "ALICE" {
		t.Fatalf("unexpected first DPE mapping: %+v", dpes[0])
	}
	if dpes[1].OrdinalNumber != "503738-1" || dpes[1].VetLastname != "MARTIN" || dpes[1].VetFirstname != "BOB" {
		t.Fatalf("unexpected second DPE mapping: %+v", dpes[1])
	}
	if len(requestedPages) != 2 || requestedPages[0] != 0 || requestedPages[1] != 1 {
		t.Fatalf("unexpected requested pages: %+v", requestedPages)
	}
}

func parseIntQuery(values url.Values, key string) (int, error) {
	return strconv.Atoi(values.Get(key))
}
