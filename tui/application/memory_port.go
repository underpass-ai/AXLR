package application

import "context"

// MemoryPort is KMP as the ceremony driver sees it.
type MemoryPort interface {
	// Wake returns bounded context for the about, or "" when it has none yet.
	Wake(ctx context.Context, about string) (string, error)
	// WakeFocused is Wake with an intent the store may focus the recall on;
	// it also returns the refs the recall exposed, which links may name.
	WakeFocused(ctx context.Context, about, intent string) (text string, refs []string, err error)
	Record(ctx context.Context, about string, labels map[string][]string, id, summary, evidence string) error
	// RecordLinked writes one memory with its relations to existing refs and
	// returns the stored ref, so later memories can link to it exactly.
	RecordLinked(ctx context.Context, about string, labels map[string][]string, record MemoryRecord) (ref string, err error)
}
