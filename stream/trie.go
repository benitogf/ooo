package stream

import (
	"strings"
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

// splitPath splits a path into segments.
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// insert adds a pool to the trie at the given key path.
func (t *poolTrie) insert(key string, pool *Pool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	segments := splitPath(key)
	node := t.root

	for _, seg := range segments {
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
	}
	node.pool = pool
}

// remove removes a pool from the trie at the given key path.
// Returns true if the pool was found and removed.
func (t *poolTrie) remove(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	segments := splitPath(key)
	return t.removeRecursive(t.root, segments, 0)
}

// removeRecursive removes a path from the trie and cleans up empty nodes.
func (t *poolTrie) removeRecursive(node *trieNode, segments []string, depth int) bool {
	if depth == len(segments) {
		if node.pool == nil {
			return false
		}
		node.pool = nil
		return true
	}

	seg := segments[depth]
	child, exists := node.children[seg]
	if !exists {
		return false
	}

	removed := t.removeRecursive(child, segments, depth+1)

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
	pathSegments := splitPath(path)

	// We need to find pools where:
	// 1. pool.Key matches path (pool has wildcard that matches path)
	// 2. path matches pool.Key (path has wildcard that matches pool)
	//
	// For case 1: traverse trie, at each level check both exact match and "*"
	// For case 2: if path contains "*", we need to check all pools at that level

	t.matchRecursive(t.root, pathSegments, 0, seen)

	results := make([]*Pool, 0, len(seen))
	for pool := range seen {
		results = append(results, pool)
	}
	return results
}

// matchRecursive traverses the trie to find matching pools.
func (t *poolTrie) matchRecursive(node *trieNode, pathSegments []string, depth int, seen map[*Pool]struct{}) {
	if depth == len(pathSegments) {
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

	seg := pathSegments[depth]

	// Case 1: Exact segment match
	if child, exists := node.children[seg]; exists {
		t.matchRecursive(child, pathSegments, depth+1, seen)
	}

	// Case 2: Wildcard in trie matches any segment in path
	// e.g., pool="users/*" should match path="users/123"
	if seg != "*" {
		if wildcard, exists := node.children["*"]; exists {
			t.matchRecursive(wildcard, pathSegments, depth+1, seen)
		}
	}

	// Case 3: Wildcard in path matches any segment in trie
	// e.g., path="users/*" should match pool="users/123"
	if seg == "*" {
		for childSeg, child := range node.children {
			if childSeg != "*" {
				t.matchRecursive(child, pathSegments, depth+1, seen)
			}
		}
		// Also match wildcard to wildcard
		if wildcard, exists := node.children["*"]; exists {
			t.matchRecursive(wildcard, pathSegments, depth+1, seen)
		}
	}
}

// get retrieves a pool by exact key.
func (t *poolTrie) get(key string) *Pool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	segments := splitPath(key)
	node := t.root

	for _, seg := range segments {
		child, exists := node.children[seg]
		if !exists {
			return nil
		}
		node = child
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
