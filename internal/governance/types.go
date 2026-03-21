package governance

import (
	"encoding/json"
	"strings"
	"time"
)

// GenericParam represents a generic parameter from the params module
type GenericParam struct {
	Subspace string          `json:"subspace"`
	Key      string          `json:"key"`
	Value    json.RawMessage `json:"value"`
}

// ModuleParams holds parameters for a specific module as raw JSON
type ModuleParams struct {
	Module      string
	Source      string // "direct" for standard endpoints, "generic" for params module
	RawJSON     json.RawMessage
	LastFetched time.Time
	Error       error
}

// AllParams aggregates all module parameters
type AllParams struct {
	Modules     map[string]*ModuleParams // module name -> parameters
	LastUpdated time.Time
	Error       error
}

// NewAllParams creates a new AllParams instance
func NewAllParams() *AllParams {
	return &AllParams{
		Modules: make(map[string]*ModuleParams),
	}
}

// StandardModuleEndpoints defines the direct REST endpoints for standard modules
var StandardModuleEndpoints = map[string]string{
	"gov":          "/cosmos/gov/v1beta1/params/voting",
	"mint":         "/cosmos/mint/v1beta1/params",
	"staking":      "/cosmos/staking/v1beta1/params",
	"slashing":     "/cosmos/slashing/v1beta1/params",
	"distribution": "/cosmos/distribution/v1beta1/params",
	"auth":         "/cosmos/auth/v1beta1/params",
	"bank":         "/cosmos/bank/v1beta1/params",
}

// ModuleOrder defines the display order of modules
var ModuleOrder = []string{"gov", "mint", "staking", "slashing", "distribution", "auth", "bank", "deployment", "market", "transfer", "ibc", "crisis"}

// GenericModuleSubspaces defines the generic param subspaces to query
var GenericModuleSubspaces = []string{
	"deployment",
	"market",
	"transfer",
	"ibc",
	"crisis",
}

// PrettyModuleNames maps module names to display names
var PrettyModuleNames = map[string]string{
	"gov":          "Governance",
	"mint":         "Minting",
	"staking":      "Staking",
	"slashing":     "Slashing",
	"distribution": "Distribution",
	"auth":         "Auth",
	"bank":         "Bank",
	"deployment":   "Deployment",
	"market":       "Market",
	"transfer":     "Transfer",
	"ibc":          "IBC",
	"crisis":       "Crisis",
}

// GetModuleDisplayName returns the pretty name for a module
func GetModuleDisplayName(module string) string {
	if name, ok := PrettyModuleNames[module]; ok {
		return name
	}
	return module
}

// FormatJSON formats raw JSON for display
func FormatJSON(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "{}", nil
	}

	var data interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}

	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	return string(formatted), nil
}

// CountJSONLines returns the number of lines in formatted JSON
func CountJSONLines(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 1
	}
	formatted, err := FormatJSON(raw)
	if err != nil {
		return 1
	}
	lines := strings.Split(formatted, "\n")
	return len(lines)
}
