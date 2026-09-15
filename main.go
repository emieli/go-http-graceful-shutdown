package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	router := http.NewServeMux()
	router.HandleFunc("GET /{sleepTimer}", Sleep)
	web := http.Server{
		Addr:         ":5001",
		Handler:      router,
		IdleTimeout:  time.Minute,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		log.Info("web server accepting new connection")
		if err := web.ListenAndServe(); err != nil {
			log.Error("web server stopped accepting new connections", "error", err)
		}
	})

	// Block until shutdown signal (ctrl+c, etc) is received from OS
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	log.Info("shutdown signal received", "signal", sig)

	// Shutdown signal received, stop accepting new requests.
	// Give existing requests 30 seconds to complete before app is forcefully closed
	gshutCtx, gshutTimer := context.WithTimeout(context.Background(), 5*time.Second)
	defer gshutTimer()
	log.Info("waiting for active connections to finish...")
	if err := web.Shutdown(gshutCtx); err != nil {
		log.Error("server was shutdown forcefully, some connections were dropped", "reason", err)
	} else {
		log.Info("server was shutdown gracefully, all active connections closed normally")
	}

	// Wait for all goroutines to close. (optional)
	wg.Wait()
	log.Info("all goroutines have returned, closing program")

}

func Sleep(w http.ResponseWriter, r *http.Request) {
	sleepTimer, err := time.ParseDuration(r.PathValue("sleepTimer") + "s")
	if err != nil {
		http.Error(w, "invalid sleep timer", http.StatusBadRequest)
		return
	}
	time.Sleep(sleepTimer)
}
