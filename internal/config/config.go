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
	CityName       string  `json:"city_name"`
	UpdateInterval int     `json:"update_interval"`
	Units          string  `json:"units"`
	PressureUnit   string  `json:"pressure_unit"`
	WindUnit       string  `json:"wind_unit"`
	IconTheme      string  `json:"icon_theme"`
	Language       string  `json:"language"`
	FontScale      int     `json:"font_scale"`
}

func Default() *Config {
	return &Config{
		Latitude:       50.4501,
		Longitude:      30.5234,
		CityName:       "Kyiv",
		UpdateInterval: 10,
		Units:          "celsius",
		PressureUnit:   "hpa",
		WindUnit:       "ms",
		IconTheme:      "auto",
		Language:       "en",
		FontScale:      100,
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

func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Delete() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (c *Config) Interval() time.Duration {
	d := time.Duration(c.UpdateInterval) * time.Minute
	if d < time.Minute {
		d = time.Minute
	}
	return d
}

func (c *Config) String() string {
	return fmt.Sprintf("City: %s (%.4f, %.4f) | Interval: %d min | Temp: %s | Pressure: %s | Theme: %s | Lang: %s",
		c.CityName, c.Latitude, c.Longitude, c.UpdateInterval, c.Units, c.PressureUnit, c.IconTheme, c.Language)
}
