package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nussync/internal/config"
	"nussync/internal/store"
)

// courseColors is a fixed, readable palette; index chosen from the course ID.
var courseColors = []string{
	"#4f46e5", "#0891b2", "#059669", "#d97706", "#dc2626",
	"#7c3aed", "#db2777", "#0284c7", "#65a30d", "#ea580c",
	"#0d9488", "#9333ea",
}

// colorFor derives a stable hex colour from a course ID.
func colorFor(id int) string {
	if id < 0 {
		id = -id
	}
	return courseColors[id%len(courseColors)]
}

func toCourse(c store.Course) Course {
	return Course{
		ID:         c.ID,
		Code:       c.Code,
		Name:       c.Name,
		Term:       c.Term,
		FileCount:  c.FileCount,
		LastSynced: c.LastSynced,
		Enabled:    c.Enabled,
		Color:      colorFor(c.ID),
	}
}

// toFileNode maps a store row. seenAt is the feed_seen_at stamp; a file whose
// last_changed_at is newer is flagged IsNew so the Files view can badge it.
func toFileNode(f store.File, seenAt string) FileNode {
	return FileNode{
		ID:         f.ID,
		CourseID:   f.CourseID,
		Name:       f.Name,
		Path:       f.AbsPath,
		RelPath:    f.RelPath,
		IsDir:      false,
		Size:       f.Size,
		ModifiedAt: f.ModifiedAt,
		Source:     f.Source,
		Module:     f.Module,
		Synced:     f.Synced,
		IsNew:      f.LastChangedAt != "" && f.LastChangedAt > seenAt,
	}
}

// toFeedItem maps one what's-new row.
func toFeedItem(ff store.FeedFile) FeedItem {
	kind := "updated"
	if ff.New {
		kind = "new"
	}
	return FeedItem{
		ID:         ff.File.ID,
		CourseID:   ff.File.CourseID,
		CourseCode: ff.CourseCode,
		Name:       ff.File.Name,
		Path:       ff.File.AbsPath,
		RelPath:    ff.File.RelPath,
		Size:       ff.File.Size,
		ChangedAt:  ff.ChangedAt,
		Kind:       kind,
		Module:     ff.File.Module,
	}
}

func toDeadline(d store.Deadline) Deadline {
	return Deadline{
		ID:             d.ID,
		CourseID:       d.CourseID,
		CourseCode:     d.CourseCode,
		Title:          d.Title,
		Type:           d.Type,
		DueAt:          d.DueAt,
		Submitted:      d.Submitted,
		URL:            d.URL,
		PointsPossible: d.PointsPossible,
	}
}

func toAnnouncement(a store.Announcement) Announcement {
	return Announcement{
		ID:         a.ID,
		CourseCode: a.CourseCode,
		Title:      a.Title,
		PostedAt:   a.PostedAt,
		HTML:       a.HTML,
		Text:       a.Text,
		URL:        a.URL,
		Read:       a.Read,
	}
}

func toGrade(g store.Grade) Grade {
	return Grade{
		CourseCode: g.CourseCode,
		Title:      g.Title,
		Score:      g.Score,
		Possible:   g.Possible,
		GradedAt:   g.GradedAt,
		URL:        g.URL,
		Mean:       g.Mean,
	}
}

func toSettings(c config.Settings) Settings {
	return Settings{
		CanvasURL:           c.CanvasURL,
		CanvasToken:         c.CanvasToken,
		SyncDir:             c.SyncDir,
		TelegramToken:       c.TelegramToken,
		TelegramChatID:      c.TelegramChatID,
		ReminderLadder:      c.ReminderLadder,
		MaxFileMB:           c.MaxFileMB,
		SkipExts:            c.SkipExts,
		SyncIntervalMin:     c.SyncIntervalMin,
		NotifyAnnouncements: c.NotifyAnnouncements,
		NotifyGrades:        c.NotifyGrades,
		NotifyDesktop:       c.NotifyDesktop,
		LaunchAtLogin:       c.LaunchAtLogin,
		Theme:               c.Theme,
		Hotkey:              c.Hotkey,
	}
}

func fromSettings(s Settings) config.Settings {
	return config.Settings{
		CanvasURL:           s.CanvasURL,
		CanvasToken:         s.CanvasToken,
		SyncDir:             s.SyncDir,
		TelegramToken:       s.TelegramToken,
		TelegramChatID:      s.TelegramChatID,
		ReminderLadder:      s.ReminderLadder,
		MaxFileMB:           s.MaxFileMB,
		SkipExts:            s.SkipExts,
		SyncIntervalMin:     s.SyncIntervalMin,
		NotifyAnnouncements: s.NotifyAnnouncements,
		NotifyGrades:        s.NotifyGrades,
		NotifyDesktop:       s.NotifyDesktop,
		LaunchAtLogin:       s.LaunchAtLogin,
		Theme:               s.Theme,
		Hotkey:              s.Hotkey,
	}
}

// BuildTree turns a flat file list into a nested directory tree, using each
// file's RelPath (forward slashes) relative to the course root.
func BuildTree(courseID int, root string, files []store.File, seenAt string) []FileNode {
	type dirNode struct {
		node     *FileNode
		children map[string]*dirNode
		files    []FileNode
	}
	newDir := func(name, rel string) *dirNode {
		abs := root
		if rel != "" {
			abs = filepath.Join(append([]string{root}, strings.Split(rel, "/")...)...)
		}
		return &dirNode{
			node: &FileNode{
				CourseID: courseID, Name: name, Path: abs, RelPath: rel, IsDir: true,
			},
			children: map[string]*dirNode{},
		}
	}

	top := newDir("", "")
	for _, f := range files {
		parts := strings.Split(f.RelPath, "/")
		cur := top
		relSoFar := ""
		for i := 0; i < len(parts)-1; i++ {
			seg := parts[i]
			if seg == "" {
				continue
			}
			if relSoFar == "" {
				relSoFar = seg
			} else {
				relSoFar += "/" + seg
			}
			child, ok := cur.children[seg]
			if !ok {
				child = newDir(seg, relSoFar)
				cur.children[seg] = child
			}
			cur = child
		}
		cur.files = append(cur.files, toFileNode(f, seenAt))
	}

	var collect func(d *dirNode) []FileNode
	collect = func(d *dirNode) []FileNode {
		names := make([]string, 0, len(d.children))
		for n := range d.children {
			names = append(names, n)
		}
		sort.Strings(names)

		out := make([]FileNode, 0, len(names)+len(d.files))
		for _, n := range names {
			c := d.children[n]
			node := *c.node
			node.Children = collect(c)
			for _, ch := range node.Children {
				node.Size += ch.Size
			}
			out = append(out, node)
		}
		sort.Slice(d.files, func(i, j int) bool {
			return strings.ToLower(d.files[i].Name) < strings.ToLower(d.files[j].Name)
		})
		out = append(out, d.files...)
		return out
	}
	return collect(top)
}

// humanBytes renders a byte count for CLI output.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
