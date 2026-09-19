package web_test

import (
	"context"
	"gshut/cmd/web"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	appCloseTimer                 = time.Duration(5 * time.Second)
	gracefulShutdownTimerDuration = time.Duration(5 * time.Second)
	maxRequestWait                = 15
)

// Launch web server for appCloseTimer duration, let's say five seconds.
// Then launch 15 concurrent requests to the '/sleep/X' endpoint.
// First request sleeps for zero second, second for one seconds, third for two seconds, etc
// As the server is forcefully closed after five seconds, only the first
// five requests will finish.
// The other ten requests are cancelled by the server and not allowed to finish.
func TestV1(t *testing.T) {

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	appCtx, cancelAppCtxOnTimerExpire := context.WithTimeout(t.Context(), appCloseTimer)
	defer cancelAppCtxOnTimerExpire()

	var wgServer, wgRequests sync.WaitGroup
	wgServer.Go(func() {
		web.V1(appCtx, logger)
	})

	httpClient := &http.Client{Timeout: time.Duration(maxRequestWait) * time.Second}
	var requestSuccess, requestFail atomic.Uint32

	// First GET takes 1s, second takes 2s, third takes 3s, etc up to maxRequestWait seconds
	// Requests longer than maxServerGracefulShutdownTimer will be aborted.
	for i := range maxRequestWait {
		wgRequests.Go(func() {
			resp, err := httpClient.Get("http://localhost:5001/sleep/" + strconv.Itoa(i))
			if err != nil {
				requestFail.Add(1)
				// t.Fatalf("client do: %v", err)
				return
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				requestFail.Add(1)
				t.Errorf("error reading body: %v", err)
				return
			}
			if string(body) == "OK\n" {
				requestSuccess.Add(1)
			}
		})
	}

	wgRequests.Wait()
	wgServer.Wait()

	if got, want := requestSuccess.Load(), uint32(5); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := requestFail.Load(), uint32(10); got != want {
		t.Errorf("got %v, want %v", got, want)
	}

}

// Launch web server for appCloseTimer duration, let's say five seconds.
// Then launch 15 concurrent requests to the '/sleep/X' endpoint.
// First request sleeps for zero second, second for one seconds, third for two seconds, etc
// After five seconds, the server graceful shutdown is triggered.
// New connections are not rejected. Active sessions are given five seconds to finish.
// The ten first requests should succeed, the last five should fail.
func TestV2(t *testing.T) {

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	appCtx, cancelAppCtxOnTimerExpire := context.WithTimeout(t.Context(), appCloseTimer)
	defer cancelAppCtxOnTimerExpire()

	var wgServer, wgRequests sync.WaitGroup
	wgServer.Go(func() {
		web.V2(appCtx, logger, gracefulShutdownTimerDuration)
	})

	httpClient := &http.Client{Timeout: time.Duration(maxRequestWait) * time.Second}
	var requestSuccess, requestFail atomic.Uint32

	// First GET takes 1s, second takes 2s, third takes 3s, etc up to maxRequestWait seconds
	// Requests longer than maxServerGracefulShutdownTimer will be aborted.
	for i := range maxRequestWait {
		wgRequests.Go(func() {
			resp, err := httpClient.Get("http://localhost:5001/sleep/" + strconv.Itoa(i))
			if err != nil {
				requestFail.Add(1)
				// t.Fatalf("client do: %v", err)
				return
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				requestFail.Add(1)
				t.Errorf("error reading body: %v", err)
				return
			}
			if string(body) == "OK\n" {
				requestSuccess.Add(1)
			}
		})
	}

	wgRequests.Wait()
	wgServer.Wait()

	if got, want := requestSuccess.Load(), uint32(10); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := requestFail.Load(), uint32(5); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
