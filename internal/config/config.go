package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	UpdateInterval int     `json:"update_interval"`
	Units          string  `json:"units"`
}

func Default() *Config {
	return &Config{
		Latitude:       55.7558,
		Longitude:      37.6173,
		UpdateInterval: 10,
		Units:          "celsius",
	}
}

func configDir() (string, error) {
	cd, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cd, "Nimbus"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return Default(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), err
	}
	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return Default(), err
	}
	return cfg, nil
}

func (c *Config) Save() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) Interval() time.Duration {
	d := time.Duration(c.UpdateInterval) * time.Minute
	if d < time.Minute {
		d = time.Minute
	}
	return d
}

func (c *Config) String() string {
	return fmt.Sprintf("Location: %.4f, %.4f | Interval: %d min | Units: %s",
		c.Latitude, c.Longitude, c.UpdateInterval, c.Units)
}
