package suwayomi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// graphqlURL returns the GraphQL endpoint by replacing /v1 in the base URL.
func (c *Client) graphqlURL() string {
	return strings.Replace(c.baseURL, "/v1", "", 1) + "/graphql"
}

// GetServerSettings fetches the Suwayomi server settings.
func (c *Client) GetServerSettings(ctx context.Context) (*SuwayomiSettings, error) {
	var result SuwayomiSettings
	if err := c.doJSON(ctx, http.MethodGet, "/settings", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetServerSettings updates the Suwayomi server settings via REST (partial update).
func (c *Client) SetServerSettings(ctx context.Context, settings interface{}) error {
	return c.doJSON(ctx, http.MethodPatch, "/settings", settings, nil)
}

// SetServerSettingsGraphQL updates settings via Suwayomi's GraphQL inline mutation.
// Suwayomi's GraphQL implementation requires inline mutations (not parameterized variables).
// This is required for fields like extensionRepos that are only settable through GraphQL.
func (c *Client) SetServerSettingsGraphQL(ctx context.Context, settings map[string]interface{}) error {
	// Build inline settings fields for the GraphQL mutation
	fields := make([]string, 0, len(settings))
	for k, v := range settings {
		fields = append(fields, fmt.Sprintf("%s: %s", k, toGraphQLValue(v)))
	}

	query := fmt.Sprintf(
		`mutation { setSettings(input: { settings: { %s } }) { settings { extensionRepos maxSourcesInParallel flareSolverrEnabled } } }`,
		strings.Join(fields, ", "),
	)

	body := map[string]interface{}{
		"query": query,
	}

	resp, err := c.doRequestURL(ctx, http.MethodPost, c.graphqlURL(), body)
	if err != nil {
		return fmt.Errorf("graphql settings update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("graphql settings update: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// toGraphQLValue converts a Go value to a GraphQL literal string.
func toGraphQLValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case []string:
		items := make([]string, len(val))
		for i, s := range val {
			items[i] = fmt.Sprintf("%q", s)
		}
		return "[" + strings.Join(items, ", ") + "]"
	case []interface{}:
		items := make([]string, len(val))
		for i, item := range val {
			items[i] = toGraphQLValue(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	default:
		return fmt.Sprintf("%v", val)
	}
}
