package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/handler"
	"github.com/technobecet/kaizoku-go/internal/job"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
	"github.com/technobecet/kaizoku-go/internal/ws"
)

type Server struct {
	echo        *echo.Echo
	config      *config.Config
	db          *ent.Client
	handler     *handler.Handler
	ProgressHub *ws.Hub
}

func New(cfg *config.Config, db *ent.Client, sw *suwayomi.Client, jobMgr *job.Manager, hub *ws.Hub) *Server {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	h := handler.New(cfg, db, sw, jobMgr)

	s := &Server{
		echo:        e,
		config:      cfg,
		db:          db,
		handler:     h,
		ProgressHub: hub,
	}

	setupMiddleware(e)
	registerRoutes(e, h)
	registerProgressHub(e, hub)
	registerStaticFiles(e)

	return s
}

func registerProgressHub(e *echo.Echo, hub *ws.Hub) {
	e.POST("/progress/negotiate", hub.HandleNegotiate)
	e.GET("/progress", hub.HandleWebSocket)
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Server.Port)
	return s.echo.Start(addr)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}

// HealthCheck returns 200 OK.
func HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// registerStaticFiles serves the built Nuxt frontend with SPA fallback.
func registerStaticFiles(e *echo.Echo) {
	// Try common frontend paths
	frontendDir := ""
	candidates := []string{
		"frontend",                           // Docker / production
		"../KaizokuFrontend/.output/public",  // Development (Nuxt generate)
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			frontendDir, _ = filepath.Abs(dir)
			break
		}
	}
	if frontendDir == "" {
		return // No frontend found, skip static file serving
	}

	// Serve static files with SPA fallback
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path

			// Skip API routes, WebSocket, and health check
			if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/progress") || path == "/health" {
				return next(c)
			}

			// Try to serve the exact file
			filePath := filepath.Join(frontendDir, path)
			if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
				return c.File(filePath)
			}

			// Try path.html (static export convention)
			htmlPath := filePath + ".html"
			if _, err := os.Stat(htmlPath); err == nil {
				return c.File(htmlPath)
			}

			// Try path/index.html
			indexPath := filepath.Join(filePath, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				return c.File(indexPath)
			}

			// Fallback: serve index.html for client-side routing
			rootIndex := filepath.Join(frontendDir, "index.html")
			if _, err := os.Stat(rootIndex); err == nil {
				return c.File(rootIndex)
			}

			return next(c)
		}
	})
}
