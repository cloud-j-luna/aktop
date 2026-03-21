package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	MonikerSchemaVersion = 1
	MonikerCacheFileName = "monikers.json"
)

type MonikerCacheData struct {
	SchemaVersion int               `json:"schema_version"`
	LastUpdated   time.Time         `json:"last_updated"`
	Monikers      map[string]string `json:"monikers"`
}

type MonikerCache struct {
	data MonikerCacheData
	mu   sync.RWMutex
	path string
}

func LoadMonikerCache(cacheDir string) (*MonikerCache, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cachePath := filepath.Join(cacheDir, MonikerCacheFileName)
	cache := &MonikerCache{
		path: cachePath,
		data: MonikerCacheData{
			SchemaVersion: MonikerSchemaVersion,
			Monikers:      make(map[string]string),
		},
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return nil, fmt.Errorf("failed to read moniker cache: %w", err)
	}

	var cacheData MonikerCacheData
	if err := json.Unmarshal(data, &cacheData); err != nil {
		return cache, nil
	}

	if cacheData.SchemaVersion != MonikerSchemaVersion {
		return cache, nil
	}

	cache.data = cacheData
	if cache.data.Monikers == nil {
		cache.data.Monikers = make(map[string]string)
	}

	return cache, nil
}

func (c *MonikerCache) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal moniker cache: %w", err)
	}

	tmpPath := c.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write moniker cache: %w", err)
	}

	if err := os.Rename(tmpPath, c.path); err != nil {
		return fmt.Errorf("failed to rename moniker cache: %w", err)
	}

	return nil
}

func (c *MonikerCache) Get() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]string, len(c.data.Monikers))
	for k, v := range c.data.Monikers {
		result[k] = v
	}
	return result
}

func (c *MonikerCache) Set(monikers map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data.Monikers = monikers
	c.data.LastUpdated = time.Now()
}

func (c *MonikerCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data.Monikers = make(map[string]string)
	c.data.LastUpdated = time.Time{}

	if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear moniker cache: %w", err)
	}

	return nil
}

func (c *MonikerCache) HasMonikers() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data.Monikers) > 0
}
