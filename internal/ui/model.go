package ui

import (
	"context"
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
	// Core dependencies
	client     *rpc.Client
	rpcClient  *rpc.RPCProviderClient
	httpClient *http.Client
	cache      cache.ProviderStore

	// Consensus state
	state       *consensus.State
	monikers    map[string]string // pubkey to moniker
	lastUpdate  time.Time
	refreshRate time.Duration

	// UI state
	width     int
	height    int
	activeTab Tab
	scrollPos int // for scrolling validator list
	quitting  bool

	// Provider state (embedded)
	providers ProviderList
	loader    ProviderLoader
	detail    ProviderDetail
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
		owner     string
		isOnline  bool
		version   string
		cpuAvail  uint64
		cpuTotal  uint64
		memAvail  uint64
		memTotal  uint64
		gpuAvail  uint64
		gpuTotal  uint64
		gpuModels []string
	}

	// chainSyncMsg is sent after syncing providers from chain
	chainSyncMsg struct {
		newProviders         []string
		activeLeaseProviders map[string]bool
		err                  error
	}

	// initialLoadMsg signals to start initial provider loading
	initialLoadMsg struct{}

	// providerDetailMsg is sent when provider detail fetch completes
	providerDetailMsg struct {
		nodes []rpc.ProviderNodeWithGPU
		err   error
	}
)

// ModelConfig holds configuration options for creating a new Model
type ModelConfig struct {
	Client             *rpc.Client
	RPCClient          *rpc.RPCProviderClient
	Cache              cache.ProviderStore
	RefreshRate        time.Duration
	InsecureSkipVerify bool
}

// NewModel creates a new UI model
func NewModel(cfg ModelConfig) Model {
	return Model{
		client:      cfg.Client,
		rpcClient:   cfg.RPCClient,
		httpClient:  rpc.NewProviderHTTPClient(cfg.InsecureSkipVerify),
		cache:       cfg.Cache,
		refreshRate: cfg.RefreshRate,
		monikers:    make(map[string]string),
		width:       80,
		height:      24,
		activeTab:   TabOverview,
		loader: ProviderLoader{
			FirstRun: !cfg.Cache.HasProviders(),
			InFlight: make(map[string]bool),
		},
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
	ctx := context.Background()

	onChainProviders, err := m.fetchProviders(ctx)
	if err != nil {
		return chainSyncMsg{err: err}
	}

	activeLeaseProviders, err := m.rpcClient.GetActiveLeaseProviders(ctx, m.client.RESTEndpoint())
	if err != nil {
		activeLeaseProviders = make(map[string]bool)
	}

	newProviders := m.cache.SyncWithChain(onChainProviders)

	return chainSyncMsg{
		newProviders:         newProviders,
		activeLeaseProviders: activeLeaseProviders,
	}
}

func (m Model) fetchProviders(ctx context.Context) ([]rpc.OnChainProvider, error) {
	if m.loader.FirstRun && !m.cache.HasProviders() {
		providers, err := m.rpcClient.GetProvidersFromSeed(ctx)
		if err == nil {
			return providers, nil
		}
	}
	return m.rpcClient.GetProvidersOnChain(ctx)
}

func (m Model) checkProvider(owner string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		p, exists := m.cache.GetProvider(owner)
		if !exists {
			return providerCheckedMsg{owner: owner, isOnline: false}
		}

		// Try gRPC first for full GPU info, fall back to REST
		nodes, err := rpc.QueryProviderStatusGRPC(ctx, p.HostURI)
		if err != nil {
			// Fall back to REST (no GPU model info)
			status, restErr := rpc.QueryProviderStatus(ctx, m.httpClient, p.HostURI)
			if restErr != nil {
				return providerCheckedMsg{owner: owner, isOnline: false}
			}
			cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal := aggregateResourcesREST(status)
			version := m.queryProviderVersion(ctx, p.HostURI)
			return providerCheckedMsg{
				owner:    owner,
				isOnline: true,
				version:  version,
				cpuAvail: cpuAvail,
				cpuTotal: cpuTotal,
				memAvail: memAvail,
				memTotal: memTotal,
				gpuAvail: gpuAvail,
				gpuTotal: gpuTotal,
			}
		}

		cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal, gpuModels := aggregateResourcesGRPC(nodes)
		version := m.queryProviderVersion(ctx, p.HostURI)

		return providerCheckedMsg{
			owner:     owner,
			isOnline:  true,
			version:   version,
			cpuAvail:  cpuAvail,
			cpuTotal:  cpuTotal,
			memAvail:  memAvail,
			memTotal:  memTotal,
			gpuAvail:  gpuAvail,
			gpuTotal:  gpuTotal,
			gpuModels: gpuModels,
		}
	}
}

func aggregateResourcesREST(status *rpc.ProviderStatusResponse) (cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal uint64) {
	for _, node := range status.Cluster.Inventory.Available.Nodes {
		cpuAvail += node.Available.CPU
		cpuTotal += node.Allocatable.CPU
		memAvail += node.Available.Memory
		memTotal += node.Allocatable.Memory
		gpuAvail += node.Available.GPU
		gpuTotal += node.Allocatable.GPU
	}
	return
}

func aggregateResourcesGRPC(nodes []rpc.ProviderNodeWithGPU) (cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal uint64, gpuModels []string) {
	modelSet := make(map[string]bool)
	for _, node := range nodes {
		cpuAvail += node.CPUAvailable
		cpuTotal += node.CPUAllocatable
		memAvail += node.MemAvailable
		memTotal += node.MemAllocatable
		gpuAvail += node.GPUAvailable
		gpuTotal += node.GPUAllocatable

		// Collect unique GPU models
		for _, gpu := range node.GPUs {
			model := formatGPUModelShort(gpu)
			if model != "" && !modelSet[model] {
				modelSet[model] = true
				gpuModels = append(gpuModels, model)
			}
		}
	}
	return
}

func formatGPUModelShort(gpu rpc.GPUInfo) string {
	if gpu.Name == "" {
		return ""
	}
	// Return just the GPU name (e.g., "H100", "A100", "RTX 4090")
	return gpu.Name
}

func (m Model) queryProviderVersion(ctx context.Context, hostURI string) string {
	versionResp, err := rpc.QueryProviderVersion(ctx, m.httpClient, hostURI)
	if err != nil {
		return "unknown"
	}
	return versionResp.Akash.Version
}

func (m *Model) saveCache() {
	if err := m.cache.Save(); err != nil {
		m.loader.LastSaveError = err
	}
}

func (m *Model) dispatchProviderChecks() []tea.Cmd {
	available := MaxConcurrentChecks - len(m.loader.InFlight)
	if available <= 0 {
		return nil
	}

	var cmds []tea.Cmd
	dispatched := 0

	for _, owner := range m.loader.Queue {
		if dispatched >= available {
			break
		}
		if !m.loader.InFlight[owner] {
			m.loader.InFlight[owner] = true
			cmds = append(cmds, m.checkProvider(owner))
			dispatched++
		}
	}

	return cmds
}

// fetchState fetches the consensus state
func (m Model) fetchState() tea.Msg {
	ctx := context.Background()
	state, err := m.client.GetConsensusStateWithValidators(ctx)
	if err != nil {
		return stateMsg{err: err}
	}
	return stateMsg{state: state}
}

// fetchMonikers fetches validator monikers
func (m Model) fetchMonikers() tea.Msg {
	ctx := context.Background()
	monikers, err := m.client.GetValidatorMonikers(ctx)
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
			GPUAvailable: p.GPUAvailable,
			GPUTotal:     p.GPUTotal,
			GPUModels:    p.GPUModels,
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
	items := make([]rpc.Provider, 0, len(byURI))
	for _, pt := range byURI {
		items = append(items, pt.provider)
	}

	// Sort: selected version first, then by version (latest first), then by URL
	sort.SliceStable(items, func(i, j int) bool {
		iSelected := items[i].AkashVersion == m.providers.Version
		jSelected := items[j].AkashVersion == m.providers.Version

		// Selected version comes first
		if iSelected != jSelected {
			return iSelected
		}

		// Within same selection status, sort by version (latest first)
		cmp := rpc.CompareVersions(items[i].AkashVersion, items[j].AkashVersion)
		if cmp != 0 {
			return cmp > 0
		}
		return items[i].HostURI < items[j].HostURI
	})

	m.providers.Items = items
	m.providers.Versions = rpc.GetProviderVersions(items)

	// Update selected version if needed
	if m.providers.Version == "" && len(m.providers.Versions) > 0 {
		m.providers.Version = m.providers.Versions[0]
		m.providers.VersionIdx = 0
	}
}

// sortProviders re-sorts the provider list based on selected version
func (m *Model) sortProviders() {
	sort.SliceStable(m.providers.Items, func(i, j int) bool {
		iSelected := m.providers.Items[i].AkashVersion == m.providers.Version
		jSelected := m.providers.Items[j].AkashVersion == m.providers.Version

		// Selected version comes first
		if iSelected != jSelected {
			return iSelected
		}

		// Within same selection status, sort by version (latest first)
		cmp := rpc.CompareVersions(m.providers.Items[i].AkashVersion, m.providers.Items[j].AkashVersion)
		if cmp != 0 {
			return cmp > 0
		}
		return m.providers.Items[i].HostURI < m.providers.Items[j].HostURI
	})
}

// buildProviderQueue builds the queue of providers to check based on priority
func (m *Model) buildProviderQueue(activeLeaseProviders map[string]bool) {
	m.loader.ActiveLease = activeLeaseProviders

	// Get all providers sorted by priority
	allProviders := m.cache.GetProvidersByPriority()

	// If first run, prioritize active lease providers
	if m.loader.FirstRun && len(activeLeaseProviders) > 0 {
		var prioritized []string
		var others []string

		for _, owner := range allProviders {
			if activeLeaseProviders[owner] {
				prioritized = append(prioritized, owner)
			} else {
				others = append(others, owner)
			}
		}

		m.loader.Queue = append(prioritized, others...)
	} else {
		m.loader.Queue = allProviders
	}

	m.loader.Total = len(m.loader.Queue)
	m.loader.Checked = 0
	m.loader.Loading = len(m.loader.Queue) > 0
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
		m.saveCache()
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
	case providerDetailMsg:
		return m.handleProviderDetailMsg(msg)
	}
	return m, nil
}

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle detail view keys first
	if m.detail.Showing {
		return m.handleDetailViewKeys(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		m.saveCache()
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
		m.providers.ScrollPos = 0
		m.providers.SelectedIdx = 0
	case "tab":
		m.activeTab = (m.activeTab + 1) % 3
		m.resetScrollForTab()
	case "up", "k":
		m.scrollUp()
	case "down", "j":
		m.scrollDown()
	case "home", "g":
		m.scrollPos = 0
		m.providers.ScrollPos = 0
		m.providers.SelectedIdx = 0
	case "end", "G":
		m.scrollToEnd()
	case "left", "h":
		m.selectPreviousVersion()
	case "right", "l":
		m.selectNextVersion()
	case "enter":
		if m.activeTab == TabProviders {
			return m.enterProviderDetail()
		}
	}
	return m, nil
}

func (m *Model) handleDetailViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		m.saveCache()
		return m, tea.Quit
	case "esc", "backspace":
		m.detail.Showing = false
		m.detail.Nodes = nil
		m.detail.Provider = nil
		m.detail.Error = nil
		m.detail.Loading = false
		m.detail.ScrollPos = 0
	case "up", "k":
		if m.detail.ScrollPos > 0 {
			m.detail.ScrollPos--
		}
	case "down", "j":
		m.scrollDetailDown()
	case "home", "g":
		m.detail.ScrollPos = 0
	case "end", "G":
		m.scrollDetailToEnd()
	case "1", "2", "3", "tab":
		// Exit detail view and switch tabs
		m.detail.Showing = false
		m.detail.Nodes = nil
		m.detail.Provider = nil
		m.detail.Error = nil
		m.detail.Loading = false
		m.detail.ScrollPos = 0
		// Re-handle the key for tab switching
		return m.handleKeyMsg(msg)
	}
	return m, nil
}

func (m *Model) resetScrollForTab() {
	if m.activeTab == TabValidators {
		m.scrollPos = 0
	} else if m.activeTab == TabProviders {
		m.providers.ScrollPos = 0
	}
}

func (m *Model) scrollUp() {
	if m.activeTab == TabValidators && m.scrollPos > 0 {
		m.scrollPos--
	} else if m.activeTab == TabProviders {
		m.moveProviderSelection(-1)
	}
}

func (m *Model) scrollDown() {
	if m.activeTab == TabValidators && m.state != nil {
		maxScroll := max(len(m.state.Validators)-(m.height-15), 0)
		if m.scrollPos < maxScroll {
			m.scrollPos++
		}
	} else if m.activeTab == TabProviders {
		m.moveProviderSelection(1)
	}
}

func (m *Model) moveProviderSelection(delta int) {
	filtered := m.getFilteredProviders()
	if len(filtered) == 0 {
		return
	}

	m.providers.SelectedIdx += delta
	if m.providers.SelectedIdx < 0 {
		m.providers.SelectedIdx = 0
	} else if m.providers.SelectedIdx >= len(filtered) {
		m.providers.SelectedIdx = len(filtered) - 1
	}

	m.ensureSelectionVisible()
}

func (m *Model) ensureSelectionVisible() {
	visibleRows := max(m.height-providerListOverhead, 5)
	if len(m.getFilteredProviders()) > visibleRows {
		visibleRows -= 2
	}

	if m.providers.SelectedIdx < m.providers.ScrollPos {
		m.providers.ScrollPos = m.providers.SelectedIdx
	} else if m.providers.SelectedIdx >= m.providers.ScrollPos+visibleRows {
		m.providers.ScrollPos = m.providers.SelectedIdx - visibleRows + 1
	}
}

func (m *Model) getFilteredProviders() []rpc.Provider {
	return filterNonLocalProviders(m.providers.Items)
}

func (m *Model) scrollToEnd() {
	if m.activeTab == TabValidators && m.state != nil {
		m.scrollPos = max(len(m.state.Validators)-(m.height-15), 0)
	} else if m.activeTab == TabProviders {
		filtered := m.getFilteredProviders()
		if len(filtered) > 0 {
			m.providers.SelectedIdx = len(filtered) - 1
			m.ensureSelectionVisible()
		}
	}
}

func (m *Model) selectPreviousVersion() {
	if m.activeTab != TabProviders || len(m.providers.Versions) == 0 {
		return
	}
	m.providers.VersionIdx--
	if m.providers.VersionIdx < 0 {
		m.providers.VersionIdx = len(m.providers.Versions) - 1
	}
	m.providers.Version = m.providers.Versions[m.providers.VersionIdx]
	m.providers.ScrollPos = 0
	m.providers.SelectedIdx = 0
	m.sortProviders()
}

func (m *Model) selectNextVersion() {
	if m.activeTab != TabProviders || len(m.providers.Versions) == 0 {
		return
	}
	m.providers.VersionIdx = (m.providers.VersionIdx + 1) % len(m.providers.Versions)
	m.providers.Version = m.providers.Versions[m.providers.VersionIdx]
	m.providers.ScrollPos = 0
	m.providers.SelectedIdx = 0
	m.sortProviders()
}

func (m *Model) enterProviderDetail() (tea.Model, tea.Cmd) {
	filtered := m.getFilteredProviders()
	if len(filtered) == 0 || m.providers.SelectedIdx >= len(filtered) {
		return m, nil
	}

	provider := filtered[m.providers.SelectedIdx]
	m.detail.Provider = &provider
	m.detail.Loading = true
	m.detail.Error = nil
	m.detail.Nodes = nil
	m.detail.Showing = true
	m.detail.ScrollPos = 0

	return m, m.fetchProviderDetail(provider.HostURI)
}

func (m *Model) fetchProviderDetail(hostURI string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		nodes, err := rpc.QueryProviderStatusGRPC(ctx, hostURI)
		if err != nil {
			return providerDetailMsg{err: err}
		}
		return providerDetailMsg{nodes: nodes}
	}
}

func (m *Model) scrollDetailDown() {
	visibleRows := max(m.height-nodeListOverhead, minVisibleNodes)
	maxScroll := max(len(m.detail.Nodes)-visibleRows, 0)
	if m.detail.ScrollPos < maxScroll {
		m.detail.ScrollPos++
	}
}

func (m *Model) scrollDetailToEnd() {
	visibleRows := max(m.height-nodeListOverhead, minVisibleNodes)
	m.detail.ScrollPos = max(len(m.detail.Nodes)-visibleRows, 0)
}

func (m *Model) handleProviderDetailMsg(msg providerDetailMsg) (tea.Model, tea.Cmd) {
	m.detail.Loading = false
	if msg.err != nil {
		m.detail.Error = msg.err
	} else {
		m.detail.Nodes = msg.nodes
	}
	return m, nil
}

func (m *Model) handleStateMsg(msg stateMsg) (tea.Model, tea.Cmd) {
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

func (m *Model) handleChainSyncMsg(msg chainSyncMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	m.loader.LastSync = time.Now()
	m.buildProviderQueue(msg.activeLeaseProviders)
	m.rebuildProviderList()

	cmds := m.dispatchProviderChecks()
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m *Model) handleProviderCheckedMsg(msg providerCheckedMsg) (tea.Model, tea.Cmd) {
	delete(m.loader.InFlight, msg.owner)
	m.removeFromQueue(msg.owner)
	m.loader.Checked++

	if msg.isOnline {
		m.cache.MarkProviderOnline(msg.owner, msg.version, msg.cpuAvail, msg.cpuTotal, msg.memAvail, msg.memTotal, msg.gpuAvail, msg.gpuTotal, msg.gpuModels)
	} else {
		m.cache.MarkProviderOffline(msg.owner)
	}

	m.rebuildProviderList()

	if len(m.loader.Queue) == 0 && len(m.loader.InFlight) == 0 {
		m.loader.Loading = false
		m.loader.FirstRun = false
		m.loader.Queue = m.cache.GetProvidersDueForCheck()
	}

	return m, nil
}

func (m *Model) removeFromQueue(owner string) {
	for i, o := range m.loader.Queue {
		if o == owner {
			m.loader.Queue = append(m.loader.Queue[:i], m.loader.Queue[i+1:]...)
			return
		}
	}
}

// View renders the UI
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("Goodbye!\n")
	}

	ctx := ViewContext{
		State:     m.state,
		Endpoint:  m.client.Endpoint(),
		Width:     m.width,
		Height:    m.height,
		ActiveTab: m.activeTab,
		Monikers:  m.monikers,
		ScrollPos: m.scrollPos,
		Providers: ProviderViewState{
			Providers: m.providers.Items,
			Versions:  m.providers.Versions,
			Selected:  m.providers.Version,
			ScrollPos: m.providers.ScrollPos,
			Loading:   m.loader.Loading,
			Loaded:    m.loader.Checked,
			Total:     m.loader.Total,
			Detail: ProviderDetailState{
				Showing:     m.detail.Showing,
				Provider:    m.detail.Provider,
				Nodes:       m.detail.Nodes,
				Loading:     m.detail.Loading,
				Error:       m.detail.Error,
				ScrollPos:   m.detail.ScrollPos,
				SelectedIdx: m.providers.SelectedIdx,
			},
		},
	}

	v := tea.NewView(RenderView(ctx))
	v.AltScreen = true
	return v
}
