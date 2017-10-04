package app

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-errors/errors"
	log "github.com/sirupsen/logrus"
)

var (
	//ErrAppInitialized is returned when the application has already been initialized
	ErrAppInitialized = errors.New("Application already initialized")
)

type gracefulChannelShutdown struct {
	name string
	ch   chan chan error
}

type Application struct {
	name, version         string
	defaultLoggingFields  log.Fields
	config                interface{}
	initConfig            *ApplicationInitConfig
	internalConfig        *BaseAppConfig
	logMutex              sync.Mutex
	log                   *log.Entry
	shutdownChannelsMutex sync.Mutex
	shutdownChannels      []*gracefulChannelShutdown
	shutdown              chan os.Signal
	services              []GracefulService
	servicesMutex         sync.Mutex
	stoppedMutex          sync.Mutex
	stopped               bool
	gracefulTimeout       time.Duration
}

//ApplicationInitConfig is the default application config
type ApplicationInitConfig struct {
	Name, Version               string
	DefaultLoggingFields        log.Fields
	DevFormatter, ProdFormatter log.Formatter
	Config                      interface{}
	GracefulTimeout             time.Duration
	LogHooks                    []log.Hook
}

var app *Application

//Init initializes the application, then the package methods can be used
func Init(appConfig *ApplicationInitConfig) (*Application, error) {
	err := initApp(appConfig)
	if err != nil {
		return nil, err
	}

	handleGracefulShutdown()

	return app, nil
}

func Set(newApp *Application) {
	app = newApp
}

func getVersion(c *ApplicationInitConfig) string {
	var version string
	if c.Version != "" {
		version = c.Version
	} else {
		version = "development"
	}

	return version
}

func initApp(appConfig *ApplicationInitConfig) error {
	if app != nil {
		return ErrAppInitialized
	}

	if appConfig.DevFormatter == nil {
		appConfig.DevFormatter = &log.TextFormatter{}
	}
	if appConfig.ProdFormatter == nil {
		appConfig.ProdFormatter = &log.JSONFormatter{}
	}
	if appConfig.GracefulTimeout == 0 {
		appConfig.GracefulTimeout = 30 * time.Second
	}
	if appConfig.LogHooks == nil {
		appConfig.LogHooks = make([]log.Hook, 0)
	}

	version := getVersion(appConfig)

	internalConfig, err := initConfig(appConfig)
	if err != nil {
		return err
	}

	logEntry, err := initLogging(appConfig)
	if err != nil {
		return err
	}

	app = &Application{
		name:                 appConfig.Name,
		version:              version,
		defaultLoggingFields: appConfig.DefaultLoggingFields,
		initConfig:           appConfig,
		config:               appConfig.Config,
		internalConfig:       internalConfig,
		logMutex:             sync.Mutex{},
		log:                  logEntry,
		shutdownChannelsMutex: sync.Mutex{},
		shutdownChannels:      make([]*gracefulChannelShutdown, 0),
		shutdown:              make(chan os.Signal),
		services:              make([]GracefulService, 0),
		stoppedMutex:          sync.Mutex{},
		stopped:               false,
		gracefulTimeout:       appConfig.GracefulTimeout,
	}

	return nil
}

//Name returns the name of the application
func Name() string {
	return app.name
}

//Version returns the version of the application
func Version() string {
	return app.version
}

//Config returns the full config of the application
func Config() interface{} {
	return app.config
}

//Stopped returns whether the application is in a stopped state
func Stopped() bool {
	if app == nil {
		return false
	}

	app.stoppedMutex.Lock()
	defer app.stoppedMutex.Unlock()
	return app.stopped
}

//WaitShutdown waits for the application to receive a shutdown signal and stop all services
func WaitShutdown() <-chan os.Signal {
	return app.shutdown
}

//ShutdownSignalReceived is used to get notified when an os terminate signal is received
//by the application to clean up resources before exiting
//each channel call will block with a timeout
func ShutdownSignalReceived(shutdown chan chan error, identifier string) {
	app.shutdownChannelsMutex.Lock()
	defer app.shutdownChannelsMutex.Unlock()
	adhocShutdown := &gracefulChannelShutdown{
		name: identifier, //TODO: name,
		ch:   shutdown,
	}

	app.shutdownChannels = append(app.shutdownChannels, adhocShutdown)
}

func (g *gracefulChannelShutdown) String() string {
	return g.name
}

func (g *gracefulChannelShutdown) Shutdown() error {
	err := make(chan error)
	g.ch <- err
	return <-err
}

func handleGracefulShutdown() {
	shutdownCh := make(chan os.Signal)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		shutdownSignal := <-shutdownCh
		app.stoppedMutex.Lock()
		app.stopped = true
		app.stoppedMutex.Unlock()
		Log().Infof("Received graceful shutdown message %s", shutdownSignal)

		app.servicesMutex.Lock()
		services := app.services[:]
		app.servicesMutex.Unlock()

		app.shutdownChannelsMutex.Lock()
		shutdownChannels := app.shutdownChannels[:]
		app.shutdownChannelsMutex.Unlock()

		for _, adhocService := range shutdownChannels {
			services = append(services, adhocService)
		}

		if len(services) == 0 {
			Log().Infof("No services to stop, exiting.")
			app.shutdown <- shutdownSignal
			return
		}

		Log().Infof("Stopping a total of %d services: %s", len(services), services)

		var wg sync.WaitGroup
		for _, s := range services {
			wg.Add(1)
			go func(s GracefulService) {
				app.log.Infof("Stopping service %s", s)
				defer wg.Done()

				complete := make(chan struct{})
				go func(s GracefulService, complete chan struct{}) {
					err := s.Shutdown()
					if err != nil {
						app.log.Errorf("Error during graceful shutdown %s", err)
					}

					complete <- struct{}{}
				}(s, complete)

				select {
				case <-complete:
					app.log.Infof("Stopped service %s", s)
				case <-time.After(app.gracefulTimeout):
					app.log.Infof("The service: %s did not shutdown gracefully in the %d timeout, skiping", s, gracefulTimeout)
				}
			}(s)
		}

		wg.Wait()

		app.shutdown <- shutdownSignal
	}()
}
