package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	logctx "github.com/alcounit/browser-controller/pkg/log"
	"github.com/alcounit/browser-service/pkg/client"
	"github.com/alcounit/selenosis/v2/pkg/env"
	"github.com/alcounit/selenosis/v2/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func main() {

	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log := zerolog.New(os.Stdout).With().Timestamp().Logger()

	cfg, listenAddr, apiURL, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	clientConfig := client.ClientConfig{
		BaseURL:    apiURL,
		HTTPClient: http.DefaultClient,
		Logger:     log,
	}

	client, err := client.NewClient(clientConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Browser client")
	}

	svc := service.NewService(client, cfg)

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		fn := func(rw http.ResponseWriter, req *http.Request) {

			reqId := uuid.NewString()

			logger := log.With().
				Str("method", req.Method).
				Str("path", req.URL.Path).
				Str("reqId", reqId).
				Logger()

			req.Header.Add("Selenosis-Request-ID", reqId)
			ctx := req.Context()
			ctx = logctx.IntoContext(ctx, logger)

			next.ServeHTTP(rw, req.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	})

	selenium := chi.NewRouter()

	selenium.Post("/session", svc.CreateSession)
	selenium.Route("/session/{sessionId}", func(r chi.Router) {
		r.HandleFunc("/*", svc.ProxySession)
	})
	selenium.Get("/status", svc.SessionStatus)

	router.Mount("/", selenium)
	router.Mount("/wd/hub", selenium)

	router.Route("/selenosis/v1/sessions/{sessionId}", func(r chi.Router) {
		r.Route("/proxy", func(r chi.Router) {
			r.HandleFunc("/http/*", svc.RouteHTTP)
		})
	})

	srv := &http.Server{
		Addr:    listenAddr,
		Handler: router,
	}

	go func() {
		log.Info().Msgf("HTTP server listening %s", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Err(err).Msg("HTTP server error")
			os.Exit(1)
		}
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	<-stopCh
	log.Info().Msg("Shutting down HTTP server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Err(err).Msg("HTTP server shutdown error")
		os.Exit(1)
	}
}

func loadConfig() (service.ServiceConfig, string, string, error) {
	var cfg service.ServiceConfig

	addr := env.GetEnvOrDefault("LISTEN_ADDR", ":4444")
	apiURL := env.GetEnvOrDefault("BROWSER_SERVICE_URL", "http://browser-service:8080")

	cfg.SidecarPort = env.GetEnvOrDefault("PROXY_PORT", "4445")
	cfg.SessionCreateAttempts = env.GetEnvIntOrDefault("SESSION_CREATE_ATTEMPTS", 5)
	cfg.SessionCreateTimeout = env.GetEnvDurationOrDefault("SESSION_CREATE_TIMEOUT", 3*time.Minute)
	cfg.Namespace = env.GetEnvOrDefault("NAMESPACE", "default")

	if addr == "" {
		return cfg, "", "", errors.New("LISTEN_ADDR must be provided")
	}
	if cfg.SidecarPort == "" {
		return cfg, "", "", errors.New("PROXY_PORT must be provided")
	}
	if apiURL == "" {
		return cfg, "", "", errors.New("BROWSER_SERVICE_URL must be provided")
	}
	if cfg.Namespace == "" {
		return cfg, "", "", errors.New("NAMESPACE must be provided")
	}

	return cfg, addr, apiURL, nil
}

func normalizePath(p string) string {
	if strings.HasPrefix(p, "/wd/hub") {
		return strings.TrimPrefix(p, "/wd/hub")
	}
	return p
}
