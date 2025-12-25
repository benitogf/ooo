package key

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/benitogf/ooo/monotonic"
)

var (
	ErrInvalidGlobCount = errors.New("key: contains more than one glob pattern")
	ErrGlobNotAtEnd     = errors.New("key: glob pattern must be at the end of the path")
)

// IsGlob returns true if the path ends with a glob pattern (/*).
func IsGlob(path string) bool {
	return LastIndex(path) == "*"
}

// HasGlob returns true if the path contains a glob pattern (*) anywhere.
func HasGlob(path string) bool {
	return strings.Contains(path, "*")
}

// isValidChar checks if a character is valid for a key path.
// Valid characters: a-z, A-Z, 0-9, *, /
func isValidChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '*' || c == '/'
}

// isValidEndChar checks if a character is valid for start/end of a key.
// Valid characters: a-z, A-Z, 0-9, *
func isValidEndChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '*'
}

// IsValid checks that the key pattern is supported.
// Uses string-based validation instead of regex for better performance.
// Valid patterns: ^[a-zA-Z*\d]$|^[a-zA-Z*\d][a-zA-Z*\d/]+[a-zA-Z*\d]$
func IsValid(key string) bool {
	if len(key) == 0 {
		return false
	}

	// Check for invalid sequences
	if strings.Contains(key, "//") || strings.Contains(key, "**") {
		return false
	}

	// Single character: must be valid end char (no /)
	if len(key) == 1 {
		return isValidEndChar(key[0])
	}

	// Multi-character: first and last must be valid end chars
	if !isValidEndChar(key[0]) || !isValidEndChar(key[len(key)-1]) {
		return false
	}

	// Middle characters can include /
	for i := 1; i < len(key)-1; i++ {
		if !isValidChar(key[i]) {
			return false
		}
	}

	return true
}

// Match checks if a key is part of a path (glob)
// Optimized for common patterns like "prefix/*" and "a/*/b/*"
func Match(path string, key string) bool {
	if path == key {
		return true
	}
	if !HasGlob(path) {
		return false
	}

	// Fast path: simple trailing glob "prefix/*"
	if strings.HasSuffix(path, "/*") && strings.Count(path, "*") == 1 {
		prefix := path[:len(path)-1] // "prefix/"
		if !strings.HasPrefix(key, prefix) {
			return false
		}
		// Check no additional slashes in the matched part
		suffix := key[len(prefix):]
		return !strings.Contains(suffix, "/")
	}

	// General case: use filepath.Match
	match, err := filepath.Match(path, key)
	if err != nil {
		return false
	}
	countPath := strings.Count(path, "/")
	countKey := strings.Count(key, "/")
	return match && countPath == countKey
}

func Peer(a string, b string) bool {
	return Match(a, b) || Match(b, a)
}

// LastIndex will return the last sub path of the key
func LastIndex(key string) string {
	return key[strings.LastIndexByte(key, '/')+1:]
}

// Build a new key for a path using monotonic clock
func Build(key string) string {
	idx := strings.IndexByte(key, '*')
	if idx == -1 {
		return key
	}

	now := monotonic.Now()
	// Use strings.Builder to avoid intermediate allocations
	var b strings.Builder
	b.Grow(len(key) + 16) // key length + hex timestamp estimate
	b.WriteString(key[:idx])
	b.WriteString(strconv.FormatInt(now, 16))
	if idx+1 < len(key) {
		b.WriteString(key[idx+1:])
	}
	return b.String()
}

// Decode key to timestamp
func Decode(key string) int64 {
	res, err := strconv.ParseInt(key, 16, 64)
	if err != nil {
		return 0
	}

	return res
}

// Contains find match in an array of paths
func Contains(s []string, e string) bool {
	for _, a := range s {
		if Peer(a, e) {
			return true
		}
	}
	return false
}

// ValidateGlob checks if a key has valid glob pattern placement.
// Returns an error if:
// - More than one glob (*) is present
// - Glob is not at the end of the path
// Returns nil if the key is valid or has no glob.
func ValidateGlob(key string) error {
	countGlob := strings.Count(key, "*")
	if countGlob > 1 {
		return ErrInvalidGlobCount
	}
	if countGlob == 1 {
		where := strings.Index(key, "*")
		if where != len(key)-1 {
			return ErrGlobNotAtEnd
		}
	}
	return nil
}
