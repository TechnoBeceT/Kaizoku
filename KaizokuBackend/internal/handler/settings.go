package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/service/komga"
	settingssvc "github.com/technobecet/kaizoku-go/internal/service/settings"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
	"github.com/technobecet/kaizoku-go/internal/types"
)

type SettingsHandler struct {
	config   *config.Config
	db       *ent.Client
	settings *settingssvc.Service
}

func NewSettingsHandler(cfg *config.Config, db *ent.Client, sw *suwayomi.Client) *SettingsHandler {
	return &SettingsHandler{
		config:   cfg,
		db:       db,
		settings: settingssvc.NewService(db, cfg, sw),
	}
}

func (h *SettingsHandler) GetSettings(c echo.Context) error {
	settings, err := h.settings.Get(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, settings)
}

func (h *SettingsHandler) GetLanguages(c echo.Context) error {
	languages, err := h.settings.GetLanguages(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, languages)
}

func (h *SettingsHandler) UpdateSettings(c echo.Context) error {
	var settings types.Settings
	if err := c.Bind(&settings); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if err := h.settings.Save(c.Request().Context(), &settings); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Settings updated successfully"})
}

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
		return c.JSON(http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
	}

	libs, _ := client.GetLibraries(c.Request().Context())
	return c.JSON(http.StatusOK, map[string]any{
		"success":   true,
		"libraries": libs,
	})
}
