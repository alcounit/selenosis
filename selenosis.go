package selenosis

import (
	"time"

	"github.com/alcounit/selenosis/config"
	"github.com/alcounit/selenosis/platform"
	"github.com/alcounit/selenosis/storage"
	log "github.com/sirupsen/logrus"
)

//Configuration ....
type Configuration struct {
	SelenosisHost      string
	ServiceName        string
	SidecarPort        string
	SessionLimit       int
	SessionRetryCount  int
	BrowserWaitTimeout time.Duration
	SessionIdleTimeout time.Duration
	BuildVersion       string
}

//App ...
type App struct {
	logger             *log.Logger
	client             platform.Platform
	browsers           *config.BrowsersConfig
	selenosisHost      string
	serviceName        string
	sidecarPort        string
	sessionLimit       int
	sessionRetryCount  int
	sessionIdleTimeout time.Duration
	browserWaitTimeout time.Duration
	buildVersion       string
	stats              *storage.Storage
}

//New ...
func New(logger *log.Logger, client platform.Platform, browsers *config.BrowsersConfig, cfg Configuration) *App {

	storage := storage.New()

	services, err := client.List()
	if err != nil {
		logger.Errorf("failed to get list of active pods: %v", err)
	}

	for _, service := range services {
		storage.Put(service.SessionID, service)
	}

	ch := client.Watch()
	go func() {
		for {
			select {
			case event := <-ch:
				switch event.Type {
				case platform.Added:
					storage.Put(event.Service.SessionID, event.Service)
				case platform.Updated:
					storage.Put(event.Service.SessionID, event.Service)
				case platform.Deleted:
					storage.Delete(event.Service.SessionID)
				}
			default:
				break
			}
		}
	}()

	return &App{
		logger:             logger,
		client:             client,
		browsers:           browsers,
		selenosisHost:      cfg.SelenosisHost,
		serviceName:        cfg.ServiceName,
		sidecarPort:        cfg.SidecarPort,
		sessionLimit:       cfg.SessionLimit,
		sessionRetryCount:  cfg.SessionRetryCount,
		browserWaitTimeout: cfg.BrowserWaitTimeout,
		sessionIdleTimeout: cfg.SessionIdleTimeout,
		buildVersion:       cfg.BuildVersion,
		stats:              storage,
	}
}
