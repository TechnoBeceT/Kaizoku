package suwayomi

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// GetChapters fetches all chapters for a manga.
func (c *Client) GetChapters(ctx context.Context, mangaID int, onlineFetch bool) ([]SuwayomiChapter, error) {
	var result []SuwayomiChapter
	path := fmt.Sprintf("/manga/%d/chapters?onlineFetch=%t", mangaID, onlineFetch)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetChapter fetches a single chapter by manga ID and chapter index.
func (c *Client) GetChapter(ctx context.Context, mangaID, chapterIndex int) (*SuwayomiChapter, error) {
	var result SuwayomiChapter
	path := fmt.Sprintf("/manga/%d/chapter/%d", mangaID, chapterIndex)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPage fetches a single page image from a chapter.
// Returns ErrNotFound if the page does not exist (HTTP 404).
func (c *Client) GetPage(ctx context.Context, mangaID, chapterIndex, page int) ([]byte, string, error) {
	path := fmt.Sprintf("/manga/%d/chapter/%d/page/%d", mangaID, chapterIndex, page)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, "", ErrNotFound
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("unexpected status %d on GET %s", resp.StatusCode, path)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	return data, contentType, nil
}

// UpdateChapter updates chapter properties (read status, bookmarked, etc).
func (c *Client) UpdateChapter(ctx context.Context, mangaID, chapterIndex int, update ChapterUpdate) error {
	path := fmt.Sprintf("/manga/%d/chapter/%d", mangaID, chapterIndex)
	return c.doJSON(ctx, http.MethodPatch, path, update, nil)
}

// DeleteChapter deletes a downloaded chapter.
func (c *Client) DeleteChapter(ctx context.Context, mangaID, chapterIndex int) error {
	path := fmt.Sprintf("/manga/%d/chapter/%d", mangaID, chapterIndex)
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}
