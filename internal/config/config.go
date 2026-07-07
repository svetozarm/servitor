package config

import (
	"net"
	"strconv"
)

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Frontend FrontendConfig `yaml:"frontend"`
	Backend  BackendConfig  `yaml:"backend"`
	Sync     SyncConfig     `yaml:"sync"`
	Apps     []AppEntry     `yaml:"apps"`
}

// AppEntry references a sub-application in multi-app mode.
type AppEntry struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// AppConfig holds the resolved configuration for a single app.
// Name "" means the app is served at root /.
type AppConfig struct {
	Name     string
	Dir      string
	Frontend FrontendConfig
	Backend  BackendConfig
	Sync     SyncConfig
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type FrontendConfig struct {
	Path string `yaml:"path"`
}

type BackendConfig struct {
	Path      string `yaml:"path"`
	APIPrefix string `yaml:"api_prefix"`
	Timeout   Duration `yaml:"timeout"`
}

type SyncConfig struct {
	Enabled         bool     `yaml:"enabled"`
	InactivityDelay Duration `yaml:"inactivity_delay"`
	MaxInterval     Duration `yaml:"max_interval"`
}

// Addr returns the host:port address string.
func (c *Config) Addr() string {
	return net.JoinHostPort(c.Server.Host, strconv.Itoa(c.Server.Port))
}

// IsMultiApp returns true if the config defines multiple named apps.
func (c *Config) IsMultiApp() bool {
	return len(c.Apps) > 0
}
