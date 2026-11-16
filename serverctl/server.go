package serverctl

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	server  *http.Server
	handler http.Handler
)

func SetHandler(h http.Handler) {
	mu.Lock()
	handler = h
	mu.Unlock()
}

func Start(port int) error {
	mu.Lock()
	defer mu.Unlock()
	if handler == nil {
		return fmt.Errorf("HTTP handler not set")
	}
	return startLocked(port)
}

func Reload(port int) error {
	mu.Lock()
	defer mu.Unlock()
	if handler == nil {
		return fmt.Errorf("HTTP handler not set")
	}
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = server.Shutdown(ctx)
		cancel()
		server = nil
	}
	log.Printf("Reloading HTTP server on :%d", port)
	return startLocked(port)
}

func startLocked(port int) error {
	addr := fmt.Sprintf(":%d", port)
	server = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func(s *http.Server) {
		log.Printf("HTTP server listening on %s", addr)
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}(server)
	return nil
}
