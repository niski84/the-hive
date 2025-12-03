// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package drone

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the drone client configuration
type Config struct {
	Server      ServerConfig   `mapstructure:"server"`
	WatchPaths  []string       `mapstructure:"watch_paths"`
	WebServer   WebServerConfig `mapstructure:"web_server"`
}

// ServerConfig holds Hive server connection settings
type ServerConfig struct {
	Address string `mapstructure:"address"`
}

// WebServerConfig holds web server settings
type WebServerConfig struct {
	Port int `mapstructure:"port"`
}

// LoadConfig loads configuration from file and environment
func LoadConfig(configPath string) (*Config, error) {
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")

	// Set default values
	viper.SetDefault("server.address", "localhost:50051")
	viper.SetDefault("watch_paths", []string{"./watch"})
	viper.SetDefault("web_server.port", 9090)

	// If config path is provided, use it
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		// Otherwise, look in home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir := filepath.Join(home, ".the-hive")
		configFile := filepath.Join(configDir, "config.yaml")

		// Create config directory if it doesn't exist
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}

		// If config file doesn't exist, create default one
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			if err := generateDefaultConfig(configFile); err != nil {
				return nil, fmt.Errorf("failed to generate default config: %w", err)
			}
		}

		viper.SetConfigFile(configFile)
	}

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		// If file doesn't exist, try to create it
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			configFile := viper.ConfigFileUsed()
			if configFile != "" {
				if err := generateDefaultConfig(configFile); err != nil {
					return nil, fmt.Errorf("failed to generate default config: %w", err)
				}
				if err := viper.ReadInConfig(); err != nil {
					return nil, fmt.Errorf("failed to read config: %w", err)
				}
			} else {
				// No config file specified, use defaults
				log.Printf("No config file found, using defaults")
			}
		} else {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	// Allow environment variables
	viper.SetEnvPrefix("DRONE")
	viper.AutomaticEnv()

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// SaveConfig saves the current configuration to file
func SaveConfig(config *Config, configPath string) error {
	viper.SetConfigType("yaml")

	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(home, ".the-hive", "config.yaml")
	}

	// Set values
	viper.Set("server.address", config.Server.Address)
	viper.Set("watch_paths", config.WatchPaths)
	viper.Set("web_server.port", config.WebServer.Port)

	// Write to file
	if err := viper.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// generateDefaultConfig creates a default configuration file
func generateDefaultConfig(configFile string) error {
	defaultConfig := `# The Hive Drone Client Configuration

server:
  address: "localhost:50051"  # Hive server gRPC address

watch_paths:
  - "./watch"  # Directories to watch for files

web_server:
  port: 9090  # Web UI port
`

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
		return err
	}

	return os.WriteFile(configFile, []byte(defaultConfig), 0644)
}

// ApplyCLIFlags applies command-line flags to override config values
func ApplyCLIFlags(config *Config, serverAddr string, watchDirs []string, webPort int) {
	if serverAddr != "" {
		config.Server.Address = serverAddr
	}
	if len(watchDirs) > 0 {
		config.WatchPaths = watchDirs
	}
	if webPort > 0 {
		config.WebServer.Port = webPort
	}
}

