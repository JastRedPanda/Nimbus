package weather

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type WeatherData struct {
	Temperature      float64   `json:"temperature"`
	ApparentTemp     float64   `json:"apparent_temperature"`
	Humidity         float64   `json:"humidity"`
	WindSpeed        float64   `json:"wind_speed"`
	SurfacePressure  float64   `json:"surface_pressure"`
	WeatherCode      int       `json:"weather_code"`
	Location         string    `json:"-"`
	FetchedAt        time.Time `json:"-"`
}

type openMeteoResponse struct {
	Current struct {
		Temperature2M      float64 `json:"temperature_2m"`
		ApparentTemp       float64 `json:"apparent_temperature"`
		RelativeHumidity2M float64 `json:"relative_humidity_2m"`
		WindSpeed10M       float64 `json:"wind_speed_10m"`
		SurfacePressure    float64 `json:"surface_pressure"`
		WeatherCode        int     `json:"weather_code"`
	} `json:"current"`
}

func Fetch(lat, lon float64) (*WeatherData, error) {
	urlStr := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,relative_humidity_2m,apparent_temperature,weather_code,wind_speed_10m,surface_pressure&timezone=auto",
		lat, lon,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var omr openMeteoResponse
	if err := json.Unmarshal(body, &omr); err != nil {
		return nil, fmt.Errorf("parse JSON failed: %w", err)
	}

	return &WeatherData{
		Temperature:     omr.Current.Temperature2M,
		ApparentTemp:    omr.Current.ApparentTemp,
		Humidity:        omr.Current.RelativeHumidity2M,
		WindSpeed:       omr.Current.WindSpeed10M,
		SurfacePressure: omr.Current.SurfacePressure,
		WeatherCode:     omr.Current.WeatherCode,
		FetchedAt:       time.Now(),
	}, nil
}

func (w *WeatherData) Emoji() string {
	switch {
	case w.WeatherCode == 0:
		return "\u2600\ufe0f"
	case w.WeatherCode <= 2:
		return "\u26c5"
	case w.WeatherCode == 3:
		return "\u2601\ufe0f"
	case w.WeatherCode == 45 || w.WeatherCode == 48:
		return "\U0001f32b\ufe0f"
	case w.WeatherCode >= 51 && w.WeatherCode <= 55:
		return "\U0001f326\ufe0f"
	case w.WeatherCode >= 61 && w.WeatherCode <= 65:
		return "\U0001f327\ufe0f"
	case w.WeatherCode >= 71 && w.WeatherCode <= 77:
		return "\u2744\ufe0f"
	case w.WeatherCode >= 80 && w.WeatherCode <= 82:
		return "\U0001f327\ufe0f"
	case w.WeatherCode >= 85 && w.WeatherCode <= 86:
		return "\U0001f328\ufe0f"
	case w.WeatherCode >= 95:
		return "\u26c8\ufe0f"
	default:
		return "\U0001f321\ufe0f"
	}
}

type GeoResult struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Country   string  `json:"country"`
	Admin1    string  `json:"admin1"`
}

type geoResponse struct {
	Results []GeoResult `json:"results"`
}

func SearchCity(query, lang string) ([]GeoResult, error) {
	urlStr := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=15&language=%s&format=json",
		url.QueryEscape(query), url.QueryEscape(lang))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("geocoding request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read geocoding response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geocoding API error %d: %s", resp.StatusCode, string(body))
	}

	var gr geoResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("parse geocoding JSON failed: %w", err)
	}

	return gr.Results, nil
}

type ipGeoData struct {
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	City    string  `json:"city"`
	Country string  `json:"country"`
}

func DetectLocation() (string, float64, float64, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/")
	if err != nil {
		return "", 0, 0, fmt.Errorf("IP geolocation request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, fmt.Errorf("read IP geolocation failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, 0, fmt.Errorf("IP geolocation API error %d: %s", resp.StatusCode, string(body))
	}

	var d ipGeoData
	if err := json.Unmarshal(body, &d); err != nil {
		return "", 0, 0, fmt.Errorf("parse IP geolocation failed: %w", err)
	}

	return d.City + ", " + d.Country, d.Lat, d.Lon, nil
}
