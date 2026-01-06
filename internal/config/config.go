package config

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type Config struct {
	Env        string `mapstructure:"ENV" validate:"required"`
	ServerPort string `mapstructure:"SERVER_PORT" validate:"required,numeric"`
	DBConn     string `mapstructure:"DB_CONN" validate:"required"`
	RedisAddr  string `mapstructure:"REDIS_ADDR" validate:"required"`
	SecretKey  string `mapstructure:"SECRET_KEY" validate:"required"`
	BaseURL    string `mapstructure:"BASE_URL" validate:"required"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Ignore if config file is not found
		} else {
			return nil, fmt.Errorf("fatal error config file: %w", err)
		}

	}
	var cfg Config
	viper.Unmarshal(&cfg)
	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	return &cfg, nil
}
