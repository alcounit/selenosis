package selenosis

import (
	"time"

	"github.com/alcounit/selenosis/config"
	"github.com/alcounit/selenosis/platform"
	log "github.com/sirupsen/logrus"
)

//Configuration ....
type Configuration struct {
	SelenosisHost       string
	ServiceName         string
	SidecarPort         string
	SessionRetryCount   int
	BrowserWaitTimeout  time.Duration
	SessionIddleTimeout time.Duration
}

//App ...
type App struct {
	logger              *log.Logger
	client              platform.Platform
	browsers            *config.BrowsersConfig
	selenosisHost       string
	serviceName         string
	sidecarPort         string
	sessionRetryCount   int
	sessionIddleTimeout time.Duration
	browserWaitTimeout  time.Duration
}

//New ...
func New(logger *log.Logger, client platform.Platform, browsers *config.BrowsersConfig, settings Configuration) *App {
	return &App{
		logger:              logger,
		client:              client,
		browsers:            browsers,
		selenosisHost:       settings.SelenosisHost,
		serviceName:         settings.ServiceName,
		sidecarPort:         settings.SidecarPort,
		sessionRetryCount:   settings.SessionRetryCount,
		browserWaitTimeout:  settings.BrowserWaitTimeout,
		sessionIddleTimeout: settings.SessionIddleTimeout,
	}
}
