package handlers

import (
	"strings"
	"unicode"
)

// NormalizeSiteName is the single normalization entry for site (网点) names (#2477).
//
// Rules (Decision Lock D2):
//   - collapse every run of whitespace-class runes into one ASCII space;
//   - drop leading/trailing whitespace (equivalent to strings.TrimSpace);
//   - whitespace includes U+3000 (full-width ideographic space), TAB and newline,
//     which is what makes "测试网点 A" and "测试网点　A" (U+3000) the same token;
//   - do NOT fold case and do NOT run full Unicode NFC: case differences are
//     owned by IAM organization-name uniqueness (Owner ruling B, 2026-11-26).
//
// The duplicate-name authority is IAM (Owner ruling B), so this function never
// performs a local existence check. It only guarantees that the same string is
// written to IAM and to the local sites.name mirror (D3).
func NormalizeSiteName(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	pendingSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			// Defer the space: it is only emitted once a real rune follows, which
			// folds consecutive whitespace and trims the trailing run automatically.
			pendingSpace = true
			continue
		}
		if pendingSpace && b.Len() > 0 {
			b.WriteByte(' ')
		}
		pendingSpace = false
		b.WriteRune(r)
	}

	return b.String()
}
