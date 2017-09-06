package app

import "github.com/kelseyhightower/envconfig"

//BaseAppConfig is the basic config every app has
type BaseAppConfig struct {
	IsDebug bool `default:"true"`

	ApiServerUrl string `default:"http://localhost:3000" envconfig:"API_SERVER_URL"`

	LoggingLevel string `default:"debug" envconfig:"LOGGING_LEVEL"`
	LoggingHost  string `default:"local" envconfig:"LOGGING_HOST"`

	MaxStatsIntervalInSec int `default:"60" envconfig:"MAX_STATS_INTERVAL_IN_SEC"` // 1 minute
	MaxStatsCount         int `default:"100" envconfig:"MAX_STATS_COUNT"`
}

func initConfig(c *ApplicationInitConfig) (*BaseAppConfig, error) {
	internalConfig := &BaseAppConfig{}
	err := envconfig.Process(c.Name, internalConfig)
	if err != nil {
		return nil, err
	}

	return internalConfig, envconfig.Process(c.Name, c.Config)
}
