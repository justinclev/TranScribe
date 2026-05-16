package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	_ "github.com/justinclev/transcribe/docs" // swagger docs
	"github.com/justinclev/transcribe/internal/api"
)

// @title TranScribe API
// @version 1.0
// @description API for converting Docker Compose files to Infrastructure as Code (Terraform, Pulumi, CDK, Helm)
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url https://github.com/justinclev/transcribe
// @contact.email support@transcribe.dev

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /
// @schemes http https

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Health check — required for ALB/ECS health probes
	// @Summary Health check endpoint
	// @Description Returns OK status for load balancer health probes
	// @Tags health
	// @Produce json
	// @Success 200 {object} map[string]string
	// @Router /healthz [get]
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	api.RegisterRoutes(mux)

	addr := ":" + port
	fmt.Printf("Transcribe API server listening on %s\n", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
