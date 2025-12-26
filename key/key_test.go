package key

import (
	"testing"

	"github.com/benitogf/ooo/monotonic"
	"github.com/stretchr/testify/require"
)

func TestIsValidPatterns(t *testing.T) {
	// Valid patterns (previously tested via GlobRegex)
	require.True(t, IsValid("*"))
	require.True(t, IsValid("a/b/*"))
	require.True(t, IsValid("a/b/c"))
	// Invalid patterns
	require.False(t, IsValid("/a/b/c")) // starts with /
	require.False(t, IsValid("a/b/c/")) // ends with /
	require.False(t, IsValid("a:b/c"))  // contains invalid char :
	require.False(t, IsValid(""))       // empty string
}

func TestKeyIsValid(t *testing.T) {
	require.True(t, IsValid("test"))
	require.True(t, IsValid("test/1"))
	require.False(t, IsValid("test//1"))
	require.False(t, IsValid("test///1"))
}

func TestKeyMatch(t *testing.T) {
	require.True(t, Match("*", "thing"))
	require.True(t, Match("games/*", "games/*"))
	require.True(t, Match("thing/*", "thing/123"))
	require.True(t, Match("thing/123/*", "thing/123/234"))
	require.True(t, Match("thing/glob/*/*", "thing/glob/test/234"))
	require.True(t, Match("thing/123/*", "thing/123/123"))
	require.False(t, Match("thing/*/*", "thing/123/234/234"))
	require.False(t, Match("thing/123", "thing/12"))
	require.False(t, Match("thing/1", "thing/123"))
	require.False(t, Match("thing/123/*", "thing/123/123/123"))
}

func TestValidateGlob(t *testing.T) {
	// Valid cases - no glob
	require.NoError(t, ValidateGlob("test"))
	require.NoError(t, ValidateGlob("test/path"))
	require.NoError(t, ValidateGlob("test/path/deep"))

	// Valid cases - glob at end
	require.NoError(t, ValidateGlob("test/*"))
	require.NoError(t, ValidateGlob("test/path/*"))
	require.NoError(t, ValidateGlob("*"))

	// Invalid - multiple globs
	require.Error(t, ValidateGlob("test/*/*"))
	require.Error(t, ValidateGlob("*/test/*"))
	require.Error(t, ValidateGlob("*/*"))
	require.ErrorIs(t, ValidateGlob("test/*/*"), ErrInvalidGlobCount)

	// Invalid - glob not at end
	require.Error(t, ValidateGlob("*/test"))
	require.Error(t, ValidateGlob("test/*/path"))
	require.Error(t, ValidateGlob("*test"))
	require.ErrorIs(t, ValidateGlob("*/test"), ErrGlobNotAtEnd)
}

func BenchmarkIsValid(b *testing.B) {
	for b.Loop() {
		IsValid("test/path/to/resource")
	}
}

func BenchmarkMatch(b *testing.B) {
	for b.Loop() {
		Match("test/path/*", "test/path/123")
	}
}

func BenchmarkBuild(b *testing.B) {
	monotonic.Init()
	b.ResetTimer()
	for b.Loop() {
		Build("test/path/*")
	}
}

func BenchmarkValidateGlob(b *testing.B) {
	for b.Loop() {
		ValidateGlob("test/path/*")
	}
}
