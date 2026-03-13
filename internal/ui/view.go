package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloud-j-luna/aktop/internal/consensus"
	"github.com/cloud-j-luna/aktop/internal/rpc"
)

// Column width constants for provider list
const (
	colWidthIndex    = 4
	colWidthProvider = 28
	colWidthVersion  = 10
	colWidthCPU      = 12
	colWidthMem      = 12
	colWidthGPU      = 18
	colWidthCountry  = 4
	colWidthNodeName = 20
)

// Layout constants
const (
	providerListOverhead = 25 // header, tabs, status bar, scroll indicator, etc.
	nodeListOverhead     = 14 // header for detail view with node list
	minVisibleNodes      = 3  // minimum visible rows for node list
	minVisibleProviders  = 5  // minimum visible rows for provider list
)

// Address display constants
const (
	addrPrefixLen = 8 // characters to show at start of truncated address
	addrSuffixLen = 4 // characters to show at end of truncated address
)

// ProviderDetailState holds the state for provider detail view
type ProviderDetailState struct {
	Showing     bool
	Provider    *rpc.Provider
	Nodes       []rpc.ProviderNodeWithGPU
	Loading     bool
	Error       error
	ScrollPos   int
	SelectedIdx int
}

// ProviderViewState holds the state for the providers tab
type ProviderViewState struct {
	Providers []rpc.Provider
	Versions  []string
	Selected  string
	ScrollPos int
	Loading   bool
	Loaded    int
	Total     int
	Detail    ProviderDetailState
}

// ViewContext holds all data needed to render the view
type ViewContext struct {
	State     *consensus.State
	Endpoint  string
	Width     int
	Height    int
	ActiveTab Tab
	Monikers  map[string]string
	ScrollPos int
	Providers ProviderViewState
}

// RenderView renders the complete view
func RenderView(ctx ViewContext) string {
	var b strings.Builder

	// Title and tabs
	title := titleStyle.Render("aktop - Akash Network Monitor")
	b.WriteString(title)
	b.WriteString("\n")

	// Tab bar
	b.WriteString(renderTabBar(ctx.ActiveTab))
	b.WriteString("\n\n")

	// Error state
	if ctx.State == nil {
		b.WriteString(errorStyle.Render("Connecting to RPC endpoint..."))
		b.WriteString("\n")
		b.WriteString(renderStatusBar(ctx.Endpoint, ctx.ActiveTab, ctx.Providers.Detail.Showing))
		return b.String()
	}

	if ctx.State.Error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", ctx.State.Error)))
		b.WriteString("\n\n")
		// Still show last known state if available
		if ctx.State.Height == 0 {
			b.WriteString(renderStatusBar(ctx.Endpoint, ctx.ActiveTab, ctx.Providers.Detail.Showing))
			return b.String()
		}
	}

	// Render based on active tab
	switch ctx.ActiveTab {
	case TabOverview:
		b.WriteString(renderOverviewTab(ctx.State, ctx.Width))
	case TabValidators:
		b.WriteString(renderValidatorsTab(ctx.State, ctx.Monikers, ctx.Height, ctx.ScrollPos))
	case TabProviders:
		if ctx.Providers.Detail.Showing {
			b.WriteString(renderProviderDetailView(ctx.Providers.Detail, ctx.Height))
		} else {
			b.WriteString(renderProvidersTab(ctx.Providers, ctx.Height))
		}
	}

	b.WriteString("\n")

	// Help & status
	b.WriteString(renderStatusBar(ctx.Endpoint, ctx.ActiveTab, ctx.Providers.Detail.Showing))

	return b.String()
}

// renderTabBar renders the tab navigation bar
func renderTabBar(activeTab Tab) string {
	tab1 := " 1: Overview "
	tab2 := " 2: Validators "
	tab3 := " 3: Providers "

	switch activeTab {
	case TabOverview:
		tab1 = tabActiveStyle.Render(tab1)
		tab2 = tabInactiveStyle.Render(tab2)
		tab3 = tabInactiveStyle.Render(tab3)
	case TabValidators:
		tab1 = tabInactiveStyle.Render(tab1)
		tab2 = tabActiveStyle.Render(tab2)
		tab3 = tabInactiveStyle.Render(tab3)
	case TabProviders:
		tab1 = tabInactiveStyle.Render(tab1)
		tab2 = tabInactiveStyle.Render(tab2)
		tab3 = tabActiveStyle.Render(tab3)
	}

	return tab1 + " " + tab2 + " " + tab3
}

func renderOverviewTab(state *consensus.State, width int) string {
	var b strings.Builder
	b.WriteString(renderConsensusSection(state))
	b.WriteString("\n\n")
	b.WriteString(renderVoteSection(state))
	b.WriteString("\n\n")
	b.WriteString(renderGridSection(state, width))
	return b.String()
}

func renderValidatorsTab(state *consensus.State, monikers map[string]string, termHeight, scrollPos int) string {
	var b strings.Builder
	b.WriteString(renderConsensusSection(state))
	b.WriteString("\n\n")
	b.WriteString(renderVoteSection(state))
	b.WriteString("\n\n")
	b.WriteString(renderValidatorList(state, monikers, termHeight, scrollPos))
	return b.String()
}

func renderConsensusSection(state *consensus.State) string {
	header := headerStyle.Render("Consensus State")

	elapsed := state.Elapsed
	if elapsed < 0 {
		elapsed = 0
	}

	proposerAddr := truncateAddress(state.ProposerAddress, 12)

	content := fmt.Sprintf(
		"%s %s    %s %d    %s %s\n%s %s    %s %s (index: %d)",
		labelStyle.Render("Height:"),
		valueStyle.Render(formatNumber(state.Height)),
		labelStyle.Render("Round:"),
		state.Round,
		labelStyle.Render("Step:"),
		valueStyle.Render(state.Step),
		labelStyle.Render("Elapsed:"),
		valueStyle.Render(formatDuration(elapsed)),
		labelStyle.Render("Proposer:"),
		valueStyle.Render(proposerAddr),
		state.ProposerIndex,
	)

	return header + "\n" + content
}

func truncateAddress(addr string, maxLen int) string {
	if len(addr) <= maxLen {
		return addr
	}
	return addr[:addrPrefixLen] + "..." + addr[len(addr)-addrSuffixLen:]
}

func renderVoteSection(state *consensus.State) string {
	header := headerStyle.Render("Vote Progress")
	prevoteLine := renderVoteLine("Prevotes:", state.PrevotePercent, state.PrevotePower, state.TotalVotingPower)
	precommitLine := renderVoteLine("Precommits:", state.PrecommitPercent, state.PrecommitPower, state.TotalVotingPower)
	return header + "\n" + prevoteLine + "\n" + precommitLine
}

func renderVoteLine(label string, percent float64, power, totalPower int64) string {
	bar := ProgressBar(percent, progressBarWidth)
	pct := FormatPercent(percent)
	powerStr := fmt.Sprintf("(%s / %s)", formatPower(power), formatPower(totalPower))
	return fmt.Sprintf("%s %s %s %s", labelStyle.Render(label), bar, pct, mutedStyle.Render(powerStr))
}

func renderGridSection(state *consensus.State, termWidth int) string {
	header := headerStyle.Render(fmt.Sprintf("Validator Grid (%d validators)", state.TotalValidators))

	gridWidth := clamp(termWidth-10, 20, 100)
	grid := FormatVoteGrid(state.PrevoteBitArray, gridWidth)

	legend := fmt.Sprintf("%s voted  %s not voted",
		gridVotedStyle.Render("●"),
		gridNotVotedStyle.Render("○"))

	return header + "\n" + grid + "\n\n" + mutedStyle.Render(legend)
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func renderValidatorList(state *consensus.State, monikers map[string]string, termHeight, scrollPos int) string {
	if len(state.Validators) == 0 {
		return mutedStyle.Render("Loading validators...")
	}

	visibleRows := max(termHeight-18, 5)
	header := headerStyle.Render(fmt.Sprintf("Validators (%d)", len(state.Validators)))

	colHeader := fmt.Sprintf("  %s  %s  %s  %s  %s",
		mutedStyle.Render(fmt.Sprintf("%-4s", "#")),
		mutedStyle.Render(fmt.Sprintf("%-24s", "Validator")),
		mutedStyle.Render(fmt.Sprintf("%10s", "Power")),
		mutedStyle.Render(fmt.Sprintf("%-8s", "Prevote")),
		mutedStyle.Render(fmt.Sprintf("%-10s", "Precommit")))

	var lines []string
	lines = append(lines, colHeader)

	startIdx, endIdx := scrollRange(scrollPos, visibleRows, len(state.Validators))

	for i := startIdx; i < endIdx; i++ {
		lines = append(lines, renderValidatorRow(state.Validators[i], monikers))
	}

	if len(state.Validators) > visibleRows {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf(
			"Showing %d-%d of %d (↑/↓ or j/k to scroll)", startIdx+1, endIdx, len(state.Validators))))
	}

	return header + "\n" + strings.Join(lines, "\n")
}

func renderValidatorRow(v consensus.ValidatorStatus, monikers map[string]string) string {
	displayName := getValidatorDisplayName(v, monikers)

	prevoteStatus := voteIndicator(v.Prevoted, "  ")
	precommitStatus := voteIndicator(v.Precommited, "    ")

	proposerMark := "  "
	if v.IsProposer {
		proposerMark = proposerStyle.Render("★ ")
	}

	return fmt.Sprintf("%s%s  %s  %10s  %s     %s",
		proposerMark,
		mutedStyle.Render(fmt.Sprintf("%-4d", v.Index)),
		monikerStyle.Render(fmt.Sprintf("%-24s", displayName)),
		mutedStyle.Render(formatPower(v.VotingPower)),
		prevoteStatus,
		precommitStatus)
}

func getValidatorDisplayName(v consensus.ValidatorStatus, monikers map[string]string) string {
	displayName := ""
	if monikers != nil && v.PubKey != "" {
		displayName = monikers[v.PubKey]
	}
	if displayName == "" {
		displayName = truncateAddress(v.Address, 12)
	}
	if len(displayName) > 20 {
		displayName = displayName[:17] + "..."
	}
	return displayName
}

func voteIndicator(voted bool, prefix string) string {
	if voted {
		return gridVotedStyle.Render(prefix + "✓")
	}
	return gridNotVotedStyle.Render(prefix + "○")
}

func scrollRange(scrollPos, visibleRows, totalItems int) (start, end int) {
	start = scrollPos
	end = scrollPos + visibleRows
	if end > totalItems {
		end = totalItems
	}
	return
}

func renderProvidersTab(pv ProviderViewState, termHeight int) string {
	var b strings.Builder

	if pv.Loading && pv.Total > 0 {
		progress := fmt.Sprintf("Scanning providers... %d/%d checked, %d online", pv.Loaded, pv.Total, len(pv.Providers))
		b.WriteString(ProgressBar(float64(pv.Loaded)/float64(pv.Total), 40))
		b.WriteString(" ")
		b.WriteString(mutedStyle.Render(progress))
		b.WriteString("\n\n")
	}

	b.WriteString(renderVersionDistribution(pv.Providers, pv.Versions, pv.Selected))
	b.WriteString("\n\n")
	b.WriteString(renderProviderList(pv.Providers, pv.Selected, termHeight, pv.ScrollPos, pv.Detail.SelectedIdx))

	return b.String()
}

func renderVersionDistribution(providers []rpc.Provider, providerVersions []string, selectedVersion string) string {
	header := headerStyle.Render("Provider Version Distribution")

	if len(providers) == 0 {
		return header + "\n" + mutedStyle.Render("Loading providers...")
	}

	filtered := filterNonLocalProviders(providers)
	if len(filtered) == 0 {
		return header + "\n" + mutedStyle.Render("No providers found")
	}

	versionCounts := countByVersion(filtered)

	var lines []string
	for _, version := range providerVersions {
		lines = append(lines, renderVersionLine(version, versionCounts[version], len(filtered), selectedVersion))
	}

	help := mutedStyle.Render("← / → or h/l: select version")
	return header + "\n" + strings.Join(lines, "\n") + "\n\n" + help
}

func countByVersion(providers []rpc.Provider) map[string]int {
	counts := make(map[string]int)
	for _, p := range providers {
		counts[p.AkashVersion]++
	}
	return counts
}

func renderVersionLine(version string, count, total int, selectedVersion string) string {
	percentage := float64(count) / float64(total) * 100
	numDots := min(count, 50)

	var dots string
	marker := "  "
	if version == selectedVersion {
		dots = gridVotedStyle.Render(repeatChar('●', numDots))
		marker = proposerStyle.Render("► ")
	} else {
		dots = mutedStyle.Render(repeatChar('○', numDots))
	}

	return fmt.Sprintf("%s%-12s %s %3d (%5.1f%%)", marker, version, dots, count, percentage)
}

func isLocalhost(hostURI string) bool {
	return strings.Contains(hostURI, "localhost") || strings.Contains(hostURI, "127.0.0.1")
}

func filterNonLocalProviders(providers []rpc.Provider) []rpc.Provider {
	var filtered []rpc.Provider
	for _, p := range providers {
		if !isLocalhost(p.HostURI) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func renderProviderList(providers []rpc.Provider, selectedVersion string, termHeight, scrollPos, selectedIdx int) string {
	filtered := filterNonLocalProviders(providers)
	if len(filtered) == 0 {
		return mutedStyle.Render("No providers found")
	}

	visibleRows := max(termHeight-providerListOverhead, 5)
	if len(filtered) > visibleRows {
		visibleRows -= 2
	}

	matchCount := countVersionMatches(filtered, selectedVersion)
	header := headerStyle.Render(fmt.Sprintf("Providers (%d total, %d on %s)", len(filtered), matchCount, selectedVersion))

	colHeader := fmt.Sprintf("  %s  %s  %s  %s  %s  %s  %s",
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthIndex, "#")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthProvider, "Provider")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthVersion, "Version")),
		mutedStyle.Render(fmt.Sprintf("%*s", colWidthCPU, "CPU")),
		mutedStyle.Render(fmt.Sprintf("%*s", colWidthMem, "Memory")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthGPU, "GPU")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthCountry, "Loc")))

	var lines []string
	lines = append(lines, colHeader)

	startIdx, endIdx := scrollRange(scrollPos, visibleRows, len(filtered))

	for i := startIdx; i < endIdx; i++ {
		isRowSelected := i == selectedIdx
		lines = append(lines, renderProviderRow(filtered[i], i+1, selectedVersion, isRowSelected))
	}

	if len(filtered) > visibleRows {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf(
			"Showing %d-%d of %d (↑/↓ or j/k to scroll, Enter for details)", startIdx+1, endIdx, len(filtered))))
	}

	return header + "\n" + strings.Join(lines, "\n")
}

func countVersionMatches(providers []rpc.Provider, version string) int {
	count := 0
	for _, p := range providers {
		if p.AkashVersion == version {
			count++
		}
	}
	return count
}

func renderProviderRow(p rpc.Provider, index int, selectedVersion string, isRowSelected bool) string {
	displayURL := formatProviderURL(p.HostURI, colWidthProvider-2)

	isVersionMatch := p.AkashVersion == selectedVersion
	versionDisplay := formatVersionDisplay(p.AkashVersion, isVersionMatch)
	marker := versionMarker(isVersionMatch)

	country := p.Country
	if country == "" {
		country = "--"
	}

	cpuStr := formatResourceRatio(p.CPUAvailable/1000, p.CPUTotal/1000)
	memStr := formatMemoryRatio(p.MemAvailable, p.MemTotal)
	gpuStr := formatProviderGPU(p)

	// Selection cursor
	cursor := "  "
	if isRowSelected {
		cursor = proposerStyle.Render("> ")
	}

	indexStr := fmt.Sprintf("%-*d", colWidthIndex, index)
	urlStr := fmt.Sprintf("%-*s", colWidthProvider, displayURL)
	cpuFmt := fmt.Sprintf("%*s", colWidthCPU, cpuStr)
	memFmt := fmt.Sprintf("%*s", colWidthMem, memStr)

	if isRowSelected {
		// Highlight the entire row
		return fmt.Sprintf("%s%s%s  %s  %s  %s  %s  %s  %s",
			cursor,
			marker,
			highlightStyle.Render(indexStr),
			highlightStyle.Render(urlStr),
			versionDisplay,
			highlightStyle.Render(cpuFmt),
			highlightStyle.Render(memFmt),
			formatProviderGPUStyled(gpuStr, true),
			highlightStyle.Render(country))
	}

	return fmt.Sprintf("%s%s%s  %s  %s  %s  %s  %s  %s",
		cursor,
		marker,
		mutedStyle.Render(indexStr),
		monikerStyle.Render(urlStr),
		versionDisplay,
		mutedStyle.Render(cpuFmt),
		mutedStyle.Render(memFmt),
		formatProviderGPUStyled(gpuStr, false),
		mutedStyle.Render(country))
}

// formatProviderGPU formats GPU info for the provider list.
func formatProviderGPU(p rpc.Provider) string {
	if p.GPUTotal == 0 {
		return "-"
	}

	countStr := fmt.Sprintf("%d/%d", p.GPUAvailable, p.GPUTotal)

	// Add first model name if available
	if len(p.GPUModels) > 0 {
		model := p.GPUModels[0]
		// Truncate model name if needed
		maxModelLen := colWidthGPU - len(countStr) - 2
		if len(model) > maxModelLen && maxModelLen > 3 {
			model = model[:maxModelLen-2] + ".."
		}
		return fmt.Sprintf("%s %s", countStr, model)
	}

	return countStr
}

// formatProviderGPUStyled applies styling to GPU display in provider list.
func formatProviderGPUStyled(gpuStr string, isSelected bool) string {
	formatted := fmt.Sprintf("%-*s", colWidthGPU, gpuStr)
	if isSelected {
		return highlightStyle.Render(formatted)
	}
	return mutedStyle.Render(formatted)
}

func formatProviderURL(hostURI string, maxLen int) string {
	url := strings.TrimPrefix(hostURI, "https://")
	url = strings.TrimPrefix(url, "http://")
	if idx := strings.LastIndex(url, ":"); idx > 0 {
		url = url[:idx]
	}
	if len(url) > maxLen {
		url = url[:maxLen-3] + "..."
	}
	return url
}

func formatVersionDisplay(version string, isSelected bool) string {
	formatted := fmt.Sprintf("%-*s", colWidthVersion, version)
	if isSelected {
		return gridVotedStyle.Render(formatted)
	}
	return mutedStyle.Render(formatted)
}

func versionMarker(isSelected bool) string {
	if isSelected {
		return gridVotedStyle.Render("● ")
	}
	return gridNotVotedStyle.Render("○ ")
}

// renderProviderDetailView renders the provider detail view with node list
func renderProviderDetailView(state ProviderDetailState, termHeight int) string {
	var b strings.Builder

	if state.Provider == nil {
		return errorStyle.Render("No provider selected")
	}

	p := state.Provider

	// Header
	b.WriteString(detailHeaderStyle.Render("Provider Details"))
	b.WriteString("\n\n")

	// Provider info
	displayURL := formatProviderURL(p.HostURI, 50)
	b.WriteString(fmt.Sprintf("%s %s\n", detailLabelStyle.Render("Name:"), detailValueStyle.Render(p.Name)))
	b.WriteString(fmt.Sprintf("%s %s\n", detailLabelStyle.Render("URL:"), detailValueStyle.Render(displayURL)))
	b.WriteString(fmt.Sprintf("%s %s\n", detailLabelStyle.Render("Version:"), gridVotedStyle.Render(p.AkashVersion)))

	country := p.Country
	if country == "" {
		country = "--"
	}
	b.WriteString(fmt.Sprintf("%s %s\n", detailLabelStyle.Render("Location:"), detailValueStyle.Render(country)))

	// Total resources
	cpuStr := formatResourceRatio(p.CPUAvailable/1000, p.CPUTotal/1000)
	memStr := formatMemoryRatio(p.MemAvailable, p.MemTotal)
	b.WriteString(fmt.Sprintf("%s CPU %s | Memory %s\n", detailLabelStyle.Render("Total:"), detailValueStyle.Render(cpuStr), detailValueStyle.Render(memStr)))
	b.WriteString("\n")

	// Loading state
	if state.Loading {
		b.WriteString(mutedStyle.Render("Fetching node details via gRPC..."))
		return b.String()
	}

	// Error state
	if state.Error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", state.Error)))
		return b.String()
	}

	// Node list
	if len(state.Nodes) == 0 {
		b.WriteString(mutedStyle.Render("No node information available"))
		return b.String()
	}

	// Count total GPUs across all nodes
	totalGPUAvail, totalGPUTotal := uint64(0), uint64(0)
	for _, node := range state.Nodes {
		totalGPUAvail += node.GPUAvailable
		totalGPUTotal += node.GPUAllocatable
	}

	nodeHeaderText := fmt.Sprintf("Nodes (%d total)", len(state.Nodes))
	if totalGPUTotal > 0 {
		nodeHeaderText = fmt.Sprintf("Nodes (%d total, %d/%d GPUs avail)", len(state.Nodes), totalGPUAvail, totalGPUTotal)
	}
	b.WriteString(detailHeaderStyle.Render(nodeHeaderText))
	b.WriteString("\n")

	// Node table header - include GPU column
	nodeHeader := fmt.Sprintf("  %s  %s  %s  %s",
		mutedStyle.Render(fmt.Sprintf("%-20s", "Name")),
		mutedStyle.Render(fmt.Sprintf("%14s", "CPU")),
		mutedStyle.Render(fmt.Sprintf("%16s", "Memory")),
		mutedStyle.Render(fmt.Sprintf("%-30s", "GPU")))
	b.WriteString(nodeHeader)
	b.WriteString("\n")

	// Calculate visible rows for nodes
	visibleRows := max(termHeight-nodeListOverhead, minVisibleNodes)

	startIdx := state.ScrollPos
	endIdx := min(startIdx+visibleRows, len(state.Nodes))

	for i := startIdx; i < endIdx; i++ {
		node := state.Nodes[i]
		cpuNodeStr := formatResourceRatio(node.CPUAvailable/1000, node.CPUAllocatable/1000)
		memNodeStr := formatMemoryRatio(node.MemAvailable, node.MemAllocatable)

		nodeName := node.Name
		if nodeName == "" {
			nodeName = fmt.Sprintf("node-%d", i+1)
		}
		if len(nodeName) > colWidthNodeName {
			nodeName = nodeName[:colWidthNodeName-3] + "..."
		}

		// Format GPU info
		gpuStr := formatNodeGPU(node)

		line := fmt.Sprintf("  %s  %s  %s  %s",
			monikerStyle.Render(fmt.Sprintf("%-*s", colWidthNodeName, nodeName)),
			detailValueStyle.Render(fmt.Sprintf("%14s", cpuNodeStr)),
			detailValueStyle.Render(fmt.Sprintf("%16s", memNodeStr)),
			formatGPUDisplay(gpuStr, node.GPUAllocatable > 0))
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(state.Nodes) > visibleRows {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Showing %d-%d of %d nodes", startIdx+1, endIdx, len(state.Nodes))))
	}

	return b.String()
}

// formatNodeGPU formats GPU information for a node.
func formatNodeGPU(node rpc.ProviderNodeWithGPU) string {
	if node.GPUAllocatable == 0 {
		return "-"
	}

	// Show GPU count and model info
	countStr := fmt.Sprintf("%d/%d", node.GPUAvailable, node.GPUAllocatable)

	if len(node.GPUs) == 0 {
		return countStr
	}

	// Get first GPU model info (typically nodes have homogeneous GPUs)
	gpu := node.GPUs[0]
	modelStr := formatGPUModel(gpu)

	return fmt.Sprintf("%s %s", countStr, modelStr)
}

// formatGPUModel formats a single GPU's model information.
func formatGPUModel(gpu rpc.GPUInfo) string {
	// Prefer showing: Vendor Name (Memory)
	// e.g., "NVIDIA A100 (80Gi)" or "NVIDIA H100"
	name := gpu.Name
	if name == "" {
		name = "Unknown"
	}

	// Shorten common vendor names
	vendor := gpu.Vendor
	switch vendor {
	case "nvidia":
		vendor = "NVIDIA"
	case "amd":
		vendor = "AMD"
	}

	result := name
	if vendor != "" && !strings.HasPrefix(strings.ToUpper(name), strings.ToUpper(vendor)) {
		result = vendor + " " + name
	}

	if gpu.MemorySize != "" {
		result += " (" + gpu.MemorySize + ")"
	}

	// Truncate if too long
	if len(result) > 28 {
		result = result[:25] + "..."
	}

	return result
}

// formatGPUDisplay applies styling to GPU display string.
func formatGPUDisplay(gpuStr string, hasGPU bool) string {
	if hasGPU {
		return gridVotedStyle.Render(gpuStr)
	}
	return mutedStyle.Render(gpuStr)
}

// renderStatusBar renders the bottom status bar
func renderStatusBar(endpoint string, activeTab Tab, showingDetail bool) string {
	var helpText string
	switch activeTab {
	case TabValidators:
		helpText = "q: quit | r: refresh | Tab/1/2/3: switch tabs | j/k or ↑/↓: scroll"
	case TabProviders:
		if showingDetail {
			helpText = "Esc/Backspace: back to list | j/k or ↑/↓: scroll nodes | q: quit"
		} else {
			helpText = "q: quit | r: refresh | Tab/1/2/3: switch | h/l: version | j/k: scroll | Enter: details"
		}
	default:
		helpText = "q: quit | r: refresh | Tab/1/2/3: switch tabs"
	}
	help := helpStyle.Render(helpText)
	status := statusBarStyle.Render(fmt.Sprintf("RPC: %s", endpoint))
	return help + "\n" + status
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%.0fs", int(d.Minutes()), d.Seconds()-float64(int(d.Minutes()))*60)
}

// formatNumber formats a number with thousand separators
func formatNumber(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

// formatPower formats voting power in a compact way
func formatPower(power int64) string {
	if power >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(power)/1_000_000_000)
	}
	if power >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(power)/1_000_000)
	}
	if power >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(power)/1_000)
	}
	return fmt.Sprintf("%d", power)
}

// formatResourceRatio formats available/total as "avail/total"
func formatResourceRatio(available, total uint64) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d", available, total)
}

// formatMemoryRatio formats memory available/total in human-readable format
func formatMemoryRatio(available, total uint64) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%s/%s", formatBytes(available), formatBytes(total))
}

// formatBytes formats bytes into Kubernetes-style binary units (Gi/Ti/Mi)
func formatBytes(bytes uint64) string {
	const (
		Mi = 1024 * 1024
		Gi = 1024 * Mi
		Ti = 1024 * Gi
	)
	if bytes >= Ti {
		return fmt.Sprintf("%.0fTi", float64(bytes)/float64(Ti))
	}
	if bytes >= Gi {
		return fmt.Sprintf("%.0fGi", float64(bytes)/float64(Gi))
	}
	return fmt.Sprintf("%dMi", bytes/Mi)
}
