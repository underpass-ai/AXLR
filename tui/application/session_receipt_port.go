package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// SessionReceiptPort keeps each session's receipt journal: its model
// requests and tool calls as they start and end, in order, durable before
// the next step, so the execution's receipt survives a restart.
type SessionReceiptPort interface {
	AppendReceipt(ctx context.Context, id domain.SessionID, entry domain.ReceiptEntry) error
	LoadReceipt(ctx context.Context, id domain.SessionID) ([]domain.ReceiptEntry, error)
}

// recordReceipt journals one entry of the session's receipt. An entry that
// cannot be written loses one line of the receipt, never the turn, as a
// ledger that cannot be saved does: the receipt then says it is not
// complete.
func (u ContinueTurnUseCase) recordReceipt(ctx context.Context, session domain.Session, entry domain.ReceiptEntry) {
	if u.Receipts == nil {
		return
	}
	entry.At = entry.At.UTC()
	_ = u.Receipts.AppendReceipt(context.WithoutCancel(ctx), session.ID(), entry)
}
