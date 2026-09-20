package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"web"
)

const (
	appCloseTimer                 = time.Duration(5 * time.Second)
	gracefulShutdownTimerDuration = time.Duration(5 * time.Second)
	maxRequests                   = 10
)

// Launch web server for appCloseTimer duration, let's say five seconds.
// Then launch ten requests to the '/sleep/2' endpoint, one per second.
// Each request takes two seconds to complete.
//
// After five seconds, graceful server shutdown is triggered.
// New and active connections are rejected.
// The three first requests should succeed, the rest should fail.
func TestV1(t *testing.T) {

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	appCtx, cancelAppCtxOnTimerExpire := context.WithTimeout(t.Context(), appCloseTimer)
	defer cancelAppCtxOnTimerExpire()

	var wgServer, wgRequests sync.WaitGroup
	wgServer.Go(func() {
		web.V1(appCtx, logger)
	})

	httpClient := &http.Client{Timeout: time.Duration(maxRequests) * time.Second}
	var requestSuccess, requestFail atomic.Uint32

	wgRequests.Go(func() {
		requestInterval := time.NewTicker(time.Second)
		defer requestInterval.Stop()

		for i := range maxRequests {
			wgRequests.Add(1)
			go func(id int) {
				defer wgRequests.Done()

				resp, err := httpClient.Get("http://localhost:5001/sleep/2")
				if err != nil {
					requestFail.Add(1)
					logger.Debug("request fail", "id", id)
					// t.Fatalf("client do: %v", err)
					return
				}
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					requestFail.Add(1)
					logger.Debug("request fail", "id", id)
					t.Errorf("error reading body: %v", err)
					return
				}
				if string(body) == "OK\n" {
					logger.Debug("request OK", "id", id)
					requestSuccess.Add(1)
				}
			}(i)
			<-requestInterval.C
		}
	})

	wgRequests.Wait()
	wgServer.Wait()

	if got, want := requestSuccess.Load(), uint32(3); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := requestFail.Load(), uint32(7); got != want {
		t.Errorf("got %v, want %v", got, want)
	}

}

// Launch web server for appCloseTimer duration, let's say five seconds.
// Then launch ten requests to the '/sleep/2' endpoint, one per second.
// Each request takes two seconds to complete.
//
// After five seconds, graceful server shutdown is triggered.
// New connections are rejected. Active sessions are given five seconds to finish.
// The five first requests should succeed, the last five should fail.
func TestV2(t *testing.T) {

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	appCtx, cancelAppCtxOnTimerExpire := context.WithTimeout(t.Context(), appCloseTimer)
	defer cancelAppCtxOnTimerExpire()

	var wgServer, wgRequests sync.WaitGroup
	wgServer.Go(func() {
		web.V2(appCtx, logger, gracefulShutdownTimerDuration)
	})

	httpClient := &http.Client{Timeout: time.Duration(maxRequests) * time.Second}
	var requestSuccess, requestFail atomic.Uint32

	// First GET takes 1s, second takes 2s, third takes 3s, etc up to maxRequestWait seconds
	// Requests longer than maxServerGracefulShutdownTimer will be aborted.
	wgRequests.Go(func() {
		requestInterval := time.NewTicker(time.Second)
		defer requestInterval.Stop()

		for i := range maxRequests {
			wgRequests.Add(1)
			go func(id int) {
				defer wgRequests.Done()

				resp, err := httpClient.Get("http://localhost:5001/sleep/2")
				if err != nil {
					requestFail.Add(1)
					logger.Debug("request fail", "id", id)
					// t.Fatalf("client do: %v", err)
					return
				}
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					requestFail.Add(1)
					logger.Debug("request fail", "id", id)
					t.Errorf("error reading body: %v", err)
					return
				}
				if string(body) == "OK\n" {
					logger.Debug("request OK", "id", id)
					requestSuccess.Add(1)
				}
			}(i)
			<-requestInterval.C
		}
	})

	wgRequests.Wait()
	wgServer.Wait()

	if got, want := requestSuccess.Load(), uint32(5); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := requestFail.Load(), uint32(5); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
