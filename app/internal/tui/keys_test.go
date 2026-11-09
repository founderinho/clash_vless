package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestLatinKey(t *testing.T) {
	for in, want := range map[rune]string{'ф': "a", 'ы': "s", 'й': "q", 'Н': "Y", 'a': "a", '3': "3", 'ё': "ё"} {
		if got := latinKey(runeKey(in)).String(); got != want {
			t.Errorf("latinKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInKeyBurst(t *testing.T) {
	m := &model{}
	t0 := time.Now()
	if m.inKeyBurst(runeKey('v'), t0) {
		t.Error("first key flagged as a burst")
	}
	if !m.inKeyBurst(runeKey('e'), t0.Add(time.Millisecond)) {
		t.Error("pasted key not flagged")
	}
	if !m.inKeyBurst(tea.KeyMsg{Type: tea.KeyEnter}, t0.Add(2*time.Millisecond)) {
		t.Error("pasted newline not flagged")
	}
	if m.inKeyBurst(runeKey('a'), t0.Add(200*time.Millisecond)) {
		t.Error("typed key flagged as a burst")
	}
	if m.inKeyBurst(tea.KeyMsg{Type: tea.KeyDown}, t0.Add(201*time.Millisecond)) {
		t.Error("arrow key flagged as a burst")
	}
}
