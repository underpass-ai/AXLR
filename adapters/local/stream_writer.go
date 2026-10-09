package local

type streamWriter struct {
	c      *capture
	stderr bool
}

func (w streamWriter) Write(p []byte) (int, error) {
	w.c.mu.Lock()
	defer w.c.mu.Unlock()
	take := len(p)
	available := w.c.budget - w.c.captured
	if take > available {
		take = available
	}
	if take < 0 {
		take = 0
	}
	if w.stderr {
		_, _ = w.c.stderr.Write(p[:take])
		w.c.stderrCut = w.c.stderrCut || take < len(p)
	} else {
		_, _ = w.c.stdout.Write(p[:take])
		w.c.stdoutCut = w.c.stdoutCut || take < len(p)
	}
	w.c.captured += take
	w.c.discarded += int64(len(p) - take)
	return len(p), nil
}
