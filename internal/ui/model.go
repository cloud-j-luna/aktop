package ui

import (
	"net/http"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/cloud-j-luna/aktop/internal/cache"
	"github.com/cloud-j-luna/aktop/internal/consensus"
	"github.com/cloud-j-luna/aktop/internal/rpc"
)

// Tab represents the active view tab
type Tab int

const (
	TabOverview Tab = iota
	TabValidators
	TabProviders
)

const (
	ChainSyncInterval     = 10 * time.Minute
	ProviderCheckInterval = 200 * time.Millisecond
	CacheSaveInterval     = 30 * time.Second
	MaxConcurrentChecks   = 10
)

// Model represents the application state
type Model struct {
	client      *rpc.Client
	rpcClient   *rpc.RPCProviderClient
	httpClient  *http.Client
	cache       *cache.ProviderCache
	state       *consensus.State
	monikers    map[string]string // pubkey to moniker
	refreshRate time.Duration
	lastUpdate  time.Time
	width       int
	height      int
	activeTab   Tab
	scrollPos   int // for scrolling validator list
	quitting    bool

	// Provider state
	providers          []rpc.Provider
	providerVersions   []string // unique versions, sorted latest first
	selectedVersion    string   // currently selected version filter
	selectedVersionIdx int      // index in providerVersions
	providerScrollPos  int      // scroll position for provider list

	// Provider loading state
	isFirstRun           bool
	providersLoading     bool
	providersTotal       int
	providersChecked     int
	activeLeaseProviders map[string]bool // providers with active leases (priority)

	// Background refresh state
	providerQueue  []string        // providers to check
	inFlightChecks map[string]bool // providers currently being checked
	lastChainSync  time.Time
	lastCacheSave  time.Time
}

// Message types
type (
	tickMsg              time.Time
	providerCheckTickMsg time.Time
	chainSyncTickMsg     time.Time
	cacheSaveTickMsg     time.Time

	stateMsg struct {
		state *consensus.State
		err   error
	}

	monikersMsg struct {
		monikers map[string]string
		err      error
	}

	// providerCheckedMsg is sent when a single provider has been checked
	providerCheckedMsg struct {
		owner    string
		isOnline bool
		version  string
		cpuAvail uint64
		cpuTotal uint64
		memAvail uint64
		memTotal uint64
	}

	// chainSyncMsg is sent after syncing providers from chain
	chainSyncMsg struct {
		newProviders         []string
		activeLeaseProviders map[string]bool
		err                  error
	}

	// initialLoadMsg signals to start initial provider loading
	initialLoadMsg struct{}
)

// NewModel creates a new UI model
func NewModel(client *rpc.Client, rpcClient *rpc.RPCProviderClient, providerCache *cache.ProviderCache, refreshRate time.Duration) Model {
	return Model{
		client:         client,
		rpcClient:      rpcClient,
		httpClient:     rpc.NewProviderHTTPClient(),
		cache:          providerCache,
		refreshRate:    refreshRate,
		monikers:       make(map[string]string),
		width:          80,
		height:         24,
		activeTab:      TabOverview,
		isFirstRun:     !providerCache.HasProviders(),
		inFlightChecks: make(map[string]bool),
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.fetchState,
		m.fetchMonikers,
		m.tick(),
		m.providerCheckTick(),
		m.chainSyncTick(),
		m.cacheSaveTick(),
	}

	if m.cache.HasProviders() {
		// Load from cache immediately
		cmds = append(cmds, m.loadFromCache)
	}

	// Always sync with chain on startup
	cmds = append(cmds, m.syncChain)

	return tea.Batch(cmds...)
}

// loadFromCache loads providers from cache and updates the UI
func (m Model) loadFromCache() tea.Msg {
	return chainSyncMsg{
		newProviders: nil, // No new providers, just loading from cache
		err:          nil,
	}
}

func (m Model) syncChain() tea.Msg {
	onChainProviders, err := m.fetchProviders()
	if err != nil {
		return chainSyncMsg{err: err}
	}

	activeLeaseProviders, err := m.rpcClient.GetActiveLeaseProviders(m.client.RESTEndpoint())
	if err != nil {
		activeLeaseProviders = make(map[string]bool)
	}

	cacheProviders := convertToCacheProviders(onChainProviders)
	newProviders := m.cache.SyncWithChain(cacheProviders)

	return chainSyncMsg{
		newProviders:         newProviders,
		activeLeaseProviders: activeLeaseProviders,
	}
}

func (m Model) fetchProviders() ([]rpc.OnChainProvider, error) {
	if m.isFirstRun && !m.cache.HasProviders() {
		providers, err := m.rpcClient.GetProvidersFromSeed()
		if err == nil {
			return providers, nil
		}
	}
	return m.rpcClient.GetProvidersOnChain()
}

func convertToCacheProviders(providers []rpc.OnChainProvider) []cache.OnChainProvider {
	result := make([]cache.OnChainProvider, len(providers))
	for i, p := range providers {
		result[i] = cache.OnChainProvider{
			Owner:      p.Owner,
			HostURI:    p.HostURI,
			Attributes: p.Attributes,
			IsOnline:   p.IsOnline,
		}
	}
	return result
}

func (m Model) checkProvider(owner string) tea.Cmd {
	return func() tea.Msg {
		p, exists := m.cache.GetProvider(owner)
		if !exists {
			return providerCheckedMsg{owner: owner, isOnline: false}
		}

		status, err := rpc.QueryProviderStatus(m.httpClient, p.HostURI)
		if err != nil {
			return providerCheckedMsg{owner: owner, isOnline: false}
		}

		cpuAvail, cpuTotal, memAvail, memTotal := aggregateResources(status)
		version := m.queryProviderVersion(p.HostURI)

		return providerCheckedMsg{
			owner:    owner,
			isOnline: true,
			version:  version,
			cpuAvail: cpuAvail,
			cpuTotal: cpuTotal,
			memAvail: memAvail,
			memTotal: memTotal,
		}
	}
}

func aggregateResources(status *rpc.ProviderStatusResponse) (cpuAvail, cpuTotal, memAvail, memTotal uint64) {
	for _, node := range status.Cluster.Inventory.Available.Nodes {
		cpuAvail += node.Available.CPU
		cpuTotal += node.Allocatable.CPU
		memAvail += node.Available.Memory
		memTotal += node.Allocatable.Memory
	}
	return
}

func (m Model) queryProviderVersion(hostURI string) string {
	versionResp, err := rpc.QueryProviderVersion(m.httpClient, hostURI)
	if err != nil {
		return "unknown"
	}
	return versionResp.Akash.Version
}

func (m *Model) dispatchProviderChecks() []tea.Cmd {
	available := MaxConcurrentChecks - len(m.inFlightChecks)
	if available <= 0 {
		return nil
	}

	var cmds []tea.Cmd
	dispatched := 0

	for _, owner := range m.providerQueue {
		if dispatched >= available {
			break
		}
		if !m.inFlightChecks[owner] {
			m.inFlightChecks[owner] = true
			cmds = append(cmds, m.checkProvider(owner))
			dispatched++
		}
	}

	return cmds
}

// fetchState fetches the consensus state
func (m Model) fetchState() tea.Msg {
	state, err := m.client.GetConsensusStateWithValidators()
	if err != nil {
		return stateMsg{err: err}
	}
	return stateMsg{state: state}
}

// fetchMonikers fetches validator monikers
func (m Model) fetchMonikers() tea.Msg {
	monikers, err := m.client.GetValidatorMonikers()
	if err != nil {
		return monikersMsg{err: err}
	}
	return monikersMsg{monikers: monikers}
}

// tick commands
func (m Model) tick() tea.Cmd {
	return tea.Tick(m.refreshRate, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) providerCheckTick() tea.Cmd {
	return tea.Tick(ProviderCheckInterval, func(t time.Time) tea.Msg {
		return providerCheckTickMsg(t)
	})
}

func (m Model) chainSyncTick() tea.Cmd {
	return tea.Tick(ChainSyncInterval, func(t time.Time) tea.Msg {
		return chainSyncTickMsg(t)
	})
}

func (m Model) cacheSaveTick() tea.Cmd {
	return tea.Tick(CacheSaveInterval, func(t time.Time) tea.Msg {
		return cacheSaveTickMsg(t)
	})
}

// rebuildProviderList rebuilds the provider list from cache
func (m *Model) rebuildProviderList() {
	cached := m.cache.GetAllProviders()

	// Deduplicate by HostURI - keep the most recently seen provider
	type providerWithTime struct {
		provider     rpc.Provider
		lastSeenTime time.Time
	}
	byURI := make(map[string]providerWithTime)

	for owner, p := range cached {
		if !p.IsOnline {
			continue
		}
		if strings.Contains(p.HostURI, "localhost") || strings.Contains(p.HostURI, "127.0.0.1") {
			continue
		}
		if p.Version == "" || p.Version == "unknown" {
			continue
		}

		provider := rpc.Provider{
			Owner:        owner,
			HostURI:      p.HostURI,
			Name:         p.Name,
			AkashVersion: p.Version,
			IsOnline:     p.IsOnline,
			Country:      p.Country,
			CPUAvailable: p.CPUAvailable,
			CPUTotal:     p.CPUTotal,
			MemAvailable: p.MemAvailable,
			MemTotal:     p.MemTotal,
		}

		// Keep the most recently seen provider for each URI
		existing, exists := byURI[p.HostURI]
		if !exists || p.LastSeenOnline.After(existing.lastSeenTime) {
			byURI[p.HostURI] = providerWithTime{
				provider:     provider,
				lastSeenTime: p.LastSeenOnline,
			}
		}
	}

	// Convert map to slice
	providers := make([]rpc.Provider, 0, len(byURI))
	for _, pt := range byURI {
		providers = append(providers, pt.provider)
	}

	// Sort: selected version first, then by version (latest first), then by URL
	sort.SliceStable(providers, func(i, j int) bool {
		iSelected := providers[i].AkashVersion == m.selectedVersion
		jSelected := providers[j].AkashVersion == m.selectedVersion

		// Selected version comes first
		if iSelected != jSelected {
			return iSelected
		}

		// Within same selection status, sort by version (latest first)
		cmp := rpc.CompareVersions(providers[i].AkashVersion, providers[j].AkashVersion)
		if cmp != 0 {
			return cmp > 0
		}
		return providers[i].HostURI < providers[j].HostURI
	})

	m.providers = providers
	m.providerVersions = rpc.GetProviderVersions(providers)

	// Update selected version if needed
	if m.selectedVersion == "" && len(m.providerVersions) > 0 {
		m.selectedVersion = m.providerVersions[0]
		m.selectedVersionIdx = 0
	}
}

// sortProviders re-sorts the provider list based on selected version
func (m *Model) sortProviders() {
	sort.SliceStable(m.providers, func(i, j int) bool {
		iSelected := m.providers[i].AkashVersion == m.selectedVersion
		jSelected := m.providers[j].AkashVersion == m.selectedVersion

		// Selected version comes first
		if iSelected != jSelected {
			return iSelected
		}

		// Within same selection status, sort by version (latest first)
		cmp := rpc.CompareVersions(m.providers[i].AkashVersion, m.providers[j].AkashVersion)
		if cmp != 0 {
			return cmp > 0
		}
		return m.providers[i].HostURI < m.providers[j].HostURI
	})
}

// buildProviderQueue builds the queue of providers to check based on priority
func (m *Model) buildProviderQueue(activeLeaseProviders map[string]bool) {
	m.activeLeaseProviders = activeLeaseProviders

	// Get all providers sorted by priority
	allProviders := m.cache.GetProvidersByPriority()

	// If first run, prioritize active lease providers
	if m.isFirstRun && len(activeLeaseProviders) > 0 {
		var prioritized []string
		var others []string

		for _, owner := range allProviders {
			if activeLeaseProviders[owner] {
				prioritized = append(prioritized, owner)
			} else {
				others = append(others, owner)
			}
		}

		m.providerQueue = append(prioritized, others...)
	} else {
		m.providerQueue = allProviders
	}

	m.providersTotal = len(m.providerQueue)
	m.providersChecked = 0
	m.providersLoading = len(m.providerQueue) > 0
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.fetchState, m.tick())
	case providerCheckTickMsg:
		cmds := m.dispatchProviderChecks()
		cmds = append(cmds, m.providerCheckTick())
		return m, tea.Batch(cmds...)
	case chainSyncTickMsg:
		return m, tea.Batch(m.syncChain, m.chainSyncTick())
	case cacheSaveTickMsg:
		m.cache.Save()
		return m, m.cacheSaveTick()
	case stateMsg:
		return m.handleStateMsg(msg)
	case monikersMsg:
		if msg.err == nil {
			m.monikers = msg.monikers
		}
		return m, nil
	case chainSyncMsg:
		return m.handleChainSyncMsg(msg)
	case providerCheckedMsg:
		return m.handleProviderCheckedMsg(msg)
	}
	return m, nil
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		m.cache.Save()
		return m, tea.Quit
	case "r":
		if m.activeTab == TabProviders {
			return m, m.syncChain
		}
		return m, m.fetchState
	case "1":
		m.activeTab = TabOverview
	case "2":
		m.activeTab = TabValidators
		m.scrollPos = 0
	case "3":
		m.activeTab = TabProviders
		m.providerScrollPos = 0
	case "tab":
		m.activeTab = (m.activeTab + 1) % 3
		m.resetScrollForTab()
	case "up", "k":
		m.scrollUp()
	case "down", "j":
		m.scrollDown()
	case "home", "g":
		m.scrollPos = 0
		m.providerScrollPos = 0
	case "end", "G":
		m.scrollToEnd()
	case "left", "h":
		m.selectPreviousVersion()
	case "right", "l":
		m.selectNextVersion()
	}
	return m, nil
}

func (m *Model) resetScrollForTab() {
	if m.activeTab == TabValidators {
		m.scrollPos = 0
	} else if m.activeTab == TabProviders {
		m.providerScrollPos = 0
	}
}

func (m *Model) scrollUp() {
	if m.activeTab == TabValidators && m.scrollPos > 0 {
		m.scrollPos--
	} else if m.activeTab == TabProviders && m.providerScrollPos > 0 {
		m.providerScrollPos--
	}
}

func (m *Model) scrollDown() {
	if m.activeTab == TabValidators && m.state != nil {
		maxScroll := maxInt(len(m.state.Validators)-(m.height-15), 0)
		if m.scrollPos < maxScroll {
			m.scrollPos++
		}
	} else if m.activeTab == TabProviders {
		maxScroll := maxInt(len(m.providers)-(m.height-providerListOverhead), 0)
		if m.providerScrollPos < maxScroll {
			m.providerScrollPos++
		}
	}
}

func (m *Model) scrollToEnd() {
	if m.activeTab == TabValidators && m.state != nil {
		m.scrollPos = maxInt(len(m.state.Validators)-(m.height-15), 0)
	} else if m.activeTab == TabProviders {
		m.providerScrollPos = maxInt(len(m.providers)-(m.height-providerListOverhead), 0)
	}
}

func (m *Model) selectPreviousVersion() {
	if m.activeTab != TabProviders || len(m.providerVersions) == 0 {
		return
	}
	m.selectedVersionIdx--
	if m.selectedVersionIdx < 0 {
		m.selectedVersionIdx = len(m.providerVersions) - 1
	}
	m.selectedVersion = m.providerVersions[m.selectedVersionIdx]
	m.providerScrollPos = 0
	m.sortProviders()
}

func (m *Model) selectNextVersion() {
	if m.activeTab != TabProviders || len(m.providerVersions) == 0 {
		return
	}
	m.selectedVersionIdx = (m.selectedVersionIdx + 1) % len(m.providerVersions)
	m.selectedVersion = m.providerVersions[m.selectedVersionIdx]
	m.providerScrollPos = 0
	m.sortProviders()
}

func (m Model) handleStateMsg(msg stateMsg) (tea.Model, tea.Cmd) {
	m.lastUpdate = time.Now()
	if msg.err != nil {
		if m.state == nil {
			m.state = &consensus.State{}
		}
		m.state.Error = msg.err
	} else {
		m.state = msg.state
	}
	return m, nil
}

func (m Model) handleChainSyncMsg(msg chainSyncMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	m.lastChainSync = time.Now()
	m.buildProviderQueue(msg.activeLeaseProviders)
	m.rebuildProviderList()

	cmds := m.dispatchProviderChecks()
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) handleProviderCheckedMsg(msg providerCheckedMsg) (tea.Model, tea.Cmd) {
	delete(m.inFlightChecks, msg.owner)
	m.removeFromQueue(msg.owner)
	m.providersChecked++

	if msg.isOnline {
		m.cache.MarkProviderOnline(msg.owner, msg.version, msg.cpuAvail, msg.cpuTotal, msg.memAvail, msg.memTotal)
	} else {
		m.cache.MarkProviderOffline(msg.owner)
	}

	m.rebuildProviderList()

	if len(m.providerQueue) == 0 && len(m.inFlightChecks) == 0 {
		m.providersLoading = false
		m.isFirstRun = false
		m.providerQueue = m.cache.GetProvidersDueForCheck()
	}

	return m, nil
}

func (m *Model) removeFromQueue(owner string) {
	for i, o := range m.providerQueue {
		if o == owner {
			m.providerQueue = append(m.providerQueue[:i], m.providerQueue[i+1:]...)
			return
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// View renders the UI
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("Goodbye!\n")
	}

	v := tea.NewView(RenderView(
		m.state,
		m.client.Endpoint(),
		m.width,
		m.height,
		m.activeTab,
		m.monikers,
		m.scrollPos,
		m.providers,
		m.providerVersions,
		m.selectedVersion,
		m.providerScrollPos,
		m.providersLoading,
		m.providersChecked,
		m.providersTotal,
	))
	v.AltScreen = true
	return v
}
