package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alcounit/selenosis"
	"github.com/alcounit/selenosis/config"
	"github.com/alcounit/selenosis/platform"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

//Command ...
func command() *cobra.Command {

	var (
		cfgFile             string
		address             string
		proxyPort           string
		namespace           string
		service             string
		sessionRetryCount   int
		browserWaitTimeout  time.Duration
		sessionWaitTimeout  time.Duration
		sessionIddleTimeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "selenosis",
		Short: "Scallable, stateless selenium grid for Kubernetes cluster",
		Run: func(cmd *cobra.Command, args []string) {

			logger := logrus.New()
			logger.Info("Starting selenosis")

			browsers, err := config.NewBrowsersConfig(cfgFile)
			if err != nil {
				logger.Fatalf("Failed to read config: %v", err)
			}

			logger.Info("Browsers config file loaded")

			client, err := platform.NewClient(platform.ClientConfig{
				Namespace:        namespace,
				Service:          service,
				ReadinessTimeout: browserWaitTimeout,
				IddleTimeout:     sessionIddleTimeout,
				ServicePort:      proxyPort,
			})

			if err != nil {
				logger.Fatalf("Failed to create kubernetes client: %v", err)
			}

			logger.Info("Kubernetes client created")

			hostname, _ := os.Hostname()

			app := selenosis.New(logger, client, browsers, selenosis.Configuration{
				SelenosisHost:       hostname,
				ServiceName:         service,
				SidecarPort:         proxyPort,
				SessionRetryCount:   sessionRetryCount,
				BrowserWaitTimeout:  browserWaitTimeout,
				SessionIddleTimeout: sessionIddleTimeout,
			})

			router := mux.NewRouter()
			router.HandleFunc("/wd/hub/session", app.HandleSession).Methods(http.MethodPost)
			router.PathPrefix("/wd/hub/session/{sessionId}").HandlerFunc(app.HandleProxy)
			router.PathPrefix("/vnc/{sessionId}").Handler(websocket.Handler(app.HandleVNC))

			srv := &http.Server{
				Addr:    address,
				Handler: router,
			}

			stop := make(chan os.Signal, 1)
			signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

			e := make(chan error)
			go func() {
				e <- srv.ListenAndServe()
			}()

			select {
			case err := <-e:
				logger.Fatalf("failed to start: %v", err)
			case <-stop:
				logger.Warn("stopping selenosis")
			}
		},
	}

	cmd.Flags().StringVar(&address, "port", ":4444", "port for selenosis")
	cmd.Flags().StringVar(&proxyPort, "proxy-port", "4445", "proxy continer port")
	cmd.Flags().StringVar(&cfgFile, "browsers-config", "config/browsers.yaml", "browsers config")
	cmd.Flags().StringVar(&namespace, "namespace", "default", "kubernetes namespace")
	cmd.Flags().StringVar(&service, "service-name", "selenosis", "kubernetes service name for browsers")
	cmd.Flags().DurationVar(&browserWaitTimeout, "browser-wait-timeout", 30*time.Second, "time in seconds that a browser will be ready")
	cmd.Flags().DurationVar(&sessionWaitTimeout, "session-wait-timeout", 60*time.Second, "time in seconds that a session will be ready")
	cmd.Flags().DurationVar(&sessionIddleTimeout, "session-iddle-timeout", 5*time.Minute, "time in seconds that a session will iddle")
	cmd.Flags().IntVar(&sessionRetryCount, "session-retry-count", 3, "session retry count")
	cmd.Flags().SortFlags = false

	return cmd
}

func main() {
	if err := command().Execute(); err != nil {
		os.Exit(1)
	}
}
