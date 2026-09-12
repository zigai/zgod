package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zigai/zgod/internal/config"
)

func TestInterruptBypassesTransientViews(t *testing.T) {
	for _, overlay := range []string{"help", "preview", "none"} {
		t.Run(overlay, func(t *testing.T) {
			m := NewModel(config.Default(), nil, "", "", 10, false, "")
			m.showHelp = overlay == "help"
			m.showPreview = overlay == "preview"

			_, command := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
			if command == nil || !m.Interrupted() || !m.Canceled() || m.View() != "" {
				t.Fatalf("Ctrl-C did not terminate %s: %+v", overlay, m)
			}

			if _, ok := command().(tea.QuitMsg); !ok {
				t.Fatal("Ctrl-C did not issue quit")
			}
		})
	}
}

func TestNarrowTerminalShowsResizeNotice(t *testing.T) {
	m := NewModel(config.Default(), nil, "", "", 10, false, "")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 22})

	view := m.View()
	if !strings.Contains(view, "Widen terminal") || lipgloss.Width(view) > 30 {
		t.Fatalf("narrow viewport=%q", view)
	}

	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if strings.Contains(m.View(), "Widen terminal") {
		t.Fatal("resize notice remained after widening")
	}
}

func TestEOFAndViewerQuitPreserveSearchTyping(t *testing.T) {
	m := NewModel(config.Default(), nil, "", "", 10, false, "")

	_, command := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if command == nil || !m.Canceled() || m.Interrupted() {
		t.Fatal("empty Ctrl-D must cancel without SIGINT")
	}

	m = NewModel(config.Default(), nil, "", "", 10, false, "")

	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if m.quitting || m.input.Value() != "q" {
		t.Fatal("q must remain searchable text")
	}

	m.showHelp = true

	_, command = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if command == nil || !m.Canceled() {
		t.Fatal("q must exit a viewer")
	}
}
