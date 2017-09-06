package app

import (
	"io"
	"reflect"

	"os"

	log "github.com/Sirupsen/logrus"
)

func initLogging(c *ApplicationInitConfig) (*log.Entry, error) {
	configValue := reflect.Indirect(reflect.ValueOf(c.Config))

	loggingEnv := configValue.FieldByName("LoggingHost").String()
	version := getVersion(c)

	log.SetFormatter(&log.JSONFormatter{})
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = loggingEnv
	}

	logEntry := log.WithFields(c.DefaultLoggingFields).WithFields(log.Fields{
		"component": c.Name,
		"version":   version,
		"host":      loggingEnv,
		"hostname":  hostname,
	})

	logLevel := configValue.FieldByName("LoggingLevel").String()
	lvl, err := log.ParseLevel(logLevel)
	if err != nil {
		return nil, err
	}

	logEntry.Logger.Level = lvl
	return logEntry, nil
}

//TODO: add methods for the other levels when needed

//GetLogWriter returns a writer for the specific logLevel
func GetLogWriter(level log.Level) *io.PipeWriter {
	return Log().Logger.WriterLevel(level)
}

//InfoLogWriter is a log writer with info log level
func InfoLogWriter() *io.PipeWriter {
	return GetLogWriter(log.InfoLevel)
}

//FatalLogWriter is a log writer with fatal log level
func FatalLogWriter() *io.PipeWriter {
	return GetLogWriter(log.FatalLevel)
}

//ErrorLogWriter is a log writer with error log level
func ErrorLogWriter() *io.PipeWriter {
	return GetLogWriter(log.ErrorLevel)
}
