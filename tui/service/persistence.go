package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// sessionStore adds monotone service revisions without changing the TUI use cases.
// Every application Save, including checkpoints before effects, passes this adapter.
type sessionStore struct {
	mu   sync.Mutex
	next interface {
		Save(context.Context, domain.Session) error
		Load(context.Context, domain.SessionID) (domain.Session, error)
		List(context.Context) ([]domain.SessionSummary, error)
	}
}

func (s *sessionStore) Save(ctx context.Context, session domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := session.Export()
	old, err := s.next.Load(ctx, state.ID)
	if err == nil {
		prior := old.Export()
		if prior.Owner != "" && state.Owner != prior.Owner {
			return errors.New("session owner changed")
		}
		session.SetServiceMetadata(state.Owner, prior.Revision+1, state.OperationID)
	} else if errors.Is(err, os.ErrNotExist) {
		session.SetServiceMetadata(state.Owner, 1, state.OperationID)
	} else {
		return err
	}
	return s.next.Save(ctx, session)
}

func (s *sessionStore) Load(ctx context.Context, id domain.SessionID) (domain.Session, error) {
	return s.next.Load(ctx, id)
}

func (s *sessionStore) List(ctx context.Context) ([]domain.SessionSummary, error) {
	return s.next.List(ctx)
}

type Event struct {
	SessionID   string          `json:"session_id"`
	OperationID string          `json:"operation_id"`
	Sequence    uint64          `json:"sequence"`
	Time        time.Time       `json:"time"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
}

// eventStore uses length+CRC framing. Recovery removes only an incomplete or
// corrupt tail; a caller never sees an event before fsync completes.
type eventStore struct {
	mu      sync.Mutex
	dir     string
	changed map[string]chan struct{}
}

func newEventStore(dir string) (*eventStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &eventStore{dir: dir, changed: map[string]chan struct{}{}}, nil
}

func (s *eventStore) path(id string) string { return filepath.Join(s.dir, id+".events") }

func (s *eventStore) readLocked(id string) ([]Event, error) {
	f, err := os.OpenFile(s.path(id), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(f)
	var events []Event
	var offset int64
	truncatedTail := false
	for {
		var header [8]byte
		_, err := io.ReadFull(reader, header[:])
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			truncatedTail = true
			break
		}
		if err != nil {
			return nil, err
		}
		n := binary.BigEndian.Uint32(header[:4])
		if n == 0 || n > 4<<20 {
			return nil, errors.New("event journal has an invalid frame length")
		}
		body := make([]byte, n)
		if _, err = io.ReadFull(reader, body); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				truncatedTail = true
				break
			}
			return nil, err
		}
		if crc32.ChecksumIEEE(body) != binary.BigEndian.Uint32(header[4:]) {
			return nil, errors.New("event journal checksum mismatch")
		}
		var event Event
		if err = json.Unmarshal(body, &event); err != nil || event.Sequence != uint64(len(events)+1) || event.SessionID != id {
			return nil, errors.New("event journal contains an invalid event")
		}
		events = append(events, event)
		offset += int64(8 + n)
	}
	if truncatedTail {
		// An inflated length in an interior header can make a later valid
		// record look like a partial tail. Preserve the file for inspection.
		if later, err := hasLaterValidFrame(s.path(id), offset, id, uint64(len(events))); err != nil {
			return nil, err
		} else if later {
			return nil, errors.New("event journal has a damaged interior frame")
		}
		if err := f.Truncate(offset); err != nil {
			return nil, err
		}
		if err := f.Sync(); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func hasLaterValidFrame(path string, after int64, sessionID string, sequence uint64) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	for start := int(after) + 1; start+8 <= len(data); start++ {
		n := int(binary.BigEndian.Uint32(data[start : start+4]))
		if n == 0 || n > 4<<20 || start+8+n > len(data) {
			continue
		}
		body := data[start+8 : start+8+n]
		if crc32.ChecksumIEEE(body) != binary.BigEndian.Uint32(data[start+4:start+8]) {
			continue
		}
		var event Event
		if json.Unmarshal(body, &event) == nil && event.SessionID == sessionID && event.Sequence > sequence {
			return true, nil
		}
	}
	return false, nil
}

func (s *eventStore) Read(id string, after uint64) ([]Event, <-chan struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := s.readLocked(id)
	if err != nil {
		return nil, nil, err
	}
	if after > uint64(len(events)) {
		return nil, nil, errors.New("event cursor is ahead of journal")
	}
	ch := s.changed[id]
	if ch == nil {
		ch = make(chan struct{})
		s.changed[id] = ch
	}
	return append([]Event(nil), events[after:]...), ch, nil
}

func (s *eventStore) Append(id, op, kind string, payload any) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := s.readLocked(id)
	if err != nil {
		return Event{}, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	event := Event{SessionID: id, OperationID: op, Sequence: uint64(len(events) + 1), Time: time.Now().UTC(), Type: kind, Payload: data}
	body, err := json.Marshal(event)
	if err != nil {
		return Event{}, err
	}
	f, err := os.OpenFile(s.path(id), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return Event{}, err
	}
	var header [8]byte
	binary.BigEndian.PutUint32(header[:4], uint32(len(body)))
	binary.BigEndian.PutUint32(header[4:], crc32.ChecksumIEEE(body))
	if _, err = f.Write(append(header[:], body...)); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return Event{}, err
	}
	if closeErr != nil {
		return Event{}, closeErr
	}
	if ch := s.changed[id]; ch != nil {
		close(ch)
	}
	s.changed[id] = make(chan struct{})
	return event, nil
}

type idempotencyRecord struct {
	Principal string `json:"principal"`
	Request   string `json:"request"`
	Resource  string `json:"resource"`
	Path      string `json:"path,omitempty"`
}

type idempotencyStore struct {
	mu  sync.Mutex
	dir string
}

func (s *idempotencyStore) Lookup(principal, key, method, path string, body []byte) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity := sha256.Sum256([]byte(principal + "\x00" + key))
	request := sha256.Sum256(append([]byte(method+"\x00"+path+"\x00"), body...))
	data, err := os.ReadFile(filepath.Join(s.dir, hex.EncodeToString(identity[:])+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var old idempotencyRecord
	if json.Unmarshal(data, &old) != nil {
		return "", false, errors.New("invalid idempotency record")
	}
	if old.Principal != principal || old.Request != hex.EncodeToString(request[:]) {
		return "", false, errIdempotencyConflict
	}
	return old.Resource, true, nil
}

func newIdempotencyStore(dir string) (*idempotencyStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &idempotencyStore{dir: dir}, nil
}

var errIdempotencyConflict = errors.New("idempotency conflict")

func (s *idempotencyStore) Claim(principal, key, method, path string, body []byte, resource string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity := sha256.Sum256([]byte(principal + "\x00" + key))
	request := sha256.Sum256(append([]byte(method+"\x00"+path+"\x00"), body...))
	record := idempotencyRecord{Principal: principal, Request: hex.EncodeToString(request[:]), Resource: resource, Path: path}
	filePath := filepath.Join(s.dir, hex.EncodeToString(identity[:])+".json")
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return "", false, readErr
		}
		var old idempotencyRecord
		if json.Unmarshal(data, &old) != nil {
			return "", false, errors.New("invalid idempotency record")
		}
		if old.Principal != principal || old.Request != record.Request {
			return "", false, errIdempotencyConflict
		}
		return old.Resource, true, nil
	}
	if err != nil {
		return "", false, err
	}
	data, _ := json.Marshal(record)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", false, err
	}
	d, err := os.Open(s.dir)
	if err != nil {
		return "", false, err
	}
	err = syncDirectoryFile(d)
	d.Close()
	return resource, false, err
}

func (s *idempotencyStore) Records() ([]idempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var records []idempotencyRecord
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record idempotencyRecord
		if err := json.Unmarshal(data, &record); err != nil || record.Principal == "" || record.Resource == "" {
			return nil, errors.New("invalid idempotency record")
		}
		records = append(records, record)
	}
	return records, nil
}
