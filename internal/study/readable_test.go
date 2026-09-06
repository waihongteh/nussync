package study

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// minimalPDF is a hand-written, definitely-unencrypted PDF 1.4 with one page.
// Building the fixture by hand rather than with pdfcpu keeps the "plain PDF"
// side of the encryption test independent of the library under test.
const minimalPDF = `%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]/Resources<</Font<</F1 4 0 R>>>>/Contents 5 0 R>>endobj
4 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj
5 0 obj<</Length 46>>stream
BT /F1 18 Tf 20 100 Td (HELLO NUSSYNC) Tj ET
endstream
endobj
trailer<</Size 6/Root 1 0 R>>
`

func writePlainPDF(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(minimalPDF), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeEncryptedPDF produces an owner-password-protected PDF with an empty user
// password — the shape NUS lecture PDFs arrive in: they open in any viewer but
// declare /Encrypt and set restriction flags.
func writeEncryptedPDF(t *testing.T, src, dst string) {
	t.Helper()
	api.DisableConfigDir()
	conf := model.NewAESConfiguration("", "ownersecret", 256)
	conf.ValidationMode = model.ValidationRelaxed
	if err := api.EncryptFile(src, dst, conf); err != nil {
		t.Skipf("pdfcpu could not encrypt the fixture: %v", err)
	}
}

func TestEncryptedPDFDetection(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.pdf")
	writePlainPDF(t, plain)

	if EncryptedPDF(plain) {
		t.Error("plain PDF reported as encrypted")
	}

	enc := filepath.Join(dir, "enc.pdf")
	writeEncryptedPDF(t, plain, enc)
	if !EncryptedPDF(enc) {
		t.Error("owner-password PDF not reported as encrypted")
	}

	// Detection must be cheap and total: a missing file and a non-PDF are both
	// "not encrypted" rather than an error the callers would have to handle.
	if EncryptedPDF(filepath.Join(dir, "nope.pdf")) {
		t.Error("missing file reported as encrypted")
	}
	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if EncryptedPDF(txt) {
		t.Error("text file reported as encrypted")
	}
}

func TestResolveDecryptsEncryptedPDF(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	plain := filepath.Join(dir, "plain.pdf")
	writePlainPDF(t, plain)
	enc := filepath.Join(dir, "enc.pdf")
	writeEncryptedPDF(t, plain, enc)

	p := Resolve(enc, 42, func() string { return "extracted text" })
	if !p.Readable {
		t.Fatal("encrypted PDF did not resolve to anything readable")
	}
	if p.TextOnly {
		t.Fatal("fell back to text even though the PDF decrypts")
	}
	if p.Path == enc {
		t.Fatal("resolved to the encrypted original instead of a cached copy")
	}
	if EncryptedPDF(p.Path) {
		t.Error("the cached copy is still encrypted")
	}

	// Second call must reuse the cache rather than rebuild it.
	fi, err := os.Stat(p.Path)
	if err != nil {
		t.Fatal(err)
	}
	again := Resolve(enc, 42, func() string { return "extracted text" })
	if again.Path != p.Path {
		t.Errorf("cache key not stable: %s then %s", p.Path, again.Path)
	}
	fi2, err := os.Stat(again.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !fi2.ModTime().Equal(fi.ModTime()) {
		t.Error("cached copy was rewritten on the second call")
	}
}

func TestResolveFallsBackToText(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	// A file that is a PDF by name but that pdfcpu cannot rewrite: the resolver
	// must reach the extracted text rather than give up or hand back a path the
	// Read tool will refuse.
	broken := filepath.Join(dir, "broken.pdf")
	if err := os.WriteFile(broken, []byte("%PDF-1.4\nnot really a pdf\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Resolve(broken, 7, func() string { return "lecture text about MDPs" })
	if !p.TextOnly {
		t.Fatalf("expected a text-only fallback, got %+v", p)
	}
	if !p.Readable || !strings.HasSuffix(p.Path, ".txt") {
		t.Fatalf("text fallback is not a readable .txt: %+v", p)
	}
	if p.Note == "" {
		t.Error("text fallback carries no caveat for the prompt")
	}
	body, err := os.ReadFile(p.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "MDPs") {
		t.Errorf("extract does not hold the text: %q", body)
	}
}

func TestResolveNoTextAvailable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	broken := filepath.Join(dir, "broken.pdf")
	if err := os.WriteFile(broken, []byte("%PDF-1.4\nnot really a pdf\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Nothing extracted either: hand the original over so Claude reports the
	// real failure instead of the job silently losing its only source.
	p := Resolve(broken, 8, func() string { return "   " })
	if p.Path != broken || !p.Readable || p.TextOnly {
		t.Fatalf("expected the original as a last resort, got %+v", p)
	}
}

func TestResolvePassesThroughNonPDFs(t *testing.T) {
	dir := t.TempDir()

	md := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(md, []byte("# notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := Resolve(md, 1, nil); p.Path != md || !p.Readable || p.TextOnly {
		t.Errorf("markdown should pass straight through: %+v", p)
	}

	// Formats the Read tool cannot open keep the existing inline-text path,
	// signalled by Readable == false.
	pptx := filepath.Join(dir, "deck.pptx")
	if err := os.WriteFile(pptx, []byte("PK"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := Resolve(pptx, 2, nil); p.Path != pptx || p.Readable {
		t.Errorf("pptx should not be marked readable: %+v", p)
	}

	if p := Resolve("", 3, nil); p.Path != "" || p.Readable {
		t.Errorf("empty path should resolve to nothing: %+v", p)
	}
}

func TestResolveKeepsSmallPDFButNormalisesIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	plain := filepath.Join(dir, "plain.pdf")
	writePlainPDF(t, plain)

	p := Resolve(plain, 99, nil)
	if !p.Readable || p.TextOnly {
		t.Fatalf("a small plain PDF must stay a readable PDF: %+v", p)
	}
	// Even an unencrypted PDF goes through pdfcpu: the Read tool refuses some
	// structurally odd small files that a rewrite repairs.
	if p.Path == plain {
		t.Error("expected a normalised cached copy, not the original")
	}
	if filepath.Dir(p.Path) == dir {
		t.Error("cached copy should live in the study cache, not beside the source")
	}
}
