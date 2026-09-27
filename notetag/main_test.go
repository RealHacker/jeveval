package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func put(t *testing.T, p, s string) {
	t.Helper()
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		t.Fatal(e)
	}
}
func read(t *testing.T, p string) string {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestFormatting(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"body", "#go #ai\n\nbody"},
		{"---\ntitle: Example\n---\n\nbody", "---\ntitle: Example\n---\n#go #ai\n\nbody"},
		{"\ufeff---\r\ntitle: Example\r\n...\r\nbody", "\ufeff---\r\ntitle: Example\r\n...\r\n#go #ai\r\n\r\nbody"},
		{"---\ntitle: Example\n---", "---\ntitle: Example\n---\n#go #ai\n\n"},
		{"", "#go #ai\n\n"},
	} {
		got, e := insertTags([]byte(tc.in), []string{"go", "ai"})
		if e != nil || string(got) != tc.want {
			t.Fatalf("%q: %q, %v", tc.in, got, e)
		}
	}
	if _, e := insertTags([]byte("---\ntitle: Example"), []string{"go"}); e == nil {
		t.Fatal("accepted unclosed header")
	}
	if got, e := insertTags([]byte("body"), nil); e != nil || string(got) != "body" {
		t.Fatal("zero tags changed content")
	}
}
func TestSelection(t *testing.T) {
	tags := []string{"h", "g", "f", "e", "d", "c", "b", "a"}
	p := map[string]float64{}
	for _, s := range tags {
		p[s] = 0.125
	}
	if got := strings.Join(selectTags(tags, p, 0.1), ","); got != "a,b,c,d,e,f" {
		t.Fatal(got)
	}
	if len(selectTags(tags, p, 0.125)) != 0 {
		t.Fatal("threshold must be strict")
	}
	if got := firstWords("one \n two\tthree", 2); got != "one \n two" {
		t.Fatal(got)
	}
}
func TestCLIIntegration(t *testing.T) {
	root := t.TempDir()
	tags := filepath.Join(root, "tags.txt")
	put(t, tags, "go\nai\nother\n")
	os.Mkdir(filepath.Join(root, "nested"), 0700)
	old := filepath.Join(root, "z.md")
	newer := filepath.Join(root, "nested", "a.MD")
	put(t, old, strings.Repeat("word ", 1005))
	put(t, newer, "---\ntitle: hello\n---\nbody")
	oldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Hour)
	os.Chtimes(old, oldTime, oldTime)
	os.Chtimes(newer, newTime, newTime)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("bad HTTP contract")
		}
		var req struct {
			State     string
			Model     string
			Questions map[string]struct {
				Type     string
				Criteria map[string]any
			}
		}
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			t.Error(e)
		}
		if req.Model != "jev-latest" || req.Questions["tags"].Type != "choice" || len(req.Questions["tags"].Criteria) != 3 || len(strings.Fields(req.State)) > 1000 || utf8.RuneCountInString(req.State) > 3000 {
			t.Error("bad Jev request")
		}
		json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"tags": map[string]any{"type": "choice", "probabilities": map[string]float64{"go": 0.6, "ai": 0.3, "other": 0.1}}}})
	}))
	defer srv.Close()
	t.Setenv("TYPESAFE_API_KEY", "test-key")
	args := []string{"tag", "--dir", root, "--tagsfile", tags, "--endpoint", srv.URL}
	for i := 0; i < 2; i++ {
		if e := run(args); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 2 {
		t.Fatalf("extra calls: %d", calls)
	}
	for i := 0; i < 2; i++ {
		if e := run([]string{"apply", "--dir", root}); e != nil {
			t.Fatal(e)
		}
	}
	if !strings.HasPrefix(read(t, old), "#go #ai\n\nword") {
		t.Fatal("tags missing")
	}
	if got := read(t, newer); got != "---\ntitle: hello\n---\n#go #ai\n\nbody" {
		t.Fatal(got)
	}
	for p, want := range map[string]time.Time{old: oldTime, newer: newTime} {
		st, _ := os.Stat(p)
		if !st.ModTime().Equal(want) {
			t.Fatal("mtime changed")
		}
	}
	if e := run(args); e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatal("applied files retagged")
	}
	os.Remove(filepath.Join(root, ".tags", "applied.json"))
	if e := run([]string{"apply", "--dir", root}); e != nil {
		t.Fatal(e)
	}
	if strings.Count(read(t, old), "#go #ai") != 1 {
		t.Fatal("recovery duplicated tags")
	}
}
func TestOldestFirstAndStale(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "z.md")
	newer := filepath.Join(root, "a.md")
	put(t, old, "old")
	put(t, newer, "changed")
	base := time.Now().Add(-time.Hour)
	os.Chtimes(old, base, base)
	os.Chtimes(newer, base.Add(time.Minute), base.Add(time.Minute))
	docs, e := discover(root)
	if e != nil {
		t.Fatal(e)
	}
	results := state{"z.md": {Hash: hash([]byte("old")), Tags: []string{"go"}}, "a.md": {Hash: hash([]byte("original")), Tags: []string{"go"}}}
	e = apply(docs, results, state{}, filepath.Join(root, "results.json"), filepath.Join(root, "applied.json"))
	if e == nil || !strings.Contains(e.Error(), "changed since tag") {
		t.Fatal(e)
	}
	if !strings.HasPrefix(read(t, old), "#go\n\n") {
		t.Fatal("older document must apply first")
	}
	if read(t, newer) != "changed" {
		t.Fatal("stale document overwritten")
	}
}
func TestMalformedResponse(t *testing.T) {
	for _, prob := range []map[string]any{{"go": nil}, {"go": 0.2}, {"unknown": 1}, {"go": -1}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"tags": map[string]any{"type": "choice", "probabilities": prob}}})
		}))
		_, e := evaluate(srv.Client(), srv.URL, "key", "jev-latest", "body", []string{"go"})
		srv.Close()
		if e == nil {
			t.Fatal("accepted", prob)
		}
	}
}
func TestTagInput(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tags.txt")
	put(t, p, "\ufeffgo\r\nai\r\ngo\r\n\n")
	tags, e := readTags(p)
	if e != nil || strings.Join(tags, ",") != "go,ai" {
		t.Fatal(tags, e)
	}
	for _, s := range []string{"", "#go", "two words"} {
		put(t, p, s)
		if _, e := readTags(p); e == nil {
			t.Fatal("accepted", s)
		}
	}
}

func TestRoundedProbabilities(t *testing.T) {
	for _, tc := range []struct {
		a, b  float64
		valid bool
	}{
		{0.6, 0.39, true}, // Reported live response: total 0.99.
		{0.6, 0.41, true},
		{0.6, 0.35, true}, // Inclusive lower tolerance boundary.
		{0.6, 0.45, true}, // Inclusive upper tolerance boundary.
		{0.6, 0.34, false},
		{0.6, 0.46, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"tags": map[string]any{"type": "choice", "probabilities": map[string]float64{"go": tc.a, "ai": tc.b}}}})
		}))
		scores, err := evaluate(srv.Client(), srv.URL, "key", "jev-latest", "body", []string{"go", "ai"})
		srv.Close()
		if (err == nil) != tc.valid {
			t.Fatalf("total %g: %v", tc.a+tc.b, err)
		}
		if tc.valid && (scores["go"] != tc.a || scores["ai"] != tc.b) {
			t.Fatal("probabilities must not be rescaled")
		}
	}
}

func TestDocumentExcerpt(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"long Chinese", strings.Repeat("中文内容", 30000), strings.Repeat("中文内容", 750)},
		{"mixed", strings.Repeat("Go中文🙂", 20000), strings.Repeat("Go中文🙂", 600)},
		{"long Latin token", strings.Repeat("x", 100001), strings.Repeat("x", 3000)},
		{"word limit", strings.Repeat("a ", 1001), strings.TrimSpace(strings.Repeat("a ", 1000))},
		{"short unchanged", "# 标题\n\nHello 世界🙂", "# 标题\n\nHello 世界🙂"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := documentExcerpt(tc.input)
			if got != tc.want || !utf8.ValidString(got) {
				t.Fatalf("incorrect excerpt: %d characters", utf8.RuneCountInString(got))
			}
		})
	}
}

func TestHTTPErrorDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("context length exceeded secret-token " + strings.Repeat("文", 3000)))
	}))
	defer srv.Close()
	_, err := evaluate(srv.Client(), srv.URL, "secret-token", "jev-latest", "body", []string{"go"})
	if err == nil || !strings.Contains(err.Error(), "400 Bad Request: context length exceeded") || strings.Contains(err.Error(), "secret-token") || utf8.RuneCountInString(err.Error()) > 2100 || !utf8.ValidString(err.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}
