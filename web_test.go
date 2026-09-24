package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"web"
)

const (
	appCloseTimer                 = time.Duration(6100 * time.Millisecond)
	gracefulShutdownTimerDuration = time.Duration(2 * time.Second)
	maxRequests                   = 10
	sleepDuration                 = "4"
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
	appCtx, _ := signal.NotifyContext(context.Background(), syscall.SIGHUP)

	wgServer := new(sync.WaitGroup)
	wgRequests := new(sync.WaitGroup)

	wgServer.Go(func() {
		web.V1(appCtx, logger)
	})

	requestSuccess := new(atomic.Uint32)
	requestFail := new(atomic.Uint32)

	wgRequests.Go(func() {
		requestGenerator(logger, wgRequests, requestSuccess, requestFail)
	})

	// Trigger app shutdown via OS signal
	time.Sleep(appCloseTimer)
	logger.Debug("sending OS shutdown signal (SIHGUP)...")
	cmd := exec.Command("kill", "-1", strconv.Itoa(os.Getpid()))
	if err := cmd.Run(); err != nil {
		t.Fatalf("send OS signal: %v", err)
	}

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
	appCtx, _ := signal.NotifyContext(t.Context(), syscall.SIGHUP)

	wgServer := new(sync.WaitGroup)
	wgRequests := new(sync.WaitGroup)

	wgServer.Go(func() {
		web.V2(appCtx, logger, gracefulShutdownTimerDuration)
	})

	requestSuccess := new(atomic.Uint32)
	requestFail := new(atomic.Uint32)

	wgRequests.Go(func() {
		requestGenerator(logger, wgRequests, requestSuccess, requestFail)
	})

	// Trigger app shutdown via OS signal
	time.Sleep(appCloseTimer)
	logger.Debug("sending OS shutdown signal (SIHGUP)...")
	cmd := exec.Command("kill", "-1", strconv.Itoa(os.Getpid()))
	if err := cmd.Run(); err != nil {
		t.Fatalf("send OS signal: %v", err)
	}

	wgRequests.Wait()
	wgServer.Wait()

	if got, want := requestSuccess.Load(), uint32(5); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := requestFail.Load(), uint32(5); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func requestGenerator(log *slog.Logger, wg *sync.WaitGroup, success, fail *atomic.Uint32) {
	httpClient := &http.Client{Timeout: time.Duration(maxRequests) * time.Second}

	requestInterval := time.NewTicker(time.Second)
	defer requestInterval.Stop()

	for range maxRequests {
		wg.Go(func() {
			resp, err := httpClient.Get("http://localhost:5001/sleep/" + sleepDuration)
			if err != nil {
				fail.Add(1)
				log.Error("get request", "error", err)
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fail.Add(1)
				log.Error("read body", "error", err)
				return
			}

			if string(body) == "OK\n" {
				success.Add(1)
			}
		})
		<-requestInterval.C
	}
}
