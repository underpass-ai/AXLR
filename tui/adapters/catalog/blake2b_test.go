package catalog

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// The vectors of RFC 7693 (appendix A) and of the BLAKE2 reference
// implementation's test suite.
func TestBlake2b512MatchesTheReferenceVectors(t *testing.T) {
	for message, want := range map[string]string{
		"":    "786a02f742015903c6c6fd852552d272912f4740e15847618a86e217f71f5419d25e1031afee585313896444934eb04b903a685b1448b755d56f701afe9be2ce",
		"abc": "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923",
		"The quick brown fox jumps over the lazy dog": "a8add4bdddfd93e4877d2746e62817b116364a1fa7bc148d95090bc7333b3673f82401cf7aa2e4cb1ecd90296e3f14cb5413f8ed77be73045b13914cdcd6a918",
	} {
		digest := blake2b512([]byte(message))
		if got := hex.EncodeToString(digest[:]); got != want {
			t.Fatalf("blake2b512(%q) = %s; want %s", message, got, want)
		}
	}
	// Block boundaries: exactly one block, one block and a byte, and the
	// reference suite's 256-byte sequence 0,1,2… (vector 255 of the
	// unkeyed test suite).
	sequence := make([]byte, 256)
	for i := range sequence {
		sequence[i] = byte(i)
	}
	digest := blake2b512(sequence[:128])
	again := blake2b512(sequence[:128])
	if !bytes.Equal(digest[:], again[:]) {
		t.Fatal("the digest is not deterministic")
	}
	if a, b := blake2b512(sequence[:128]), blake2b512(sequence[:129]); bytes.Equal(a[:], b[:]) {
		t.Fatal("one more byte left the digest unchanged")
	}
	want := "5b21c5fd8868367612474fa2e70e9cfa2201ffeee8fafab5797ad58fefa17c9b5b107da4a3db6320baaf2c8617d5a51df914ae88da3867c2d41f0cc14fa67928"
	if got := hex.EncodeToString(func() []byte { d := blake2b512(sequence[:255]); return d[:] }()); got != want {
		t.Fatalf("blake2b512(0..254) = %s; want %s", got, want)
	}
}
