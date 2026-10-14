package application

import "context"

// JournalPort writes a turn's results to KMP and returns the stored ref;
// RecordSettled writes what Jev selected from a turn (session_settle.go).
type JournalPort interface {
	RecordTurn(ctx context.Context, entry TurnEntry) (string, error)
	RecordSettled(ctx context.Context, entry SettledEntry) (string, error)
}
