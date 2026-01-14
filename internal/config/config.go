package config

import (
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type Config struct {
	Env                            string `mapstructure:"ENV" validate:"required"`
	ServerPort                     string `mapstructure:"SERVER_PORT" validate:"required,numeric"`
	DBConn                         string `mapstructure:"DB_CONN" validate:"required"`
	RedisAddr                      string `mapstructure:"REDIS_ADDR" validate:"required"`
	SecretKey                      string `mapstructure:"SECRET_KEY" validate:"required"`
	BaseURL                        string `mapstructure:"BASE_URL" validate:"required"`
	RateLimitCount                 int    `mapstructure:"RATE_LIMIT_COUNT" validate:"required,numeric"`
	OldLinksCleanUpIntervalInHours int    `mapstructure:"OLD_LINKS_CLEANUP_INTERVAL_IN_HOURS" validate:"required,numeric,gte=1"`
}

func LoadConfig() (*Config, error) {
	v := viper.New()
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AutomaticEnv()
	var cfg Config
	bindAllEnv(v, cfg)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Ignore if config file is not found
		} else {
			return nil, fmt.Errorf("fatal error config file: %w", err)
		}

	}
	v.Unmarshal(&cfg)
	validate := validator.New()
	if err := validate.Struct(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	return &cfg, nil
}

func bindAllEnv(v *viper.Viper, i interface{}) {
	t := reflect.TypeOf(i)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag != "" {
			v.BindEnv(tag)
		}
	}
}
