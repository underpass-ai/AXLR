// Package tokens counts prompt tokens with tiktoken's o200k encoding, the
// single measure of the console's context cap for every model (decision
// D10 of the lifecycle integration design): a provider's own count may
// differ by a tenth or a fifth, and the cap absorbs it.
package tokens

import (
	"crypto/sha256"
	"sync"

	"github.com/tiktoken-go/tokenizer"
	"github.com/underpass-ai/AXLR/tui/application"
)

// memoEntries bounds the memo: messages are immutable, so a count is
// taken once per content; the map is cleared when it grows past this.
const memoEntries = 8192

// Counter counts o200k tokens and remembers each text's count by digest.
type Counter struct {
	mu    sync.Mutex
	codec tokenizer.Codec
	memo  map[[32]byte]int
	err   error
}

var _ application.TokenCounterPort = (*Counter)(nil)

// New loads the o200k encoding (about 8 ms, measured on 13 October 2026).
func New() (*Counter, error) {
	codec, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return nil, err
	}
	return &Counter{codec: codec, memo: map[[32]byte]int{}}, nil
}

// Count is the text's token count; a text the codec cannot encode counts
// as a quarter of its bytes, the conservative side.
func (c *Counter) Count(text string) int {
	if text == "" {
		return 0
	}
	key := sha256.Sum256([]byte(text))
	c.mu.Lock()
	if n, ok := c.memo[key]; ok {
		c.mu.Unlock()
		return n
	}
	c.mu.Unlock()
	n, err := c.codec.Count(text)
	if err != nil {
		n = (len(text) + 3) / 4
	}
	c.mu.Lock()
	if len(c.memo) >= memoEntries {
		c.memo = map[[32]byte]int{}
	}
	c.memo[key] = n
	c.mu.Unlock()
	return n
}
