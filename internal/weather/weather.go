package weather

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type WeatherData struct {
	Temperature      float64   `json:"temperature"`
	ApparentTemp     float64   `json:"apparent_temperature"`
	Humidity         float64   `json:"humidity"`
	WindSpeed        float64   `json:"wind_speed"`
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
		WeatherCode        int     `json:"weather_code"`
	} `json:"current"`
}

func Fetch(lat, lon float64) (*WeatherData, error) {
	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,relative_humidity_2m,apparent_temperature,weather_code,wind_speed_10m&timezone=auto",
		lat, lon,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
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
		Temperature:  omr.Current.Temperature2M,
		ApparentTemp: omr.Current.ApparentTemp,
		Humidity:     omr.Current.RelativeHumidity2M,
		WindSpeed:    omr.Current.WindSpeed10M,
		WeatherCode:  omr.Current.WeatherCode,
		FetchedAt:    time.Now(),
	}, nil
}

func (w *WeatherData) Condition() string {
	switch {
	case w.WeatherCode == 0:
		return "Clear"
	case w.WeatherCode <= 3:
		return "Partly cloudy"
	case w.WeatherCode == 45 || w.WeatherCode == 48:
		return "Foggy"
	case w.WeatherCode >= 51 && w.WeatherCode <= 55:
		return "Drizzle"
	case w.WeatherCode >= 61 && w.WeatherCode <= 65:
		return "Rain"
	case w.WeatherCode >= 71 && w.WeatherCode <= 77:
		return "Snow"
	case w.WeatherCode >= 80 && w.WeatherCode <= 82:
		return "Rain showers"
	case w.WeatherCode >= 85 && w.WeatherCode <= 86:
		return "Snow showers"
	case w.WeatherCode >= 95:
		return "Thunderstorm"
	default:
		return "Unknown"
	}
}

func (w *WeatherData) Emoji() string {
	switch {
	case w.WeatherCode == 0:
		return "☀️"
	case w.WeatherCode <= 2:
		return "⛅"
	case w.WeatherCode == 3:
		return "☁️"
	case w.WeatherCode == 45 || w.WeatherCode == 48:
		return "🌫️"
	case w.WeatherCode >= 51 && w.WeatherCode <= 55:
		return "🌦️"
	case w.WeatherCode >= 61 && w.WeatherCode <= 65:
		return "🌧️"
	case w.WeatherCode >= 71 && w.WeatherCode <= 77:
		return "❄️"
	case w.WeatherCode >= 80 && w.WeatherCode <= 82:
		return "🌧️"
	case w.WeatherCode >= 85 && w.WeatherCode <= 86:
		return "🌨️"
	case w.WeatherCode >= 95:
		return "⛈️"
	default:
		return "🌡️"
	}
}

func (w *WeatherData) Tooltip() string {
	return fmt.Sprintf("%s %.1f°C | %s | Feels %.1f°C | 💧%d%% | 💨%.0f km/h",
		w.Emoji(), w.Temperature, w.Condition(), w.ApparentTemp,
		int(w.Humidity), w.WindSpeed)
}

func (w *WeatherData) Short() string {
	return fmt.Sprintf("%.1f°C %s", w.Temperature, w.Emoji())
}
