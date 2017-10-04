package app

import (
	"bytes"
	"fmt"
	"io"
	"reflect"

	"os"

	log "github.com/sirupsen/logrus"
)

func initLogging(c *ApplicationInitConfig) (*log.Entry, error) {
	configValue := reflect.Indirect(reflect.ValueOf(c.Config))

	version := getVersion(c)
	if version == "development" {
		log.SetFormatter(c.DevFormatter)
	} else {
		log.SetFormatter(c.ProdFormatter)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	logEntry := log.WithFields(c.DefaultLoggingFields).WithFields(log.Fields{
		"component": c.Name,
		"version":   version,
		"hostname":  hostname,
	})

	logLevel := configValue.FieldByName("LoggingLevel").String()
	lvl, err := log.ParseLevel(logLevel)
	if err != nil {
		return nil, err
	}

	logEntry.Logger.Level = lvl

	for _, hook := range c.LogHooks {
		log.AddHook(hook)
	}

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

type logEntry struct {
	*log.Entry
}

func (l *logEntry) ErrorWrap(err error, message string) {
	l.Error(fmt.Sprintf("%s: %s", err.Error(), message))
}

//Log returns a LogEntry with standart fields specified during init
func Log(context ...interface{}) *logEntry {
	var entry *log.Entry

	if app != nil {
		entry = app.log
	} else {
		entry = log.NewEntry(log.StandardLogger())
	}

	if len(context) > 0 {
		var b bytes.Buffer
		for i, a := range context {
			if i == 0 {
				b.WriteString(fmt.Sprintf("%s", a))
			} else {
				b.WriteString(fmt.Sprintf(": %s", a))
			}
		}

		entry = entry.WithField("context", b.String())
	}

	return &logEntry{
		&log.Entry{
			Logger: entry.Logger,
			Data:   entry.Data,
		},
	}
}
