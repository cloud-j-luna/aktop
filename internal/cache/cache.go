package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloud-j-luna/aktop/internal/rpc"
)

const (
	SchemaVersion = 1
	CacheFileName = "providers.json"

	// Refresh intervals based on provider state
	OnlineCheckInterval     = 1 * time.Minute
	RecentOfflineInterval   = 5 * time.Minute
	LongTermOfflineInterval = 6 * time.Hour
	ChainSyncInterval       = 10 * time.Minute

	// Threshold for long-term offline
	LongTermOfflineThreshold = 5
)

// CachedProvider represents a provider with cached status information
type CachedProvider struct {
	HostURI             string            `json:"host_uri"`
	Name                string            `json:"name"`
	Country             string            `json:"country"`
	Attributes          map[string]string `json:"attributes"`
	IsOnline            bool              `json:"is_online"`
	Version             string            `json:"version"`
	CPUAvailable        uint64            `json:"cpu_available"`
	CPUTotal            uint64            `json:"cpu_total"`
	MemAvailable        uint64            `json:"mem_available"`
	MemTotal            uint64            `json:"mem_total"`
	GPUAvailable        uint64            `json:"gpu_available"`
	GPUTotal            uint64            `json:"gpu_total"`
	GPUModels           []string          `json:"gpu_models,omitempty"`
	LastSeenOnline      time.Time         `json:"last_seen_online"`
	LastChecked         time.Time         `json:"last_checked"`
	ConsecutiveFailures int               `json:"consecutive_failures"`
}

// ProviderCacheData is the JSON structure for the cache file
type ProviderCacheData struct {
	SchemaVersion int                        `json:"schema_version"`
	LastChainSync time.Time                  `json:"last_chain_sync"`
	Providers     map[string]*CachedProvider `json:"providers"`
}

// ProviderStore defines the interface for provider caching operations.
// This interface enables testing with mock implementations.
type ProviderStore interface {
	HasProviders() bool
	GetProvider(owner string) (*CachedProvider, bool)
	GetAllProviders() map[string]*CachedProvider
	GetOnlineProviders() []*CachedProvider
	MarkProviderOnline(owner, version string, cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal uint64, gpuModels []string)
	MarkProviderOffline(owner string)
	SyncWithChain(onChainProviders []rpc.OnChainProvider) []string
	GetProvidersDueForCheck() []string
	GetProvidersByPriority() []string
	ProviderCount() int
	OnlineCount() int
	Save() error
}

// Ensure ProviderCache implements ProviderStore
var _ ProviderStore = (*ProviderCache)(nil)

// ProviderCache manages the provider cache with thread-safe access
type ProviderCache struct {
	data ProviderCacheData
	mu   sync.RWMutex
	path string
}

// LoadOrCreate loads the cache from disk or creates a new one
func LoadOrCreate(cacheDir string) (*ProviderCache, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cachePath := filepath.Join(cacheDir, CacheFileName)
	cache := &ProviderCache{
		path: cachePath,
		data: ProviderCacheData{
			SchemaVersion: SchemaVersion,
			Providers:     make(map[string]*CachedProvider),
		},
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cacheData ProviderCacheData
	if err := json.Unmarshal(data, &cacheData); err != nil {
		return cache, nil
	}

	if cacheData.SchemaVersion != SchemaVersion {
		return cache, nil
	}

	cache.data = cacheData
	if cache.data.Providers == nil {
		cache.data.Providers = make(map[string]*CachedProvider)
	}

	return cache, nil
}

// Save writes the cache to disk atomically
func (c *ProviderCache) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	tmpPath := c.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	if err := os.Rename(tmpPath, c.path); err != nil {
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	return nil
}

// HasProviders returns true if the cache has any providers
func (c *ProviderCache) HasProviders() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data.Providers) > 0
}

// GetProvider returns a copy of a provider by owner address
func (c *ProviderCache) GetProvider(owner string) (*CachedProvider, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.data.Providers[owner]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

// GetAllProviders returns a copy of all cached providers
func (c *ProviderCache) GetAllProviders() map[string]*CachedProvider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*CachedProvider, len(c.data.Providers))
	for k, v := range c.data.Providers {
		cp := *v
		result[k] = &cp
	}
	return result
}

// GetOnlineProviders returns all providers that are currently online
func (c *ProviderCache) GetOnlineProviders() []*CachedProvider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var online []*CachedProvider
	for _, p := range c.data.Providers {
		if p.IsOnline {
			cp := *p
			online = append(online, &cp)
		}
	}
	return online
}

// UpdateProvider updates a provider in the cache
func (c *ProviderCache) UpdateProvider(owner string, p *CachedProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.Providers[owner] = p
}

// AddNewProvider adds a new provider from on-chain data if it doesn't exist
func (c *ProviderCache) AddNewProvider(owner, hostURI string, attributes map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.data.Providers[owner]; exists {
		return
	}

	name := attributes["organization"]
	if name == "" {
		name = extractHostname(hostURI)
	}

	c.data.Providers[owner] = &CachedProvider{
		HostURI:    hostURI,
		Name:       name,
		Country:    attributes["country"],
		Attributes: attributes,
		IsOnline:   false,
		Version:    "",
	}
}

// MarkProviderOnline marks a provider as online with updated stats
func (c *ProviderCache) MarkProviderOnline(owner string, version string, cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal uint64, gpuModels []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	p, exists := c.data.Providers[owner]
	if !exists {
		return
	}

	p.IsOnline = true
	p.Version = version
	p.CPUAvailable = cpuAvail
	p.CPUTotal = cpuTotal
	p.MemAvailable = memAvail
	p.MemTotal = memTotal
	p.GPUAvailable = gpuAvail
	p.GPUTotal = gpuTotal
	p.GPUModels = gpuModels
	p.LastSeenOnline = time.Now()
	p.LastChecked = time.Now()
	p.ConsecutiveFailures = 0
}

// MarkProviderOffline marks a provider as offline
func (c *ProviderCache) MarkProviderOffline(owner string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	p, exists := c.data.Providers[owner]
	if !exists {
		return
	}

	p.IsOnline = false
	p.LastChecked = time.Now()
	p.ConsecutiveFailures++
}

// SetLastChainSync updates the last chain sync time
func (c *ProviderCache) SetLastChainSync(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.LastChainSync = t
}

// GetLastChainSync returns the last chain sync time
func (c *ProviderCache) GetLastChainSync() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data.LastChainSync
}

// SyncWithChain syncs the cache with on-chain providers
// Returns a list of new provider owners that weren't in the cache
func (c *ProviderCache) SyncWithChain(onChainProviders []rpc.OnChainProvider) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var newProviders []string

	for _, ocp := range onChainProviders {
		if _, exists := c.data.Providers[ocp.Owner]; !exists {
			// New provider
			name := ocp.Attributes["organization"]
			if name == "" {
				name = extractHostname(ocp.HostURI)
			}

			now := time.Now()
			p := &CachedProvider{
				HostURI:    ocp.HostURI,
				Name:       name,
				Country:    ocp.Attributes["country"],
				Attributes: ocp.Attributes,
				IsOnline:   ocp.IsOnline, // Initial status, verified by polling
				Version:    "",
			}
			// If marked online (e.g., from seed), set LastSeenOnline
			if ocp.IsOnline {
				p.LastSeenOnline = now
			}
			c.data.Providers[ocp.Owner] = p
			newProviders = append(newProviders, ocp.Owner)
		} else {
			// Update hostURI and attributes in case they changed
			p := c.data.Providers[ocp.Owner]
			p.HostURI = ocp.HostURI
			p.Attributes = ocp.Attributes
			if name := ocp.Attributes["organization"]; name != "" {
				p.Name = name
			}
			if country := ocp.Attributes["country"]; country != "" {
				p.Country = country
			}
		}
	}

	c.data.LastChainSync = time.Now()
	return newProviders
}

// GetProvidersDueForCheck returns providers that need to be checked based on smart scheduling
func (c *ProviderCache) GetProvidersDueForCheck() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	var due []string

	for owner, p := range c.data.Providers {
		var interval time.Duration

		if p.IsOnline {
			interval = OnlineCheckInterval
		} else if p.ConsecutiveFailures >= LongTermOfflineThreshold {
			interval = LongTermOfflineInterval
		} else {
			interval = RecentOfflineInterval
		}

		if now.Sub(p.LastChecked) >= interval {
			due = append(due, owner)
		}
	}

	return due
}

// GetUncheckedProviders returns providers that have never been checked
func (c *ProviderCache) GetUncheckedProviders() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var unchecked []string
	for owner, p := range c.data.Providers {
		if p.LastChecked.IsZero() {
			unchecked = append(unchecked, owner)
		}
	}
	return unchecked
}

// GetProvidersByPriority returns providers sorted by check priority:
// unchecked (0) > online (1) > recently offline (2) > long-term offline (3)
func (c *ProviderCache) GetProvidersByPriority() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	type providerPriority struct {
		owner     string
		priority  int
		lastCheck time.Time
	}

	var providers []providerPriority
	for owner, p := range c.data.Providers {
		priority := c.calculatePriority(p)
		providers = append(providers, providerPriority{owner, priority, p.LastChecked})
	}

	sort.Slice(providers, func(i, j int) bool {
		if providers[i].priority != providers[j].priority {
			return providers[i].priority < providers[j].priority
		}
		return providers[i].lastCheck.Before(providers[j].lastCheck)
	})

	result := make([]string, len(providers))
	for i, p := range providers {
		result[i] = p.owner
	}
	return result
}

func (c *ProviderCache) calculatePriority(p *CachedProvider) int {
	if p.LastChecked.IsZero() {
		return 0
	}
	if p.IsOnline {
		return 1
	}
	if p.ConsecutiveFailures < LongTermOfflineThreshold {
		return 2
	}
	return 3
}

// ProviderCount returns the total number of providers in cache
func (c *ProviderCache) ProviderCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data.Providers)
}

// OnlineCount returns the number of online providers
func (c *ProviderCache) OnlineCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, p := range c.data.Providers {
		if p.IsOnline {
			count++
		}
	}
	return count
}

// extractHostname extracts the hostname from a URL
func extractHostname(hostURI string) string {
	host := strings.TrimPrefix(hostURI, "https://")
	host = strings.TrimPrefix(host, "http://")
	if idx := strings.IndexAny(host, ":/"); idx != -1 {
		host = host[:idx]
	}
	return host
}
