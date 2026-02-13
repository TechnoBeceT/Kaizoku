package handler

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/job"
	"github.com/technobecet/kaizoku-go/internal/types"
)

type DownloadsHandler struct {
	config    *config.Config
	db        *ent.Client
	downloads *job.DownloadDispatcher
}

// GetDownloads returns downloads filtered by status with pagination.
// GET /api/downloads?status=0&limit=100&offset=0&keyword=
func (h *DownloadsHandler) GetDownloads(c echo.Context) error {
	ctx := c.Request().Context()

	statusParam := c.QueryParam("status")
	limitParam := c.QueryParam("limit")
	offsetParam := c.QueryParam("offset")
	keyword := c.QueryParam("keyword")

	limit := 100
	if limitParam != "" {
		if v, err := strconv.Atoi(limitParam); err == nil && v > 0 {
			limit = v
		}
	}

	offset := 0
	if offsetParam != "" {
		if v, err := strconv.Atoi(offsetParam); err == nil && v >= 0 {
			offset = v
		}
	}

	var status *int
	if statusParam != "" {
		if v, err := strconv.Atoi(statusParam); err == nil {
			status = &v
		}
	}

	result := h.downloads.GetDownloads(ctx, status, limit, offset, keyword)
	return c.JSON(http.StatusOK, result)
}

// GetSeriesDownloads returns downloads for a specific series.
// GET /api/downloads/series?seriesId=<uuid>
func (h *DownloadsHandler) GetSeriesDownloads(c echo.Context) error {
	ctx := c.Request().Context()
	seriesID := c.QueryParam("seriesId")
	if seriesID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "seriesId required"})
	}

	downloads := h.downloads.GetSeriesDownloads(ctx, seriesID)
	return c.JSON(http.StatusOK, downloads)
}

// GetDownloadMetrics returns counts of running, queued, and failed downloads.
// GET /api/downloads/metrics
func (h *DownloadsHandler) GetDownloadMetrics(c echo.Context) error {
	ctx := c.Request().Context()
	metrics := h.downloads.GetMetrics(ctx)
	return c.JSON(http.StatusOK, metrics)
}

// ManageErrorDownload retries or deletes a failed download.
// PATCH /api/downloads?id=<uuid>&action=Retry|Delete
func (h *DownloadsHandler) ManageErrorDownload(c echo.Context) error {
	ctx := c.Request().Context()
	idStr := c.QueryParam("id")
	action := c.QueryParam("action")

	if idStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id required"})
	}

	itemID, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	switch types.ParseErrorDownloadAction(action) {
	case types.ErrorDownloadActionRetry:
		if err := h.downloads.RetryDownload(ctx, itemID); err != nil {
			log.Error().Err(err).Str("id", idStr).Msg("failed to retry download")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to retry download"})
		}
		log.Info().Str("id", idStr).Msg("download retried")

	case types.ErrorDownloadActionDelete:
		if err := h.downloads.DeleteDownload(ctx, itemID); err != nil {
			log.Error().Err(err).Str("id", idStr).Msg("failed to delete download")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete download"})
		}
		log.Info().Str("id", idStr).Msg("download deleted")

	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid action"})
	}

	return c.JSON(http.StatusOK, nil)
}
