package weather

import (
	"sync"
	"time"
)

type cacheEntry struct {
	current   *WeatherData
	daily     []DailyForecast
	fetchedAt time.Time
	lat, lon  float64
}

var (
	entry cacheEntry
	mu    sync.RWMutex
)

func Cached(lat, lon float64) (current *WeatherData, daily []DailyForecast) {
	mu.RLock()
	defer mu.RUnlock()
	if entry.current == nil || entry.lat != lat || entry.lon != lon {
		return nil, nil
	}
	return entry.current, entry.daily
}

func Store(current *WeatherData, daily []DailyForecast, lat, lon float64) {
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()
	if current != nil {
		current.FetchedAt = now
	}
	entry.current = current
	entry.daily = daily
	entry.fetchedAt = now
	entry.lat = lat
	entry.lon = lon
}
