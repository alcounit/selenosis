package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/alcounit/selenosis"
	"github.com/alcounit/selenosis/config"
	"github.com/alcounit/selenosis/platform"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

var buildVersion = "HEAD"

//Command ...
func command() *cobra.Command {

	var (
		cfgFile             string
		address             string
		proxyPort           string
		namespace           string
		service             string
		imagePullSecretName string
		proxyImage          string
		sessionRetryCount   int
		limit               int
		browserWaitTimeout  time.Duration
		sessionWaitTimeout  time.Duration
		sessionIddleTimeout time.Duration
		shutdownTimeout     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "selenosis",
		Short: "Scallable, stateless selenium grid for Kubernetes cluster",
		Run: func(cmd *cobra.Command, args []string) {

			logger := logrus.New()
			logger.Infof("starting selenosis %s", buildVersion)

			browsers, err := config.NewBrowsersConfig(cfgFile)
			if err != nil {
				logger.Fatalf("failed to read config: %v", err)
			}

			logger.Info("browsers config file loaded")

			go runConfigWatcher(logger, cfgFile, browsers)

			logger.Info("config watcher started")

			client, err := platform.NewClient(platform.ClientConfig{
				Namespace:           namespace,
				Service:             service,
				ReadinessTimeout:    browserWaitTimeout,
				IddleTimeout:        sessionIddleTimeout,
				ServicePort:         proxyPort,
				ImagePullSecretName: imagePullSecretName,
				ProxyImage:          proxyImage,
			})

			if err != nil {
				logger.Fatalf("failed to create kubernetes client: %v", err)
			}

			logger.Info("kubernetes client created")

			hostname, _ := os.Hostname()

			app := selenosis.New(logger, client, browsers, selenosis.Configuration{
				SelenosisHost:       hostname,
				ServiceName:         service,
				SidecarPort:         proxyPort,
				SessionLimit:        limit,
				SessionRetryCount:   sessionRetryCount,
				BrowserWaitTimeout:  browserWaitTimeout,
				SessionIddleTimeout: sessionIddleTimeout,
			})

			router := mux.NewRouter()
			router.HandleFunc("/wd/hub/session", app.HandleSession).Methods(http.MethodPost)
			router.PathPrefix("/wd/hub/session/{sessionId}").HandlerFunc(app.HandleProxy)
			router.HandleFunc("/wd/hub/status", app.HadleHubStatus).Methods(http.MethodGet)
			router.PathPrefix("/vnc/{sessionId}").Handler(websocket.Handler(app.HandleVNC()))
			router.PathPrefix("/logs/{sessionId}").Handler(websocket.Handler(app.HandleLogs()))
			router.PathPrefix("/devtools/{sessionId}").HandlerFunc(app.HandleReverseProxy)
			router.PathPrefix("/download/{sessionId}").HandlerFunc(app.HandleReverseProxy)
			router.PathPrefix("/clipboard/{sessionId}").HandlerFunc(app.HandleReverseProxy)
			router.PathPrefix("/status").HandlerFunc(app.HandleStatus)
			router.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}).Methods(http.MethodGet)

			srv := &http.Server{
				Addr:    address,
				Handler: router,
			}

			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

			e := make(chan error)
			go func() {
				e <- srv.ListenAndServe()
			}()

			select {
			case err := <-e:
				logger.Fatalf("failed to start selenosis: %v", err)
			case <-stop:
				logger.Warn("stopping selenosis")
			}

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			if err := srv.Shutdown(ctx); err != nil {
				logger.Fatalf("faled to stop selenosis", err)
			}
		},
	}

	cmd.Flags().StringVar(&address, "port", ":4444", "port for selenosis")
	cmd.Flags().StringVar(&proxyPort, "proxy-port", "4445", "proxy continer port")
	cmd.Flags().StringVar(&cfgFile, "browsers-config", "./config/browsers.yaml", "browsers config")
	cmd.Flags().IntVar(&limit, "browser-limit", 10, "active sessions max limit")
	cmd.Flags().StringVar(&namespace, "namespace", "selenosis", "kubernetes namespace")
	cmd.Flags().StringVar(&service, "service-name", "seleniferous", "kubernetes service name for browsers")
	cmd.Flags().DurationVar(&browserWaitTimeout, "browser-wait-timeout", 30*time.Second, "time in seconds that a browser will be ready")
	cmd.Flags().DurationVar(&sessionWaitTimeout, "session-wait-timeout", 60*time.Second, "time in seconds that a session will be ready")
	cmd.Flags().DurationVar(&sessionIddleTimeout, "session-iddle-timeout", 5*time.Minute, "time in seconds that a session will iddle")
	cmd.Flags().IntVar(&sessionRetryCount, "session-retry-count", 3, "session retry count")
	cmd.Flags().DurationVar(&shutdownTimeout, "graceful-shutdown-timeout", 30*time.Second, "time in seconds  gracefull shutdown timeout")
	cmd.Flags().StringVar(&imagePullSecretName, "image-pull-secret-name", "", "secret name to private registry")
	cmd.Flags().StringVar(&proxyImage, "proxy-image", "alcounit/seleniferous:latest", "in case you use private registry replace with image from private registry")
	cmd.Flags().SortFlags = false

	return cmd
}

func runConfigWatcher(logger *logrus.Logger, filename string, config *config.BrowsersConfig) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Fatalf("failed to create watcher: %v", err)
		}
		defer watcher.Close()

		configFile := filepath.Clean(filename)
		configDir, _ := filepath.Split(configFile)
		realConfigFile, _ := filepath.EvalSymlinks(filename)

		done := make(chan bool)
		go func() {
			for {
				select {
				case event := <-watcher.Events:
					currentConfigFile, _ := filepath.EvalSymlinks(filename)
					if (filepath.Clean(event.Name) == configFile &&
						(event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create)) ||
						(currentConfigFile != "" && currentConfigFile != realConfigFile) {

						realConfigFile = currentConfigFile
						err := config.Reload()
						if err != nil {
							logger.Errorf("config reload failed: %v", err)
						} else {
							logger.Infof("config %s reloaded", configFile)
						}
					}
				case err := <-watcher.Errors:
					logger.Errorf("config watcher error: %v", err)
				}
			}
		}()
		watcher.Add(configDir)
		wg.Done()
		<-done
	}()
	wg.Wait()
}

func main() {
	if err := command().Execute(); err != nil {
		os.Exit(1)
	}
}
