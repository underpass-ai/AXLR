# file_hash_tree

Builds a checksum manifest of a set of files and checks files against one, in the format of
GNU `sha256sum`.

## Manifest

Files come inline as `files` (`{path: text}`, hashed as UTF-8) or `files_hex` (`{path: hex}`
for binary content), or from a relative `directory` walked recursively without following
symlinks, filtered by `include` and `exclude` glob patterns (matched against the relative path
or the file name). Paths are sorted by their UTF-8 bytes, as `LC_ALL=C sort` does.

Each line is `<hex digest>␠␠<path>`, exactly what `sha256sum` prints; a name containing a
backslash or a newline is escaped (`\\`, `\n`) and its line starts with a backslash, as GNU
coreutils does, so `sha256sum -c` reads the manifest back. `bsd_manifest` has the same entries
as `SHA256 (path) = digest` (the `--tag` format). `tree_digest` is the digest of the manifest
text itself: one value that changes when any file, name or the set of files changes. The
algorithm is SHA-256 by default, or SHA-512, SHA-1, MD5 or BLAKE2b (512-bit).

## Verify

`operation: "verify"` with `manifest` (GNU lines in text or binary `*` mode, BSD lines, comments
with `#`) compares: `ok`, `changed` (with expected and actual digests), `missing` (in the
manifest, not among the files), `extra` (among the files, not in the manifest) and
`manifest_problems` (malformed lines, wrong digest length, duplicates). `valid` is true only when
everything matches.

## Tests

FIPS 180-2's test vectors: SHA-256 of "abc" (`ba7816bf…f20015ad`), SHA-256 of the empty string
and SHA-512 of "abc"; a four-file manifest compared with the output of `sha256sum` sorted with
`LC_ALL=C`, including the escaped backslash name; BSD lines; verification with each kind of
mismatch and against its own manifest; a directory walk; and errors.

## Limits

Inline text is hashed as UTF-8 (use `files_hex` for exact bytes); the directory mode reads at
most 20000 files and 500 MB; file modes, owners and empty directories are not part of the
manifest.
