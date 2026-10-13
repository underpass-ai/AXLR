package application

import (
	"context"
	"errors"
	"strings"
	"time"
)

// /memory (decisions A4, D7, E15 and I3 of the lifecycle integration
// design): the person's three memory commands. save exports the project
// about as the repository's bundle, compare reads KMP's proposals between
// the session's about and others, forget closes an entry with a
// supersedes relation and never removes it. The person has no write
// command: nothing reaches the project about without evidence.

// BundleSaved is what /memory save wrote.
type BundleSaved struct {
	Path   string
	About  string
	Events int
}

// MemoryBundlePort exports one about as the checkout's bundle.
type MemoryBundlePort interface {
	Save(ctx context.Context, about string) (BundleSaved, error)
}

// CompareRequest asks for KMP's proposals between the session's about and
// the others named, in the session's context.
type CompareRequest struct {
	About     string
	Others    []string
	ContextID string
}

// CompareResult is kmp_relate's reading, compact: the relations declared
// between the abouts, the proposals KMP makes from shared coordinates, and
// how many entries it read per about.
type CompareResult struct {
	Abouts    []string
	Declared  []MemoryRelation
	Proposals []MemoryRelation
	Facts     map[string]int
}

// MemoryRelation is one stored or proposed relation between two refs.
type MemoryRelation struct {
	From, To, Rel, Why string
	// Dimension and Scope name the coordinate a proposal rests on.
	Dimension, Scope string
}

// MemoryComparePort reads comparisons; it writes nothing.
type MemoryComparePort interface {
	Compare(ctx context.Context, request CompareRequest) (CompareResult, error)
}

// ForgetRequest closes one entry: the ref, the person's reason, and what
// they typed, kept as evidence with the time.
type ForgetRequest struct {
	Ref, Why  string
	Command   string
	At        time.Time
	Labels    map[string][]string
	ContextID string
}

// ForgetReceipt says what was closed, its text, and the entry that
// supersedes it.
type ForgetReceipt struct {
	About, Ref, Text, Superseding string
}

// MemoryForgetPort closes an entry by superseding it.
type MemoryForgetPort interface {
	Forget(ctx context.Context, request ForgetRequest) (ForgetReceipt, error)
}

// ForgetWhy is the reason recorded when the person gives none.
const ForgetWhy = "the person asked the console to forget it"

// RefAbout reads the about a memory ref belongs to: KMP returns refs as
// <about>:entry:<kind>:<slug>. The ref itself is copied, never built.
func RefAbout(ref string) (string, error) {
	about, _, found := strings.Cut(strings.TrimSpace(ref), ":entry:")
	if !found || about == "" {
		return "", errors.New("a memory ref reads <about>:entry:<kind>:<slug>, as the memory engine returns it")
	}
	return about, nil
}
