// Package api provides the HTTP server, REST handlers, and MCP endpoint.
package api

import (
	"database/sql"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	clog "github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/mark3labs/mcp-go/server"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/library"
	"github.com/jeeftor/camspeak/internal/tts"
)

var staticFiles embed.FS

// Version is set via -ldflags at build time.
var Version = "dev"

// apiLogLevel controls the log level for API handlers. Set by cmd package
// at startup from CAMSPEAK_LOG_LEVEL env var.
var apiLogLevel = clog.InfoLevel

// SetStaticFiles sets the embedded frontend filesystem (called from main.go).
func SetStaticFiles(fs embed.FS) {
	staticFiles = fs
}

// SetVersion sets the application version (called from main.go).
func SetVersion(v string) {
	Version = v
}

// SetLogLevel sets the log level for API handlers (called from cmd at startup).
func SetLogLevel(level clog.Level) {
	apiLogLevel = level
}

// Server is the HTTP server.
type Server struct {
	echo     *echo.Echo
	handlers *Handlers
	log      *clog.Logger
}

// New creates a configured Echo server.
func New(
	cfg *config.Config,
	reg *cameras.Registry,
	store *library.Store,
	ttsClient *tts.Client,
	database *sql.DB,
) *Server {
	// Create tmp dir under the data dir so temp files survive container
	// restarts and live on the same persistent volume as the library.
	tmpDir := filepath.Join(cfg.Library, "..", "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		tmpDir = "" // fall back to os temp
	}

	h := &Handlers{
		cfg:    cfg,
		reg:    reg,
		store:  store,
		tts:    ttsClient,
		events: newEventBus(store.DB()),
		db:     database,
		tmpDir: tmpDir,
		log:    clog.NewWithOptions(os.Stderr, clog.Options{Prefix: "api", Level: apiLogLevel}),
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true,
		LogMethod: true,
		LogURI:    true,
		LogLatency: true,
		Skipper: func(c echo.Context) bool {
			// Skip health checks and SSE streams
			uri := c.Request().URL.Path
			return uri == "/api/health" || uri == "/api/events"
		},
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			h.log.Debug("request",
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
				"latency", v.Latency,
			)
			return nil
		},
	}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		// LAN-only service — allow all origins so the SPA works from any
		// host:port the browser uses (localhost, 127.0.0.1, LAN IP, etc.)
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowHeaders: []string{"Content-Type", "Authorization"},
	}))

	// REST routes
	api := e.Group("/api")
	api.POST("/speak", h.Speak)
	api.POST("/play", h.Play)
	api.POST("/beep", h.Beep)
	api.POST("/broadcast", h.Broadcast)
	api.GET("/cameras", h.Cameras)
	api.GET("/voices", h.Voices)
	api.GET("/library", h.ListLibrary)
	api.POST("/library", h.GeneratePreset)
	api.POST("/library/upload", h.UploadPreset)
	api.DELETE("/library/:category/:name", h.DeletePreset)
	api.GET("/library/:category/:name/preview", h.PreviewPreset)
	api.GET("/events", h.Events)
	api.GET("/health", h.Health)

	// Config routes
	api.GET("/config", h.GetConfig)
	api.GET("/config/tts", h.ListTTSPresets)
	api.POST("/config/tts", h.CreateTTSPreset)
	api.PUT("/config/tts/:name", h.UpdateTTSPreset)
	api.DELETE("/config/tts/:name", h.DeleteTTSPreset)
	api.POST("/config/tts/:name/activate", h.ActivateTTSPreset)
	api.GET("/config/cameras", h.ListCamerasConfig)
	api.POST("/config/cameras", h.CreateCamera)
	api.PATCH("/config/cameras/:name/toggle", h.ToggleCamera)
	api.DELETE("/config/cameras/:name", h.DeleteCameraConfig)
	api.GET("/config/rules", h.ListRules)
	api.POST("/config/rules", h.CreateRule)

	// MCP endpoint
	mcpServer := buildMCPServer(h)
	e.Any("/mcp", echo.WrapHandler(server.NewStreamableHTTPServer(mcpServer)))

	// Svelte SPA — serve from embedded frontend/dist with SPA fallback
	distFS, err := fs.Sub(staticFiles, "frontend/dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(distFS))

		// Pre-read index.html for SPA fallback (avoids http.FileServer redirect loop)
		indexHTML, _ := fs.ReadFile(distFS, "index.html")

		e.GET("/*", func(c echo.Context) error {
			path := c.Param("*")
			// If the file exists, serve it directly
			if path != "" {
				if _, statErr := fs.Stat(distFS, path); statErr == nil {
					return echo.WrapHandler(fileServer)(c)
				}
			}
			// SPA fallback — return index.html for client-side routing
			if indexHTML != nil {
				return c.Blob(http.StatusOK, "text/html; charset=utf-8", indexHTML)
			}
			return c.String(http.StatusOK, "camspeak — frontend not built")
		})
	} else {
		// Dev fallback: no frontend built yet
		e.GET("/", func(c echo.Context) error {
			return c.String(http.StatusOK, "camspeak — run 'make frontend' to build the UI")
		})
	}

	return &Server{
		echo:     e,
		handlers: h,
		log:      clog.NewWithOptions(os.Stderr, clog.Options{Prefix: "api"}),
	}
}

// Handlers returns the handlers for external wiring (e.g. MQTT).
func (s *Server) Handlers() *Handlers {
	return s.handlers
}

// Start listens on addr (e.g. ":8585").
func (s *Server) Start(addr string) error {
	s.log.Info("starting", "addr", addr)

	return s.echo.Start(addr)
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	return s.echo.Close()
}
