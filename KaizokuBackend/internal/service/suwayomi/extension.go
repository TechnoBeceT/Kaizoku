package suwayomi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// GetExtensions fetches all available extensions.
func (c *Client) GetExtensions(ctx context.Context) ([]SuwayomiExtension, error) {
	var result []SuwayomiExtension
	if err := c.doJSON(ctx, http.MethodGet, "/extension/list", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// InstallExtension installs an extension by package name.
func (c *Client) InstallExtension(ctx context.Context, pkgName string) error {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/extension/install/%s", pkgName), nil, nil)
}

// UpdateExtension updates an extension by package name.
func (c *Client) UpdateExtension(ctx context.Context, pkgName string) error {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/extension/update/%s", pkgName), nil, nil)
}

// UninstallExtension uninstalls an extension by package name.
func (c *Client) UninstallExtension(ctx context.Context, pkgName string) error {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/extension/uninstall/%s", pkgName), nil, nil)
}

// InstallExtensionFromFile installs an extension from an APK file upload.
func (c *Client) InstallExtensionFromFile(ctx context.Context, filename string, fileData io.Reader) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.Copy(part, fileData); err != nil {
		return fmt.Errorf("copy file data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	url := c.baseURL + "/extension/install"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// GetExtensionIcon fetches the icon for an extension by APK name.
func (c *Client) GetExtensionIcon(ctx context.Context, apkName string) ([]byte, string, error) {
	return c.doRaw(ctx, http.MethodGet, fmt.Sprintf("/extension/icon/%s", apkName))
}
