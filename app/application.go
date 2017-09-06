package app

import (
	"os"
	"sync"

	"reflect"

	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/go-errors/errors"
)

var (
	//ErrAppInitialized is returned when the application has already been initialized
	ErrAppInitialized = errors.New("Application already initialized")
)

type application struct {
	name, version        string
	defaultLoggingFields log.Fields
	config               interface{}
	initConfig           *ApplicationInitConfig
	internalConfig       *BaseAppConfig
	log                  *log.Entry
	shutdown             chan os.Signal
	services             []GracefulService
	servicesMutex        sync.Mutex
	stopped              bool
}

//ApplicationInitConfig is the default application config
type ApplicationInitConfig struct {
	Name, Version        string
	DefaultLoggingFields log.Fields
	Config               interface{}
}

var app *application

//Init initializes the application, then the package methods can be used
func Init(appConfig *ApplicationInitConfig) error {
	err := initApp(appConfig)
	if err != nil {
		return err
	}

	handleGracefulShutdown()

	return nil
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

	version := getVersion(appConfig)

	internalConfig, err := initConfig(appConfig)
	if err != nil {
		return err
	}

	logEntry, err := initLogging(appConfig)
	if err != nil {
		return err
	}

	app = &application{
		name:                 appConfig.Name,
		version:              version,
		defaultLoggingFields: appConfig.DefaultLoggingFields,
		initConfig:           appConfig,
		config:               appConfig.Config,
		internalConfig:       internalConfig,
		log:                  logEntry,
		shutdown:             make(chan os.Signal),
		services:             make([]GracefulService, 0),
		stopped:              false,
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

//Log returns a LogEntry with standart fields specified during init
func Log() *log.Entry {
	var entry *log.Entry

	if app != nil {
		entry = app.log
	} else {
		entry = log.NewEntry(log.StandardLogger())
	}

	entry.Level = entry.Logger.Level
	return entry
}

//Stopped returns whether the application is in a stopped state
func Stopped() bool {
	if app == nil {
		return false
	}

	return app.stopped
}

//WaitShutdown waits for the application to receive a shutdown signal and stop all services
func WaitShutdown() <-chan os.Signal {
	return app.shutdown
}

//LogObject adds the fields of an object to the log entry, e.g.
//{SomeField: "FieldValue", NestedField: { NestedFieldValue: "NestedFieldValue" }} logs this ->
//{component: "comp-name", time: "2016-11-30T10:57:12+02:00", level:"info", SomeField: "FieldValue", NestedField.NestedFieldValue: "NestedFieldValue"}
func LogObject(o interface{}) *log.Entry {
	return LogObjectWithEntry(o, Log())
}

//LogObjectWithEntry for performance reasons this will get the fields of the object only when the logging level is Debug
//reflection comes with a price and when processing millions of messages things get slow
//if you are absolutely positive that you need to log something use MustLogObjectWithEntry
func LogObjectWithEntry(o interface{}, entry *log.Entry) *log.Entry {
	if entry.Level >= log.DebugLevel {
		return MustLogObjectWithEntry(o, entry)
	}

	return entry
}

//MustLogObjectWithEntry logs and object and it's values regardless of the log level
func MustLogObjectWithEntry(o interface{}, entry *log.Entry) *log.Entry {
	v := reflect.Indirect(reflect.ValueOf(o))
	if v.Kind() != reflect.Struct {
		return entry
	}

	paths, values := iterateObjectFields(v, "")
	for i := range paths {
		path := paths[i]
		value := values[i]
		entry = entry.WithField(path, value)
	}

	return entry
}

func getStructFieldValue(v reflect.Value, path string) ([]string, []interface{}) {
	paths := make([]string, 0)
	values := make([]interface{}, 0)
	if v.Kind() == reflect.Struct {
		innerPaths, innerValues := iterateObjectFields(v, path)
		paths = append(paths, innerPaths...)
		values = append(values, innerValues...)
		return paths, values
	} else if !v.CanInterface() {
		return []string{}, []interface{}{}
	}

	paths = append(paths, path)
	values = append(values, v.Interface())

	return paths, values
}

func iterateObjectFields(v reflect.Value, path string) ([]string, []interface{}) {
	fields := v.NumField()
	paths := make([]string, 0)
	values := make([]interface{}, 0)
	for i := 0; i < fields; i++ {
		fieldName := v.Type().Field(i).Name
		var fieldPath string
		if path == "" {
			fieldPath = fieldName
		} else {
			fieldPath += fmt.Sprintf("%s.%s", path, fieldName)
		}

		innerPaths, innerValues := getStructFieldValue(v.Field(i), fieldPath)
		paths = append(paths, innerPaths...)
		values = append(values, innerValues...)
	}

	return paths, values
}
