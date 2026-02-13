package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/technobecet/kaizoku-go/internal/handler"
)

func registerRoutes(e *echo.Echo, h *handler.Handler) {
	// Health check
	e.GET("/health", HealthCheck)

	// Root redirect to library
	e.GET("/", func(c echo.Context) error {
		return c.Redirect(http.StatusFound, "/library")
	})

	api := e.Group("/api")

	// Series / Library
	serie := api.Group("/serie")
	serie.GET("", h.Series.GetSeries)
	serie.GET("/library", h.Series.GetLibrary)
	serie.GET("/latest", h.Series.GetLatest)
	serie.GET("/verify", h.Series.VerifyIntegrity)
	serie.GET("/cleanup", h.Series.CleanupSeries)
	serie.GET("/deep-verify", h.Series.DeepVerify)
	serie.POST("/verify-all", h.Series.VerifyAll)
	serie.GET("/match/:providerId", h.Series.GetProviderMatch)
	serie.GET("/source", h.Series.GetSources)
	serie.GET("/source/icon/:apk", h.Series.GetSourceIcon)
	serie.GET("/thumb/:id", h.Series.GetSeriesThumbnail)
	serie.POST("", h.Series.AddSeries)
	serie.POST("/update-all", h.Series.UpdateAllSeries)
	serie.PATCH("", h.Series.UpdateSeries)
	serie.DELETE("", h.Series.DeleteSeries)
	serie.POST("/match", h.Series.SetProviderMatch)

	// Search
	search := api.Group("/search")
	search.GET("", h.Search.SearchSeries)
	search.GET("/sources", h.Search.GetSearchSources)
	search.POST("/augment", h.Search.AugmentSeries)

	// Downloads
	downloads := api.Group("/downloads")
	downloads.GET("", h.Downloads.GetDownloads)
	downloads.GET("/series", h.Downloads.GetSeriesDownloads)
	downloads.GET("/metrics", h.Downloads.GetDownloadMetrics)
	downloads.PATCH("", h.Downloads.ManageErrorDownload)

	// Provider
	provider := api.Group("/provider")
	provider.GET("/list", h.Provider.GetProviders)
	provider.GET("/preferences/:pkg", h.Provider.GetProviderPreferences)
	provider.GET("/icon/:apk", h.Provider.GetProviderIcon)
	provider.POST("/install/:pkg", h.Provider.InstallProvider)
	provider.POST("/install/file", h.Provider.InstallProviderFile)
	provider.POST("/uninstall/:pkg", h.Provider.UninstallProvider)
	provider.POST("/preferences", h.Provider.SetProviderPreferences)

	// Settings
	settings := api.Group("/settings")
	settings.GET("", h.Settings.GetSettings)
	settings.GET("/languages", h.Settings.GetLanguages)
	settings.PUT("", h.Settings.UpdateSettings)

	// Setup / Import wizard
	setup := api.Group("/setup")
	setup.POST("/scan", h.Setup.ScanLocalFiles)
	setup.POST("/install-extensions", h.Setup.InstallExtensions)
	setup.POST("/search", h.Setup.SearchProviders)
	setup.POST("/augment", h.Setup.AugmentImport)
	setup.POST("/update", h.Setup.UpdateImport)
	setup.POST("/import", h.Setup.ImportSeries)
	setup.GET("/imports", h.Setup.GetImports)
	setup.GET("/imports/totals", h.Setup.GetImportTotals)

	// Reporting
	reporting := api.Group("/reporting")
	reporting.GET("/overview", h.Reporting.GetOverview)
	reporting.GET("/sources", h.Reporting.GetSources)
	reporting.GET("/source/:sourceId/events", h.Reporting.GetSourceEvents)
	reporting.GET("/source/:sourceId/timeline", h.Reporting.GetSourceTimeline)
}
