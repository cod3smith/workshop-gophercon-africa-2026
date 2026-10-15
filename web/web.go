package web

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"
	"workshop/internal/infra/config"
	"workshop/web/handlers"
)

//go:generate go tool templ generate ./...

// assets holds the static files served under /assets.
// Note: The embedded paths keep their "assets/" prefix.
//
//go:embed assets
var assets embed.FS

// New returns a ServeMux serving the static assets and handler's patterns.
func New(logger *slog.Logger, config config.Config, sightingsSvc *sighting.Service, gamesSvc *game.Service) *http.ServeMux {
	mux := http.NewServeMux()

	// One signing key for the whole run, shared by every game cookie.
	session := handlers.NewSession()

	mux.Handle("GET /assets/", http.FileServerFS(assetsFs(config.IsProduction())))
	mux.Handle("GET /{$}", handlers.Home(logger, sightingsSvc))
	mux.Handle("GET /play", handlers.Play(logger, gamesSvc, session))
	mux.Handle("GET /play/new", handlers.NewGame(logger, gamesSvc, session))
	mux.Handle("POST /play/guess", handlers.Guess(logger, gamesSvc, session))
	mux.Handle("GET /", handlers.NotFound(logger))

	return mux
}

func assetsFs(prod bool) fs.FS {
	if prod {
		return assets
	}
	return os.DirFS("web")
}
