package terminal

import (
	"sync"

	root "github.com/underpass-ai/AXLR/domain"
)

// steerQueue holds what the person wrote while an operation ran. The console
// adds to it and the running turn takes it between model steps, so it is
// shared by pointer and guarded.
type steerQueue struct {
	mu   sync.Mutex
	text string
}

func (q *steerQueue) Add(message string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.text == "" {
		q.text = message
		return
	}
	q.text += "\n\n" + message
}

func (q *steerQueue) Peek() string {
	if q == nil {
		return ""
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.text
}

func (q *steerQueue) Take() (root.Text, bool) {
	if q == nil {
		return "", false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	text := q.text
	q.text = ""
	return root.Text(text), text != ""
}

// Restore puts back text the turn could not keep, ahead of anything written
// since.
func (q *steerQueue) Restore(text root.Text) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.text == "" {
		q.text = string(text)
		return
	}
	q.text = string(text) + "\n\n" + q.text
}
