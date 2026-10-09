package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
)

// searchTree writes files, named by slash paths, under a fresh workspace and
// returns it with an executor rooted there.
func searchTree(t *testing.T, files map[string]string) (string, *Executor) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e, err := newTestExecutor(t, Config{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	return dir, e
}

func runListing(e *Executor, tool, args string) dto.Response {
	return e.Execute(context.Background(), dto.Request{ProtocolVersion: 1, RequestID: "s", Tool: tool, Arguments: json.RawMessage(args)})
}

func searchFor(t *testing.T, e *Executor, args string) dto.SearchOutput {
	t.Helper()
	r := runListing(e, "search", args)
	if r.Status != "completed" {
		t.Fatalf("search %s: %+v", args, r.Error)
	}
	return r.Output.(dto.SearchOutput)
}

func listFor(t *testing.T, e *Executor, args string) dto.ListOutput {
	t.Helper()
	r := runListing(e, "list", args)
	if r.Status != "completed" {
		t.Fatalf("list %s: %+v", args, r.Error)
	}
	return r.Output.(dto.ListOutput)
}

// hits names each match as path:line.
func hits(out dto.SearchOutput) []string {
	names := []string{}
	for _, m := range out.Matches {
		names = append(names, fmt.Sprintf("%s:%d", m.Path, m.Line))
	}
	return names
}

func entryNames(out dto.ListOutput) []string {
	names := []string{}
	for _, entry := range out.Entries {
		names = append(names, entry.Path)
	}
	return names
}

func TestSearchReportsMatchesWithContextInWalkOrder(t *testing.T) {
	_, e := searchTree(t, map[string]string{
		"b.go":    "package b\n\nfunc Needle() {}\n// tail\n",
		"a/x.txt": "one\nneedle here\nthree\n",
		"a/y.md":  "nothing\n",
		"c.txt":   "x\r\nneedle\r\n",
	})
	out := searchFor(t, e, `{"pattern":"[Nn]eedle","context_lines":1}`)
	if got := hits(out); !reflect.DeepEqual(got, []string{"a/x.txt:2", "b.go:3", "c.txt:2"}) {
		t.Fatalf("matches %v", got)
	}
	first, second, third := out.Matches[0], out.Matches[1], out.Matches[2]
	if first.Text != "needle here" || first.Column != 1 || !reflect.DeepEqual(first.Before, []string{"one"}) || !reflect.DeepEqual(first.After, []string{"three"}) {
		t.Fatalf("first %+v", first)
	}
	if second.Column != 6 || !reflect.DeepEqual(second.Before, []string{""}) || !reflect.DeepEqual(second.After, []string{"// tail"}) {
		t.Fatalf("second %+v", second)
	}
	if third.Text != "needle" || !reflect.DeepEqual(third.Before, []string{"x"}) || third.After != nil {
		t.Fatalf("CRLF line kept its carriage return: %+v", third)
	}
	if out.FilesScanned != 4 || out.FilesSkipped != 0 || out.NextOffset != 0 || out.Truncated || out.LimitReached {
		t.Fatalf("totals %+v", out)
	}
	encoded, _ := json.Marshal(searchFor(t, e, `{"pattern":"three"}`))
	if string(encoded) != `{"matches":[{"path":"a/x.txt","line":3,"column":1,"text":"three"}],"files_scanned":4,"files_skipped":0,"truncated":false}` {
		t.Fatalf("output contract changed: %s", encoded)
	}
	encoded, _ = json.Marshal(searchFor(t, e, `{"pattern":"absent"}`))
	if !strings.HasPrefix(string(encoded), `{"matches":[],`) {
		t.Fatalf("no matches must encode as an empty list: %s", encoded)
	}
}

func TestSearchFiltersByGlobIgnoreCaseAndLiteral(t *testing.T) {
	_, e := searchTree(t, map[string]string{
		"main.go":          "A.B\n",
		"main_test.go":     "axb\n",
		"sub/util_test.go": "A.B\n",
		"sub/notes.md":     "a.b\n",
	})
	for args, want := range map[string][]string{
		`{"pattern":"a.b","ignore_case":true,"glob":"*_test.go"}`:           {"main_test.go:1", "sub/util_test.go:1"},
		`{"pattern":"a.b","literal":true,"ignore_case":true}`:               {"main.go:1", "sub/notes.md:1", "sub/util_test.go:1"},
		`{"pattern":"a.b","literal":true}`:                                  {"sub/notes.md:1"},
		`{"pattern":"A","glob":"**/*_test.go","path":"sub"}`:                {"sub/util_test.go:1"},
		`{"pattern":"A","glob":"sub/*.go"}`:                                 {"sub/util_test.go:1"},
		`{"pattern":"A","glob":"**/sub/*.go"}`:                              {"sub/util_test.go:1"},
		`{"pattern":"A","path":"sub/"}`:                                     {"sub/util_test.go:1"},
		`{"pattern":"A","path":"./sub"}`:                                    {"sub/util_test.go:1"},
		`{"pattern":"b","ignore_case":true,"path":"main.go"}`:               {"main.go:1"},
		`{"pattern":"b","ignore_case":true,"path":"main.go","glob":"*.md"}`: {"main.go:1"},
	} {
		if got := hits(searchFor(t, e, args)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v", args, got, want)
		}
	}
}

func TestSearchPaginatesByOffset(t *testing.T) {
	_, e := searchTree(t, map[string]string{"a.txt": "hit 1\nhit 2\nhit 3\n", "b.txt": "hit 4\nhit 5\n"})
	for _, step := range []struct {
		args string
		want []string
		next int
	}{
		{`{"pattern":"hit","max_results":2}`, []string{"a.txt:1", "a.txt:2"}, 2},
		{`{"pattern":"hit","max_results":2,"offset":2}`, []string{"a.txt:3", "b.txt:1"}, 4},
		{`{"pattern":"hit","max_results":2,"offset":4}`, []string{"b.txt:2"}, 0},
		{`{"pattern":"hit","max_results":3,"offset":2}`, []string{"a.txt:3", "b.txt:1", "b.txt:2"}, 0},
		{`{"pattern":"hit","offset":9}`, []string{}, 0},
	} {
		out := searchFor(t, e, step.args)
		if got := hits(out); !reflect.DeepEqual(got, step.want) || out.NextOffset != step.next || out.Truncated != (step.next != 0) {
			t.Errorf("%s: %v next %d truncated %v", step.args, got, out.NextOffset, out.Truncated)
		}
	}
}

func TestSearchSkipsBinaryLargeInvalidTextAndGit(t *testing.T) {
	_, e := searchTree(t, map[string]string{
		"bin.dat":     "hit\x00hit\n",
		"big.txt":     "hit\n" + strings.Repeat("x", 1<<20),
		"latin1.txt":  "hit \xff\n",
		".git/config": "hit\n",
		"sub/.git":    "gitdir: hit\n",
		"ok.txt":      "hit\n",
	})
	out := searchFor(t, e, `{"pattern":"hit"}`)
	if got := hits(out); !reflect.DeepEqual(got, []string{"ok.txt:1"}) {
		t.Fatalf("matches %v", got)
	}
	if out.FilesScanned != 1 || out.FilesSkipped != 3 {
		t.Fatalf("scanned %d skipped %d", out.FilesScanned, out.FilesSkipped)
	}
	// A search the caller points into .git is the caller's choice.
	if got := hits(searchFor(t, e, `{"pattern":"hit","path":".git"}`)); !reflect.DeepEqual(got, []string{".git/config:1"}) {
		t.Fatalf("explicit .git search %v", got)
	}
}

func TestSearchBoundsLongLinesAroundTheMatch(t *testing.T) {
	line := strings.Repeat("é", 400) + "needle" + strings.Repeat("z", 400)
	_, e := searchTree(t, map[string]string{"min.js": strings.Repeat("x", 1000) + "\n" + line + "\n"})
	out := searchFor(t, e, `{"pattern":"needle","context_lines":1}`)
	if len(out.Matches) != 1 {
		t.Fatalf("matches %+v", out.Matches)
	}
	m := out.Matches[0]
	if m.Column != 801 || !m.Truncated || !strings.Contains(m.Text, "needle") || !strings.HasPrefix(m.Text, "…") || !strings.HasSuffix(m.Text, "…") {
		t.Fatalf("match %+v", m)
	}
	for _, text := range append([]string{m.Text}, m.Before...) {
		if !utf8.ValidString(text) || len(text) > 300+2*len("…") {
			t.Fatalf("line not bounded at a character boundary: %d bytes", len(text))
		}
	}
	if len(m.Before) != 1 || !strings.HasSuffix(m.Before[0], "…") {
		t.Fatalf("context %+v", m.Before)
	}
}

func TestSearchAndListFitMaxBytes(t *testing.T) {
	var text strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&text, "needle %d <&> with \"quotes\" and some padding to give the line weight\n", i)
	}
	files := map[string]string{"many.txt": text.String()}
	for i := 0; i < 60; i++ {
		files[fmt.Sprintf("dir/file-%02d-with-a-long-name.txt", i)] = "x"
	}
	_, e := searchTree(t, files)
	out := searchFor(t, e, `{"pattern":"needle","max_results":200,"max_bytes":2000}`)
	encoded, _ := json.Marshal(out)
	if len(encoded) > 2000 || len(out.Matches) == 0 || len(out.Matches) >= 100 || out.NextOffset != len(out.Matches) || !out.Truncated {
		t.Fatalf("%d bytes, %d matches, next %d", len(encoded), len(out.Matches), out.NextOffset)
	}
	next := searchFor(t, e, fmt.Sprintf(`{"pattern":"needle","max_results":200,"max_bytes":2000,"offset":%d}`, out.NextOffset))
	if next.Matches[0].Line != out.NextOffset+1 {
		t.Fatalf("next page starts at line %d", next.Matches[0].Line)
	}
	// One match always comes back, so a page never stalls.
	if one := searchFor(t, e, `{"pattern":"needle","max_bytes":1}`); len(one.Matches) != 1 || one.NextOffset != 1 {
		t.Fatalf("smallest page %+v", one)
	}
	listed := listFor(t, e, `{"path":"dir","max_bytes":1000}`)
	encoded, _ = json.Marshal(listed)
	if len(encoded) > 1000 || len(listed.Entries) == 0 || listed.NextOffset != len(listed.Entries) || !listed.Truncated {
		t.Fatalf("%d bytes, %d entries, next %d", len(encoded), len(listed.Entries), listed.NextOffset)
	}
}

func TestSearchAndListNeverFollowSymlinks(t *testing.T) {
	dir, e := searchTree(t, map[string]string{"inside.txt": "needle\n", "sub/a.txt": "needle\n"})
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{"out": outside, "out.txt": filepath.Join(outside, "secret.txt"), "in.txt": filepath.Join(dir, "inside.txt"), "subl": filepath.Join(dir, "sub")} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	if got := hits(searchFor(t, e, `{"pattern":"needle"}`)); !reflect.DeepEqual(got, []string{"inside.txt:1", "sub/a.txt:1"}) {
		t.Fatalf("search followed a symlink: %v", got)
	}
	for _, args := range []string{`{"pattern":"needle","path":"out"}`, `{"pattern":"needle","path":"out/secret.txt"}`, `{"pattern":"needle","path":"in.txt"}`} {
		if r := runListing(e, "search", args); r.Status == "completed" {
			t.Fatalf("searched through a symlink: %s %+v", args, r.Output)
		}
	}
	listed := listFor(t, e, `{"recursive":true}`)
	if got := entryNames(listed); !reflect.DeepEqual(got, []string{"in.txt", "inside.txt", "out", "out.txt", "sub", "sub/a.txt", "subl"}) {
		t.Fatalf("list followed a symlink: %v", got)
	}
	for _, entry := range listed.Entries {
		if strings.HasPrefix(entry.Path, "out") || entry.Path == "subl" || entry.Path == "in.txt" {
			if entry.Type != "symlink" || entry.Size != nil {
				t.Fatalf("link entry %+v", entry)
			}
		}
	}
	if got := listFor(t, e, `{"path":"out/"}`); len(got.Entries) != 1 || got.Entries[0].Type != "symlink" {
		t.Fatalf("listing a link must describe the link: %+v", got.Entries)
	}
	if r := runListing(e, "list", `{"path":"out/secret.txt"}`); r.Status == "completed" {
		t.Fatalf("listed through an escaping link: %+v", r.Output)
	}
}

func TestSearchAndListRejectPathsOutsideTheWorkspace(t *testing.T) {
	_, e := searchTree(t, map[string]string{"a/b.txt": "x\n"})
	for _, path := range []string{"../x", "/etc", "a/../../b", "a/..", "..", "a/\x00"} {
		search, _ := json.Marshal(map[string]any{"pattern": "x", "path": path})
		list, _ := json.Marshal(map[string]any{"path": path})
		for tool, args := range map[string][]byte{"search": search, "list": list} {
			r := runListing(e, tool, string(args))
			if r.Status != "rejected" || r.Error == nil || r.Error.Code != "invalid_request" {
				t.Errorf("%s %q: %s %+v", tool, path, r.Status, r.Error)
			}
		}
	}
	if r := runListing(e, "list", `{"path":"missing"}`); r.Status != "failed" || r.Error.Code != "not_found" {
		t.Fatalf("missing path: %s %+v", r.Status, r.Error)
	}
}

func TestSearchAndListRejectInvalidArguments(t *testing.T) {
	_, e := searchTree(t, map[string]string{"a.txt": "x\n"})
	for _, args := range []string{`{}`, `{"pattern":""}`, `{"pattern":"("}`, `{"pattern":"x","glob":"["}`, `{"pattern":"x","glob":"/abs"}`, `{"pattern":"x","context_lines":6}`, `{"pattern":"x","context_lines":-1}`, `{"pattern":"x","max_results":201}`, `{"pattern":"x","max_results":-1}`, `{"pattern":"x","offset":-1}`, `{"pattern":"x","max_bytes":-1}`, `{"pattern":"x","max_bytes":2097152}`, `{"pattern":"x","surprise":1}`, `{"pattern":"` + strings.Repeat("a", 4097) + `"}`} {
		if r := runListing(e, "search", args); r.Status != "rejected" {
			t.Errorf("search accepted %.80s: %+v", args, r.Output)
		}
	}
	for _, args := range []string{`{"max_entries":501}`, `{"max_entries":-1}`, `{"recursive":true,"max_depth":9}`, `{"max_depth":-1}`, `{"offset":-1}`, `{"glob":"["}`, `{"pattern":"x"}`} {
		if r := runListing(e, "list", args); r.Status != "rejected" {
			t.Errorf("list accepted %s: %+v", args, r.Output)
		}
	}
	for _, input := range []string{
		`{"protocol_version":1,"request_id":"r1","tool":"search","arguments":{"pattern":"x"}}`,
		`{"protocol_version":1,"request_id":"r1","tool":"list","arguments":{}}`,
	} {
		if _, err := Decode(strings.NewReader(input)); err != nil {
			t.Errorf("Decode refused %s: %v", input, err)
		}
	}
	for _, tool := range []string{"search", "list"} {
		b, err := MarshalResponse(dto.Response{ProtocolVersion: 1, RequestID: "r", Tool: tool, Status: "completed"})
		if err != nil || !strings.Contains(string(b), `"tool":"`+tool+`"`) {
			t.Fatalf("response lost its tool: %s %v", b, err)
		}
	}
}

func TestListReportsTypesSizesDepthAndPages(t *testing.T) {
	_, e := searchTree(t, map[string]string{
		"a.txt":          "abc",
		"d/b.txt":        "hello",
		"d/e/c.txt":      "",
		"d/e/f/deep.txt": "deep",
		".git/HEAD":      "ref",
	})
	top := listFor(t, e, `{}`)
	if got := entryNames(top); !reflect.DeepEqual(got, []string{".git", "a.txt", "d"}) {
		t.Fatalf("top %v", got)
	}
	if top.Entries[0].Type != "dir" || top.Entries[0].Size != nil || top.Entries[1].Type != "file" || top.Entries[1].Size == nil || *top.Entries[1].Size != 3 {
		t.Fatalf("types and sizes %+v", top.Entries)
	}
	encoded, _ := json.Marshal(top)
	if string(encoded) != `{"entries":[{"path":".git","type":"dir"},{"path":"a.txt","type":"file","size":3},{"path":"d","type":"dir"}],"truncated":false}` {
		t.Fatalf("output contract changed: %s", encoded)
	}
	for args, want := range map[string][]string{
		`{"recursive":true}`:                            {".git", "a.txt", "d", "d/b.txt", "d/e", "d/e/c.txt", "d/e/f"},
		`{"recursive":true,"max_depth":1}`:              {".git", "a.txt", "d"},
		`{"recursive":false,"max_depth":8}`:             {".git", "a.txt", "d"},
		`{"recursive":true,"max_depth":8}`:              {".git", "a.txt", "d", "d/b.txt", "d/e", "d/e/c.txt", "d/e/f", "d/e/f/deep.txt"},
		`{"recursive":true,"glob":"*.txt"}`:             {"a.txt", "d/b.txt", "d/e/c.txt"},
		`{"path":"d"}`:                                  {"d/b.txt", "d/e"},
		`{"path":"a.txt"}`:                              {"a.txt"},
		`{"path":"d/e/c.txt"}`:                          {"d/e/c.txt"},
		`{"recursive":true,"max_entries":3}`:            {".git", "a.txt", "d"},
		`{"recursive":true,"max_entries":3,"offset":3}`: {"d/b.txt", "d/e", "d/e/c.txt"},
		`{"recursive":true,"max_entries":3,"offset":6}`: {"d/e/f"},
	} {
		if got := entryNames(listFor(t, e, args)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v", args, got, want)
		}
	}
	if page := listFor(t, e, `{"recursive":true,"max_entries":3,"offset":3}`); page.NextOffset != 6 || !page.Truncated {
		t.Fatalf("page %+v", page)
	}
	if last := listFor(t, e, `{"recursive":true,"max_entries":3,"offset":6}`); last.NextOffset != 0 || last.Truncated {
		t.Fatalf("last page %+v", last)
	}
	if c := listFor(t, e, `{"path":"d/e/c.txt"}`); *c.Entries[0].Size != 0 {
		t.Fatal("an empty file lost its size")
	}
}

// The scan limits are too large to reach in a unit test, so the adapter is
// driven directly with smaller ones.
func TestSearchAndListStopAtTheirLimits(t *testing.T) {
	dir, _ := searchTree(t, map[string]string{"a.txt": "hit\n", "b.txt": "hit\n", "c.txt": "hit\n"})
	files, err := local.NewFileAdapter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	mapper := RequestMapper{MaxReadBytes: hardFileBytes, MaxFileBytes: hardFileBytes, MaxOutputBytes: hardFileBytes, MaxTimeout: HardTimeout}
	command, err := mapper.Map(dto.Request{ProtocolVersion: 1, RequestID: "s", Tool: "search", Arguments: json.RawMessage(`{"pattern":"hit"}`)})
	if err != nil {
		t.Fatal(err)
	}
	search := command.(domain.SearchCommand)
	if search.MaxFiles != 20000 || search.MaxScanBytes != 64<<20 {
		t.Fatalf("limits %d %d", search.MaxFiles, search.MaxScanBytes)
	}
	search.MaxFiles = 2
	found, err := files.Search(context.Background(), search)
	if err != nil || len(found.Matches) != 2 || !found.LimitReached || found.FilesScanned != 2 {
		t.Fatalf("file limit: %+v %v", found, err)
	}
	out := (ResponseMapper{ListingBytes: 1 << 20}).Map(found).(dto.SearchOutput)
	if !out.Truncated || !out.LimitReached || out.NextOffset != 0 {
		t.Fatalf("limit not reported: %+v", out)
	}
	search.MaxFiles, search.MaxScanBytes = 20000, 9
	if found, err = files.Search(context.Background(), search); err != nil || len(found.Matches) != 2 || !found.LimitReached {
		t.Fatalf("byte limit: %+v %v", found, err)
	}
	command, err = mapper.Map(dto.Request{ProtocolVersion: 1, RequestID: "l", Tool: "list", Arguments: json.RawMessage(`{"recursive":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	list := command.(domain.ListCommand)
	list.MaxVisits = 2
	listed, err := files.List(context.Background(), list)
	if err != nil || len(listed.Entries) != 2 || !listed.LimitReached {
		t.Fatalf("visit limit: %+v %v", listed, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = files.Search(ctx, search)
	var fault *domain.Fault
	if !errors.As(err, &fault) || fault.Status != "cancelled" {
		t.Fatalf("cancelled search: %v", err)
	}
}

func TestSearchAndListPassOverUnreadableDirectories(t *testing.T) {
	if goruntime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions that bind the test user")
	}
	dir, e := searchTree(t, map[string]string{"a.txt": "hit\n", "locked/b.txt": "hit\n"})
	locked := filepath.Join(dir, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	out := searchFor(t, e, `{"pattern":"hit"}`)
	if got := hits(out); !reflect.DeepEqual(got, []string{"a.txt:1"}) || out.FilesSkipped != 1 {
		t.Fatalf("matches %v skipped %d", got, out.FilesSkipped)
	}
	if got := entryNames(listFor(t, e, `{"recursive":true}`)); !reflect.DeepEqual(got, []string{"a.txt", "locked"}) {
		t.Fatalf("entries %v", got)
	}
	if r := runListing(e, "list", `{"path":"locked"}`); r.Status != "failed" || r.Error.Code != "permission_denied" {
		t.Fatalf("unreadable scope: %s %+v", r.Status, r.Error)
	}
}
