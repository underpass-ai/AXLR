package mcpclient

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestCloseErrorProcessDone(t *testing.T) {
	for _, err := range []error{os.ErrProcessDone, fmt.Errorf("signal: %w", os.ErrProcessDone)} {
		if got := closeError("server", err); got != nil {
			t.Errorf("closeError(%v) = %v, want nil", err, got)
		}
	}
	other := errors.New("other close failure")
	if got := closeError("server", other); !errors.Is(got, other) {
		t.Errorf("closeError(%v) = %v, want wrapped error", other, got)
	}
}
