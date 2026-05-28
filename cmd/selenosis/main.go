package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	logctx "github.com/alcounit/browser-controller/pkg/log"
	"github.com/alcounit/browser-service/pkg/client"
	browserclient "github.com/alcounit/browser-service/pkg/client/browser"
	"github.com/alcounit/selenosis/v2/pkg/auth"
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, authStore, listenAddr, apiURL, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	go auth.Watch(ctx, authStore)

	clientConfig := client.ClientConfig{
		BaseURL:    apiURL,
		HTTPClient: http.DefaultClient,
		Logger:     log,
	}

	client, err := browserclient.NewClient(clientConfig)
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

	router.Use(basicAuthMiddleware(authStore, log))

	selenium := chi.NewRouter()

	selenium.Post("/session", svc.CreateSession)
	selenium.Route("/session/{sessionId}", func(r chi.Router) {
		r.HandleFunc("/*", svc.ProxySession)
	})
	selenium.Get("/status", svc.SessionStatus)

	router.Mount("/", selenium)
	router.Mount("/wd/hub", selenium)

	router.Get("/playwright/{name}/{version}", svc.Playwright)

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

	<-ctx.Done()
	stop()
	log.Info().Msg("Shutting down HTTP server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Err(err).Msg("HTTP server shutdown error")
		os.Exit(1)
	}
}

func basicAuthMiddleware(authStore *auth.AuthStore, log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			if authStore != nil {
				user, pass, ok := req.BasicAuth()
				if !ok || !authStore.Authenticate(user, pass) {
					log.Error().Msg("request authentication failed")
					http.Error(rw, "authentication failed", http.StatusUnauthorized)
					return
				}
				req.URL.User = nil
				req = req.WithContext(auth.WithOwner(req.Context(), auth.Owner{Name: user}))
			}
			next.ServeHTTP(rw, req)
		})
	}
}

func loadConfig() (service.ServiceConfig, *auth.AuthStore, string, string, error) {
	var (
		cfg       service.ServiceConfig
		authStore *auth.AuthStore
		err       error
	)

	addr := env.GetEnvOrDefault("LISTEN_ADDR", ":4444")
	apiURL := env.GetEnvOrDefault("BROWSER_SERVICE_URL", "http://browser-service:8080")

	cfg.SidecarPort = env.GetEnvOrDefault("PROXY_PORT", "4445")
	cfg.BrowserStartTimeout = env.GetEnvDurationOrDefault("BROWSER_STARTUP_TIMEOUT", 3*time.Minute)
	cfg.Namespace = env.GetEnvOrDefault("NAMESPACE", "selenosis")

	basicAuthFilePath := env.GetEnvOrDefault("BASIC_AUTH_FILE", "")
	if basicAuthFilePath != "" {
		if authStore, err = auth.LoadFromJSONFile(basicAuthFilePath); err != nil {
			return cfg, authStore, "", "", fmt.Errorf("BASIC_AUTH_FILE file read error: %v", err)
		}
	}

	return cfg, authStore, addr, apiURL, err
}
