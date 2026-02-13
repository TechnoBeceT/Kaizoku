package suwayomi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// GetSources fetches all available sources.
func (c *Client) GetSources(ctx context.Context) ([]SuwayomiSource, error) {
	var result []SuwayomiSource
	if err := c.doJSON(ctx, http.MethodGet, "/source/list", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SearchSeries searches for manga in a specific source.
func (c *Client) SearchSeries(ctx context.Context, sourceID, searchTerm string, pageNum int) (*MangaSearchResult, error) {
	var result MangaSearchResult
	path := fmt.Sprintf("/source/%s/search?searchTerm=%s&pageNum=%d",
		sourceID, url.QueryEscape(searchTerm), pageNum)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetLatestSeries fetches the latest series from a source.
func (c *Client) GetLatestSeries(ctx context.Context, sourceID string, page int) (*MangaSearchResult, error) {
	var result MangaSearchResult
	path := fmt.Sprintf("/source/%s/latest/%d", sourceID, page)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPopularSeries fetches the popular series from a source.
func (c *Client) GetPopularSeries(ctx context.Context, sourceID string, page int) (*MangaSearchResult, error) {
	var result MangaSearchResult
	path := fmt.Sprintf("/source/%s/popular/%d", sourceID, page)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetSourceIcon fetches the icon for a source.
func (c *Client) GetSourceIcon(ctx context.Context, sourceID string) ([]byte, string, error) {
	return c.doRaw(ctx, http.MethodGet, fmt.Sprintf("/source/%s/icon", sourceID))
}

// GetSourcePreferences fetches preferences for a source.
func (c *Client) GetSourcePreferences(ctx context.Context, sourceID string) ([]SuwayomiPreference, error) {
	var result []SuwayomiPreference
	path := fmt.Sprintf("/source/%s/preferences", sourceID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SetSourcePreference sets a preference value for a source.
func (c *Client) SetSourcePreference(ctx context.Context, sourceID string, position int, value interface{}) error {
	body := SetPreferenceRequest{Position: position, Value: value}
	path := fmt.Sprintf("/source/%s/preferences", sourceID)
	return c.doJSON(ctx, http.MethodPost, path, body, nil)
}
