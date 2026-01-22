package config

import (
	"time"

	"github.com/spf13/viper"
)

type JiraConfig struct {
	BaseURL string `mapstructure:"base_url"`
}

type Config struct {
	Gitlab    GitlabConfig   `mapstructure:"gitlab"`
	VK        VKConfig       `mapstructure:"vk"`
	Database  DatabaseConfig `mapstructure:"database"`
	Jira      JiraConfig     `mapstructure:"jira"`
	StartTime string         `mapstructure:"start_time"` // Optional, format: YYYY-MM-DD
}

type GitlabConfig struct {
	BaseURL      string        `mapstructure:"base_url"`
	Token        string        `mapstructure:"token"`
	PollInterval time.Duration `mapstructure:"poll_interval"`
}

type VKConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Token   string `mapstructure:"token"`
}

type DatabaseConfig struct {
	DSN string `mapstructure:"dsn"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}