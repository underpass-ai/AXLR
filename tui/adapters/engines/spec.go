package engines

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// Spec is an engine's launch: the command, its arguments and its whole
// environment. Consoles share a process only when all three are equal, so
// two stores, two engine versions or two MADE host identities never meet.
type Spec struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env"`
}

// maxSpecBytes bounds the spec a daemon reads from its stdin.
const maxSpecBytes = 1 << 20

func (s Spec) validate() error {
	if s.Command == "" || strings.ContainsRune(s.Command, 0) {
		return errors.New("engine spec needs a command")
	}
	for _, value := range append(append([]string(nil), s.Args...), s.Env...) {
		if strings.ContainsRune(value, 0) {
			return errors.New("engine spec contains NUL")
		}
	}
	return nil
}

// key names the spec's socket: a digest, since the environment may hold
// secrets, short enough for the 104-byte limit of a socket path.
func (s Spec) key() string {
	env := slices.Clone(s.Env)
	slices.Sort(env)
	data, _ := json.Marshal(Spec{Command: s.Command, Args: s.Args, Env: env})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}
