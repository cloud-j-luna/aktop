package rpc

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	providerv1beta4 "pkg.akt.dev/go/node/provider/v1beta4"
)

const (
	// ABCIQueryTimeout is the timeout for ABCI queries
	ABCIQueryTimeout = 30 * time.Second

	// ProvidersQueryPath is the ABCI query path for providers
	ProvidersQueryPath = "/akash.provider.v1beta4.Query/Providers"
)

// OnChainProvider represents a provider from the chain
type OnChainProvider struct {
	Owner      string
	HostURI    string
	Attributes map[string]string
	IsOnline   bool // Optional: hint from seed, verified by polling
}

// ABCIQueryResponse represents the response from an ABCI query
type ABCIQueryResponse struct {
	Result struct {
		Response struct {
			Code   int    `json:"code"`
			Log    string `json:"log"`
			Info   string `json:"info"`
			Value  string `json:"value"` // base64 encoded protobuf
			Height string `json:"height"`
		} `json:"response"`
	} `json:"result"`
}

// RPCProviderClient fetches providers using RPC ABCI queries
type RPCProviderClient struct {
	rpcEndpoint string
	httpClient  *http.Client
}

// NewRPCProviderClient creates a new RPC-based provider client
func NewRPCProviderClient(rpcEndpoint string) *RPCProviderClient {
	return &RPCProviderClient{
		rpcEndpoint: rpcEndpoint,
		httpClient: &http.Client{
			Timeout: ABCIQueryTimeout,
		},
	}
}

// Close is a no-op for HTTP-based client (for interface compatibility)
func (c *RPCProviderClient) Close() error {
	return nil
}

// GetProvidersOnChain fetches all providers from the chain via RPC ABCI query
func (c *RPCProviderClient) GetProvidersOnChain(ctx context.Context) ([]OnChainProvider, error) {
	var providers []OnChainProvider
	var nextKey []byte

	for {
		pageProviders, newNextKey, err := c.fetchProvidersPage(ctx, nextKey)
		if err != nil {
			return nil, err
		}

		providers = append(providers, pageProviders...)

		if len(newNextKey) == 0 {
			break
		}
		nextKey = newNextKey
	}

	return providers, nil
}

func (c *RPCProviderClient) fetchProvidersPage(ctx context.Context, nextKey []byte) ([]OnChainProvider, []byte, error) {
	queryURL := fmt.Sprintf("%s/abci_query?path=%s",
		c.rpcEndpoint,
		url.QueryEscape(fmt.Sprintf("%q", ProvidersQueryPath)),
	)

	if len(nextKey) > 0 {
		req := &providerv1beta4.QueryProvidersRequest{
			Pagination: &querytypes.PageRequest{
				Key:   nextKey,
				Limit: 100,
			},
		}
		reqData, err := req.Marshal()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal pagination request: %w", err)
		}
		queryURL += "&data=0x" + hex.EncodeToString(reqData)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query providers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("ABCI query failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	var abciResp ABCIQueryResponse
	if err := json.Unmarshal(body, &abciResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse ABCI response: %w", err)
	}

	if abciResp.Result.Response.Code != 0 {
		return nil, nil, fmt.Errorf("ABCI query error: code=%d log=%s",
			abciResp.Result.Response.Code, abciResp.Result.Response.Log)
	}

	valueBytes, err := base64.StdEncoding.DecodeString(abciResp.Result.Response.Value)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode base64 value: %w", err)
	}

	var providersResp providerv1beta4.QueryProvidersResponse
	if err := providersResp.Unmarshal(valueBytes); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal providers response: %w", err)
	}

	var providers []OnChainProvider
	for _, p := range providersResp.Providers {
		attrs := make(map[string]string)
		for _, attr := range p.Attributes {
			attrs[attr.Key] = attr.Value
		}

		providers = append(providers, OnChainProvider{
			Owner:      p.Owner,
			HostURI:    p.HostURI,
			Attributes: attrs,
		})
	}

	var newNextKey []byte
	if providersResp.Pagination != nil {
		newNextKey = providersResp.Pagination.NextKey
	}

	return providers, newNextKey, nil
}

// GRPCClient is kept for backward compatibility but now uses RPC
// Deprecated: Use RPCProviderClient directly
type GRPCClient struct {
	rpcClient *RPCProviderClient
}

// NewGRPCClient creates a new client (now uses RPC instead of gRPC)
func NewGRPCClient(endpoint string) (*GRPCClient, error) {
	// If endpoint looks like a gRPC endpoint (no protocol), convert to RPC
	if endpoint == "" || endpoint == DefaultGRPCEndpoint {
		endpoint = DefaultRPCEndpoint
	}

	return &GRPCClient{
		rpcClient: NewRPCProviderClient(endpoint),
	}, nil
}

// DefaultGRPCEndpoint is kept for backward compatibility
const DefaultGRPCEndpoint = "akash.lavenderfive.com:443"

// Close closes the client
func (c *GRPCClient) Close() error {
	return c.rpcClient.Close()
}

// GetProvidersOnChain fetches all providers from the chain
func (c *GRPCClient) GetProvidersOnChain(ctx context.Context) ([]OnChainProvider, error) {
	return c.rpcClient.GetProvidersOnChain(ctx)
}

// ActiveLeasesResponse represents the REST response for active leases
type ActiveLeasesResponse struct {
	Leases []struct {
		Lease struct {
			ID struct {
				Provider string `json:"provider"`
			} `json:"id"`
			State string `json:"state"`
		} `json:"lease"`
	} `json:"leases"`
	Pagination struct {
		NextKey string `json:"next_key"`
	} `json:"pagination"`
}

// GetActiveLeaseProviders returns a set of provider addresses that have active leases
func (c *RPCProviderClient) GetActiveLeaseProviders(ctx context.Context, restEndpoint string) (map[string]bool, error) {
	providers := make(map[string]bool)
	nextKey := ""

	for {
		pageProviders, newNextKey, err := c.fetchLeasesPage(ctx, restEndpoint, nextKey)
		if err != nil {
			return nil, err
		}

		for provider := range pageProviders {
			providers[provider] = true
		}

		if newNextKey == "" {
			break
		}
		nextKey = newNextKey
	}

	return providers, nil
}

func (c *RPCProviderClient) fetchLeasesPage(ctx context.Context, restEndpoint, nextKey string) (map[string]bool, string, error) {
	queryURL := fmt.Sprintf("%s/akash/market/v1beta5/leases/list?filters.state=active&pagination.limit=500",
		restEndpoint,
	)
	if nextKey != "" {
		queryURL += "&pagination.key=" + url.QueryEscape(nextKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query active leases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("leases query failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	var leasesResp ActiveLeasesResponse
	if err := json.Unmarshal(body, &leasesResp); err != nil {
		return nil, "", fmt.Errorf("failed to parse leases response: %w", err)
	}

	providers := make(map[string]bool)
	for _, l := range leasesResp.Leases {
		if l.Lease.ID.Provider != "" {
			providers[l.Lease.ID.Provider] = true
		}
	}

	return providers, leasesResp.Pagination.NextKey, nil
}

// SeedURL is the URL for the provider seed file used for fast bootstrapping
const SeedURL = "https://raw.githubusercontent.com/cloud-j-luna/aktop/main/data/providers-seed.json"

// SeedProviderInfo represents minimal provider info from the seed file
type SeedProviderInfo struct {
	Owner   string `json:"owner"`
	HostURI string `json:"hostUri"`
}

// GetProvidersFromSeed fetches providers from the seed file for fast bootstrapping
func (c *RPCProviderClient) GetProvidersFromSeed(ctx context.Context) ([]OnChainProvider, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SeedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from seed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("seed fetch returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read seed response: %w", err)
	}

	var seedProviders []SeedProviderInfo
	if err := json.Unmarshal(body, &seedProviders); err != nil {
		return nil, fmt.Errorf("failed to parse seed response: %w", err)
	}

	var providers []OnChainProvider
	for _, sp := range seedProviders {
		providers = append(providers, OnChainProvider{
			Owner:    sp.Owner,
			HostURI:  sp.HostURI,
			IsOnline: false,
		})
	}

	return providers, nil
}
