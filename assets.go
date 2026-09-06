package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// localAssets serves synced files to the WebView so the frontend can preview
// them in-place (PDF in an <iframe>, images in <img>, text in a <pre>) instead
// of shelling out to the OS default application.
//
// Routes (everything else is delegated to the embedded frontend):
//
//	GET /local/{fileID}       — look the absolute path up in the store
//	GET /local/path?p={abs}   — only for paths inside SyncDir; 403 otherwise
//
// Both stream with the right Content-Type, Content-Length, Accept-Ranges and
// Range support (the Chromium PDF viewer seeks), and Cache-Control: no-store so
// a re-synced file never shows a stale body.
type localAssets struct {
	app *App
}

// NewLocalAssetHandler builds the bare /local/... handler.
func NewLocalAssetHandler(app *App) http.Handler {
	return &localAssets{app: app}
}

// LocalAssetMiddleware wraps the Wails asset server so /local/... is answered
// here and everything else falls through untouched.
//
// This has to be Middleware rather than AssetServer.Handler: Handler only runs
// when the asset lookup MISSES, and under `wails dev` the lookup is a proxy to
// the Vite dev server, whose SPA fallback answers every unknown path with
// index.html — so a Handler would never see /local/... in dev. Middleware runs
// first and behaves the same in dev and in the built exe.
func LocalAssetMiddleware(app *App) func(http.Handler) http.Handler {
	local := NewLocalAssetHandler(app)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/local/") {
				local.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// contentTypes maps a lower-case extension (no dot) to the type we advertise.
// Anything textual is served as text/plain; charset=utf-8 — the viewer renders
// markdown/code itself, and letting the WebView treat .html or .svg-in-a-doc as
// live markup would run untrusted course content in the app's own origin.
var contentTypes = map[string]string{
	"pdf":  "application/pdf",
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"gif":  "image/gif",
	"webp": "image/webp",
	"bmp":  "image/bmp",
	"svg":  "image/svg+xml",
	"txt":  "text/plain; charset=utf-8",
	"md":   "text/plain; charset=utf-8",
	"csv":  "text/plain; charset=utf-8",
	"tsv":  "text/plain; charset=utf-8",
	"json": "text/plain; charset=utf-8",
	"log":  "text/plain; charset=utf-8",
	"xml":  "text/plain; charset=utf-8",
	"yml":  "text/plain; charset=utf-8",
	"yaml": "text/plain; charset=utf-8",
	"py":   "text/plain; charset=utf-8",
	"ts":   "text/plain; charset=utf-8",
	"tsx":  "text/plain; charset=utf-8",
	"js":   "text/plain; charset=utf-8",
	"jsx":  "text/plain; charset=utf-8",
	"go":   "text/plain; charset=utf-8",
	"java": "text/plain; charset=utf-8",
	"c":    "text/plain; charset=utf-8",
	"h":    "text/plain; charset=utf-8",
	"cpp":  "text/plain; charset=utf-8",
	"hpp":  "text/plain; charset=utf-8",
	"cs":   "text/plain; charset=utf-8",
	"rb":   "text/plain; charset=utf-8",
	"rs":   "text/plain; charset=utf-8",
	"php":  "text/plain; charset=utf-8",
	"sh":   "text/plain; charset=utf-8",
	"sql":  "text/plain; charset=utf-8",
	"r":    "text/plain; charset=utf-8",
	"m":    "text/plain; charset=utf-8",
	"tex":  "text/plain; charset=utf-8",
	"html": "text/plain; charset=utf-8",
	"htm":  "text/plain; charset=utf-8",
	"css":  "text/plain; charset=utf-8",
	"mp4":  "video/mp4",
	"webm": "video/webm",
	"mp3":  "audio/mpeg",
	"wav":  "audio/wav",
	"m4a":  "audio/mp4",
}

// contentTypeFor picks a Content-Type from the file name.
func contentTypeFor(name string) string {
	e := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ct, ok := contentTypes[e]; ok {
		return ct
	}
	return "application/octet-stream"
}

func (h *localAssets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if !strings.HasPrefix(p, "/local/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rest := strings.TrimPrefix(p, "/local/")
	var (
		abs  string
		name string
		err  error
	)
	if rest == "path" {
		abs, err = h.resolveRawPath(r.URL.Query().Get("p"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		name = filepath.Base(abs)
	} else {
		// Ids may be negative: downloaded papers live in the synthetic
		// "Papers" course and are numbered down from -1000.
		id, convErr := strconv.Atoi(rest)
		if convErr != nil || id == 0 {
			http.Error(w, "bad file id", http.StatusBadRequest)
			return
		}
		abs, name, err = h.resolveID(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	f, err := os.Open(abs)
	if err != nil {
		http.Error(w, "file is not on disk — sync to download it", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "not a file", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", contentTypeFor(name))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-store")
	// Defence in depth: nothing served here should ever be sniffed into an
	// executable document type.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+urlEscapeFilename(name))

	// ServeContent adds Content-Length and honours Range/If-Range for us.
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// resolveID looks a synced file's absolute path up in the store.
func (h *localAssets) resolveID(id int) (string, string, error) {
	if h.app == nil || h.app.st == nil {
		return "", "", errors.New("library is not ready")
	}
	f, ok, err := h.app.st.FileByID(id)
	if err != nil {
		log.Printf("assets: file %d: %v", id, err)
		return "", "", errors.New("lookup failed")
	}
	if !ok {
		return "", "", errors.New("no such file")
	}
	if strings.TrimSpace(f.AbsPath) == "" {
		return "", "", errors.New("file has no local copy")
	}
	return f.AbsPath, f.Name, nil
}

// resolveRawPath accepts an absolute path only when it resolves inside SyncDir.
// Symlinks are resolved first so a link planted in the library cannot escape.
func (h *localAssets) resolveRawPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("missing p")
	}
	syncDir := ""
	if h.app != nil {
		syncDir = strings.TrimSpace(h.app.settings().SyncDir)
	}
	if syncDir == "" {
		return "", errors.New("no sync directory configured")
	}
	root, err := filepath.Abs(syncDir)
	if err != nil {
		return "", errors.New("bad sync directory")
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	abs, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", errors.New("bad path")
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if !underRoot(root, abs) {
		return "", errors.New("path is outside the sync directory")
	}
	return abs, nil
}

// underRoot reports whether abs sits inside root. Uses filepath.Rel so a
// sibling like "C:\NUSSyncOther" cannot pass a naive prefix test; the compare
// is case-insensitive on Windows.
func underRoot(root, abs string) bool {
	if runtime.GOOS == "windows" {
		// filepath.Rel compares path elements byte-for-byte, so "c:\users\..."
		// and "C:\Users\..." would look unrelated on a case-insensitive volume.
		root = strings.ToLower(root)
		abs = strings.ToLower(abs)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	if rel == "." {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}

// urlEscapeFilename percent-encodes a filename for the RFC 5987 form of
// Content-Disposition (which must not carry raw non-ASCII or quotes).
func urlEscapeFilename(name string) string {
	const keep = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~"
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if strings.IndexByte(keep, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		const hex = "0123456789ABCDEF"
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}
