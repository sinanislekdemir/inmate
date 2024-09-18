package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type InfluxDBConfig struct {
	URLs         []string `yaml:"urls"`
	Port         int      `yaml:"port"`
	BindAddress  string   `yaml:"bind_address"`
	RetryDelay   int      `yaml:"retry_delay"`
	RetryCount   int      `yaml:"retry_count"`
	QueryTimeout int      `yaml:"query_timeout"`
	ChannelSize  int      `yaml:"channel_size"`
}

var config InfluxDBConfig

func LoadConfig(filename string) {
	configData, err := os.ReadFile(filename)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"filename": filename,
			"error":    err,
		}).Fatal("Error reading config file")
	}

	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error":    err,
			"filename": filename,
		}).Fatal("Error parsing config file")
	}
}
