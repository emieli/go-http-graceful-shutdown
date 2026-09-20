package web

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// SleepHandler help us test graceful shutdown working correctly
// Affected by http.Server WriteTimeout. It will cancel requests that are sleeping for too long.
func SleepHandler(w http.ResponseWriter, r *http.Request) {
	duration, err := time.ParseDuration(r.PathValue("durationSeconds") + "s")
	if err != nil {
		http.Error(w, "invalid sleep timer", http.StatusBadRequest)
		return
	}

	select {
	case <-r.Context().Done():
		// Abort sleep if request context closed (client browser closed the tab)
		return
	case <-time.After(duration):
		// Sleep over, return output to client
		w.Write([]byte("OK\n"))
	}
}

// V1 launches a web server that does not handle shutdown gracefully
func V1(appCtx context.Context, log *slog.Logger) {
	router := http.NewServeMux()
	router.HandleFunc("GET /sleep/{durationSeconds}", SleepHandler)

	srv := http.Server{
		Addr:         ":5001",
		Handler:      router,
		IdleTimeout:  time.Minute,
		WriteTimeout: time.Minute,
		ReadTimeout:  10 * time.Second,
	}

	// ListenAndServe blocks, so we launch it in separate goroutine.
	// If we don't, the code further down won't run until ListenAndServe returns,
	// which won't happen until the app is killed.
	var wg sync.WaitGroup
	wg.Go(func() {
		log.Debug("server accepting new connections")
		if err := srv.ListenAndServe(); err != nil {
			if appCtx.Err() != nil {
				log.Debug("app shutdown, server rejecting new connections")
				return
			}
			log.Error("server rejecting new connections", "reason", err)
		}
	})

	// Block until shutdown signal (ctrl+c, etc) is received from OS
	log.Debug("waiting for app shutdown signal...")
	<-appCtx.Done()
	log.Debug("received shutdown signal")

	log.Debug("forcing server close...")
	srv.Close() // Forcefully shutdown server

	log.Debug("waiting for goroutines to return...")
	wg.Wait() // Wait for ListenAndServe to return, should be immediate
	log.Debug("server stopped")
}

// V2 launches a web server that handle shutdown gracefully
func V2(appCtx context.Context, log *slog.Logger, gshutTimerDuration time.Duration) {
	router := http.NewServeMux()
	router.HandleFunc("GET /sleep/{durationSeconds}", SleepHandler)

	srv := http.Server{
		Addr:         ":5001",
		Handler:      router,
		IdleTimeout:  time.Minute,
		WriteTimeout: time.Minute,
		ReadTimeout:  10 * time.Second,
	}

	// ListenAndServe blocks, so we launch it in separate goroutine.
	// If we don't, the code further down won't run until ListenAndServe returns,
	// which won't happen until the app is killed.
	var wg sync.WaitGroup
	wg.Go(func() {
		log.Debug("server accepting new connections")
		if err := srv.ListenAndServe(); err != nil {
			if appCtx.Err() != nil {
				log.Debug("app shutdown, server rejecting new connections")
				return
			}
			log.Error("server rejecting new connections", "reason", err)
		}
	})

	// Block until shutdown signal (ctrl+c, etc) is received from OS
	log.Debug("waiting for shutdown signal...")
	<-appCtx.Done()
	log.Debug("received shutdown signal")

	// Give existing requests X seconds to complete before app is forcefully closed
	gshutCtx, gshutTimer := context.WithTimeout(context.Background(), gshutTimerDuration)
	defer gshutTimer()

	log.Debug("started timer, waiting for active connections to finish...")
	if err := srv.Shutdown(gshutCtx); err != nil {
		srv.Close() // forceful shutdown, Shutdown() returned when context expired
		log.Error("graceful shutdown failed, some connections were dropped", "reason", err)
	} else {
		log.Debug("graceful shutdown success, all connections finished")
	}

	log.Debug("waiting for goroutines to return...")
	wg.Wait() // Wait for ListenAndServe to return, should be immediate
	log.Debug("server stopped")
}
