package komga

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Client is an HTTP client for the Komga server API.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewClient creates a new Komga API client.
func NewClient(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Library represents a Komga library.
type Library struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Root string `json:"root"`
}

// SeriesResult represents a series from Komga search.
type SeriesResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// SeriesPage is the paginated response from Komga series search.
type SeriesPage struct {
	Content []SeriesResult `json:"content"`
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	reqURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}

// TestConnection checks if Komga is reachable with valid credentials.
func (c *Client) TestConnection(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/libraries", nil)
	if err != nil {
		return fmt.Errorf("komga unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("komga authentication failed (401)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("komga returned status %d", resp.StatusCode)
	}
	return nil
}

// GetLibraries returns all Komga libraries.
func (c *Client) GetLibraries(ctx context.Context) ([]Library, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/libraries", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("komga libraries: status %d", resp.StatusCode)
	}
	var libs []Library
	if err := json.NewDecoder(resp.Body).Decode(&libs); err != nil {
		return nil, fmt.Errorf("decode libraries: %w", err)
	}
	return libs, nil
}

// ScanLibrary triggers a scan on a specific Komga library.
func (c *Client) ScanLibrary(ctx context.Context, libraryID string) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/libraries/"+libraryID+"/scan", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("komga scan library: status %d", resp.StatusCode)
	}
	return nil
}

// ScanSeries triggers an analysis on a specific Komga series.
func (c *Client) ScanSeries(ctx context.Context, seriesID string) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/series/"+seriesID+"/analyze", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("komga scan series: status %d", resp.StatusCode)
	}
	return nil
}

// FindSeriesByPath searches Komga for a series whose folder name matches.
// storagePath is the full path like "/series/Manga/One Piece".
// It extracts the folder name ("One Piece") and searches Komga.
func (c *Client) FindSeriesByPath(ctx context.Context, storagePath string) (string, error) {
	folderName := filepath.Base(storagePath)

	searchURL := "/api/v1/series?search=" + url.QueryEscape(folderName) + "&size=20"
	resp, err := c.doRequest(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("komga search: status %d", resp.StatusCode)
	}
	var page SeriesPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return "", fmt.Errorf("decode search: %w", err)
	}

	// Exact match on series name (Komga uses folder name as series name by default)
	for _, s := range page.Content {
		if strings.EqualFold(s.Name, folderName) {
			return s.ID, nil
		}
	}
	return "", nil
}

// NotifySeriesUpdate finds a series in Komga by its storage path and triggers a scan.
// If the series isn't found in Komga yet, it scans all libraries to discover it.
// Returns silently on any error (caller should not block on Komga issues).
func (c *Client) NotifySeriesUpdate(ctx context.Context, storagePath string) {
	seriesID, err := c.FindSeriesByPath(ctx, storagePath)
	if err != nil {
		log.Warn().Err(err).Str("path", storagePath).Msg("komga: failed to find series")
		return
	}

	if seriesID != "" {
		if err := c.ScanSeries(ctx, seriesID); err != nil {
			log.Warn().Err(err).Str("seriesID", seriesID).Msg("komga: failed to scan series")
		} else {
			log.Debug().Str("seriesID", seriesID).Str("path", storagePath).Msg("komga: triggered series scan")
		}
		return
	}

	// Series not found — scan all libraries so Komga discovers it
	libs, err := c.GetLibraries(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("komga: failed to get libraries for fallback scan")
		return
	}
	for _, lib := range libs {
		if err := c.ScanLibrary(ctx, lib.ID); err != nil {
			log.Warn().Err(err).Str("library", lib.Name).Msg("komga: failed to scan library")
		}
	}
	log.Debug().Str("path", storagePath).Msg("komga: triggered library scan for new series")
}
