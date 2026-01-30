package tui

import (
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

type Model struct {
	input          textinput.Model
	cfg            config.Config
	styles         Styles
	allEntries     []db.HistoryEntry
	candidates     []string
	displayEntries []history.ScoredEntry
	cursor         int
	width          int
	height         int
	maxHeight      int
	selected       string
	mode           match.Mode
	enabledModes   []match.Mode
	cwdMode        bool
	dedupe         bool
	onlyFails      bool
	cwd            string
	homeDir        string
	quitting       bool
	cancelled      bool
	showHelp       bool
	showPreview    bool
	previewCommand string
	repo           *db.HistoryRepo
	dbError        error
}

func NewModel(cfg config.Config, repo *db.HistoryRepo, cwd string, homeDir string, height int, cwdMode bool, initialQuery string) Model {
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
	if cfg.Display.EnableRegex {
		enabledModes = append(enabledModes, match.ModeRegex)
	}
	if cfg.Display.EnableGlob {
		enabledModes = append(enabledModes, match.ModeGlob)
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

	m := Model{
		input:        ti,
		cfg:          cfg,
		styles:       NewStyles(cfg.Theme),
		width:        width,
		height:       height,
		maxHeight:    height,
		mode:         initialMode,
		enabledModes: enabledModes,
		cwdMode:      cwdMode,
		dedupe:       true,
		cwd:          cwd,
		homeDir:      homeDir,
		repo:         repo,
	}
	m.loadEntries()
	return m
}

func (m *Model) loadEntries() {
	entries, err := history.FetchCandidates(m.repo, history.CandidateOpts{
		Limit:     10000,
		Dedupe:    m.dedupe,
		OnlyFails: m.onlyFails,
	})
	m.dbError = err
	if err != nil {
		m.allEntries = nil
		m.candidates = nil
		m.displayEntries = nil
		return
	}
	if m.cwdMode && m.cwd != "" {
		filtered := entries[:0:0]
		for _, e := range entries {
			if e.Directory == m.cwd {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	if m.cfg.Display.HideMultiline {
		filtered := entries[:0:0]
		for _, e := range entries {
			if !strings.Contains(e.Command, "\n") {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	m.allEntries = entries
	m.candidates = make([]string, len(entries))
	for i, e := range entries {
		m.candidates[i] = e.Command
	}
	m.updateMatches()
}

func (m *Model) updateMatches() {
	query := m.input.Value()
	cwdBonus := m.cfg.Display.CWDBoost
	if m.cwdMode {
		cwdBonus = 0
	}
	if query == "" {
		opts := history.DefaultScoringOpts(m.cwd)
		opts.CWDBonus = cwdBonus
		scored := make([]history.ScoredEntry, len(m.allEntries))
		for i, e := range m.allEntries {
			score := 0
			if opts.CWD != "" && e.Directory == opts.CWD {
				score += opts.CWDBonus
			}
			recency := max(opts.RecencyBase-(i/100), 0)
			score += recency
			scored[i] = history.ScoredEntry{
				Entry:      e,
				MatchInfo:  match.Match{Index: i},
				FinalScore: score,
			}
		}
		sort.SliceStable(scored, func(a, b int) bool {
			return scored[a].FinalScore > scored[b].FinalScore
		})
		m.displayEntries = scored
		m.cursor = 0
		return
	}

	matcher := match.New(m.mode)
	matches := matcher.Match(query, m.candidates)

	opts := history.DefaultScoringOpts(m.cwd)
	opts.CWDBonus = cwdBonus

	m.displayEntries = history.ScoreAndSort(m.allEntries, matches, opts)
	m.cursor = 0
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
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
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showPreview {
		m.showPreview = false
		m.previewCommand = ""
		return m, nil
	}

	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	switch {
	case matchKey(msg, m.cfg.Keys.Help):
		m.showHelp = true
		return m, nil

	case matchKey(msg, m.cfg.Keys.Cancel) || matchKeyStr(msg, "ctrl+c"):
		m.quitting = true
		m.cancelled = true
		return m, tea.Quit

	case matchKey(msg, m.cfg.Keys.Accept):
		if len(m.displayEntries) > 0 && m.cursor < len(m.displayEntries) {
			m.selected = m.displayEntries[m.cursor].Entry.Command
		}
		m.quitting = true
		return m, tea.Quit

	case matchKey(msg, m.cfg.Keys.Up) || matchKeyStr(msg, "ctrl+p"):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case matchKey(msg, m.cfg.Keys.Down) || matchKeyStr(msg, "ctrl+n"):
		if m.cursor < len(m.displayEntries)-1 {
			m.cursor++
		}
		return m, nil

	case matchKey(msg, m.cfg.Keys.PageUp):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case matchKey(msg, m.cfg.Keys.PageDown):
		if m.cursor < len(m.displayEntries)-1 {
			m.cursor++
		}
		return m, nil

	case matchKey(msg, m.cfg.Keys.Top):
		m.cursor = 0
		return m, nil

	case matchKey(msg, m.cfg.Keys.Bottom):
		if len(m.displayEntries) > 0 {
			m.cursor = len(m.displayEntries) - 1
		}
		return m, nil

	case matchKey(msg, m.cfg.Keys.ModeNext):
		m.mode = m.mode.Next(m.enabledModes)
		m.updateMatches()
		return m, nil

	case matchKey(msg, m.cfg.Keys.ModeFuzzy):
		if m.cfg.Display.EnableFuzzy {
			m.mode = match.ModeFuzzy
			m.updateMatches()
		}
		return m, nil

	case matchKey(msg, m.cfg.Keys.ModeRegex):
		if m.cfg.Display.EnableRegex {
			m.mode = match.ModeRegex
			m.updateMatches()
		}
		return m, nil

	case matchKey(msg, m.cfg.Keys.ModeGlob):
		if m.cfg.Display.EnableGlob {
			m.mode = match.ModeGlob
			m.updateMatches()
		}
		return m, nil

	case matchKey(msg, m.cfg.Keys.ToggleCWD):
		m.cwdMode = !m.cwdMode
		m.loadEntries()
		return m, nil

	case matchKey(msg, m.cfg.Keys.ToggleDedupe):
		m.dedupe = !m.dedupe
		m.loadEntries()
		return m, nil

	case matchKey(msg, m.cfg.Keys.ToggleFails):
		m.onlyFails = !m.onlyFails
		m.loadEntries()
		return m, nil

	case matchKey(msg, m.cfg.Keys.PreviewCommand):
		if m.cfg.Display.MultilinePreview == "popup" && len(m.displayEntries) > 0 && m.cursor < len(m.displayEntries) {
			cmd := m.displayEntries[m.cursor].Entry.Command
			if strings.Contains(cmd, "\n") {
				m.showPreview = true
				m.previewCommand = cmd
			}
		}
		return m, nil
	}

	prevValue := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prevValue {
		m.updateMatches()
	}
	return m, cmd
}

func (m Model) Selected() string {
	return m.selected
}

func (m Model) Cancelled() bool {
	return m.cancelled
}

func matchKey(msg tea.KeyMsg, spec string) bool {
	return matchKeyStr(msg, spec)
}

func matchKeyStr(msg tea.KeyMsg, spec string) bool {
	return msg.String() == spec
}
