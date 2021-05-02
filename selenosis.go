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

	state, err := client.State()
	if err != nil {
		logger.Errorf("failed to get cluster state: %v", err)
	}

	for _, service := range state.Services {
		storage.Sessions().Put(service.SessionID, service)
	}

	for _, worker := range state.Workers {
		storage.Workers().Put(worker.Name, worker)
	}

	limit := cfg.SessionLimit
	currentTotal := func() int64 {
		return int64(storage.Workers().Len() + limit)
	}
	var quota *platform.Quota
	if quota, err = client.Quota().Get(); err != nil {
		quota, err = client.Quota().Create(currentTotal())
		if err != nil {
			logger.Fatalf("failed to create quota resource: %v", err)
		}
	}

	if quota.CurrentMaxLimit != int64(currentTotal()) {
		quota, err = client.Quota().Update(currentTotal())
		if err != nil {
			logger.Warnf("failed to update quota resource %v:", err)
		}
	}

	storage.Quota().Put(quota)

	logger.Infof("current cluster state: sessions - %d, workers - %d, session limit - %d", storage.Sessions().Len(), storage.Workers().Len(), limit)

	ch := client.Watch()
	go func() {
		for event := <-ch: {

			
		}
			select {
			case event := <-ch:
				switch event.PlatformObject.(type) {
				case *platform.Service:
					service := event.PlatformObject.(*platform.Service)
					switch event.Type {
					case platform.Added:
						storage.Sessions().Put(service.SessionID, service)
					case platform.Updated:
						storage.Sessions().Put(service.SessionID, service)
					case platform.Deleted:
						storage.Sessions().Delete(service.SessionID)
					}

				case *platform.Worker:
					worker := event.PlatformObject.(*platform.Worker)
					switch event.Type {
					case platform.Added:
						storage.Workers().Put(worker.Name, worker)
						result, err := client.Quota().Update(currentTotal())
						if err != nil {
							logger.Warnf("failed to update resource quota: %v", err)
							break
						}
						storage.Quota().Put(result)
						logger.Infof("selenosis worker: %s added, current namespace quota limit: %d", worker.Name, storage.Quota().Get().CurrentMaxLimit)
					case platform.Deleted:
						storage.Workers().Delete(worker.Name)
						result, err := client.Quota().Update(currentTotal())
						if err != nil {
							logger.Warnf("failed to update resource quota: %v", err)
							break
						}
						storage.Quota().Put(result)
						logger.Infof("selenosis worker: %s removed, current namespace quota limit: %d", worker.Name, storage.Quota().Get().CurrentMaxLimit)
					}

				case *platform.Quota:
					quota := event.PlatformObject.(*platform.Quota)
					switch event.Type {
					case platform.Added:
						if quota.CurrentMaxLimit != currentTotal() {
							quota, err = client.Quota().Update(currentTotal())
							if err != nil {
								logger.Warnf("failed to update quota resource %v:", err)
								break
							}
							storage.Quota().Put(quota)
						}
					case platform.Updated:
						if quota.CurrentMaxLimit != currentTotal() {
							quota, err = client.Quota().Update(currentTotal())
							if err != nil {
								logger.Warnf("failed to update quota resource %v:", err)
								break
							}
							storage.Quota().Put(quota)
						}
					case platform.Deleted:
						quota, err = client.Quota().Create(currentTotal())
						if err != nil {
							logger.Warnf("failed to update quota resource %v:", err)
							break
						}
						storage.Quota().Put(quota)
					}
				}
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
