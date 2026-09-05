package sync

import (
	"regexp"
	"strconv"
	"strings"
)

// fileLinkRe matches every form Canvas uses to reference a file inside rich
// HTML bodies (page bodies, assignment descriptions, announcement messages):
//
//	/courses/97040/files/1234567?wrap=1
//	/courses/97040/files/1234567/download?download_frd=1
//	/api/v1/courses/97040/files/1234567          (data-api-endpoint)
//	/files/1234567/preview                       (embedded <img>/<iframe>)
//	https://canvas.nus.edu.sg/files/1234567
//
// A bare "/files/<digits>" covers all of them, so one pattern suffices; the
// digit run is greedy, so no id is ever matched truncated.
var fileLinkRe = regexp.MustCompile(`/files/([0-9]+)`)

// FileIDsInHTML extracts Canvas file ids referenced anywhere in an HTML body,
// in first-appearance order, de-duplicated. Ids may belong to another course;
// the caller resolves each through /api/v1/files/:id and drops the ones it is
// not allowed to see.
func FileIDsInHTML(body string) []int {
	if body == "" {
		return nil
	}
	// Canvas sometimes stores bodies with escaped slashes (\/files\/123) or
	// HTML-encoded ampersands; normalise the cheap cases before the fast path.
	s := body
	if strings.Contains(s, `\/`) {
		s = strings.ReplaceAll(s, `\/`, "/")
	}
	if !strings.Contains(s, "/files/") {
		return nil
	}
	s = strings.ReplaceAll(s, "&amp;", "&")

	var out []int
	seen := map[int]bool{}
	for _, m := range fileLinkRe.FindAllStringSubmatch(s, -1) {
		id, err := strconv.Atoi(m[1])
		if err != nil || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
