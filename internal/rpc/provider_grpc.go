package rpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/emptypb"

	inventoryv1 "pkg.akt.dev/go/inventory/v1"
	providerv1 "pkg.akt.dev/go/provider/v1"
)

const (
	// GRPCProviderPort is the default gRPC port for Akash providers
	GRPCProviderPort = "8444"

	// GRPCQueryTimeout is the timeout for gRPC queries
	GRPCQueryTimeout = 15 * time.Second
)

// GPUInfo represents GPU model information for a node.
type GPUInfo struct {
	Vendor     string
	VendorID   string
	Name       string
	ModelID    string
	Interface  string
	MemorySize string
}

// ProviderNodeWithGPU represents a node with full GPU information.
type ProviderNodeWithGPU struct {
	Name           string
	CPUAllocatable uint64
	CPUAvailable   uint64
	MemAllocatable uint64
	MemAvailable   uint64
	GPUAllocatable uint64
	GPUAvailable   uint64
	GPUs           []GPUInfo
}

// QueryProviderStatusGRPC queries a provider's status via gRPC to get full GPU information.
// The hostURI should be the provider's base URI (e.g., "https://provider.example.com:8443").
// This function will convert it to the gRPC endpoint (port 8444).
func QueryProviderStatusGRPC(ctx context.Context, hostURI string) ([]ProviderNodeWithGPU, error) {
	grpcHost, err := convertToGRPCEndpoint(hostURI)
	if err != nil {
		return nil, fmt.Errorf("invalid host URI: %w", err)
	}

	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	ctx, cancel := context.WithTimeout(ctx, GRPCQueryTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, grpcHost, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to provider: %w", err)
	}
	defer conn.Close()

	client := providerv1.NewProviderRPCClient(conn)

	resp, err := client.GetStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get provider status: %w", err)
	}

	return extractNodesWithGPU(resp), nil
}

// convertToGRPCEndpoint converts a provider's REST URI to its gRPC endpoint.
// e.g., "https://provider.example.com:8443" -> "provider.example.com:8444"
func convertToGRPCEndpoint(hostURI string) (string, error) {
	parsed, err := url.Parse(hostURI)
	if err != nil {
		return "", err
	}

	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("no hostname in URI: %s", hostURI)
	}

	return fmt.Sprintf("%s:%s", host, GRPCProviderPort), nil
}

// extractNodesWithGPU extracts node information including GPU details from provider status.
func extractNodesWithGPU(status *providerv1.Status) []ProviderNodeWithGPU {
	if status == nil || status.Cluster == nil {
		return nil
	}

	inventory := status.Cluster.Inventory
	cluster := inventory.GetCluster()

	nodes := make([]ProviderNodeWithGPU, 0, len(cluster.Nodes))

	for _, node := range cluster.Nodes {
		n := ProviderNodeWithGPU{
			Name: node.Name,
		}

		res := node.Resources

		// CPU - use MilliValue() since Kubernetes stores CPU in millicores
		if res.CPU.Quantity.Allocatable != nil {
			n.CPUAllocatable = uint64(res.CPU.Quantity.Allocatable.MilliValue())
			n.CPUAvailable = n.CPUAllocatable // default: all available
		}
		if res.CPU.Quantity.Allocated != nil && n.CPUAllocatable > 0 {
			allocated := uint64(res.CPU.Quantity.Allocated.MilliValue())
			if allocated <= n.CPUAllocatable {
				n.CPUAvailable = n.CPUAllocatable - allocated
			}
		}

		// Memory - set allocatable first, then calculate available
		if res.Memory.Quantity.Allocatable != nil {
			n.MemAllocatable = uint64(res.Memory.Quantity.Allocatable.Value())
			n.MemAvailable = n.MemAllocatable // default: all available
		}
		if res.Memory.Quantity.Allocated != nil && n.MemAllocatable > 0 {
			allocated := uint64(res.Memory.Quantity.Allocated.Value())
			if allocated <= n.MemAllocatable {
				n.MemAvailable = n.MemAllocatable - allocated
			}
		}

		// GPU - set allocatable first, then calculate available
		if res.GPU.Quantity.Allocatable != nil {
			n.GPUAllocatable = uint64(res.GPU.Quantity.Allocatable.Value())
			n.GPUAvailable = n.GPUAllocatable // default: all available
		}
		if res.GPU.Quantity.Allocated != nil && n.GPUAllocatable > 0 {
			allocated := uint64(res.GPU.Quantity.Allocated.Value())
			if allocated <= n.GPUAllocatable {
				n.GPUAvailable = n.GPUAllocatable - allocated
			}
		}

		// GPU Info (models)
		n.GPUs = extractGPUInfo(res.GPU.Info)

		nodes = append(nodes, n)
	}

	return nodes
}

// extractGPUInfo converts inventory GPU info to our GPUInfo type.
func extractGPUInfo(info inventoryv1.GPUInfoS) []GPUInfo {
	if len(info) == 0 {
		return nil
	}

	gpus := make([]GPUInfo, 0, len(info))
	for _, g := range info {
		gpus = append(gpus, GPUInfo{
			Vendor:     g.Vendor,
			VendorID:   g.VendorID,
			Name:       g.Name,
			ModelID:    g.ModelID,
			Interface:  g.Interface,
			MemorySize: g.MemorySize,
		})
	}

	return gpus
}
