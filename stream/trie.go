package stream

import (
	"sync"
)

// trieNode represents a node in the path trie.
// Each node can have children keyed by path segment (e.g., "users", "123", "*").
type trieNode struct {
	children map[string]*trieNode
	pool     *Pool // non-nil if this node represents a complete pool path
}

// poolTrie is a trie data structure for efficient pool path matching.
// It supports O(k) lookup where k is the number of path segments,
// instead of O(n) where n is the total number of pools.
type poolTrie struct {
	mu   sync.RWMutex
	root *trieNode
}

// newPoolTrie creates a new empty pool trie.
func newPoolTrie() *poolTrie {
	return &poolTrie{
		root: &trieNode{
			children: make(map[string]*trieNode),
		},
	}
}

// pathSegment represents a segment of a path without allocation.
type pathSegment struct {
	start, end int
}

// countSegments counts the number of segments in a path (zero-alloc).
func countSegments(path string) int {
	if path == "" {
		return 0
	}
	count := 1
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			count++
		}
	}
	return count
}

// getSegment returns the segment at the given index (zero-alloc).
// Returns the segment as a substring of the original path.
func getSegment(path string, index int) string {
	if path == "" {
		return ""
	}
	start := 0
	current := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if current == index {
				return path[start:i]
			}
			current++
			start = i + 1
		}
	}
	return ""
}

// iterSegments iterates over path segments without allocation.
// The callback receives each segment as a substring of the original path.
// Return false from the callback to stop iteration.
func iterSegments(path string, fn func(segment string) bool) {
	if path == "" {
		return
	}
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if !fn(path[start:i]) {
				return
			}
			start = i + 1
		}
	}
}

// insert adds a pool to the trie at the given key path.
func (t *poolTrie) insert(key string, pool *Pool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	node := t.root
	iterSegments(key, func(seg string) bool {
		if node.children == nil {
			node.children = make(map[string]*trieNode)
		}
		child, exists := node.children[seg]
		if !exists {
			child = &trieNode{
				children: make(map[string]*trieNode),
			}
			node.children[seg] = child
		}
		node = child
		return true
	})
	node.pool = pool
}

// remove removes a pool from the trie at the given key path.
// Returns true if the pool was found and removed.
func (t *poolTrie) remove(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	numSegments := countSegments(key)
	return t.removeRecursive(t.root, key, numSegments, 0)
}

// removeRecursive removes a path from the trie and cleans up empty nodes.
func (t *poolTrie) removeRecursive(node *trieNode, path string, numSegments, depth int) bool {
	if depth == numSegments {
		if node.pool == nil {
			return false
		}
		node.pool = nil
		return true
	}

	seg := getSegment(path, depth)
	child, exists := node.children[seg]
	if !exists {
		return false
	}

	removed := t.removeRecursive(child, path, numSegments, depth+1)

	// Clean up empty child nodes
	if removed && child.pool == nil && len(child.children) == 0 {
		delete(node.children, seg)
	}

	return removed
}

// findMatching finds all pools that match the given path using key.Peer logic.
// This is the core optimization: instead of checking all pools, we traverse
// only the relevant branches of the trie.
func (t *poolTrie) findMatching(path string) []*Pool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Use a map to deduplicate results
	seen := make(map[*Pool]struct{})
	numSegments := countSegments(path)

	// We need to find pools where:
	// 1. pool.Key matches path (pool has wildcard that matches path)
	// 2. path matches pool.Key (path has wildcard that matches pool)
	//
	// For case 1: traverse trie, at each level check both exact match and "*"
	// For case 2: if path contains "*", we need to check all pools at that level

	t.matchRecursive(t.root, path, numSegments, 0, seen)

	results := make([]*Pool, 0, len(seen))
	for pool := range seen {
		results = append(results, pool)
	}
	return results
}

// matchRecursive traverses the trie to find matching pools.
func (t *poolTrie) matchRecursive(node *trieNode, path string, numSegments, depth int, seen map[*Pool]struct{}) {
	if depth == numSegments {
		// We've consumed all path segments
		// Check if there's a pool at this exact location
		if node.pool != nil {
			seen[node.pool] = struct{}{}
		}
		// Also check for wildcard pools at this level (e.g., path="users/123", pool="users/*")
		if wildcard, exists := node.children["*"]; exists && wildcard.pool != nil {
			seen[wildcard.pool] = struct{}{}
		}
		return
	}

	seg := getSegment(path, depth)

	// Case 1: Exact segment match
	if child, exists := node.children[seg]; exists {
		t.matchRecursive(child, path, numSegments, depth+1, seen)
	}

	// Case 2: Wildcard in trie matches any segment in path
	// e.g., pool="users/*" should match path="users/123"
	if seg != "*" {
		if wildcard, exists := node.children["*"]; exists {
			t.matchRecursive(wildcard, path, numSegments, depth+1, seen)
		}
	}

	// Case 3: Wildcard in path matches any segment in trie
	// e.g., path="users/*" should match pool="users/123"
	if seg == "*" {
		for childSeg, child := range node.children {
			if childSeg != "*" {
				t.matchRecursive(child, path, numSegments, depth+1, seen)
			}
		}
		// Also match wildcard to wildcard
		if wildcard, exists := node.children["*"]; exists {
			t.matchRecursive(wildcard, path, numSegments, depth+1, seen)
		}
	}
}

// get retrieves a pool by exact key.
func (t *poolTrie) get(key string) *Pool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.root
	var found = true
	iterSegments(key, func(seg string) bool {
		child, exists := node.children[seg]
		if !exists {
			found = false
			return false
		}
		node = child
		return true
	})
	if !found {
		return nil
	}
	return node.pool
}

// all returns all pools in the trie.
func (t *poolTrie) all() []*Pool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var results []*Pool
	t.collectAll(t.root, &results)
	return results
}

// collectAll recursively collects all pools in the trie.
func (t *poolTrie) collectAll(node *trieNode, results *[]*Pool) {
	if node.pool != nil {
		*results = append(*results, node.pool)
	}
	for _, child := range node.children {
		t.collectAll(child, results)
	}
}

// size returns the number of pools in the trie.
func (t *poolTrie) size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	t.countAll(t.root, &count)
	return count
}

// countAll recursively counts all pools in the trie.
func (t *poolTrie) countAll(node *trieNode, count *int) {
	if node.pool != nil {
		*count++
	}
	for _, child := range node.children {
		t.countAll(child, count)
	}
}
