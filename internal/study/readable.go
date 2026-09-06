package study

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Claude Code's Read tool refuses a large fraction of real-world lecture PDFs
// with the message "PDF is password-protected. Please provide an unprotected
// version." That message is misleading: measured against the CLI (2.1.251) and
// the NUS library on 2026-09-06, two independent things make it fire.
//
//  1. Size. A PDF over roughly 128 KiB is always refused, whatever it contains
//     and however it was produced. Under that it is accepted.
//  2. Structure. Some smaller files are refused too (a 38 KB PDF 1.3 essay
//     prompt, an 84 KB PDF with a table). Rewriting exactly the same pages
//     through pdfcpu makes them readable, so the parser is choking on how the
//     file is assembled, not on its content.
//
// Genuinely owner-password-encrypted PDFs are a third, rarer case and pdfcpu
// decrypts those with an empty password. Rather than trying to tell the three
// apart, every PDF is normalised through pdfcpu into a cache directory once,
// and anything still over ReadableLimit afterwards falls back to the text the
// FTS indexer already extracted.

// ReadableLimit is the largest PDF the Read tool will open. The observed cliff
// sits between 104 KiB (accepted) and 142 KiB (refused); 120 KiB is the
// conservative side of it.
const ReadableLimit = 120 << 10

// EncryptedPDF reports whether a PDF declares /Encrypt in its trailer. The
// trailer lives at the end of the file, so only the tail is read — this runs on
// every source of every job and must stay cheap.
func EncryptedPDF(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	const tail = 8 << 10
	n := fi.Size()
	off := int64(0)
	if n > tail {
		off = n - tail
		n = tail
	}
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, off); err != nil {
		return false
	}
	return strings.Contains(string(buf), "/Encrypt")
}

// CacheDir is where normalised copies and text extracts are kept:
// %LOCALAPPDATA%/NUSSync/study-cache (os.UserCacheDir elsewhere).
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "NUSSync", "study-cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Prepared is the outcome of resolving one library file into something the Read
// tool can actually open.
type Prepared struct {
	Path     string // the file Claude should Read; "" when there is nothing
	Readable bool   // Path may be handed to the Read tool
	TextOnly bool   // Path is a .txt extract, not the document itself
	Note     string // extra sentence for the prompt, "" when unremarkable
}

// pdfcpu keeps a process-wide config directory unless told not to; disabling it
// once keeps concurrent jobs off the filesystem and off each other.
var pdfcpuOnce sync.Once

func pdfcpuConf() *model.Configuration {
	pdfcpuOnce.Do(api.DisableConfigDir)
	c := model.NewDefaultConfiguration()
	c.ValidationMode = model.ValidationRelaxed
	c.UserPW = ""
	c.OwnerPW = ""
	return c
}

// cacheKey identifies a source file by identity + mtime + size, so an edited or
// re-synced file gets a new cache entry instead of a stale one.
func cacheKey(path string, fileID int) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d",
		strings.ToLower(filepath.Clean(path)), fi.Size(), fi.ModTime().UnixNano())))
	return fmt.Sprintf("%d-%s", fileID, hex.EncodeToString(sum[:6])), nil
}

// normalizePDF rewrites src through pdfcpu into dst. Encrypted files are
// decrypted with an empty password (NUS lecture PDFs set an owner password and
// leave the user password blank, so they open in viewers but confuse readers);
// everything else is re-serialised, which repairs the structures the Read tool
// rejects. The write goes to a temp file and is renamed, so a crashed or
// concurrent run can never leave a half-written PDF in the cache.
func normalizePDF(src, dst string) error {
	tmp := dst + fmt.Sprintf(".%d.tmp", os.Getpid())
	defer os.Remove(tmp)

	err := decryptOrOptimize(src, tmp)
	if err != nil {
		return err
	}
	if fi, serr := os.Stat(tmp); serr != nil || fi.Size() == 0 {
		return errors.New("pdfcpu produced no output")
	}
	return os.Rename(tmp, dst)
}

// decryptOrOptimize is split out because pdfcpu panics on some malformed files
// and the whole call has to be guarded.
func decryptOrOptimize(src, tmp string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdfcpu panicked: %v", r)
		}
	}()
	if EncryptedPDF(src) {
		if derr := api.DecryptFile(src, tmp, pdfcpuConf()); derr == nil {
			return nil
		}
		// A false positive (the string appears outside the trailer dict) or a
		// real user password: fall through to a plain rewrite.
	}
	return api.OptimizeFile(src, tmp, pdfcpuConf())
}

// Resolve turns one library file into a path the Read tool can open.
//
// Non-PDFs are returned untouched: the readable ones (txt/md/images/source) the
// Read tool already handles, and the rest (pptx/docx/xlsx) keep the existing
// inline-text behaviour, signalled by Readable == false.
//
// PDFs are normalised into the cache. When even the normalised copy is over
// ReadableLimit — a big slide deck, typically — text() is called and its result
// is written to <cache>/<fileID>.txt for Claude to Read instead, with a note
// saying figures are unavailable. text is a func so the database is only hit on
// the path that needs it.
func Resolve(path string, fileID int, text func() string) Prepared {
	if strings.TrimSpace(path) == "" {
		return Prepared{}
	}
	if !ClaudeCanRead(path) {
		return Prepared{Path: path}
	}
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return Prepared{Path: path, Readable: true}
	}

	if p, ok := preparePDF(path, fileID); ok {
		return p
	}
	if p, ok := prepareText(fileID, text); ok {
		return p
	}
	// Nothing better to offer: hand over the original and let Claude report the
	// failure rather than silently dropping the source.
	return Prepared{Path: path, Readable: true}
}

// preparePDF returns the cached normalised copy, building it when missing.
func preparePDF(path string, fileID int) (Prepared, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return Prepared{}, false
	}
	dir, err := CacheDir()
	if err != nil {
		// No cache to write into: the original is only worth offering when it
		// is small enough to have a chance.
		if fi.Size() <= ReadableLimit && !EncryptedPDF(path) {
			return Prepared{Path: path, Readable: true}, true
		}
		return Prepared{}, false
	}
	key, err := cacheKey(path, fileID)
	if err != nil {
		return Prepared{}, false
	}
	dst := filepath.Join(dir, key+".pdf")

	if cfi, serr := os.Stat(dst); serr != nil || cfi.Size() == 0 {
		if err := normalizePDF(path, dst); err != nil {
			return Prepared{}, false
		}
	}
	cfi, err := os.Stat(dst)
	if err != nil {
		return Prepared{}, false
	}
	if cfi.Size() > ReadableLimit {
		return Prepared{}, false
	}
	return Prepared{Path: dst, Readable: true}, true
}

// prepareText writes the FTS-extracted text to the cache so Claude can Read it.
func prepareText(fileID int, text func() string) (Prepared, bool) {
	if text == nil {
		return Prepared{}, false
	}
	txt := strings.TrimSpace(text())
	if txt == "" {
		return Prepared{}, false
	}
	dir, err := CacheDir()
	if err != nil {
		return Prepared{}, false
	}
	dst := filepath.Join(dir, fmt.Sprintf("%d.txt", fileID))
	if err := os.WriteFile(dst, []byte(ClampText(txt)), 0o644); err != nil {
		return Prepared{}, false
	}
	return Prepared{
		Path:     dst,
		Readable: true,
		TextOnly: true,
		Note: "This is a text-only extract of the document — figures, diagrams and " +
			"slide layout are unavailable, and page numbers may be missing or " +
			"approximate. Work from the text and do not describe images.",
	}, true
}
