package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

func main() {
	// 1. Root context that gets cancelled on shutdown
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// 🔹 Init OTel
	shutdownTracer, err := initTracer(ctx)
	if err != nil {
		log.Fatalf("otel init failed: %v", err)
	}
	defer func() {
		_ = shutdownTracer(context.Background())
	}()
	// 2. HTTP server
	srv := &http.Server{
		Addr:    ":8090",
		Handler: handler(),
	}

	// 3. Run server in background
	go func() {
		log.Println("server starting on :8090")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// 4. Wait for shutdown signal
	<-ctx.Done()
	log.Println("shutdown signal received")

	// 5. Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}

	log.Println("server stopped")
}

func handler() http.Handler {
	mux := http.NewServeMux()

	type HealthStatus struct {
		Status    string            `json:"status"`
		Checks    map[string]string `json:"checks"`
		Timestamp time.Time         `json:"timestamp"`
	}
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
		defer cancel()

		tracer := otel.Tracer("health")
		ctx, span := tracer.Start(ctx, "readiness.check")
		defer span.End()

		results := make(chan struct {
			name string
			err  error
		}, 3)

		go runCheck(ctx, "db", checkDB, results)
		go runCheck(ctx, "cache", checkCache, results)
		go runCheck(ctx, "external", checkExternal, results)

		status := HealthStatus{
			Status:    "ok",
			Checks:    map[string]string{},
			Timestamp: time.Now(),
		}

		for i := 0; i < 3; i++ {
			r := <-results
			if r.err != nil {
				status.Status = "unhealthy"
				status.Checks[r.name] = r.err.Error()
				span.RecordError(r.err)
			} else {
				status.Checks[r.name] = "ok"
			}
		}

		if status.Status != "ok" {
			span.SetStatus(codes.Error, "readiness failed")
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	return otelhttp.NewHandler(mux, "http-server")
}

func runCheck(
	parent context.Context,
	name string,
	fn func(context.Context) error,
	results chan<- struct {
		name string
		err  error
	},
) {
	tracer := otel.Tracer("health.checks")
	ctx, span := tracer.Start(parent, name+".check")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	results <- struct {
		name string
		err  error
	}{name, err}
}

func checkDB(ctx context.Context) error {
	select {
	case <-time.After(50 * time.Millisecond): // simulate ping latency
		return nil
	case <-ctx.Done():
		return errors.New("db timeout")
	}
}

func checkCache(ctx context.Context) error {
	select {
	case <-time.After(20 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return errors.New("cache timeout")
	}
}

func checkExternal(ctx context.Context) error {
	select {
	case <-time.After(80 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return errors.New("external dependency timeout")
	}
}
