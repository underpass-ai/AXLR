package tokens

import (
	"strings"
	"testing"
)

func TestCounterCountsAndRemembers(t *testing.T) {
	counter, err := New()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("The console counts prompt tokens with one tokenizer. ", 50)
	n := counter.Count(text)
	if n < 400 || n > 700 {
		t.Fatalf("tokens %d", n)
	}
	if counter.Count(text) != n || counter.Count("") != 0 || len(counter.memo) != 1 {
		t.Fatalf("memo %d", len(counter.memo))
	}
	if counter.Count("hola, ¿qué tal?") == 0 {
		t.Fatal("unicode text counts")
	}
}
