package stream

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrieInsertAndGet(t *testing.T) {
	trie := newPoolTrie()

	pool1 := &Pool{Key: "users"}
	pool2 := &Pool{Key: "users/123"}
	pool3 := &Pool{Key: "users/*"}
	pool4 := &Pool{Key: "posts/456"}

	trie.insert("users", pool1)
	trie.insert("users/123", pool2)
	trie.insert("users/*", pool3)
	trie.insert("posts/456", pool4)

	require.Equal(t, pool1, trie.get("users"))
	require.Equal(t, pool2, trie.get("users/123"))
	require.Equal(t, pool3, trie.get("users/*"))
	require.Equal(t, pool4, trie.get("posts/456"))
	require.Nil(t, trie.get("nonexistent"))
	require.Nil(t, trie.get("users/999"))
}

func TestTrieRemove(t *testing.T) {
	trie := newPoolTrie()

	pool1 := &Pool{Key: "users/123"}
	pool2 := &Pool{Key: "users/456"}

	trie.insert("users/123", pool1)
	trie.insert("users/456", pool2)

	require.Equal(t, 2, trie.size())

	removed := trie.remove("users/123")
	require.True(t, removed)
	require.Nil(t, trie.get("users/123"))
	require.Equal(t, pool2, trie.get("users/456"))
	require.Equal(t, 1, trie.size())

	removed = trie.remove("nonexistent")
	require.False(t, removed)
}

func TestTrieFindMatching_ExactMatch(t *testing.T) {
	trie := newPoolTrie()

	pool := &Pool{Key: "users/123"}
	trie.insert("users/123", pool)

	matches := trie.findMatching("users/123")
	require.Len(t, matches, 1)
	require.Equal(t, pool, matches[0])
}

func TestTrieFindMatching_WildcardPoolMatchesPath(t *testing.T) {
	trie := newPoolTrie()

	// Pool with wildcard should match specific path
	pool := &Pool{Key: "users/*"}
	trie.insert("users/*", pool)

	matches := trie.findMatching("users/123")
	require.Len(t, matches, 1)
	require.Equal(t, pool, matches[0])

	matches = trie.findMatching("users/456")
	require.Len(t, matches, 1)
	require.Equal(t, pool, matches[0])

	// Should not match different prefix
	matches = trie.findMatching("posts/123")
	require.Len(t, matches, 0)
}

func TestTrieFindMatching_PathWildcardMatchesPools(t *testing.T) {
	trie := newPoolTrie()

	// Specific pools should be matched by wildcard path
	pool1 := &Pool{Key: "users/123"}
	pool2 := &Pool{Key: "users/456"}
	pool3 := &Pool{Key: "posts/789"}

	trie.insert("users/123", pool1)
	trie.insert("users/456", pool2)
	trie.insert("posts/789", pool3)

	matches := trie.findMatching("users/*")
	require.Len(t, matches, 2)

	// Should not match posts
	matches = trie.findMatching("posts/*")
	require.Len(t, matches, 1)
	require.Equal(t, pool3, matches[0])
}

func TestTrieFindMatching_BothWildcards(t *testing.T) {
	trie := newPoolTrie()

	// Wildcard pool should match wildcard path
	pool := &Pool{Key: "users/*"}
	trie.insert("users/*", pool)

	matches := trie.findMatching("users/*")
	require.Len(t, matches, 1)
	require.Equal(t, pool, matches[0])
}

func TestTrieFindMatching_DeepPaths(t *testing.T) {
	trie := newPoolTrie()

	pool1 := &Pool{Key: "api/v1/users/123"}
	pool2 := &Pool{Key: "api/v1/users/*"}
	pool3 := &Pool{Key: "api/v1/posts/456"}

	trie.insert("api/v1/users/123", pool1)
	trie.insert("api/v1/users/*", pool2)
	trie.insert("api/v1/posts/456", pool3)

	// Exact match
	matches := trie.findMatching("api/v1/users/123")
	require.Len(t, matches, 2) // both exact and wildcard match

	// Wildcard path matches specific pools
	matches = trie.findMatching("api/v1/users/*")
	require.Len(t, matches, 2) // pool1 and pool2

	// Different path
	matches = trie.findMatching("api/v1/posts/456")
	require.Len(t, matches, 1)
	require.Equal(t, pool3, matches[0])
}

func TestTrieAll(t *testing.T) {
	trie := newPoolTrie()

	pool1 := &Pool{Key: "users/123"}
	pool2 := &Pool{Key: "users/456"}
	pool3 := &Pool{Key: "posts/789"}

	trie.insert("users/123", pool1)
	trie.insert("users/456", pool2)
	trie.insert("posts/789", pool3)

	all := trie.all()
	require.Len(t, all, 3)
}

func TestTrieSize(t *testing.T) {
	trie := newPoolTrie()

	require.Equal(t, 0, trie.size())

	trie.insert("a", &Pool{Key: "a"})
	require.Equal(t, 1, trie.size())

	trie.insert("b", &Pool{Key: "b"})
	require.Equal(t, 2, trie.size())

	trie.remove("a")
	require.Equal(t, 1, trie.size())
}

// Benchmark to compare trie vs linear scan
func BenchmarkTrieFindMatching(b *testing.B) {
	trie := newPoolTrie()

	// Insert 100 pools with various paths
	for i := 0; i < 100; i++ {
		key := "users/" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		trie.insert(key, &Pool{Key: key})
	}
	// Add some wildcard pools
	trie.insert("users/*", &Pool{Key: "users/*"})
	trie.insert("posts/*", &Pool{Key: "posts/*"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = trie.findMatching("users/a0")
	}
}

func BenchmarkTrieFindMatchingWildcard(b *testing.B) {
	trie := newPoolTrie()

	// Insert 100 pools with various paths
	for i := 0; i < 100; i++ {
		key := "users/" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		trie.insert(key, &Pool{Key: key})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = trie.findMatching("users/*")
	}
}

// BenchmarkTrie_100Pools benchmarks trie with 100 pools
func BenchmarkTrie_100Pools(b *testing.B) {
	trie := newPoolTrie()
	for i := 0; i < 100; i++ {
		key := "users/" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		trie.insert(key, &Pool{Key: key})
	}
	trie.insert("users/*", &Pool{Key: "users/*"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = trie.findMatching("users/a0")
	}
}

// BenchmarkTrie_1000Pools benchmarks trie with 1000 pools
func BenchmarkTrie_1000Pools(b *testing.B) {
	trie := newPoolTrie()
	for i := 0; i < 1000; i++ {
		key := "users/" + string(rune('a'+i%26)) + string(rune('0'+i/26%10)) + string(rune('0'+i/260))
		trie.insert(key, &Pool{Key: key})
	}
	trie.insert("users/*", &Pool{Key: "users/*"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = trie.findMatching("users/a00")
	}
}
