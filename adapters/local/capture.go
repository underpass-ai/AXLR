package local

import (
	"bytes"
	"strings"
	"sync"
	"unicode/utf8"
)

type capture struct {
	mu                   sync.Mutex
	budget               int
	stdout, stderr       bytes.Buffer
	stdoutCut, stderrCut bool
	captured             int
	discarded            int64
}

// finish returns both streams as text with the final counts. The output limit
// can stop a stream inside a character; those trailing bytes are counted as
// discarded instead of becoming a U+FFFD the program never wrote.
func (c *capture) finish() (stdout, stderr string, captured int, discarded int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stdout = c.text(c.stdout.Bytes(), c.stdoutCut)
	stderr = c.text(c.stderr.Bytes(), c.stderrCut)
	return stdout, stderr, c.captured, c.discarded
}

func (c *capture) text(data []byte, cut bool) string {
	if cut {
		partial := partialRuneLen(data)
		data = data[:len(data)-partial]
		c.captured -= partial
		c.discarded += int64(partial)
	}
	return strings.ToValidUTF8(string(data), "�")
}

// partialRuneLen is the length of an incomplete UTF-8 sequence ending data.
func partialRuneLen(data []byte) int {
	for i := len(data) - 1; i >= 0 && i > len(data)-utf8.UTFMax; i-- {
		if utf8.RuneStart(data[i]) {
			if utf8.FullRune(data[i:]) {
				return 0
			}
			return len(data) - i
		}
	}
	return 0
}
