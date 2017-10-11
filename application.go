package app

import (
	"os"
	"sync"
	"time"

	"github.com/go-errors/errors"
	log "github.com/sirupsen/logrus"
)

type ShutdownPriority int

var (
	//ErrAppInitialized is returned when the application has already been initialized
	ErrAppInitialized      = errors.New("Application already initialized")
	ShutdownPriorityLast   = ShutdownPriority(0)
	ShutdownPriorityNormal = ShutdownPriority(1)
	ShutdownPriorityFirst  = ShutdownPriority(2)

	shutdownPriorityMap map[ShutdownPriority]string = map[ShutdownPriority]string{
		ShutdownPriorityFirst:  "First",
		ShutdownPriorityLast:   "Last",
		ShutdownPriorityNormal: "Normal",
	}
)

type Application struct {
	name, version        string
	defaultLoggingFields log.Fields
	config               interface{}
	initConfig           *ApplicationInitConfig
	internalConfig       *BaseAppConfig
	logMutex             sync.Mutex
	log                  *log.Entry
	shutdown             chan os.Signal
	services             []*gracefulServiceWrapper
	servicesMutex        sync.Mutex
	stoppedMutex         sync.Mutex
	stopped              bool
	gracefulTimeout      time.Duration
}

//ApplicationInitConfig is the default application config
type ApplicationInitConfig struct {
	Name, Version               string
	DefaultLoggingFields        log.Fields
	DevFormatter, ProdFormatter log.Formatter
	Config                      interface{}
	GracefulTimeout             time.Duration
	LogHooks                    []func() log.Hook
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
		appConfig.LogHooks = make([]func() log.Hook, 0)
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
		shutdown:             make(chan os.Signal),
		services:             make([]*gracefulServiceWrapper, 0),
		stoppedMutex:         sync.Mutex{},
		stopped:              false,
		gracefulTimeout:      appConfig.GracefulTimeout,
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

func (g *gracefulChannelShutdown) String() string {
	return g.name
}

func (g *gracefulChannelShutdown) Shutdown() error {
	err := make(chan error)
	g.ch <- err
	return <-err
}
