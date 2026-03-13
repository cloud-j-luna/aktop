package ui

import (
	"time"

	"github.com/cloud-j-luna/aktop/internal/rpc"
)

// ProviderList holds the state for the provider list view.
type ProviderList struct {
	Items       []rpc.Provider
	Versions    []string // unique versions, sorted latest first
	Version     string   // currently selected version filter
	VersionIdx  int      // index in Versions
	ScrollPos   int      // scroll position for provider list
	SelectedIdx int      // currently highlighted provider in list
}

// ProviderLoader holds the state for background provider loading/checking.
type ProviderLoader struct {
	FirstRun      bool
	Loading       bool
	Total         int
	Checked       int
	ActiveLease   map[string]bool // providers with active leases (priority)
	Queue         []string        // providers to check
	InFlight      map[string]bool // providers currently being checked
	LastSync      time.Time
	LastSave      time.Time
	LastSaveError error
}

// ProviderDetail holds the state for the provider detail view.
type ProviderDetail struct {
	Showing   bool
	Provider  *rpc.Provider
	Nodes     []rpc.ProviderNodeWithGPU
	Loading   bool
	Error     error
	ScrollPos int
}
