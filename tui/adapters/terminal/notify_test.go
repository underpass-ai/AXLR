package terminal

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// raws runs a command, and a batch's commands, and returns what they
// write to the terminal.
func raws(cmd tea.Cmd) []string {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.RawMsg:
		return []string{fmt.Sprint(msg.Msg)}
	case tea.BatchMsg:
		var out []string
		for _, c := range msg {
			out = append(out, raws(c)...)
		}
		return out
	}
	return nil
}

func rings(cmd tea.Cmd) int {
	n := 0
	for _, raw := range raws(cmd) {
		if strings.HasSuffix(raw, "\a") && !strings.HasPrefix(raw, "\x1b]2;") {
			n++
		}
	}
	return n
}

// The setting chooses a bell, a desktop notification with the bell, or
// nothing.
func TestRingFollowsTheNotifySetting(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	if got := raws(m.ring("x")); len(got) != 1 || got[0] != "\a" {
		t.Fatalf("bell: %q", got)
	}
	m.deps.Notify = NotifyOSC
	if got := raws(m.ring("the turn ended")); len(got) != 1 || got[0] != "\x1b]9;AXLR: the turn ended\a\a" {
		t.Fatalf("osc: %q", got)
	}
	m.deps.Notify = NotifyOff
	if m.ring("x") != nil {
		t.Fatal("off rang")
	}
}

// An approval card rings when it appears, once; the next call's card rings
// again; a turn of ten seconds or more rings at its end, a short one does
// not.
func TestCardsAndLongTurnsRing(t *testing.T) {
	m := approvalModel(t)
	defer m.zones.Close()
	if m.lastAttention != "call:one" {
		t.Fatalf("attention %q", m.lastAttention)
	}
	if _, cmd := m.signal(nil); rings(cmd) != 0 {
		t.Fatal("the same card rang twice")
	}
	m.lastAttention = ""
	if _, cmd := m.signal(nil); rings(cmd) != 1 {
		t.Fatal("a new card did not ring")
	}
	long := sized()
	defer long.zones.Close()
	for _, tc := range []struct {
		took  time.Duration
		rings int
	}{{11 * time.Second, 1}, {3 * time.Second, 0}} {
		long.Busy, long.operationID, long.providerWaiting = true, long.operationID+1, true
		long.turnStarted = time.Now().Add(-tc.took)
		_, cmd := long.Update(operationComplete{ID: long.operationID, Session: *long.deps.Session})
		if got := rings(cmd); got != tc.rings {
			t.Fatalf("a %v turn rang %d times", tc.took, got)
		}
	}
}

// Once the title is saved the console keeps it at "AXLR · <state> ·
// <session>" and puts the old one back when it quits.
func TestTheWindowTitleFollowsTheSession(t *testing.T) {
	m := approvalModel(t)
	defer m.zones.Close()
	if m.View().WindowTitle != "" {
		t.Fatal("the title was set before the old one was saved")
	}
	next, _ := m.Update(titleReadyMsg{})
	m = next.(AppModel)
	if got := m.View().WindowTitle; got != "AXLR · waiting for you · write" {
		t.Fatalf("title %q", got)
	}
	m.Theme.Locale = Spanish
	if got := m.windowTitle(); got != "AXLR · te espera · write" {
		t.Fatalf("spanish %q", got)
	}
	next, cmd := m.quit()
	if m = next.(AppModel); !m.quitting || cmd == nil || m.View().WindowTitle != "" {
		t.Fatal("quit did not restore the title")
	}
}
