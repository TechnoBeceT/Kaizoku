# Komga Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Notify Komga to scan a series after each successful chapter download, configured via the Settings page.

**Architecture:** New `internal/service/komga/` package with HTTP client using Basic Auth. Settings stored in DB alongside existing settings (FlareSolverr pattern). Hook into `dlqueue.go` post-download completion to call Komga API. Frontend adds a new section to SettingsManager.

**Tech Stack:** Go net/http (Basic Auth), Komga REST API v1, Vue 3 + Nuxt UI v4

---

### Task 1: Komga API Client

**Files:**
- Create: `KaizokuBackend/internal/service/komga/client.go`

**Step 1: Create the Komga client package**

```go
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
```

**Step 2: Verify it compiles**

Run: `cd KaizokuBackend && go build ./internal/service/komga/`
Expected: Success (no errors)

**Step 3: Commit**

```bash
git add KaizokuBackend/internal/service/komga/client.go
git commit -m "feat: add Komga API client"
```

---

### Task 2: Add Komga Settings Fields

**Files:**
- Modify: `KaizokuBackend/internal/types/dto.go:4-25` (Settings struct)
- Modify: `KaizokuBackend/internal/service/settings/service.go:256-339` (serialize/deserialize)

**Step 1: Add Komga fields to Settings struct**

In `internal/types/dto.go`, add after line 23 (`FlareSolverrAsResponseFallback`):

```go
KomgaEnabled                             bool     `json:"komgaEnabled"`
KomgaURL                                 string   `json:"komgaUrl"`
KomgaUsername                            string   `json:"komgaUsername"`
KomgaPassword                            string   `json:"komgaPassword"`
```

In `DefaultSettings()`, add inside the return (after `FlareSolverrAsResponseFallback: false,`):

```go
KomgaEnabled:                            false,
KomgaURL:                                "",
KomgaUsername:                            "",
KomgaPassword:                           "",
```

**Step 2: Add serialize/deserialize for Komga fields**

In `internal/service/settings/service.go`, function `serialize()`, add to the map (after `FlareSolverrAsResponseFallback`):

```go
"KomgaEnabled":                       strconv.FormatBool(s.KomgaEnabled),
"KomgaUrl":                           s.KomgaURL,
"KomgaUsername":                      s.KomgaUsername,
"KomgaPassword":                     s.KomgaPassword,
```

In function `deserialize()`, add after the `FlareSolverrAsResponseFallback` block:

```go
if v, ok := kv["KomgaEnabled"]; ok {
    s.KomgaEnabled, _ = strconv.ParseBool(v)
}
if v, ok := kv["KomgaUrl"]; ok {
    s.KomgaURL = v
}
if v, ok := kv["KomgaUsername"]; ok {
    s.KomgaUsername = v
}
if v, ok := kv["KomgaPassword"]; ok {
    s.KomgaPassword = v
}
```

**Step 3: Verify it compiles**

Run: `cd KaizokuBackend && go build ./...`
Expected: Success

**Step 4: Commit**

```bash
git add KaizokuBackend/internal/types/dto.go KaizokuBackend/internal/service/settings/service.go
git commit -m "feat: add Komga settings fields to backend"
```

---

### Task 3: Hook Komga Notification into Download Flow

**Files:**
- Modify: `KaizokuBackend/internal/job/workers.go:44-54` (Deps struct)
- Modify: `KaizokuBackend/internal/job/dlqueue.go:386-393` (post-download hook)
- Modify: `KaizokuBackend/internal/job/manager.go:36-44` (wire Komga into deps)

**Step 1: Add Komga field to Deps and notifyKomga method**

In `internal/job/workers.go`, add import:
```go
"github.com/technobecet/kaizoku-go/internal/service/komga"
```

Add to Deps struct (after `RiverClient`):
```go
Komga         *komga.Client       // Optional Komga client for post-download notifications
```

Add `notifyKomga` method (after `handleReplacementSuccess`):

```go
// notifyKomga notifies Komga to scan the series after a download.
// Reads settings to check if Komga is enabled; silently returns on any error.
func (d *Deps) notifyKomga(ctx context.Context, storagePath string) {
	if d.Komga == nil || storagePath == "" {
		return
	}
	// Run in goroutine so it doesn't block the download pipeline
	go func() {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d.Komga.NotifySeriesUpdate(notifyCtx, storagePath)
	}()
}
```

**Step 2: Call notifyKomga from dlqueue.go post-download**

In `internal/job/dlqueue.go`, after line 391 (the `handleDownloadSuccess` / `handleReplacementSuccess` calls), add:

```go
// Notify Komga to scan the updated series
d.deps.notifyKomga(ctx, args.StoragePath)
```

This goes right after the `if args.IsReplacement { ... } else { ... }` block, around line 393.

**Step 3: Create and wire Komga client in manager.go**

In `internal/job/manager.go`, add import:
```go
"github.com/technobecet/kaizoku-go/internal/service/komga"
```

After line 131 (`deps.RiverClient = riverClient`), add:

```go
// Create Komga client if configured — reads settings on first use
if settings != nil {
    s, err := settings.Get(ctx)
    if err == nil && s.KomgaEnabled && s.KomgaURL != "" {
        deps.Komga = komga.NewClient(s.KomgaURL, s.KomgaUsername, s.KomgaPassword)
        log.Info().Str("url", s.KomgaURL).Msg("Komga integration enabled")
    }
}
```

**Important consideration:** The Komga client is created at startup from settings. If the user changes Komga settings at runtime, they'd need to restart. To handle runtime changes, we need a lazy approach instead.

**Step 3 (revised): Use lazy Komga client initialization**

Instead of creating the client in manager.go, modify the `notifyKomga` method to read settings and create a client each time (the overhead is negligible since it's just struct creation):

```go
// notifyKomga notifies Komga to scan the series after a download.
// Reads settings each time to pick up runtime config changes.
func (d *Deps) notifyKomga(ctx context.Context, storagePath string) {
	if d.Settings == nil || storagePath == "" {
		return
	}
	settings, err := d.Settings.Get(ctx)
	if err != nil || !settings.KomgaEnabled || settings.KomgaURL == "" {
		return
	}
	client := komga.NewClient(settings.KomgaURL, settings.KomgaUsername, settings.KomgaPassword)
	go func() {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client.NotifySeriesUpdate(notifyCtx, storagePath)
	}()
}
```

With this approach, no changes to `manager.go` or `Deps` struct needed (no `Komga` field). The `notifyKomga` method uses the existing `Settings` reader.

**Step 4: Verify it compiles**

Run: `cd KaizokuBackend && go build ./...`
Expected: Success

**Step 5: Commit**

```bash
git add KaizokuBackend/internal/job/workers.go KaizokuBackend/internal/job/dlqueue.go
git commit -m "feat: notify Komga after chapter downloads"
```

---

### Task 4: Test Connection Endpoint

**Files:**
- Modify: `KaizokuBackend/internal/handler/settings.go`
- Modify: `KaizokuBackend/internal/server/routes.go:66-69`

**Step 1: Add TestKomga handler**

In `internal/handler/settings.go`, add import:
```go
"github.com/technobecet/kaizoku-go/internal/service/komga"
```

Add handler method:

```go
func (h *SettingsHandler) TestKomga(c echo.Context) error {
	var req struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if req.URL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "URL is required"})
	}

	client := komga.NewClient(req.URL, req.Username, req.Password)
	if err := client.TestConnection(c.Request().Context()); err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	libs, _ := client.GetLibraries(c.Request().Context())
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":   true,
		"libraries": libs,
	})
}
```

**Step 2: Add route**

In `internal/server/routes.go`, after line 69 (`settings.PUT("", h.Settings.UpdateSettings)`), add:

```go
settings.POST("/komga-test", h.Settings.TestKomga)
```

**Step 3: Verify it compiles**

Run: `cd KaizokuBackend && go build ./...`
Expected: Success

**Step 4: Commit**

```bash
git add KaizokuBackend/internal/handler/settings.go KaizokuBackend/internal/server/routes.go
git commit -m "feat: add Komga test connection endpoint"
```

---

### Task 5: Frontend Settings Types and Service

**Files:**
- Modify: `KaizokuFrontend/app/types/index.ts:44-65` (Settings interface)
- Modify: `KaizokuFrontend/app/services/settingsService.ts`
- Modify: `KaizokuFrontend/app/composables/useSettings.ts`

**Step 1: Add Komga fields to Settings interface**

In `KaizokuFrontend/app/types/index.ts`, add to the `Settings` interface after `flareSolverrAsResponseFallback`:

```typescript
komgaEnabled: boolean
komgaUrl: string
komgaUsername: string
komgaPassword: string
```

**Step 2: Add test connection service method**

In `KaizokuFrontend/app/services/settingsService.ts`, add method:

```typescript
async testKomga(url: string, username: string, password: string): Promise<{ success: boolean; error?: string; libraries?: Array<{ id: string; name: string }> }> {
    return apiClient.post('/api/settings/komga-test', { url, username, password })
},
```

**Step 3: Add composable for test connection**

In `KaizokuFrontend/app/composables/useSettings.ts`, add:

```typescript
export function useTestKomga() {
  return useMutation({
    mutationFn: (params: { url: string; username: string; password: string }) =>
      settingsService.testKomga(params.url, params.username, params.password),
  })
}
```

**Step 4: Commit**

```bash
git add KaizokuFrontend/app/types/index.ts KaizokuFrontend/app/services/settingsService.ts KaizokuFrontend/app/composables/useSettings.ts
git commit -m "feat: add Komga settings types and service to frontend"
```

---

### Task 6: Frontend Settings UI Section

**Files:**
- Modify: `KaizokuFrontend/app/components/settings/SettingsManager.vue`

**Step 1: Add 'komga' to SECTION_IDS**

Change line 185:
```typescript
const SECTION_IDS = ['content-preferences', 'mihon-repositories', 'download-settings', 'schedule-tasks', 'storage', 'flaresolverr', 'komga'] as const
```

**Step 2: Add Komga test connection state**

In the `<script setup>` section, after the `newCategory` ref (around line 34), add:

```typescript
const testKomgaMutation = useTestKomga()
const komgaTestResult = ref<{ success: boolean; error?: string; libraries?: Array<{ id: string; name: string }> } | null>(null)

async function testKomga() {
  if (!localSettings.value) return
  komgaTestResult.value = null
  const result = await testKomgaMutation.mutateAsync({
    url: localSettings.value.komgaUrl,
    username: localSettings.value.komgaUsername,
    password: localSettings.value.komgaPassword,
  })
  komgaTestResult.value = result
}
```

**Step 3: Add Komga settings card to template**

After the FlareSolverr `</UCard>` (line 405), before `</div>` (line 406), add:

```vue
<!-- Komga Integration -->
<UCard v-if="showSection('komga')">
  <template #header>
    <div>
      <h3 class="font-semibold">Komga Integration</h3>
      <p class="text-sm text-muted">Connect to Komga for automatic library updates after downloads.</p>
    </div>
  </template>
  <div class="space-y-4">
    <div class="flex items-center gap-2">
      <USwitch :model-value="localSettings.komgaEnabled" @update:model-value="localSettings!.komgaEnabled = $event; notifyChange()" />
      <label class="text-sm">Enable Komga Integration</label>
    </div>
    <div v-if="localSettings.komgaEnabled" class="space-y-4 pl-6 border-l-2 border-muted">
      <div>
        <label class="text-sm font-medium">Komga URL</label>
        <UInput :model-value="localSettings.komgaUrl" placeholder="http://localhost:25600" @update:model-value="localSettings!.komgaUrl = $event as string; notifyChange()" />
      </div>
      <div>
        <label class="text-sm font-medium">Username</label>
        <UInput :model-value="localSettings.komgaUsername" placeholder="admin@example.com" @update:model-value="localSettings!.komgaUsername = $event as string; notifyChange()" />
      </div>
      <div>
        <label class="text-sm font-medium">Password</label>
        <UInput type="password" :model-value="localSettings.komgaPassword" placeholder="Password" @update:model-value="localSettings!.komgaPassword = $event as string; notifyChange()" />
      </div>
      <div class="flex items-center gap-3">
        <UButton
          size="sm"
          variant="outline"
          icon="i-lucide-plug"
          label="Test Connection"
          :loading="testKomgaMutation.isPending.value"
          :disabled="!localSettings.komgaUrl"
          @click="testKomga"
        />
        <UBadge v-if="komgaTestResult?.success" color="success" size="xs">
          Connected — {{ komgaTestResult.libraries?.length || 0 }} libraries found
        </UBadge>
        <UBadge v-else-if="komgaTestResult && !komgaTestResult.success" color="error" size="xs">
          {{ komgaTestResult.error }}
        </UBadge>
      </div>
    </div>
  </div>
</UCard>
```

**Step 4: Verify frontend builds**

Run: `cd KaizokuFrontend && bun run generate`
Expected: Success

**Step 5: Commit**

```bash
git add KaizokuFrontend/app/components/settings/SettingsManager.vue
git commit -m "feat: add Komga integration section to settings UI"
```

---

### Task 7: Build and Verify

**Step 1: Full backend build**

Run: `cd KaizokuBackend && go build ./...`
Expected: Success

**Step 2: Full frontend build**

Run: `cd KaizokuFrontend && bun run generate`
Expected: Success

**Step 3: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix: build fixes for Komga integration"
```
