# Komga Integration Design

## Overview

After each successful chapter download, Kaizoku notifies Komga to scan the affected series. Komga connection is configured via the Settings page (DB-stored). If Komga is not configured or unreachable, downloads proceed normally with a warning log.

## Decisions

- **Config location:** Settings page (DB-stored, editable from UI)
- **Scan trigger:** After each chapter download (immediate per-chapter)
- **Library mapping:** User creates Komga libraries manually pointing to same folders
- **Error handling:** Silent skip — log warning, don't interrupt downloads

## Components

### 1. Komga API Client (`internal/service/komga/client.go`)

HTTP client with Basic Auth. Methods:

- `FindSeriesByPath(ctx, folderName) → komgaSeriesID` — search Komga for series matching folder name
- `ScanSeries(ctx, komgaSeriesID)` — `POST /api/v1/series/{id}/analyze`
- `ScanLibrary(ctx, komgaLibraryID)` — fallback for full library scan
- `GetLibraries(ctx) → []Library` — list libraries (for settings UI test)
- `TestConnection(ctx) → bool` — validate URL + credentials

Errors are logged as warnings and returned to caller. Caller ignores them.

### 2. Settings Fields (DB-stored)

| Field | Type | Default |
|-------|------|---------|
| `KomgaEnabled` | bool | false |
| `KomgaURL` | string | "" |
| `KomgaUsername` | string | "" |
| `KomgaPassword` | string | "" |

### 3. Series-to-Komga Mapping

No persistent mapping. Each scan does a one-shot lookup:

1. Download completes → get series `storagePath` (e.g., `/series/Manga/One Piece`)
2. Extract folder name from path
3. Query Komga: search series by folder name
4. Found → trigger scan on that series ID
5. Not found → trigger library-level scan (Komga discovers new series)

Avoids schema changes and handles renamed/moved series naturally.

### 4. Hook Point (`workers.go`)

After `handleDownloadSuccess()` and `handleReplacementSuccess()` complete, call `notifyKomga(ctx, storagePath)`.

```
notifyKomga(ctx, storagePath):
  settings.KomgaEnabled? No → return
  komgaClient.FindSeriesByPath(folderName)
  Found? → komgaClient.ScanSeries(id)
  Not found? → komgaClient.ScanLibrary()
```

### 5. Frontend Settings Section

New "Komga Integration" section in SettingsManager:
- Enabled toggle
- URL input
- Username input
- Password input (masked)
- "Test Connection" button → `POST /api/settings/komga-test`

## Files to Create/Modify

| File | Action |
|------|--------|
| `internal/service/komga/client.go` | Create — HTTP client |
| `internal/types/dto.go` | Edit — add Komga fields to Settings struct |
| `internal/service/settings/service.go` | Edit — serialize/deserialize Komga fields |
| `internal/job/workers.go` | Edit — add notifyKomga calls after download success |
| `internal/handler/settings.go` | Edit — add TestKomga endpoint |
| `internal/server/routes.go` | Edit — add route |
| `KaizokuFrontend/app/types/index.ts` | Edit — add Komga settings fields |
| `KaizokuFrontend/app/components/settings/SettingsManager.vue` | Edit — add Komga section |
