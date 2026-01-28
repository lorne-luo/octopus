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
	// Timeouts are in seconds; 0 means "disabled".
	ReadHeaderTimeoutSec int `mapstructure:"read_header_timeout_sec"`
	ReadTimeoutSec       int `mapstructure:"read_timeout_sec"`
	WriteTimeoutSec      int `mapstructure:"write_timeout_sec"`
	IdleTimeoutSec       int `mapstructure:"idle_timeout_sec"`
}

type Log struct {
	Level string `mapstructure:"level"`
}

type Database struct {
	Type string `mapstructure:"type"`
	Path string `mapstructure:"path"`
}

type UpstreamHTTP struct {
	// Transport-level timeouts (seconds). These do NOT limit total body streaming duration.
	DialTimeoutSec           int `mapstructure:"dial_timeout_sec"`
	TLSHandshakeTimeoutSec   int `mapstructure:"tls_handshake_timeout_sec"`
	ResponseHeaderTimeoutSec int `mapstructure:"response_header_timeout_sec"`
	ExpectContinueTimeoutSec int `mapstructure:"expect_continue_timeout_sec"`
	IdleConnTimeoutSec       int `mapstructure:"idle_conn_timeout_sec"`
}

type Relay struct {
	// Non-stream requests: hard deadline for the whole request (seconds).
	NonStreamRequestTimeoutSec int `mapstructure:"non_stream_request_timeout_sec"`

	// Streaming requests: abort upstream if no SSE event is received for this duration (seconds).
	StreamIdleTimeoutSec int `mapstructure:"stream_idle_timeout_sec"`

	// Optional: abort upstream if we produce no client-visible output for this duration (seconds).
	// This catches cases where upstream is active but transformers drop all events.
	StreamNoOutputTimeoutSec int `mapstructure:"stream_no_output_timeout_sec"`
}

type Config struct {
	Server      Server      `mapstructure:"server"`
	Log         Log         `mapstructure:"log"`
	Database    Database    `mapstructure:"database"`
	UpstreamHTTP UpstreamHTTP `mapstructure:"upstream_http"`
	Relay       Relay       `mapstructure:"relay"`
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
	// Server timeouts: keep WriteTimeout disabled by default to avoid breaking SSE.
	viper.SetDefault("server.read_header_timeout_sec", 10)
	viper.SetDefault("server.read_timeout_sec", 30)
	viper.SetDefault("server.write_timeout_sec", 0)
	viper.SetDefault("server.idle_timeout_sec", 120)

	viper.SetDefault("database.type", "sqlite")
	viper.SetDefault("database.path", "data/data.db")
	viper.SetDefault("log.level", "info")

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
