package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mini-dynamo/mini-dynamo/internal/config"
	"github.com/mini-dynamo/mini-dynamo/internal/replication"
	"github.com/mini-dynamo/mini-dynamo/internal/storage"
)

// Server represents the HTTP API server
type Server struct {
	config      *config.Config
	router      *mux.Router
	httpServer  *http.Server
	storage     storage.Engine
	coordinator *replication.Coordinator
	startTime   time.Time
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, store storage.Engine, coord *replication.Coordinator) *Server {
	s := &Server{
		config:      cfg,
		router:      mux.NewRouter(),
		storage:     store,
		coordinator: coord,
		startTime:   time.Now(),
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Middleware
	s.router.Use(loggingMiddleware)
	s.router.Use(recoveryMiddleware)
	s.router.Use(corsMiddleware)

	// Health check
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Key-Value operations
	s.router.HandleFunc("/kv/{key}", s.handleGet).Methods("GET")
	s.router.HandleFunc("/kv/{key}", s.handlePut).Methods("PUT", "POST")
	s.router.HandleFunc("/kv/{key}", s.handleDelete).Methods("DELETE")

	// Admin endpoints
	s.router.HandleFunc("/admin/status", s.handleStatus).Methods("GET")
	s.router.HandleFunc("/admin/ring", s.handleRing).Methods("GET")
	s.router.HandleFunc("/admin/keys", s.handleKeys).Methods("GET")
	s.router.HandleFunc("/admin/stats", s.handleStats).Methods("GET")

	// Internal replication endpoints
	s.router.HandleFunc("/internal/replicate", s.handleReplication).Methods("POST")
	s.router.HandleFunc("/internal/read", s.handleInternalRead).Methods("GET")
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := s.config.FullAddress()
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Starting HTTP server on %s", addr)
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	log.Println("Shutting down HTTP server...")
	return s.httpServer.Shutdown(ctx)
}

// Uptime returns the server uptime duration
func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}

// GetRouter returns the mux router (for testing)
func (s *Server) GetRouter() *mux.Router {
	return s.router
}

// Helper function to format uptime
func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
