package canvas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Self returns the authenticated user.
func (c *Client) Self(ctx context.Context) (User, error) {
	var u User
	err := c.getJSON(ctx, "/api/v1/users/self", &u)
	return u, err
}

// ActiveCourses lists active enrollments with term info.
func (c *Client) ActiveCourses(ctx context.Context) ([]Course, error) {
	var out []Course
	err := c.getPaged(ctx, "/api/v1/courses?enrollment_state=active&include[]=term&per_page=100",
		func(b []byte) error {
			var page []Course
			if err := json.Unmarshal(b, &page); err != nil {
				return err
			}
			for _, cc := range page {
				if cc.AccessRestrictedByDate || cc.ID == 0 {
					continue
				}
				out = append(out, cc)
			}
			return nil
		})
	return out, err
}

// Folders lists all folders of a course.
func (c *Client) Folders(ctx context.Context, courseID int) ([]Folder, error) {
	var out []Folder
	err := c.getPaged(ctx, fmt.Sprintf("/api/v1/courses/%d/folders?per_page=100", courseID),
		func(b []byte) error {
			var page []Folder
			if err := json.Unmarshal(b, &page); err != nil {
				return err
			}
			out = append(out, page...)
			return nil
		})
	return out, err
}

// FolderFiles lists files directly inside one folder.
func (c *Client) FolderFiles(ctx context.Context, folderID int) ([]File, error) {
	var out []File
	err := c.getPaged(ctx, fmt.Sprintf("/api/v1/folders/%d/files?per_page=100", folderID),
		func(b []byte) error {
			var page []File
			if err := json.Unmarshal(b, &page); err != nil {
				return err
			}
			out = append(out, page...)
			return nil
		})
	return out, err
}

// Modules lists course modules with items inlined where Canvas provides them.
func (c *Client) Modules(ctx context.Context, courseID int) ([]Module, error) {
	var out []Module
	err := c.getPaged(ctx, fmt.Sprintf("/api/v1/courses/%d/modules?include[]=items&per_page=50", courseID),
		func(b []byte) error {
			var page []Module
			if err := json.Unmarshal(b, &page); err != nil {
				return err
			}
			out = append(out, page...)
			return nil
		})
	if err != nil {
		return nil, err
	}
	// Canvas omits items when a module has too many; fetch those separately.
	for i := range out {
		if out[i].Items != nil {
			continue
		}
		items, err := c.ModuleItems(ctx, courseID, out[i].ID)
		if err != nil {
			if IsPermission(err) || IsNotFound(err) {
				continue
			}
			return out, err
		}
		out[i].Items = items
	}
	return out, nil
}

// ModuleItems lists items of a single module.
func (c *Client) ModuleItems(ctx context.Context, courseID, moduleID int) ([]ModuleItem, error) {
	var out []ModuleItem
	err := c.getPaged(ctx, fmt.Sprintf("/api/v1/courses/%d/modules/%d/items?per_page=100", courseID, moduleID),
		func(b []byte) error {
			var page []ModuleItem
			if err := json.Unmarshal(b, &page); err != nil {
				return err
			}
			out = append(out, page...)
			return nil
		})
	return out, err
}

// FileByID fetches a single file object (used for module File items).
func (c *Client) FileByID(ctx context.Context, fileID int) (File, error) {
	var f File
	err := c.getJSON(ctx, fmt.Sprintf("/api/v1/files/%d", fileID), &f)
	return f, err
}

// Assignments lists assignments (and quizzes) including the user's submission.
func (c *Client) Assignments(ctx context.Context, courseID int) ([]Assignment, error) {
	var out []Assignment
	err := c.getPaged(ctx, fmt.Sprintf("/api/v1/courses/%d/assignments?include[]=submission&per_page=100", courseID),
		func(b []byte) error {
			var page []Assignment
			if err := json.Unmarshal(b, &page); err != nil {
				return err
			}
			out = append(out, page...)
			return nil
		})
	return out, err
}

// AssignmentDetail fetches one assignment including the user's submission and,
// when Canvas is willing to disclose it, the class score statistics. Courses
// that hide statistics simply return no score_statistics object.
func (c *Client) AssignmentDetail(ctx context.Context, courseID, assignmentID int) (Assignment, error) {
	var a Assignment
	err := c.getJSON(ctx, fmt.Sprintf(
		"/api/v1/courses/%d/assignments/%d?include[]=score_statistics&include[]=submission",
		courseID, assignmentID), &a)
	return a, err
}

// Announcements lists announcement discussion topics for a course.
func (c *Client) Announcements(ctx context.Context, courseID int) ([]DiscussionTopic, error) {
	var out []DiscussionTopic
	err := c.getPaged(ctx, fmt.Sprintf("/api/v1/courses/%d/discussion_topics?only_announcements=true&per_page=50", courseID),
		func(b []byte) error {
			var page []DiscussionTopic
			if err := json.Unmarshal(b, &page); err != nil {
				return err
			}
			out = append(out, page...)
			return nil
		})
	return out, err
}

// FolderPath returns folder id -> path relative to the course root
// ("" for the root folder itself), derived from Canvas full_name
// ("course files/Lectures/Week 1").
func FolderPath(folders []Folder) map[int]string {
	byID := make(map[int]Folder, len(folders))
	for _, f := range folders {
		byID[f.ID] = f
	}
	out := make(map[int]string, len(folders))
	for _, f := range folders {
		out[f.ID] = relFromFullName(f.FullName)
	}
	// Fall back to walking parent links when full_name is missing.
	for id, p := range out {
		if p != "" || byID[id].FullName != "" {
			continue
		}
		var parts []string
		cur := byID[id]
		for depth := 0; depth < 32; depth++ {
			if cur.ParentFolderID == nil {
				break
			}
			parent, ok := byID[*cur.ParentFolderID]
			if !ok {
				break
			}
			parts = append([]string{cur.Name}, parts...)
			cur = parent
		}
		out[id] = strings.Join(parts, "/")
	}
	return out
}

// relFromFullName strips the Canvas root segment ("course files").
func relFromFullName(full string) string {
	full = strings.Trim(full, "/")
	if full == "" {
		return ""
	}
	parts := strings.Split(full, "/")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[1:], "/")
}

// SortFolders orders folders shallowest-first for deterministic walking.
func SortFolders(fs []Folder) {
	sort.Slice(fs, func(i, j int) bool {
		a, b := fs[i].FullName, fs[j].FullName
		if a == b {
			return fs[i].ID < fs[j].ID
		}
		return a < b
	})
}

// Pages lists a course's wiki pages (bodies omitted by Canvas). Courses with
// the Pages tab disabled answer 403/404; callers treat that as "no pages".
func (c *Client) Pages(ctx context.Context, courseID int) ([]Page, error) {
	var out []Page
	err := c.getPaged(ctx, fmt.Sprintf("/api/v1/courses/%d/pages?per_page=100", courseID),
		func(b []byte) error {
			var page []Page
			if err := json.Unmarshal(b, &page); err != nil {
				return err
			}
			out = append(out, page...)
			return nil
		})
	return out, err
}

// PageBody fetches one wiki page including its body HTML. pageURL is the page
// slug ("week-1-overview"), as carried by module items in page_url.
func (c *Client) PageBody(ctx context.Context, courseID int, pageURL string) (Page, error) {
	var p Page
	err := c.getJSON(ctx, fmt.Sprintf("/api/v1/courses/%d/pages/%s",
		courseID, url.PathEscape(pageURL)), &p)
	return p, err
}
