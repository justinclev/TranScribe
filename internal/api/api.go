// Package api contains HTTP handlers and route registration for the
// Transcribe web API. Handlers delegate all business logic to the shared
// internal packages (parser, hardener, generator) so behavior is identical
// whether invoked via the CLI or the web interface.
package api

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger"
)

// RegisterRoutes attaches all API routes to mux.
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/transcribe", handleTranscribe)
	
	// Swagger documentation
	mux.HandleFunc("/swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))
}
