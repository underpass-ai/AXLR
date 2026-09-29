package openrouter

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

const maxSSEEventBytes = 1024 * 1024

// sseDecoder bounds both event framing and the entire response, including comments.
type sseDecoder struct {
	scanner *bufio.Scanner
	total   int
}

func newSSEDecoder(reader io.Reader) *sseDecoder {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxSSEEventBytes+1)
	scanner.Split(splitSSELine)
	return &sseDecoder{scanner: scanner}
}

func (d *sseDecoder) Next() ([]byte, error) {
	var data []byte
	eventBytes := 0
	hasData := false
	for d.scanner.Scan() {
		line := d.scanner.Bytes()
		d.total += len(line)
		eventBytes += len(line)
		if d.total > maxResponseBytes {
			return nil, errors.New("OpenRouter stream exceeds 8 MiB")
		}
		if eventBytes > maxSSEEventBytes {
			return nil, errors.New("OpenRouter SSE event exceeds 1 MiB")
		}
		if len(line) > 0 && line[len(line)-1] != '\r' && line[len(line)-1] != '\n' {
			return nil, io.ErrUnexpectedEOF
		}
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 {
			if hasData {
				return data, nil
			}
			eventBytes = 0
		} else if bytes.Equal(line, []byte("data")) || bytes.HasPrefix(line, []byte("data:")) {
			value := []byte(nil)
			if len(line) > 4 {
				value = bytes.TrimPrefix(line[5:], []byte{' '})
			}
			if hasData {
				data = append(data, '\n')
			}
			data = append(data, value...)
			hasData = true
		}
	}
	if err := d.scanner.Err(); err != nil {
		return nil, err
	}
	if eventBytes > 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return nil, io.EOF
}

// Keep delimiters so byte limits include framing, and accept all SSE line endings.
func splitSSELine(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' {
			return i + 1, data[:i+1], nil
		}
		if b == '\r' {
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			n := i + 1
			if n < len(data) && data[n] == '\n' {
				n++
			}
			return n, data[:n], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
