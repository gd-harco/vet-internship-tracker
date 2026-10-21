package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const searchEndpoint = "/api/directories/search-veterinaries-for-care"

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type VetResponse struct {
	Pagination   Pagination   `json:"pagination"`
	Veterinaries []Veterinary `json:"veterinaries"`
}

type Pagination struct {
	Page  int `json:"page"`
	Total int `json:"total"`
	Limit int `json:"limit"`
}

type Veterinary struct {
	OrdinalNumber int64  `json:"ordinalNumber"`
	Lastname      string `json:"lastname"`
	Firstname     string `json:"firstname"`
	DPEs          []DPE  `json:"dpes"`
}

type DPE struct {
	Name          string     `json:"name"`
	Phone         string     `json:"phone"`
	OrdinalNumber string     `json:"ordinalNumber"`
	Address       DPEAddress `json:"address"`

	VetLastname      string `json:"-"`
	VetFirstname     string `json:"-"`
	VetOrdinalNumber int64  `json:"-"`
	HasVetOrdinal    bool   `json:"-"`
}

type DPEAddress struct {
	Way                string `json:"way"`
	AdditionalAddress1 string `json:"additionalAddress1"`
	AdditionalAddress2 string `json:"additionalAddress2"`
	ZipCode            string `json:"zipCode"`
	City               string `json:"city"`
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}


func (c *Client) FetchByDepartmentWithContext(ctx context.Context, departmentID int, limit int) ([]DPE, error) {
	if departmentID <= 0 {
		return nil, fmt.Errorf("department id must be positive")
	}
	if limit <= 0 {
		limit = 15
	}

	var (
		allDPEs    []DPE
		totalPages = 1
	)

	for page := 0; page < totalPages; page++ {
		resp, err := c.fetchPage(ctx, page, limit, departmentID)
		if err != nil {
			return nil, err
		}

		if resp.Pagination.Limit > 0 {
			limit = resp.Pagination.Limit
		}
		if resp.Pagination.Total <= 0 {
			totalPages = 0
		} else {
			totalPages = int(math.Ceil(float64(resp.Pagination.Total) / float64(limit)))
		}

		for _, vet := range resp.Veterinaries {
			for _, dpe := range vet.DPEs {
				dpe.VetLastname = vet.Lastname
				dpe.VetFirstname = vet.Firstname
				dpe.VetOrdinalNumber = vet.OrdinalNumber
				dpe.HasVetOrdinal = true
				allDPEs = append(allDPEs, dpe)
			}
		}
	}

	return allDPEs, nil
}

func (c *Client) fetchPage(ctx context.Context, page, limit, departmentID int) (VetResponse, error) {
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	query.Set("departmentId", strconv.Itoa(departmentID))
	query.Set("page", strconv.Itoa(page))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+searchEndpoint+"?"+query.Encode(), nil)
	if err != nil {
		return VetResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return VetResponse{}, fmt.Errorf("request API page %d: %w", page, err)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return VetResponse{}, fmt.Errorf("api returned status %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out VetResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return VetResponse{}, fmt.Errorf("decode API response page %d: %w", page, err)
	}

	return out, nil
}
