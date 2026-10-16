package catalog

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

func testKey(t *testing.T) (PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := PublicKey{Key: public}
	copy(key.ID[:], []byte("axlrtest"))
	return key, private
}

// A signed index verifies in both minisign modes; a changed byte of the
// index, of the trusted comment or of the key id does not.
func TestMinisignVerifiesAndRefuses(t *testing.T) {
	key, private := testKey(t)
	parsed, err := ParsePublicKey("untrusted comment: minisign public key\n" + key.String() + "\n")
	if err != nil || parsed.String() != key.String() {
		t.Fatalf("parse = %+v, %v", parsed, err)
	}
	index := []byte(`{"schema_version":1,"catalog":"axlr","tools":[]}` + "\n")
	for _, prehashed := range []bool{false, true} {
		signature := Sign(private, key.ID, index, "timestamp:1760000000\tfile:index.json", prehashed)
		if err := Verify(parsed, index, signature); err != nil {
			t.Fatalf("prehashed %v: %v", prehashed, err)
		}
		tampered := append([]byte(nil), index...)
		tampered[len(tampered)-2] = ' '
		if err := Verify(parsed, tampered, signature); err == nil || !strings.Contains(err.Error(), "does not verify") {
			t.Fatalf("tampered index verified: %v", err)
		}
		comment := strings.Replace(string(signature), "timestamp:1760000000", "timestamp:1760000001", 1)
		if err := Verify(parsed, index, []byte(comment)); err == nil || !strings.Contains(err.Error(), "trusted comment") {
			t.Fatalf("changed trusted comment verified: %v", err)
		}
		other, _ := testKey(t)
		if err := Verify(other, index, signature); err == nil || !strings.Contains(err.Error(), "does not verify") {
			t.Fatalf("another key's signature verified: %v", err)
		}
		copy(other.ID[:], []byte("otherkey"))
		if err := Verify(other, index, signature); err == nil || !strings.Contains(err.Error(), "another key") {
			t.Fatalf("a signature with another key id verified: %v", err)
		}
	}
	if _, err := ParsePublicKey("RWQ"); err == nil {
		t.Fatal("a truncated key parsed")
	}
	if err := Verify(parsed, index, []byte("not a signature")); err == nil {
		t.Fatal("a non-signature verified")
	}
}
