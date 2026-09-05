package sync

import (
	"os"
	"path/filepath"

	"nussync/internal/store"
)

// MigrateCourseFolders renames course directories that still use the legacy
// naming (every cross-listed code joined by "_") to the current naming (first
// code segment only) and rewrites the stored absolute paths to match, so the
// already-downloaded files are not fetched again.
//
// It is idempotent and best-effort: any course whose rename fails is left on
// the old layout and simply re-downloads later.
func MigrateCourseFolders(st *store.Store, syncDir string) (int, error) {
	if syncDir == "" || st == nil {
		return 0, nil
	}
	courses, err := st.Courses()
	if err != nil {
		return 0, err
	}

	moved := 0
	for _, c := range courses {
		legacy := LegacyCourseFolder(c.Code)
		current := CourseFolder(c.Code)
		if legacy == current {
			continue
		}
		oldDir := filepath.Join(syncDir, legacy)
		newDir := filepath.Join(syncDir, current)

		oldInfo, err := os.Stat(oldDir)
		if err != nil || !oldInfo.IsDir() {
			continue // nothing to migrate
		}
		if _, err := os.Stat(newDir); err == nil {
			continue // target already exists; leave both alone
		}
		if err := os.Rename(oldDir, newDir); err != nil {
			continue
		}
		if _, err := st.RewriteCoursePaths(c.ID, oldDir, newDir); err != nil {
			// Paths now point at a directory that no longer exists; the next
			// sync re-downloads into the new folder, which is still correct.
			continue
		}
		moved++
	}
	return moved, nil
}
