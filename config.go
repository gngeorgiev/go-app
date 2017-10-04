package app

import "github.com/kelseyhightower/envconfig"

//BaseAppConfig is the basic config every app has
type BaseAppConfig struct {
	IsDebug      bool   `default:"true"`
	LoggingLevel string `default:"debug" envconfig:"LOGGING_LEVEL"`
}

func initConfig(c *ApplicationInitConfig) (*BaseAppConfig, error) {
	internalConfig := &BaseAppConfig{}
	err := envconfig.Process(c.Name, internalConfig)
	if err != nil {
		return nil, err
	}

	return internalConfig, envconfig.Process(c.Name, c.Config)
}
