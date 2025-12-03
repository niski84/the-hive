// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package drone

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/spf13/viper"
)

// Config holds the drone client configuration
type Config struct {
	ClientID          string          `mapstructure:"client_id"`
	Server            ServerConfig    `mapstructure:"server"`
	GrpcServerAddress string          `mapstructure:"grpc_server_address"`
	WatchPaths        []string        `mapstructure:"watch_paths"`
	WebServer         WebServerConfig `mapstructure:"web_server"`
	APIKey            string          `mapstructure:"api_key"`
}

// ServerConfig holds Hive server connection settings
type ServerConfig struct {
	Address string `mapstructure:"address"` // HTTP address for WebSocket/health checks
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
	viper.SetDefault("server.address", "http://localhost:8081")
	viper.SetDefault("grpc_server_address", "localhost:50051")
	viper.SetDefault("watch_paths", []string{"./watch"})
	viper.SetDefault("web_server.port", 9090)
	// Note: client_id will be generated if missing, not set as default

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

	// Set smart defaults for server URL
	if config.Server.Address == "" {
		config.Server.Address = "http://localhost:8081"
		log.Printf("Server URL was empty, defaulting to: %s", config.Server.Address)
	}

	// Set smart defaults for gRPC server address
	if config.GrpcServerAddress == "" {
		config.GrpcServerAddress = "localhost:50051"
		log.Printf("gRPC Server Address was empty, defaulting to: %s", config.GrpcServerAddress)
	}

	// Generate client_id if missing
	if config.ClientID == "" {
		config.ClientID = uuid.New().String()
		log.Printf("Generated new client ID: %s", config.ClientID)

		// Save the generated client_id to config file
		configFile := viper.ConfigFileUsed()
		if configFile != "" {
			viper.Set("client_id", config.ClientID)
			if err := viper.WriteConfig(); err != nil {
				log.Printf("Warning: Failed to save client_id to config file: %v", err)
			} else {
				log.Printf("Saved client_id to config file")
			}
		}
	} else {
		log.Printf("Using existing client ID: %s", config.ClientID)
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
	viper.Set("client_id", config.ClientID)
	viper.Set("server.address", config.Server.Address)
	viper.Set("grpc_server_address", config.GrpcServerAddress)
	viper.Set("api_key", config.APIKey)
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
# client_id will be auto-generated on first run

server:
  address: "http://localhost:8081"  # Hive server HTTP address (for WebSocket/health checks)

grpc_server_address: "localhost:50051"  # Hive server gRPC address (for ingestion)

api_key: ""  # API key for authentication (get from server settings)

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
