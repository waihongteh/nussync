package sync

import "testing"

func TestHTMLToText(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"plain", "hello", "hello"},
		{"tags stripped", "<p>Hi <b>there</b></p>", "Hi there"},
		{"entities", "A &amp; B &lt;x&gt; &nbsp;C", "A & B <x> C"},
		{"br becomes newline", "a<br>b", "a\nb"},
		{"style block dropped", `<style>.x{pointer-events:auto}</style><p>Real text</p>`, "Real text"},
		{"script block dropped", `<script>var a = 1 < 2;</script><p>Body</p>`, "Body"},
		{"list items", "<ul><li>one</li><li>two</li></ul>", "one\ntwo"},
		{"collapses blank lines", "<p>a</p><p></p><p></p><p>b</p>", "a\n\nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HTMLToText(tc.in); got != tc.want {
				t.Errorf("HTMLToText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
