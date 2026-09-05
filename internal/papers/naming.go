package papers

import nsync "nussync/internal/sync"

// sanitizeSegment reuses the sync package's Windows-safe path segment rules
// (illegal characters, reserved device names, trailing dots, length cap).
// Aliased because this package also uses the standard library's sync.
func sanitizeSegment(s string) string { return nsync.SanitizeSegment(s) }
