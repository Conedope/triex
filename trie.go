// Package triex implements a Unicode-aware prefix tree (trie) with insert,
// search/delete, prefix enumeration and autocomplete, prefix-sum aggregation,
// longest-prefix lookup, lexicographic extremes, and JSON persistence.
//
// Words are stored as sequences of runes; every node carries a terminal flag,
// and the root may itself be terminal, so the empty string "" is a valid
// stored word. Words are always ordered lexicographically by rune (code point)
// order, which matches Go's byte-wise string ordering for UTF-8.
//
// The zero value of Trie is not usable; construct with New.
// A Trie is safe for concurrent readers and for one writer at a time: mutating
// methods (Insert/Delete) take an exclusive lock while read methods share a
// read lock, so parallel Contains/probe calls may run alongside each other.
package triex

import (
	"encoding/json"
	"errors"
	"io"
	"sort"
	"sync"
)

// ErrPrefixNotPresent is returned by Autocomplete when the requested prefix is
// not present in the trie (no node exists for it).
var ErrPrefixNotPresent = errors.New("triex: prefix not present")

// node is a single trie node holding one rune of input.
type node struct {
	children map[rune]*node
	word     bool
}

func newNode() *node { return &node{children: make(map[rune]*node)} }

// Trie is a Unicode-aware prefix tree. It is safe for concurrent reads and a
// single writer (see the package documentation).
type Trie struct {
	mu    sync.RWMutex
	root  *node
	count int
}

// New returns an empty Trie.
func New() *Trie {
	return &Trie{root: newNode()}
}

// Insert stores word, returning true if it was newly added and false if it was
// already present (duplicate insert changes nothing).
func (t *Trie) Insert(word string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.root == nil {
		t.root = newNode()
	}
	n := t.root
	for _, r := range word {
		c, ok := n.children[r]
		if !ok {
			c = newNode()
			n.children[r] = c
		}
		n = c
	}
	if n.word {
		return false
	}
	n.word = true
	t.count++
	return true
}

// Delete removes word, returning true if it was present and removed. Nodes left
// without descendants are pruned from the trie.
func (t *Trie) Delete(word string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.root == nil {
		t.root = newNode()
	}
	rs := []rune(word)
	n := t.root
	for _, r := range rs {
		c, ok := n.children[r]
		if !ok {
			return false
		}
		n = c
	}
	if !n.word {
		return false
	}
	n.word = false
	t.count--
	t.prune(t.root, rs)
	return true
}

// prune removes now-dead nodes along the path rs beneath n.
func (t *Trie) prune(n *node, rs []rune) {
	if len(rs) == 0 {
		return
	}
	r := rs[0]
	c := n.children[r]
	t.prune(c, rs[1:])
	if !c.word && len(c.children) == 0 {
		delete(n.children, r)
	}
}

// lookup returns the node at the end of word path, or nil.
func (t *Trie) lookup(word string) *node {
	n := t.root
	if n == nil {
		return nil
	}
	for _, r := range word {
		c, ok := n.children[r]
		if !ok {
			return nil
		}
		n = c
	}
	return n
}

// Contains reports whether word is stored in the trie.
func (t *Trie) Contains(word string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := t.lookup(word)
	return n != nil && n.word
}

// Count returns the number of stored words. The empty string, when stored,
// counts as one word.
func (t *Trie) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.count
}

// IsEmpty reports whether no words are stored.
func (t *Trie) IsEmpty() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.count == 0
}

// PrefixCount returns the number of stored words that start with prefix.
func (t *Trie) PrefixCount(prefix string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := t.lookup(prefix)
	if n == nil {
		return 0
	}
	return subtreeCount(n)
}

func subtreeCount(n *node) int {
	c := 0
	if n.word {
		c++
	}
	for _, ch := range n.children {
		c += subtreeCount(ch)
	}
	return c
}

// WordsWithPrefix returns up to limit stored words beginning with prefix, in
// lexicographic (rune) order. A limit <= 0 means unlimited. Prefixes that are
// not present simply yield no words.
func (t *Trie) WordsWithPrefix(prefix string, limit int) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := t.lookup(prefix)
	if n == nil {
		return nil
	}
	out := make([]string, 0, 8)
	collect(n, prefix, &out, limit)
	return out
}

// Autocomplete is like WordsWithPrefix but reports ErrPrefixNotPresent when no
// node exists for prefix (i.e. prefix itself is not present), as opposed to
// silently returning an empty list.
func (t *Trie) Autocomplete(prefix string, limit int) ([]string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.root == nil {
		return nil, ErrPrefixNotPresent
	}
	n := t.lookup(prefix)
	if n == nil {
		return nil, ErrPrefixNotPresent
	}
	out := make([]string, 0, 8)
	collect(n, prefix, &out, limit)
	return out, nil
}

// collect appends to out every stored word in n's subtree (including n itself)
// in lexicographic order, stopping once limit (if > 0) words have been found.
func collect(n *node, acc string, out *[]string, limit int) {
	if limit > 0 && len(*out) >= limit {
		return
	}
	if n.word {
		*out = append(*out, acc)
		if limit > 0 && len(*out) >= limit {
			return
		}
	}
	keys := make([]rune, 0, len(n.children))
	for r := range n.children {
		keys = append(keys, r)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, r := range keys {
		collect(n.children[r], acc+string(r), out, limit)
	}
}

// LongestPrefix returns the longest stored word that is a prefix of input,
// including input itself when stored. When the empty string is stored it is a
// valid (shortest) result. The boolean is false when no stored word is a
// prefix of input.
func (t *Trie) LongestPrefix(input string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.root == nil {
		return "", false
	}
	n := t.root
	var best string
	var ok bool
	if n.word {
		best, ok = "", true
	}
	rs := []rune(input)
	for i, r := range rs {
		c, found := n.children[r]
		if !found {
			break
		}
		n = c
		if n.word {
			best, ok = string(rs[:i+1]), true
		}
	}
	return best, ok
}

// Extremes returns the lexicographically smallest and largest stored words.
// The boolean reports whether the trie contained any word; otherwise min and
// max are empty strings. A lone stored empty string yields ("", "").
func (t *Trie) Extremes() (min, max string, ok bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.root == nil || t.count == 0 {
		return "", "", false
	}
	if min, ok := t.minWord(); ok {
		if mx, ok := t.maxWord(); ok {
			return min, mx, true
		}
	}
	return t.extremesScan()
}

// minWord walks the smallest-child chain until the first terminal node, which
// is the minimum stored word.
func (t *Trie) minWord() (string, bool) {
	n := t.root
	if n.word {
		return "", true
	}
	var acc string
	for len(n.children) > 0 {
		best := minChildRune(n.children)
		acc += string(best)
		n = n.children[best]
		if n.word {
			return acc, true
		}
	}
	return "", false
}

// maxWord walks the largest-child chain to the leaf, keeping the deepest
// terminal node, which is the maximum stored word.
func (t *Trie) maxWord() (string, bool) {
	n := t.root
	var acc string
	var best string
	var found bool
	if n.word {
		best, found = "", true
	}
	for len(n.children) > 0 {
		r := maxChildRune(n.children)
		acc += string(r)
		n = n.children[r]
		if n.word {
			best, found = acc, true
		}
	}
	return best, found
}

// extremesScan degrades gracefully: it enumerates every word tracking min/max,
// used only when the greedy walks hit a malformed (dead) branch.
func (t *Trie) extremesScan() (min, max string, ok bool) {
	var walk func(n *node, acc string)
	walk = func(n *node, acc string) {
		if n.word {
			if !ok {
				min, max, ok = acc, acc, true
			} else {
				if acc < min {
					min = acc
				}
				if acc > max {
					max = acc
				}
			}
		}
		for r, c := range n.children {
			walk(c, acc+string(r))
		}
	}
	walk(t.root, "")
	return min, max, ok
}

func minChildRune(m map[rune]*node) rune {
	var best rune
	first := true
	for r := range m {
		if first || r < best {
			best, first = r, false
		}
	}
	return best
}

func maxChildRune(m map[rune]*node) rune {
	var best rune
	first := true
	for r := range m {
		if first || r > best {
			best, first = r, false
		}
	}
	return best
}

// jsonNode mirrors a trie node as a plain nested JSON object:
// {"word": bool, "children": {char: node}}.
type jsonNode struct {
	Word     bool                 `json:"word"`
	Children map[string]*jsonNode `json:"children"`
}

// MarshalJSON serializes the trie as a compact nested JSON object. Map keys
// are sorted by encoding/json, so equal tries always marshal identically.
func (t *Trie) MarshalJSON() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := t.root
	if n == nil {
		n = newNode()
	}
	return json.Marshal(jsonFromNode(n))
}

// UnmarshalJSON replaces the contents of the trie with the JSON representation
// produced by MarshalJSON (or any equivalent hand-written structure).
func (t *Trie) UnmarshalJSON(data []byte) error {
	var jn jsonNode
	if err := json.Unmarshal(data, &jn); err != nil {
		return err
	}
	nt := trieFromJSON(&jn)
	t.mu.Lock()
	t.root = nt.root
	t.count = nt.count
	t.mu.Unlock()
	return nil
}

// Persist writes the trie to w as a single-line nested JSON document followed
// by a newline. The output round-trips exactly through Load.
func (t *Trie) Persist(w io.Writer) error {
	b, err := t.MarshalJSON()
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// Load reads a trie previously written by Persist (or any equivalent
// hand-written nested-JSON document) and returns a new Trie.
func Load(r io.Reader) (*Trie, error) {
	var jn jsonNode
	if err := json.NewDecoder(r).Decode(&jn); err != nil {
		return nil, err
	}
	return trieFromJSON(&jn), nil
}

func jsonFromNode(n *node) *jsonNode {
	jn := &jsonNode{Word: n.word, Children: make(map[string]*jsonNode)}
	for r, c := range n.children {
		jn.Children[string(r)] = jsonFromNode(c)
	}
	return jn
}

func trieFromJSON(jn *jsonNode) *Trie {
	t := &Trie{root: newNode()}
	t.root.word = jn.Word
	for k, c := range jn.Children {
		setPath(t.root, []rune(k), nodeFromJSON(c))
	}
	t.count = countWords(t.root)
	return t
}

func nodeFromJSON(jn *jsonNode) *node {
	n := newNode()
	n.word = jn.Word
	for k, c := range jn.Children {
		setPath(n, []rune(k), nodeFromJSON(c))
	}
	return n
}

// setPath hangs val beneath n along the runes rs, creating intermediate nodes.
// Multi-rune JSON keys (from hand-written files) are expanded into chains.
func setPath(n *node, rs []rune, val *node) {
	if len(rs) == 0 {
		return
	}
	if len(rs) == 1 {
		n.children[rs[0]] = val
		return
	}
	c, ok := n.children[rs[0]]
	if !ok {
		c = newNode()
		n.children[rs[0]] = c
	}
	setPath(c, rs[1:], val)
}

func countWords(n *node) int {
	if n == nil {
		return 0
	}
	c := 0
	if n.word {
		c++
	}
	for _, ch := range n.children {
		c += countWords(ch)
	}
	return c
}