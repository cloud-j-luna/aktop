package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/cloud-j-luna/aktop/internal/cache"
	"github.com/cloud-j-luna/aktop/internal/rpc"
	"github.com/cloud-j-luna/aktop/internal/ui"
)

// Set by goreleaser via ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
	defaultRefreshRate = 1 * time.Second
	fastRefreshRate    = 250 * time.Millisecond
)

var (
	rpcEndpoint        string
	restEndpoint       string
	refreshRate        time.Duration
	fastMode           bool
	cleanCache         bool
	insecureSkipVerify bool
)

var rootCmd = &cobra.Command{
	Use:   "aktop [rpc-endpoint]",
	Short: "Akash Network Monitor",
	Long: `A terminal UI for monitoring Akash Network consensus and provider operations.

Displays real-time consensus data, validator voting status, and provider
upgrade status with CPU/memory stats.`,
	Example: `  aktop                                    # Use default Akash RPC
  aktop https://akash-rpc.polkachu.com     # Use Polkachu RPC
  aktop --fast                             # Fast polling mode (250ms)
  aktop --refresh 2s                       # Custom refresh interval
  aktop --clean-cache                      # Clear cache and start fresh`,
	Args:          cobra.MaximumNArgs(1),
	Version:       fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          run,
}

func init() {
	rootCmd.Flags().StringVar(&rpcEndpoint, "rpc", rpc.DefaultRPCEndpoint, "RPC endpoint URL")
	rootCmd.Flags().StringVar(&restEndpoint, "rest", rpc.DefaultRESTEndpoint, "REST endpoint URL")
	rootCmd.Flags().DurationVar(&refreshRate, "refresh", defaultRefreshRate, "Refresh interval")
	rootCmd.Flags().BoolVar(&fastMode, "fast", false, "Fast polling mode (250ms interval)")
	rootCmd.Flags().BoolVar(&cleanCache, "clean-cache", false, "Delete provider cache and start fresh")
	rootCmd.Flags().BoolVar(&insecureSkipVerify, "insecure", true, "Skip TLS certificate verification for providers")

	// Custom help template with keyboard controls
	rootCmd.SetHelpTemplate(`{{.Long}}

Usage:
  {{.UseLine}}

Examples:
{{.Example}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}

Tabs:
  1: Overview      Consensus state, vote progress, validator grid
  2: Validators    Detailed validator list with vote status
  3: Providers     Provider version distribution and list

Keyboard Controls:
  q, Ctrl+C        Quit
  r                Manual refresh
  Tab, 1, 2, 3     Switch between tabs
  j/k, Up/Down     Scroll lists
  h/l, Left/Right  Cycle through provider versions
  g, G             Jump to top/bottom of list

Cache:
  Provider data is cached in ~/.aktop/providers.json
`)
}

func run(cmd *cobra.Command, args []string) error {
	if fastMode {
		refreshRate = fastRefreshRate
	}

	if len(args) > 0 {
		rpcEndpoint = args[0]
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	cacheDir := filepath.Join(homeDir, ".aktop")

	if cleanCache {
		cachePath := filepath.Join(cacheDir, "providers.json")
		if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete cache: %w", err)
		}
		fmt.Println("Cache cleared")
	}

	providerCache, err := cache.LoadOrCreate(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	client := rpc.NewClient(rpcEndpoint, restEndpoint)
	rpcProviderClient := rpc.NewRPCProviderClient(rpcEndpoint)

	model := ui.NewModel(ui.ModelConfig{
		Client:             client,
		RPCClient:          rpcProviderClient,
		Cache:              providerCache,
		RefreshRate:        refreshRate,
		InsecureSkipVerify: insecureSkipVerify,
	})
	p := tea.NewProgram(model)

	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
