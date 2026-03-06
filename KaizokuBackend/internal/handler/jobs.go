package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type JobsHandler struct {
	pool *pgxpool.Pool
}

type JobKindStatus struct {
	Kind      string `json:"kind"`
	Running   int    `json:"running"`
	Available int    `json:"available"`
	Scheduled int    `json:"scheduled"`
	Pending   int    `json:"pending"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	Total     int    `json:"total"`
}

type JobsStatusResponse struct {
	Kinds []JobKindStatus `json:"kinds"`
}

// GetJobStatus returns counts of river jobs grouped by kind and state.
// GET /api/jobs/status
func (h *JobsHandler) GetJobStatus(c echo.Context) error {
	ctx := c.Request().Context()

	rows, err := h.pool.Query(ctx, `
		SELECT kind, state, COUNT(*) as cnt
		FROM river_job
		WHERE state IN ('running', 'available', 'scheduled', 'pending', 'retryable')
		   OR (state = 'completed' AND finalized_at > NOW() - INTERVAL '1 hour')
		GROUP BY kind, state
		ORDER BY kind, state
	`)
	if err != nil {
		log.Error().Err(err).Msg("failed to query river jobs")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to query jobs"})
	}
	defer rows.Close()

	kindMap := make(map[string]*JobKindStatus)
	for rows.Next() {
		var kind, state string
		var cnt int
		if err := rows.Scan(&kind, &state, &cnt); err != nil {
			continue
		}
		ks, ok := kindMap[kind]
		if !ok {
			ks = &JobKindStatus{Kind: kind}
			kindMap[kind] = ks
		}
		switch state {
		case "running":
			ks.Running = cnt
		case "available":
			ks.Available = cnt
		case "scheduled":
			ks.Scheduled = cnt
		case "pending":
			ks.Pending = cnt
		case "completed":
			ks.Completed = cnt
		case "retryable":
			ks.Failed = cnt
		}
	}

	kinds := make([]JobKindStatus, 0, len(kindMap))
	for _, ks := range kindMap {
		ks.Total = ks.Running + ks.Available + ks.Scheduled + ks.Pending + ks.Completed + ks.Failed
		kinds = append(kinds, *ks)
	}

	return c.JSON(http.StatusOK, JobsStatusResponse{Kinds: kinds})
}
