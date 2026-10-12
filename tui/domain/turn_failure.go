package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxTurnFailureRunes bounds the message a turn failure keeps.
const MaxTurnFailureRunes = 600

// TurnFailure is a turn of the session that failed, most often its model
// request, as the person sees it in the transcript after the turn: the
// error on one line and what they can do about it. On 12 October 2026 an invalid model id left a
// prompt with no reply and no reason once the footer had moved on, and
// reopening the session showed nothing either. It is a console line, never
// part of the model's history.
type TurnFailure struct {
	Message string
	// Hint names what the person can do, as the provider adapter
	// classifies it ("choose_model", "check_key", "credits",
	// "retry_later"); empty for nothing. The console words it in the
	// person's language when it draws the line.
	Hint string
}

func (f TurnFailure) validate() error {
	if strings.TrimSpace(f.Message) == "" || strings.ContainsAny(f.Message, "\r\n") || utf8.RuneCountInString(f.Message) > MaxTurnFailureRunes {
		return errors.New("a turn failure needs a message of one line")
	}
	return nil
}

// AddTurnFailure records a failed turn as a console line after the
// session's last message, between turns.
func (s *Session) AddTurnFailure(failure TurnFailure) error {
	if err := s.requireBetweenTurns("console lines"); err != nil {
		return err
	}
	if err := failure.validate(); err != nil {
		return err
	}
	line := ConsoleLine{AfterMessage: len(s.state.Messages), At: now().UTC().Truncate(time.Second), Failure: &failure}
	s.state.ConsoleLines = append(s.state.ConsoleLines, line.clone())
	return nil
}
