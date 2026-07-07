package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Load reads, parses, applies defaults, and validates the config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("configuration file not found: %s", path)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse: %w", err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "127.0.0.1"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if !cfg.IsMultiApp() {
		if cfg.Backend.Timeout.Duration == 0 {
			cfg.Backend.Timeout.Duration = 3 * time.Second
		}
		if cfg.Sync.InactivityDelay.Duration == 0 {
			cfg.Sync.InactivityDelay.Duration = 30 * time.Second
		}
		if cfg.Sync.MaxInterval.Duration == 0 {
			cfg.Sync.MaxInterval.Duration = 5 * time.Minute
		}
	}
}

func validate(cfg *Config) error {
	if err := validateServer(cfg); err != nil {
		return err
	}
	if cfg.IsMultiApp() {
		for i, app := range cfg.Apps {
			if app.Name == "" {
				return fmt.Errorf("apps[%d].name is required", i)
			}
			if app.Path == "" {
				return fmt.Errorf("apps[%d].path is required", i)
			}
		}
		return nil
	}
	if cfg.Frontend.Path == "" {
		return fmt.Errorf("frontend.path is required")
	}
	if cfg.Backend.Path == "" {
		return fmt.Errorf("backend.path is required")
	}
	if cfg.Backend.APIPrefix == "" || cfg.Backend.APIPrefix[0] != '/' {
		return fmt.Errorf("backend.api_prefix must be non-empty and start with '/'")
	}
	if cfg.Sync.Enabled {
		if cfg.Sync.InactivityDelay.Duration <= 0 {
			return fmt.Errorf("sync.inactivity_delay must be positive")
		}
		if cfg.Sync.MaxInterval.Duration <= cfg.Sync.InactivityDelay.Duration {
			return fmt.Errorf("sync.max_interval must be greater than sync.inactivity_delay")
		}
	}
	return nil
}

func validateServer(cfg *Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	return nil
}

// ResolveApps returns the list of AppConfigs for the server to mount.
// For single-app configs, it returns one AppConfig with Name="" (root-mounted).
// For multi-app configs, it loads each sub-app's config.
func ResolveApps(cfg *Config, baseDir string) ([]AppConfig, error) {
	if !cfg.IsMultiApp() {
		return []AppConfig{{
			Name:     "",
			Dir:      baseDir,
			Frontend: cfg.Frontend,
			Backend:  cfg.Backend,
			Sync:     cfg.Sync,
		}}, nil
	}
	return loadMultiApps(cfg, baseDir)
}

func loadMultiApps(cfg *Config, baseDir string) ([]AppConfig, error) {
	var apps []AppConfig
	for _, entry := range cfg.Apps {
		appDir, err := filepath.Abs(filepath.Join(baseDir, entry.Path))
		if err != nil {
			return nil, fmt.Errorf("app %q: resolving path: %w", entry.Name, err)
		}
		appCfgPath := filepath.Join(appDir, ".servitor.conf")

		data, err := os.ReadFile(appCfgPath)
		if err != nil {
			return nil, fmt.Errorf("app %q: %w", entry.Name, err)
		}

		var appCfg Config
		if err := yaml.Unmarshal(data, &appCfg); err != nil {
			return nil, fmt.Errorf("app %q: failed to parse: %w", entry.Name, err)
		}
		applyDefaults(&appCfg)

		frontend := appCfg.Frontend
		if frontend.Path == "" {
			frontend.Path = "./frontend"
		}
		frontend.Path = filepath.Join(appDir, frontend.Path)

		backend := appCfg.Backend
		if backend.Path == "" {
			backend.Path = "./backend/servitor-backend"
		}
		resolved := filepath.Join(appDir, backend.Path)
		abs, err := filepath.Abs(resolved)
		if err != nil {
			return nil, fmt.Errorf("app %q: resolving backend path: %w", entry.Name, err)
		}
		backend.Path = abs
		if backend.APIPrefix == "" {
			backend.APIPrefix = "/api"
		}

		apps = append(apps, AppConfig{
			Name:     entry.Name,
			Dir:      appDir,
			Frontend: frontend,
			Backend:  backend,
			Sync:     appCfg.Sync,
		})
	}
	return apps, nil
}
