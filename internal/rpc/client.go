package rpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloud-j-luna/aktop/internal/consensus"
)

const (
	// DefaultRPCEndpoint is the default Akash Network RPC endpoint
	DefaultRPCEndpoint = "https://rpc.akt.dev/rpc"

	// DefaultRESTEndpoint is the default Akash Network REST endpoint
	DefaultRESTEndpoint = "https://rpc.akt.dev/rest"

	// DefaultTimeout for HTTP requests
	DefaultTimeout = 10 * time.Second
)

// Client is an RPC client for fetching consensus state
type Client struct {
	rpcEndpoint    string
	restEndpoint   string
	httpClient     *http.Client
	validators     []consensus.Validator // cached validators
	validatorsErr  error
	validatorsOnce sync.Once
}

// NewClient creates a new RPC client
func NewClient(rpcEndpoint, restEndpoint string) *Client {
	if rpcEndpoint == "" {
		rpcEndpoint = DefaultRPCEndpoint
	}
	if restEndpoint == "" {
		restEndpoint = DefaultRESTEndpoint
	}

	return &Client{
		rpcEndpoint:  rpcEndpoint,
		restEndpoint: restEndpoint,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// GetConsensusState fetches the current consensus state from the RPC endpoint
func (c *Client) GetConsensusState(ctx context.Context) (*consensus.ConsensusResponse, error) {
	reqURL := fmt.Sprintf("%s/consensus_state", c.rpcEndpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch consensus state: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result consensus.ConsensusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse consensus state: %w", err)
	}

	return &result, nil
}

// GetValidators fetches all validators from the RPC endpoint with pagination
// Results are cached for subsequent calls (thread-safe)
func (c *Client) GetValidators() ([]consensus.Validator, error) {
	c.validatorsOnce.Do(func() {
		c.validators, c.validatorsErr = c.fetchValidators()
	})
	return c.validators, c.validatorsErr
}

// fetchValidators does the actual fetch (uses background context since it's called from sync.Once)
func (c *Client) fetchValidators() ([]consensus.Validator, error) {
	ctx := context.Background()
	var allValidators []consensus.Validator
	page := 1
	perPage := 100

	for {
		validators, total, err := c.fetchValidatorsPage(ctx, page, perPage)
		if err != nil {
			return nil, err
		}

		allValidators = append(allValidators, validators...)

		if len(allValidators) >= total || len(validators) == 0 {
			break
		}

		page++
	}

	return allValidators, nil
}

func (c *Client) fetchValidatorsPage(ctx context.Context, page, perPage int) ([]consensus.Validator, int, error) {
	reqURL := fmt.Sprintf("%s/validators?per_page=%d&page=%d", c.rpcEndpoint, perPage, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch validators: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response body: %w", err)
	}

	var result consensus.ValidatorsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, 0, fmt.Errorf("failed to parse validators: %w", err)
	}

	total := 0
	if _, err := fmt.Sscanf(result.Result.Total, "%d", &total); err != nil {
		return nil, 0, fmt.Errorf("failed to parse total validators: %w", err)
	}

	return result.Result.Validators, total, nil
}

// GetConsensusStateWithValidators fetches consensus state and parses it with cached validators
func (c *Client) GetConsensusStateWithValidators(ctx context.Context) (*consensus.State, error) {
	// Ensure validators are loaded
	validators, err := c.GetValidators()
	if err != nil {
		return nil, err
	}

	resp, err := c.GetConsensusState(ctx)
	if err != nil {
		return nil, err
	}

	return consensus.ParseConsensusState(resp, validators)
}

// Endpoint returns the current RPC endpoint
func (c *Client) Endpoint() string {
	return c.rpcEndpoint
}

// RESTEndpoint returns the current REST endpoint
func (c *Client) RESTEndpoint() string {
	return c.restEndpoint
}

// LCDValidatorsResponse represents the LCD API response for validators
type LCDValidatorsResponse struct {
	Validators []struct {
		Description struct {
			Moniker string `json:"moniker"`
		} `json:"description"`
		ConsensusPubkey struct {
			Type string `json:"@type"`
			Key  string `json:"key"`
		} `json:"consensus_pubkey"`
	} `json:"validators"`
	Pagination struct {
		NextKey string `json:"next_key"`
		Total   string `json:"total"`
	} `json:"pagination"`
}

// GetValidatorMonikers fetches validator monikers from the REST endpoint
// Returns a map of consensus pubkey (base64) -> moniker
func (c *Client) GetValidatorMonikers(ctx context.Context) (map[string]string, error) {
	monikers := make(map[string]string)
	nextKey := ""

	for {
		pageMonikers, newNextKey, err := c.fetchMonikersPage(ctx, nextKey)
		if err != nil {
			return nil, err
		}

		for k, v := range pageMonikers {
			monikers[k] = v
		}

		if newNextKey == "" {
			break
		}
		nextKey = newNextKey
	}

	return monikers, nil
}

func (c *Client) fetchMonikersPage(ctx context.Context, nextKey string) (map[string]string, string, error) {
	reqURL := fmt.Sprintf("%s/cosmos/staking/v1beta1/validators?pagination.limit=100", c.restEndpoint)
	if nextKey != "" {
		reqURL += "&pagination.key=" + nextKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch validators from LCD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("LCD returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read LCD response: %w", err)
	}

	var result LCDValidatorsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse LCD validators: %w", err)
	}

	monikers := make(map[string]string)
	for _, v := range result.Validators {
		if v.ConsensusPubkey.Key != "" && v.Description.Moniker != "" {
			monikers[v.ConsensusPubkey.Key] = v.Description.Moniker
		}
	}

	return monikers, result.Pagination.NextKey, nil
}

// Provider represents an Akash provider with version info
type Provider struct {
	Owner        string
	HostURI      string
	Name         string
	AkashVersion string
	IsOnline     bool
	Country      string
	CPUAvailable uint64
	CPUTotal     uint64
	MemAvailable uint64
	MemTotal     uint64
	GPUAvailable uint64
	GPUTotal     uint64
	GPUModels    []string // unique GPU model names (e.g., "NVIDIA H100")
}

// ResourceStats represents CPU/memory/storage stats
type ResourceStats struct {
	Available uint64 `json:"available"`
	Total     uint64 `json:"total"`
}

// ProviderStatusResponse represents the response from provider's /status endpoint
type ProviderStatusResponse struct {
	Cluster struct {
		Inventory struct {
			Available struct {
				Nodes []struct {
					Name        string `json:"name"`
					Allocatable struct {
						CPU    uint64 `json:"cpu"`
						GPU    uint64 `json:"gpu"`
						Memory uint64 `json:"memory"`
					} `json:"allocatable"`
					Available struct {
						CPU    uint64 `json:"cpu"`
						GPU    uint64 `json:"gpu"`
						Memory uint64 `json:"memory"`
					} `json:"available"`
				} `json:"nodes"`
			} `json:"available"`
		} `json:"inventory"`
	} `json:"cluster"`
}

// ProviderNode represents a single node's resource information
type ProviderNode struct {
	Name           string
	CPUAllocatable uint64
	CPUAvailable   uint64
	MemAllocatable uint64
	MemAvailable   uint64
	GPUAllocatable uint64
	GPUAvailable   uint64
}

// GetNodes extracts node information from the status response
func (r *ProviderStatusResponse) GetNodes() []ProviderNode {
	rawNodes := r.Cluster.Inventory.Available.Nodes
	nodes := make([]ProviderNode, 0, len(rawNodes))
	for _, n := range rawNodes {
		nodes = append(nodes, ProviderNode{
			Name:           n.Name,
			CPUAllocatable: n.Allocatable.CPU,
			CPUAvailable:   n.Available.CPU,
			MemAllocatable: n.Allocatable.Memory,
			MemAvailable:   n.Available.Memory,
			GPUAllocatable: n.Allocatable.GPU,
			GPUAvailable:   n.Available.GPU,
		})
	}
	return nodes
}

// ProviderVersionResponse represents the response from provider's /version endpoint
type ProviderVersionResponse struct {
	Akash struct {
		Version string `json:"version"`
	} `json:"akash"`
}

// CompareVersions compares two semver-like version strings
// Returns: 1 if a > b, -1 if a < b, 0 if equal
func CompareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}

	for i := 0; i < maxLen; i++ {
		var numA, numB int
		if i < len(partsA) {
			// Remove any non-numeric suffix (e.g., "6-rc3" -> "6")
			numStr := strings.Split(partsA[i], "-")[0]
			fmt.Sscanf(numStr, "%d", &numA)
		}
		if i < len(partsB) {
			numStr := strings.Split(partsB[i], "-")[0]
			fmt.Sscanf(numStr, "%d", &numB)
		}

		if numA > numB {
			return 1
		}
		if numA < numB {
			return -1
		}
	}

	// If base versions are equal, non-RC is higher than RC
	if strings.Contains(a, "-") && !strings.Contains(b, "-") {
		return -1
	}
	if !strings.Contains(a, "-") && strings.Contains(b, "-") {
		return 1
	}

	return 0
}

// GetProviderVersions returns unique versions from providers, sorted latest first
func GetProviderVersions(providers []Provider) []string {
	versionSet := make(map[string]bool)
	for _, p := range providers {
		if p.AkashVersion != "" {
			versionSet[p.AkashVersion] = true
		}
	}

	versions := make([]string, 0, len(versionSet))
	for v := range versionSet {
		versions = append(versions, v)
	}

	sort.Slice(versions, func(i, j int) bool {
		return CompareVersions(versions[i], versions[j]) > 0
	})

	return versions
}

const ProviderQueryTimeout = 5 * time.Second

// NewProviderHTTPClient creates an HTTP client configured for querying providers.
// If insecureSkipVerify is true, TLS certificate verification is disabled.
// This is often needed for providers with self-signed certificates.
func NewProviderHTTPClient(insecureSkipVerify bool) *http.Client {
	return &http.Client{
		Timeout: ProviderQueryTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureSkipVerify,
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// QueryProviderStatus queries a provider's /status endpoint
func QueryProviderStatus(ctx context.Context, httpClient *http.Client, hostURI string) (*ProviderStatusResponse, error) {
	reqURL := strings.TrimSuffix(hostURI, "/") + "/status"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result ProviderStatusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// QueryProviderVersion queries a provider's /version endpoint
func QueryProviderVersion(ctx context.Context, httpClient *http.Client, hostURI string) (*ProviderVersionResponse, error) {
	reqURL := strings.TrimSuffix(hostURI, "/") + "/version"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result ProviderVersionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ExtractHostname extracts the hostname from a URL
func ExtractHostname(hostURI string) string {
	// Remove protocol
	host := strings.TrimPrefix(hostURI, "https://")
	host = strings.TrimPrefix(host, "http://")

	// Remove port
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	return host
}
