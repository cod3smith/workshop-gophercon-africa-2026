package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
)

// render writes component as the response body, falling back to the error
// page when the component itself fails.
func render(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := component.Render(ctx, w); err != nil {
		logger.ErrorContext(ctx, "handlers: rendering page: "+err.Error())
		renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to render page.")
	}
}
