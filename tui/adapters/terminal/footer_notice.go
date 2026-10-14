package terminal

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// The footer's notice and error. On 14 October 2026 an error stayed until
// another replaced it and hid what came after it: a failed restore's error
// sat under Ctrl+D's next "y restores?" question, and after an editor
// error the second Ctrl+C's "press Ctrl+C again to quit" never showed, so
// a third press would have quit unwarned. A notice ("Exported to …") stayed
// for minutes across turns. A new notice or question now displaces an
// error that an earlier update left, a notice shows beside an error that
// stays, and a notice goes after noticeLife.

// noticeLife is how long a notice stays in the footer without a key press.
const noticeLife = 10 * time.Second

// noticeTickEvery is how often an idle console looks for a notice to end.
// The tick is armed once, in Init, and rearms itself: a tick returned from
// every update that raised a notice would make each followed command wait
// for it.
const noticeTickEvery = 2 * time.Second

// noticeTick looks for a notice past its life.
type noticeTick struct{}

func nextNoticeTick() tea.Cmd {
	return tea.Tick(noticeTickEvery, func(time.Time) tea.Msg { return noticeTick{} })
}

// footerBefore is what the footer said before an update.
type footerBefore struct {
	notice, err, changesNotice string
	asking                     bool
}

func (m AppModel) footerBefore() footerBefore {
	return footerBefore{notice: m.Status.Notice, err: m.Status.Error, changesNotice: m.Changes.notice, asking: m.Changes.confirm != ""}
}

// settleFooter lets a notice or question this update raised displace the
// error an earlier one left, and dates the notice.
func (m AppModel) settleFooter(before footerBefore, msg tea.Msg) AppModel {
	_, pressed := msg.(tea.KeyPressMsg)
	raised := m.Status.Notice != "" && (m.Status.Notice != before.notice || pressed)
	asked := m.Changes.confirm != "" && !before.asking || m.Changes.notice != "" && m.Changes.notice != before.changesNotice
	if (raised || asked) && m.Status.Error != "" && m.Status.Error == before.err {
		m.clearStaleError()
	}
	if raised {
		m.noticeAt = time.Now()
	}
	return m
}

// noticeLasts reports a notice no time ends: a question that waits for
// its answer (/undo's y) and the budget warning stay until a key.
func (m AppModel) noticeLasts() bool {
	return m.undoAsking || m.Status.Notice == m.lastingNotice
}

// visibleNotice is the footer's notice, or "" once its life is over; a
// redraw during a turn drops it before the tick does.
func (m AppModel) visibleNotice() string {
	if m.Status.Notice == "" || m.noticeLasts() || m.noticeAt.IsZero() || time.Since(m.noticeAt) < noticeLife {
		return m.Status.Notice
	}
	return ""
}

// noticeTicked ends a notice past its life and rearms the tick.
func (m AppModel) noticeTicked() (AppModel, tea.Cmd) {
	if m.quitting {
		return m, nil
	}
	if m.visibleNotice() == "" {
		m.Status.Notice = ""
	}
	return m, nextNoticeTick()
}
