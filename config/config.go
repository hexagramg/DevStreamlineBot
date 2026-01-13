package config

import (
	"time"

	"github.com/spf13/viper"
)

// Config holds the application configuration.
type Config struct {
	Gitlab    GitlabConfig   `mapstructure:"gitlab"`
	VK        VKConfig       `mapstructure:"vk"`
	Database  DatabaseConfig `mapstructure:"database"`
	StartTime string         `mapstructure:"start_time"` // Optional, format: YYYY-MM-DD
}

// GitlabConfig contains settings for the GitLab integration.
type GitlabConfig struct {
	BaseURL      string        `mapstructure:"base_url"`
	Token        string        `mapstructure:"token"`
	PollInterval time.Duration `mapstructure:"poll_interval"`
}

// VKConfig contains settings for the VK Teams integration.
type VKConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Token   string `mapstructure:"token"`
}

// DatabaseConfig contains database connection settings.
type DatabaseConfig struct {
	DSN string `mapstructure:"dsn"`
}

// LoadConfig reads configuration from a YAML file.
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