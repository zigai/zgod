package tui

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
	"github.com/zigai/zgod/internal/match"
)

const modelRecencyIndexStep = 100

type historyBatchLoadedMsg struct {
	generation uint64
	entries    []db.HistoryEntry
	done       bool
	err        error
}

type Model struct {
	input                textinput.Model
	cfg                  config.Config
	styles               Styles
	allEntries           []db.HistoryEntry
	candidates           []string
	displayEntries       []history.ScoredEntry
	cursor               int
	width                int
	height               int
	maxHeight            int
	terminalHeight       int
	selected             string
	mode                 match.Mode
	enabledModes         []match.Mode
	cwdMode              bool
	dedupe               bool
	failFilter           db.FailFilterMode
	cwd                  string
	homeDir              string
	quitting             bool
	canceled             bool
	showHelp             bool
	showPreview          bool
	previewCommand       string
	hoverFooterAction    footerShortcutAction
	hoverIndicatorAction indicatorAction
	repo                 *db.HistoryRepo
	dbError              error
	loadingHistory       bool
	historyComplete      bool
	historyLoadGen       uint64
	indicatorCache       indicatorCache
	footerCache          footerCache
	lastQuery            string
	lastMode             match.Mode
	lastEntryCount       int
	matchBuf             []match.Match
	indexBuf             []int
	lineCache            map[int]cachedResultLine
	resultsCache         cachedResultsBlock
	headerCache          cachedResultsHeader
	regexCache           regexRenderCache
	searchRegex          regexRenderCache
}

type indicatorCache struct {
	key   indicatorCacheKey
	value string
	valid bool
}

type indicatorCacheKey struct {
	width         int
	mode          match.Mode
	cwdMode       bool
	dedupe        bool
	failFilter    db.FailFilterMode
	hoveredAction indicatorAction
}

type footerCache struct {
	showPreviewHint bool
	hoveredAction   footerShortcutAction
	left            string
	valid           bool
}

type cachedResultLine struct {
	key  resultLineCacheKey
	line string
}

type resultLineCacheKey struct {
	width          int
	cmdWidth       int
	entryID        int64
	query          string
	mode           match.Mode
	showDir        bool
	timeFormat     string
	durationFormat string
	timeBucket     int64
}

type cachedResultsBlock struct {
	key   resultsBlockCacheKey
	value string
	valid bool
}

type resultsBlockCacheKey struct {
	width            int
	height           int
	cursor           int
	displayLen       int
	query            string
	mode             match.Mode
	showDir          bool
	multilinePreview string
	timeFormat       string
	durationFormat   string
	timeBucket       int64
}

type cachedResultsHeader struct {
	key   resultsHeaderCacheKey
	value string
	valid bool
}

type resultsHeaderCacheKey struct {
	width      int
	cmdWidth   int
	dirWidth   int
	timeWidth  int
	showDir    bool
	barChar    string
	prefixSize int
}

type regexRenderCache struct {
	pattern string
	re      *regexp.Regexp
	valid   bool
}

func NewModel(cfg config.Config, repo *db.HistoryRepo, cwd string, homeDir string, height int, cwdMode bool, initialQuery string) *Model {
	width := 80

	if height < 1 {
		height = 1
	}

	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = width - 4
	ti.Prompt = ""
	ti.SetValue(initialQuery)

	var enabledModes []match.Mode
	if cfg.Display.EnableFuzzy {
		enabledModes = append(enabledModes, match.ModeFuzzy)
	}

	if cfg.Display.EnableGlob {
		enabledModes = append(enabledModes, match.ModeGlob)
	}

	if cfg.Display.EnableRegex {
		enabledModes = append(enabledModes, match.ModeRegex)
	}

	initialMode := enabledModes[0]
	if parsed, ok := match.ParseMode(cfg.Display.DefaultMode); ok {
		if slices.Contains(enabledModes, parsed) {
			initialMode = parsed
		}
	}

	if cfg.Display.DefaultScope == "cwd" {
		cwdMode = true
	}

	failFilter, _ := db.ParseFailFilterMode(cfg.Display.DefaultFailFilter)

	m := Model{
		input:          ti,
		cfg:            cfg,
		styles:         NewStyles(cfg.Theme),
		width:          width,
		height:         height,
		maxHeight:      height,
		mode:           initialMode,
		enabledModes:   enabledModes,
		cwdMode:        cwdMode,
		dedupe:         true,
		failFilter:     failFilter,
		cwd:            cwd,
		homeDir:        homeDir,
		repo:           repo,
		loadingHistory: true,
		historyLoadGen: 1,
	}

	return &m
}

func (m *Model) Selected() string {
	return m.selected
}

func (m *Model) Canceled() bool {
	return m.canceled
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tea.EnableMouseAllMotion,
		m.loadEntriesCmd(m.startupLimit(), false, m.historyLoadGen),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.WindowSizeMsg:
		m.terminalHeight = msg.Height

		innerWidth := max(msg.Width-panelBorderW-(panelPaddingX*2), 1)
		m.width = innerWidth

		available := max(msg.Height-m.chromeHeight()-panelBorderH-(panelPaddingY*2), 1)
		if m.maxHeight < 1 {
			m.maxHeight = 1
		}

		if available > m.maxHeight {
			available = m.maxHeight
		}

		m.height = available
		m.input.Width = max(m.width-4, 1)

		return m, nil
	case historyBatchLoadedMsg:
		return m.handleHistoryBatchLoaded(msg)
	}

	var cmd tea.Cmd

	m.input, cmd = m.input.Update(msg)

	return m, cmd
}

func (m *Model) startupLimit() int {
	if m.cfg.Display.StartupLimit < 0 {
		return 0
	}

	return m.cfg.Display.StartupLimit
}

func (m *Model) loadEntriesCmd(limit int, done bool, generation uint64) tea.Cmd {
	candidateCWD := ""
	if m.cwdMode {
		candidateCWD = m.cwd
	}

	opts := history.CandidateOpts{
		Limit:      limit,
		Dedupe:     m.dedupe,
		FailFilter: m.failFilter,
		CWD:        candidateCWD,
	}

	return func() tea.Msg {
		entries, err := history.FetchCandidates(m.repo, opts)
		batchDone := done || limit <= 0 || (err == nil && limit > 0 && len(entries) < limit)

		return historyBatchLoadedMsg{
			generation: generation,
			entries:    entries,
			done:       batchDone,
			err:        err,
		}
	}
}

func (m *Model) startLoadingEntries() tea.Cmd {
	m.historyLoadGen++
	m.loadingHistory = true
	m.historyComplete = false
	m.dbError = nil
	m.allEntries = nil
	m.candidates = nil
	m.displayEntries = nil
	m.cursor = 0
	m.invalidateHistoryCaches()

	return m.loadEntriesCmd(m.startupLimit(), false, m.historyLoadGen)
}

func (m *Model) handleHistoryBatchLoaded(msg historyBatchLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.historyLoadGen {
		return m, nil
	}

	m.applyLoadedEntries(msg.entries, msg.err, msg.done)

	if msg.err != nil || msg.done {
		return m, nil
	}

	return m, m.loadEntriesCmd(0, true, msg.generation)
}

func (m *Model) applyLoadedEntries(entries []db.HistoryEntry, err error, done bool) {
	m.historyComplete = done

	m.dbError = err
	if err != nil {
		m.loadingHistory = false
		m.allEntries = nil
		m.candidates = nil
		m.displayEntries = nil
		m.invalidateHistoryCaches()

		return
	}

	m.loadingHistory = !done

	entries = m.prepareEntries(entries)
	m.allEntries = entries

	m.candidates = make([]string, len(entries))
	for i, e := range entries {
		m.candidates[i] = m.searchCandidate(e.Command)
	}

	m.lastQuery = ""
	m.lastEntryCount = len(m.candidates)
	m.updateMatches()
}

func (m *Model) loadEntries() {
	candidateCWD := ""
	if m.cwdMode {
		candidateCWD = m.cwd
	}

	entries, err := history.FetchCandidates(m.repo, history.CandidateOpts{
		Dedupe:     m.dedupe,
		FailFilter: m.failFilter,
		CWD:        candidateCWD,
	})

	m.applyLoadedEntries(entries, err, true)
}

func (m *Model) searchCandidate(command string) string {
	if !strings.ContainsAny(command, "\n\r\t") {
		return command
	}

	collapsed, _ := collapseMultiline(command, nil, m.cfg.Display.MultilineCollapse)

	return collapsed
}

func (m *Model) prepareEntries(entries []db.HistoryEntry) []db.HistoryEntry {
	if m.cfg.Display.HideMultiline {
		filtered := entries[:0:0]
		for _, e := range entries {
			if !strings.Contains(e.Command, "\n") {
				filtered = append(filtered, e)
			}
		}

		entries = filtered
	}

	return entries
}

func (m *Model) invalidateHistoryCaches() {
	m.lineCache = nil
	m.resultsCache = cachedResultsBlock{}
	m.regexCache = regexRenderCache{}
	m.searchRegex = regexRenderCache{}
	m.footerCache = footerCache{}
}

func (m *Model) updateMatches() {
	query := m.input.Value()

	cwdBonus := m.cfg.Display.CWDBoost
	if m.cwdMode {
		cwdBonus = 0
	}

	if query == "" {
		m.updateEmptyQueryMatches(query, cwdBonus)
		return
	}

	matches := m.matchCandidates(query)

	opts := history.DefaultScoringOpts(m.cwd)
	opts.CWDBonus = cwdBonus

	m.displayEntries = history.ScoreAndSortInto(m.displayEntries, m.allEntries, matches, opts)
	m.cursor = 0
	m.lineCache = nil

	m.resultsCache = cachedResultsBlock{}
	if m.mode != match.ModeRegex || query != m.regexCache.pattern {
		m.regexCache = regexRenderCache{}
	}

	if m.mode != match.ModeRegex || query != m.searchRegex.pattern {
		m.searchRegex = regexRenderCache{}
	}

	m.lastQuery = query
	m.lastMode = m.mode
	m.lastEntryCount = len(m.candidates)
}

func (m *Model) updateEmptyQueryMatches(query string, cwdBonus int) {
	opts := history.DefaultScoringOpts(m.cwd)
	opts.CWDBonus = cwdBonus

	scored := m.emptyQueryScoredEntries(opts)

	partitionCWD := opts.CWD != "" && opts.CWDBonus > opts.RecencyBase
	if !partitionCWD && opts.CWD != "" && opts.CWDBonus != 0 {
		sort.Sort(history.ScoredEntriesByScore(scored))
	}

	m.displayEntries = scored
	m.cursor = 0
	m.lineCache = nil
	m.resultsCache = cachedResultsBlock{}
	m.regexCache = regexRenderCache{}
	m.searchRegex = regexRenderCache{}
	m.lastQuery = query
	m.lastMode = m.mode
	m.lastEntryCount = len(m.candidates)
}

func (m *Model) emptyQueryScoredEntries(opts history.ScoringOpts) []history.ScoredEntry {
	scored := m.displayEntries[:0]
	if cap(scored) < len(m.allEntries) {
		scored = make([]history.ScoredEntry, 0, len(m.allEntries))
	}

	scored = scored[:len(m.allEntries)]
	partitionCWD := opts.CWD != "" && opts.CWDBonus > opts.RecencyBase
	cwdWrite := 0
	otherWrite := m.emptyQueryCWDPartitionStart(opts, partitionCWD)

	for i, e := range m.allEntries {
		scoredEntry := emptyQueryScoredEntry(e, i, opts)
		switch {
		case partitionCWD && e.Directory == opts.CWD:
			scored[cwdWrite] = scoredEntry
			cwdWrite++
		case partitionCWD:
			scored[otherWrite] = scoredEntry
			otherWrite++
		default:
			scored[i] = scoredEntry
		}
	}

	return scored
}

func (m *Model) emptyQueryCWDPartitionStart(opts history.ScoringOpts, partitionCWD bool) int {
	if !partitionCWD {
		return 0
	}

	count := 0

	for _, e := range m.allEntries {
		if e.Directory == opts.CWD {
			count++
		}
	}

	return count
}

func emptyQueryScoredEntry(e db.HistoryEntry, index int, opts history.ScoringOpts) history.ScoredEntry {
	score := 0
	if opts.CWD != "" && e.Directory == opts.CWD {
		score += opts.CWDBonus
	}

	recency := max(opts.RecencyBase-(index/modelRecencyIndexStep), 0)
	score += recency

	return history.ScoredEntry{
		Entry:      e,
		MatchInfo:  match.Match{Index: index, Score: 0, MatchedRanges: nil},
		FinalScore: score,
	}
}

func (m *Model) matchCandidates(query string) []match.Match {
	if m.canIncrementalFuzzyMatch(query) {
		indexes := m.indexBuf[:0]
		if cap(indexes) < len(m.displayEntries) {
			indexes = make([]int, 0, len(m.displayEntries))
		}

		for _, entry := range m.displayEntries {
			indexes = append(indexes, entry.MatchInfo.Index)
		}

		m.indexBuf = indexes

		m.matchBuf = (&match.FuzzyMatcher{}).MatchIndexedInto(query, m.candidates, indexes, m.matchBuf)

		return m.matchBuf
	}

	if m.mode == match.ModeFuzzy {
		m.matchBuf = (&match.FuzzyMatcher{}).MatchInto(query, m.candidates, m.matchBuf)

		return m.matchBuf
	}

	if m.mode == match.ModeRegex {
		if match.IsLiteralRegex(query) {
			m.matchBuf = match.MatchLiteralCommandsFold(query, m.candidates, m.matchBuf)

			return m.matchBuf
		}

		re := m.searchRegex.re
		if !m.searchRegex.valid || m.searchRegex.pattern != query {
			compiled, err := regexp.Compile("(?i)" + query)
			if err != nil {
				return nil
			}

			m.searchRegex = regexRenderCache{
				pattern: query,
				re:      compiled,
				valid:   true,
			}
			re = compiled
		}

		m.matchBuf = match.MatchRegexCommands(re, m.candidates, m.matchBuf)

		return m.matchBuf
	}

	if m.mode == match.ModeGlob {
		m.matchBuf = (&match.GlobMatcher{}).MatchInto(query, m.candidates, m.matchBuf)

		return m.matchBuf
	}

	matcher := match.New(m.mode)

	return matcher.Match(query, m.candidates)
}

func (m *Model) canIncrementalFuzzyMatch(query string) bool {
	return m.mode == match.ModeFuzzy &&
		m.lastMode == m.mode &&
		m.lastQuery != "" &&
		strings.HasPrefix(query, m.lastQuery) &&
		len(m.candidates) == m.lastEntryCount &&
		len(m.displayEntries) < len(m.candidates)
}

func (m *Model) handleNavigation(msg tea.KeyMsg) bool {
	switch {
	case matchKey(msg, m.cfg.Keys.Up) || matchKeyStr(msg, "ctrl+p"):
		if m.cursor > 0 {
			m.cursor--
		}
	case matchKey(msg, m.cfg.Keys.Down) || matchKeyStr(msg, "ctrl+n") || matchKeyStr(msg, "ctrl+r"):
		if m.cursor < len(m.displayEntries)-1 {
			m.cursor++
		}
	case matchKey(msg, m.cfg.Keys.PageUp):
		if m.cursor > 0 {
			m.cursor = max(m.cursor-m.pageSize(), 0)
		}
	case matchKey(msg, m.cfg.Keys.PageDown):
		if m.cursor < len(m.displayEntries)-1 {
			m.cursor = min(m.cursor+m.pageSize(), len(m.displayEntries)-1)
		}
	case matchKey(msg, m.cfg.Keys.Top):
		m.cursor = 0
	case matchKey(msg, m.cfg.Keys.Bottom):
		if len(m.displayEntries) > 0 {
			m.cursor = len(m.displayEntries) - 1
		}
	default:
		return false
	}

	return true
}

func (m *Model) pageSize() int {
	headerRows := resultsHeaderRows
	if m.height <= resultsHeaderRows {
		headerRows = 0
	}

	size := m.height - headerRows
	if size < 1 {
		return 1
	}

	return size
}

func (m *Model) handleModeSwitch(msg tea.KeyMsg) bool {
	switch {
	case matchKey(msg, m.cfg.Keys.ModeNext):
		m.mode = m.mode.Next(m.enabledModes)
	case matchKey(msg, m.cfg.Keys.ModeFuzzy) && m.cfg.Display.EnableFuzzy:
		m.mode = match.ModeFuzzy
	case matchKey(msg, m.cfg.Keys.ModeRegex) && m.cfg.Display.EnableRegex:
		m.mode = match.ModeRegex
	case matchKey(msg, m.cfg.Keys.ModeGlob) && m.cfg.Display.EnableGlob:
		m.mode = match.ModeGlob
	default:
		return false
	}

	m.updateMatches()

	return true
}

func (m *Model) handleToggle(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case matchKey(msg, m.cfg.Keys.ToggleCWD):
		m.cwdMode = !m.cwdMode
	case matchKey(msg, m.cfg.Keys.ToggleDedupe):
		m.dedupe = !m.dedupe
	case matchKey(msg, m.cfg.Keys.ToggleFails):
		m.failFilter = m.failFilter.Next()
	default:
		return nil, false
	}

	return m.startLoadingEntries(), true
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.dismissTransientViews() {
		return m, nil
	}

	if cmd, handled := m.handleControlKeys(msg); handled {
		return m, cmd
	}

	prevValue := m.input.Value()

	var cmd tea.Cmd

	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prevValue {
		m.updateMatches()
	}

	return m, cmd
}

func (m *Model) dismissTransientViews() bool {
	if m.showPreview {
		m.showPreview = false
		m.previewCommand = ""

		return true
	}

	if m.showHelp {
		m.showHelp = false
		return true
	}

	return false
}

func (m *Model) handleControlKeys(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case matchKey(msg, m.cfg.Keys.Help):
		m.showHelp = true
		return nil, true
	case matchKey(msg, m.cfg.Keys.Cancel) || matchKeyStr(msg, "ctrl+c"):
		m.quitting = true
		m.canceled = true

		return tea.Quit, true
	case matchKey(msg, m.cfg.Keys.Accept):
		if !m.acceptCurrentSelection() {
			return nil, true
		}

		m.quitting = true

		return tea.Quit, true
	case m.handleNavigation(msg):
		return nil, true
	case m.handleModeSwitch(msg):
		return nil, true
	case m.handlePreview(msg):
		return nil, true
	default:
		return m.handleToggle(msg)
	}
}

func (m *Model) acceptCurrentSelection() bool {
	if cmd, ok := m.currentResultCommand(); ok {
		m.selected = cmd
		return true
	}

	if m.loadingHistory || !m.historyComplete {
		return false
	}

	// If nothing is selected from history, accept the currently typed command.
	m.selected = m.input.Value()

	return true
}

func (m *Model) handlePreview(msg tea.KeyMsg) bool {
	if !matchKey(msg, m.cfg.Keys.PreviewCommand) {
		return false
	}

	m.showCurrentPreview()

	return true
}

func (m *Model) currentResultCommand() (string, bool) {
	if len(m.displayEntries) == 0 {
		return "", false
	}

	if m.cursor < 0 || m.cursor >= len(m.displayEntries) {
		return "", false
	}

	return m.displayEntries[m.cursor].Entry.Command, true
}

func matchKey(msg tea.KeyMsg, spec string) bool {
	return matchKeyStr(msg, spec)
}

func matchKeyStr(msg tea.KeyMsg, spec string) bool {
	return msg.String() == spec
}
