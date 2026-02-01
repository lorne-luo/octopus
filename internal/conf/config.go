package conf

import (
	"fmt"
	"os"
	"strings"

	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/spf13/viper"
)

type Server struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type Log struct {
	Level string `mapstructure:"level"`
	Path  string `mapstructure:"path"`
}

type Database struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
}

type Config struct {
	Log          Log          `mapstructure:"log"`
	Database     Database     `mapstructure:"database"`
	UpstreamHTTP UpstreamHTTP `mapstructure:"upstream_http"`
	Relay        Relay        `mapstructure:"relay"`
}

var AppConfig Config

func Load(path string) error {
	if path != "" {
		viper.SetConfigFile(path)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("json")
		viper.AddConfigPath("data")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix(APP_NAME)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	setDefaults()

	if err := viper.ReadInConfig(); err == nil {
		log.Infof("Using config file: %s", viper.ConfigFileUsed())
	} else {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Infof("Config file not found, creating default config")
			if err := os.MkdirAll("data", 0755); err != nil {
				log.Errorf("Failed to create data directory: %v", err)
			}
			if err := viper.SafeWriteConfigAs("data/config.json"); err != nil {
				log.Errorf("Failed to create default config: %v", err)
			}
		} else {
			return fmt.Errorf("error reading config file: %w", err)
		}
	}

	if err := viper.Unmarshal(&AppConfig); err != nil {
		return fmt.Errorf("unable to decode config into struct: %w", err)
	}
	return nil
}

func setDefaults() {
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("database.type", "sqlite")
	viper.SetDefault("database.path", "data/data.db")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.path", "data/logs.log")

	// Upstream HTTP transport-level timeouts.
	viper.SetDefault("upstream_http.dial_timeout_sec", 10)
	viper.SetDefault("upstream_http.tls_handshake_timeout_sec", 10)
	viper.SetDefault("upstream_http.response_header_timeout_sec", 30)
	viper.SetDefault("upstream_http.expect_continue_timeout_sec", 1)
	viper.SetDefault("upstream_http.idle_conn_timeout_sec", 90)

	// Relay timeouts.
	viper.SetDefault("relay.non_stream_request_timeout_sec", 120)
	viper.SetDefault("relay.stream_idle_timeout_sec", 180)
	viper.SetDefault("relay.stream_no_output_timeout_sec", 0)
}
