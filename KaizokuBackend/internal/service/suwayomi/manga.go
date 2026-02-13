package suwayomi

import (
	"context"
	"fmt"
	"net/http"
)

// GetManga fetches a single manga by ID.
func (c *Client) GetManga(ctx context.Context, id int) (*SuwayomiSeries, error) {
	var result SuwayomiSeries
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/manga/%d", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetFullSeriesData fetches a manga with all data including chapters.
func (c *Client) GetFullSeriesData(ctx context.Context, id int, onlineFetch bool) (*SuwayomiSeries, error) {
	var result SuwayomiSeries
	path := fmt.Sprintf("/manga/%d/full?onlineFetch=%t", id, onlineFetch)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetMangaThumbnail fetches the thumbnail image for a manga.
func (c *Client) GetMangaThumbnail(ctx context.Context, id int) ([]byte, string, error) {
	return c.doRaw(ctx, http.MethodGet, fmt.Sprintf("/manga/%d/thumbnail", id))
}

// AddToLibrary adds a manga to the Suwayomi library.
func (c *Client) AddToLibrary(ctx context.Context, id int) error {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/manga/%d/library", id), nil, nil)
}

// RemoveFromLibrary removes a manga from the Suwayomi library.
func (c *Client) RemoveFromLibrary(ctx context.Context, id int) error {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/manga/%d/library", id), nil, nil)
}

// UpdateMangaMetadata updates a metadata key-value pair on a manga.
func (c *Client) UpdateMangaMetadata(ctx context.Context, id int, key, value string) error {
	body := MetadataUpdate{Key: key, Value: value}
	return c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/manga/%d/meta", id), body, nil)
}
