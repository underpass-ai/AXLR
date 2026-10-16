package catalog

import (
	"encoding/binary"
	"math/bits"
)

// BLAKE2b-512 as RFC 7693 specifies it, unkeyed, for minisign's prehashed
// signatures ("ED"): minisign signs the BLAKE2b-512 digest of the file, not
// the file. The standard library has no BLAKE2b and the workspace's module
// graph cannot take golang.org/x/crypto offline, so the compression function
// lives here; blake2b_test.go checks it against the RFC's vectors.

var blake2bIV = [8]uint64{
	0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
}

var blake2bSigma = [12][16]int{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
	{11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4},
	{7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8},
	{9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13},
	{2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9},
	{12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11},
	{13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10},
	{6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5},
	{10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0},
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
}

// blake2b512 is the 64-byte unkeyed BLAKE2b digest of message.
func blake2b512(message []byte) [64]byte {
	h := blake2bIV
	// The parameter block of an unkeyed 64-byte digest: fanout and depth 1.
	h[0] ^= 0x01010000 ^ 64
	var counted uint64
	for len(message) > 128 {
		var block [128]byte
		copy(block[:], message[:128])
		counted += 128
		blake2bCompress(&h, block, counted, false)
		message = message[128:]
	}
	// The last block, zero padded; an empty message is one zero block.
	var block [128]byte
	copy(block[:], message)
	counted += uint64(len(message))
	blake2bCompress(&h, block, counted, true)
	var digest [64]byte
	for i, word := range h {
		binary.LittleEndian.PutUint64(digest[i*8:], word)
	}
	return digest
}

func blake2bCompress(h *[8]uint64, block [128]byte, counted uint64, last bool) {
	var m [16]uint64
	for i := range m {
		m[i] = binary.LittleEndian.Uint64(block[i*8:])
	}
	var v [16]uint64
	copy(v[:8], h[:])
	copy(v[8:], blake2bIV[:])
	// The counter's low word; an index is nowhere near 2^64 bytes.
	v[12] ^= counted
	if last {
		v[14] = ^v[14]
	}
	g := func(a, b, c, d int, x, y uint64) {
		v[a] += v[b] + x
		v[d] = bits.RotateLeft64(v[d]^v[a], -32)
		v[c] += v[d]
		v[b] = bits.RotateLeft64(v[b]^v[c], -24)
		v[a] += v[b] + y
		v[d] = bits.RotateLeft64(v[d]^v[a], -16)
		v[c] += v[d]
		v[b] = bits.RotateLeft64(v[b]^v[c], -63)
	}
	for _, s := range blake2bSigma {
		g(0, 4, 8, 12, m[s[0]], m[s[1]])
		g(1, 5, 9, 13, m[s[2]], m[s[3]])
		g(2, 6, 10, 14, m[s[4]], m[s[5]])
		g(3, 7, 11, 15, m[s[6]], m[s[7]])
		g(0, 5, 10, 15, m[s[8]], m[s[9]])
		g(1, 6, 11, 12, m[s[10]], m[s[11]])
		g(2, 7, 8, 13, m[s[12]], m[s[13]])
		g(3, 4, 9, 14, m[s[14]], m[s[15]])
	}
	for i := range h {
		h[i] ^= v[i] ^ v[i+8]
	}
}
