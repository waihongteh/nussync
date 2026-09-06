package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nussync/internal/config"
)

func TestContentTypeFor(t *testing.T) {
	cases := map[string]string{
		"Lecture 3.pdf":  "application/pdf",
		"diagram.PNG":    "image/png",
		"notes.md":       "text/plain; charset=utf-8",
		"main.go":        "text/plain; charset=utf-8",
		"deck.pptx":      "application/octet-stream",
		"noextension":    "application/octet-stream",
		"page.html":      "text/plain; charset=utf-8", // never served as live markup
		"recording.webm": "video/webm",
	}
	for name, want := range cases {
		if got := contentTypeFor(name); got != want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestUnderRoot(t *testing.T) {
	root := filepath.Join("C:", "Users", "me", "NUSSync")
	ok := []string{
		filepath.Join(root, "CS2100", "notes.pdf"),
		filepath.Join(strings.ToUpper(root), "cs2100", "notes.pdf"),
	}
	bad := []string{
		root,
		filepath.Join("C:", "Users", "me", "NUSSyncOther", "x.pdf"),
		filepath.Join("C:", "Users", "me", "secrets.txt"),
		filepath.Join(root, "..", "secrets.txt"),
	}
	for _, p := range ok {
		if !underRoot(root, filepath.Clean(p)) {
			t.Errorf("underRoot(%q) = false, want true", p)
		}
	}
	for _, p := range bad {
		if underRoot(root, filepath.Clean(p)) {
			t.Errorf("underRoot(%q) = true, want false", p)
		}
	}
}

// TestServeLocalPath covers the /local/path guard end to end: a file inside
// SyncDir streams with the right headers and honours Range; anything outside
// is 403.
func TestServeLocalPath(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(inside, []byte("hello viewer"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.cfg = config.Settings{SyncDir: dir}
	h := NewLocalAssetHandler(app)

	get := func(target string, hdr map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	w := get("/local/path?p="+inside, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("inside: status %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if w.Header().Get("Accept-Ranges") != "bytes" {
		t.Error("missing Accept-Ranges: bytes")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	if w.Body.String() != "hello viewer" {
		t.Errorf("body = %q", w.Body.String())
	}

	if w := get("/local/path?p="+outside, nil); w.Code != http.StatusForbidden {
		t.Errorf("outside: status %d, want 403", w.Code)
	}
	if w := get("/local/path", nil); w.Code != http.StatusForbidden {
		t.Errorf("missing p: status %d, want 403", w.Code)
	}
	if w := get("/local/abc", nil); w.Code != http.StatusBadRequest {
		t.Errorf("bad id: status %d, want 400", w.Code)
	}
	if w := get("/index.html", nil); w.Code != http.StatusNotFound {
		t.Errorf("non-/local path: status %d, want 404", w.Code)
	}

	// Range: the Chromium PDF viewer seeks, so partial content must work.
	w = get("/local/path?p="+inside, map[string]string{"Range": "bytes=6-10"})
	if w.Code != http.StatusPartialContent {
		t.Fatalf("range: status %d, want 206", w.Code)
	}
	if w.Body.String() != "viewe" {
		t.Errorf("range body = %q, want %q", w.Body.String(), "viewe")
	}
}
