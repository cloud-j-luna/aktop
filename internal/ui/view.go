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
	colWidthProvider = 32
	colWidthVersion  = 12
	colWidthCPU      = 14
	colWidthMem      = 14
	colWidthCountry  = 4
)

// Layout constants
const (
	providerListOverhead = 25 // header, tabs, status bar, scroll indicator, etc.
)

// RenderView renders the complete view
func RenderView(state *consensus.State, endpoint string, width, height int, activeTab Tab, monikers map[string]string, scrollPos int, providers []rpc.Provider, providerVersions []string, selectedVersion string, providerScrollPos int, providersLoading bool, providersLoaded, providersTotal int) string {
	var b strings.Builder

	// Title and tabs
	title := titleStyle.Render("aktop - Akash Network Monitor")
	b.WriteString(title)
	b.WriteString("\n")

	// Tab bar
	b.WriteString(renderTabBar(activeTab))
	b.WriteString("\n\n")

	// Error state
	if state == nil {
		b.WriteString(errorStyle.Render("Connecting to RPC endpoint..."))
		b.WriteString("\n")
		b.WriteString(renderStatusBar(endpoint, activeTab))
		return b.String()
	}

	if state.Error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", state.Error)))
		b.WriteString("\n\n")
		// Still show last known state if available
		if state.Height == 0 {
			b.WriteString(renderStatusBar(endpoint, activeTab))
			return b.String()
		}
	}

	// Render based on active tab
	switch activeTab {
	case TabOverview:
		b.WriteString(renderOverviewTab(state, width))
	case TabValidators:
		b.WriteString(renderValidatorsTab(state, monikers, height, scrollPos))
	case TabProviders:
		b.WriteString(renderProvidersTab(providers, providerVersions, selectedVersion, height, providerScrollPos, providersLoading, providersLoaded, providersTotal))
	}

	b.WriteString("\n")

	// Help & status
	b.WriteString(renderStatusBar(endpoint, activeTab))

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
	return addr[:8] + "..." + addr[len(addr)-4:]
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func renderProvidersTab(providers []rpc.Provider, providerVersions []string, selectedVersion string, termHeight, scrollPos int, loading bool, loaded, total int) string {
	var b strings.Builder

	if loading && total > 0 {
		progress := fmt.Sprintf("Scanning providers... %d/%d checked, %d online", loaded, total, len(providers))
		b.WriteString(ProgressBar(float64(loaded)/float64(total), 40))
		b.WriteString(" ")
		b.WriteString(mutedStyle.Render(progress))
		b.WriteString("\n\n")
	}

	b.WriteString(renderVersionDistribution(providers, providerVersions, selectedVersion))
	b.WriteString("\n\n")
	b.WriteString(renderProviderList(providers, selectedVersion, termHeight, scrollPos))

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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func renderProviderList(providers []rpc.Provider, selectedVersion string, termHeight, scrollPos int) string {
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

	colHeader := fmt.Sprintf("  %s  %s  %s  %s  %s  %s",
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthIndex, "#")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthProvider, "Provider")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthVersion, "Version")),
		mutedStyle.Render(fmt.Sprintf("%*s", colWidthCPU, "CPU")),
		mutedStyle.Render(fmt.Sprintf("%*s", colWidthMem, "Memory")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthCountry, "Loc")))

	var lines []string
	lines = append(lines, colHeader)

	startIdx, endIdx := scrollRange(scrollPos, visibleRows, len(filtered))

	for i := startIdx; i < endIdx; i++ {
		lines = append(lines, renderProviderRow(filtered[i], i+1, selectedVersion))
	}

	if len(filtered) > visibleRows {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf(
			"Showing %d-%d of %d (↑/↓ or j/k to scroll)", startIdx+1, endIdx, len(filtered))))
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

func renderProviderRow(p rpc.Provider, index int, selectedVersion string) string {
	displayURL := formatProviderURL(p.HostURI, colWidthProvider-2)

	isSelected := p.AkashVersion == selectedVersion
	versionDisplay := formatVersionDisplay(p.AkashVersion, isSelected)
	marker := versionMarker(isSelected)

	country := p.Country
	if country == "" {
		country = "--"
	}

	cpuStr := formatResourceRatio(p.CPUAvailable/1000, p.CPUTotal/1000)
	memStr := formatMemoryRatio(p.MemAvailable, p.MemTotal)

	return fmt.Sprintf("%s%s  %s  %s  %s  %s  %s",
		marker,
		mutedStyle.Render(fmt.Sprintf("%-*d", colWidthIndex, index)),
		monikerStyle.Render(fmt.Sprintf("%-*s", colWidthProvider, displayURL)),
		versionDisplay,
		mutedStyle.Render(fmt.Sprintf("%*s", colWidthCPU, cpuStr)),
		mutedStyle.Render(fmt.Sprintf("%*s", colWidthMem, memStr)),
		mutedStyle.Render(country))
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

// renderStatusBar renders the bottom status bar
func renderStatusBar(endpoint string, activeTab Tab) string {
	var helpText string
	switch activeTab {
	case TabValidators:
		helpText = "q: quit | r: refresh | Tab/1/2/3: switch tabs | j/k or ↑/↓: scroll"
	case TabProviders:
		helpText = "q: quit | r: refresh | Tab/1/2/3: switch tabs | h/l or ←/→: version | j/k or ↑/↓: scroll"
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
