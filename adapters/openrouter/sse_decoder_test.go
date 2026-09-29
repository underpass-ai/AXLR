package openrouter

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestSSEFramesAcrossSingleBytes(t *testing.T) {
	d := newSSEDecoder(iotest.OneByteReader(strings.NewReader(": ping\r\ndata: {\"text\":\r\ndata: \"é🌍\"}\r\n\r\ndata: [DONE]\n\n")))
	for _, want := range []string{"{\"text\":\n\"é🌍\"}", "[DONE]"} {
		got, err := d.Next()
		if err != nil || string(got) != want {
			t.Fatalf("event = %q, %v; want %q", got, err, want)
		}
	}
	if _, err := d.Next(); err != io.EOF {
		t.Fatalf("end = %v", err)
	}
}

func TestSSERejectsOversizedAndTruncatedEvents(t *testing.T) {
	for _, body := range []string{"data: unfinished", "data: x\n", "data: " + strings.Repeat("x", 1024*1024) + "\n\n", strings.Repeat(": comment\n", 120000) + "\n"} {
		if _, err := newSSEDecoder(strings.NewReader(body)).Next(); err == nil {
			t.Fatal("invalid event accepted")
		}
	}
}

func TestSSEAggregateLimitIncludesComments(t *testing.T) {
	d := newSSEDecoder(strings.NewReader(strings.Repeat(": "+strings.Repeat("x", 1020)+"\n\n", 8300)))
	if _, err := d.Next(); err == nil || err == io.EOF {
		t.Fatalf("aggregate limit = %v", err)
	}
}

func TestSSECarriageReturnLineEndings(t *testing.T) {
	d := newSSEDecoder(iotest.OneByteReader(strings.NewReader("data: first\r\rdata: second\r\r")))
	for _, want := range []string{"first", "second"} {
		got, err := d.Next()
		if err != nil || string(got) != want {
			t.Fatalf("event=%q err=%v", got, err)
		}
	}
}
