package triex

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func mustContain(t *testing.T, tr *Trie, w string) {
	t.Helper()
	if !tr.Contains(w) {
		t.Errorf("Contains(%q) = false, want true", w)
	}
}

func mustNotContain(t *testing.T, tr *Trie, w string) {
	t.Helper()
	if tr.Contains(w) {
		t.Errorf("Contains(%q) = true, want false", w)
	}
}

func TestInsertAndContains(t *testing.T) {
	tr := New()
	if !tr.IsEmpty() {
		t.Fatal("new trie is not empty")
	}
	words := []string{"apple", "app", "banana", "", "世界", "🍎 pie", "hello world", "caf\u00e9"}
	for _, w := range words {
		if !tr.Insert(w) {
			t.Errorf("first Insert(%q) reported duplicate", w)
		}
	}
	if tr.Insert("apple") {
		t.Error("duplicate Insert(apple) returned true")
	}
	if tr.Insert("") {
		t.Error("duplicate Insert(empty) returned true")
	}
	for _, w := range words {
		mustContain(t, tr, w)
	}
	mustNotContain(t, tr, "appl")
	mustNotContain(t, tr, "🍎")
	mustNotContain(t, tr, "世界语")
	if tr.Count() != len(words) {
		t.Errorf("Count() = %d, want %d", tr.Count(), len(words))
	}
}

func TestDelete(t *testing.T) {
	tr := New()
	tr.Insert("a")
	tr.Insert("ab")
	tr.Insert("abc")
	tr.Insert("b")
	tr.Insert("")
	if !tr.Delete("abc") {
		t.Fatal("Delete(present) = false")
	}
	mustNotContain(t, tr, "abc")
	mustContain(t, tr, "ab")
	mustContain(t, tr, "a")
	if tr.Delete("abc") {
		t.Error("Delete(absent) = true")
	}
	if !tr.Delete("") {
		t.Error("Delete(empty) = false")
	}
	mustNotContain(t, tr, "")
	if tr.IsEmpty() {
		t.Fatal("trie should still hold a, ab, b")
	}
	if tr.Count() != 3 {
		t.Errorf("Count() = %d, want 3", tr.Count())
	}
	tr.Delete("a")
	tr.Delete("ab")
	tr.Delete("b")
	if !tr.IsEmpty() {
		t.Error("trie should be empty after deleting all")
	}
	if tr.Count() != 0 {
		t.Errorf("Count() = %d, want 0", tr.Count())
	}
	if tr.Contains("b") {
		t.Error("pruned node still reachable")
	}
}

func TestDeleteOnlyWord(t *testing.T) {
	tr := New()
	tr.Insert("solo")
	if !tr.Delete("solo") {
		t.Fatal("delete failed")
	}
	if !tr.IsEmpty() {
		t.Error("not empty after deleting sole word")
	}
	if _, _, ok := tr.Extremes(); ok {
		t.Error("extremes of empty trie reported ok")
	}
	if tr.PrefixCount("s") != 0 {
		t.Error("prefix count after full delete nonzero")
	}
}

func TestPrefixCount(t *testing.T) {
	tr := New()
	for _, w := range []string{"a", "an", "ant", "anteater", "be", "bet", "beta", "b"} {
		tr.Insert(w)
	}
	cases := []struct {
		prefix string
		want   int
	}{
		{"", 8},
		{"a", 4},
		{"an", 3},
		{"ant", 2},
		{"b", 4},
		{"be", 3},
		{"zzz", 0},
	}
	for _, c := range cases {
		if got := tr.PrefixCount(c.prefix); got != c.want {
			t.Errorf("PrefixCount(%q) = %d, want %d", c.prefix, got, c.want)
		}
	}
}

func TestWordsWithPrefix(t *testing.T) {
	tr := New()
	tr.Insert("app")
	tr.Insert("apple")
	tr.Insert("application")
	tr.Insert("banana")
	tr.Insert("band")
	tr.Insert("a")

	got := tr.WordsWithPrefix("app", 0)
	want := []string{"app", "apple", "application"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WordsWithPrefix(app) = %v, want %v", got, want)
	}

	restricted := tr.WordsWithPrefix("app", 2)
	if !reflect.DeepEqual(restricted, []string{"app", "apple"}) {
		t.Errorf("limit 2 = %v", restricted)
	}

	all := tr.WordsWithPrefix("app", 1)
	if !reflect.DeepEqual(all, []string{"app"}) {
		t.Errorf("limit 1 = %v", all)
	}

	if got := tr.WordsWithPrefix("xyz", 0); got != nil {
		t.Errorf("missing prefix returned %v", got)
	}

	full := tr.WordsWithPrefix("", 0)
	wantFull := []string{"a", "app", "apple", "application", "banana", "band"}
	if !reflect.DeepEqual(full, wantFull) {
		t.Errorf("all words = %v, want %v", full, wantFull)
	}
}

func TestWordsWithPrefixRuneOrder(t *testing.T) {
	tr := New()
	tr.Insert("e")
	tr.Insert("é")  // U+00E9, sorts after ASCII letters
	tr.Insert("世") // U+4E16
	tr.Insert("🍎") // U+1F34E
	tr.Insert("界") // U+754C
	tr.Insert("É")  // U+00C9

	got := tr.WordsWithPrefix("", 0)
	want := []string{"e", "É", "é", "世", "界", "🍎"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rune order = %v, want %v", got, want)
	}
}

func TestAutocomplete(t *testing.T) {
	tr := New()
	tr.Insert("a")
	tr.Insert("ab")
	tr.Insert("abc")
	tr.Insert("abcd")

	got, err := tr.Autocomplete("ab", 0)
	if err != nil {
		t.Fatalf("Autocomplete(ab): %v", err)
	}
	if !reflect.DeepEqual(got, []string{"ab", "abc", "abcd"}) {
		t.Errorf("Autocomplete(ab) = %v", got)
	}

	// prefix node exists but is not itself a stored word
	tr2 := New()
	tr2.Insert("hello")
	got, err = tr2.Autocomplete("hell", 3)
	if err != nil {
		t.Fatalf("Autocomplete(hell): %v", err)
	}
	if !reflect.DeepEqual(got, []string{"hello"}) {
		t.Errorf("Autocomplete(hell) = %v", got)
	}

	// missing prefix errors
	if _, err := tr.Autocomplete("qqq", 0); err == nil {
		t.Error("Autocomplete(missing) = nil error")
	} else if !errors.Is(err, ErrPrefixNotPresent) {
		t.Errorf("Autocomplete(missing) = %v, want ErrPrefixNotPresent", err)
	}

	if _, err := New().Autocomplete("x", 0); !errors.Is(err, ErrPrefixNotPresent) {
		t.Errorf("empty trie Autocomplete = %v, want ErrPrefixNotPresent", err)
	}

	// empty prefix on empty trie is "present" yet yields nothing
	got, err = New().Autocomplete("", 0)
	if err != nil || len(got) != 0 {
		t.Errorf("Autocomplete(empty on empty trie) = %v, %v", got, err)
	}
}

func TestLongestPrefix(t *testing.T) {
	t.Run("empty string stored", func(t *testing.T) {
		tr := New()
		tr.Insert("")
		got, ok := tr.LongestPrefix("")
		if !ok || got != "" {
			t.Errorf("LongestPrefix(\"\") = (%q, %v), want (\"\", true)", got, ok)
		}
		got, ok = tr.LongestPrefix("anything")
		if !ok || got != "" {
			t.Errorf("longest with only empty stored = (%q, %v), want (\"\", true)", got, ok)
		}
	})

	tr := New()
	for _, w := range []string{"chat", "cha", "chats"} {
		tr.Insert(w)
	}
	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{"chats", "chats", true},
		{"chatter", "chat", true}, // partial input: longest stored prefix so far
		{"chat", "chat", true},    // exact input
		{"c", "", false},          // no stored word is a prefix of "c"
		{"zzz", "", false},
	}
	for _, c := range cases {
		got, ok := tr.LongestPrefix(c.input)
		if got != c.want || ok != c.ok {
			t.Errorf("LongestPrefix(%q) = (%q, %v), want (%q, %v)", c.input, got, ok, c.want, c.ok)
		}
	}

	// a shorter stored word is still the longest prefix of a partial input
	tr2 := New()
	tr2.Insert("c")
	if got, ok := tr2.LongestPrefix("ch"); !ok || got != "c" {
		t.Errorf("LongestPrefix(ch) with only \"c\" = (%q, %v), want (\"c\", true)", got, ok)
	}
}

func TestExtremes(t *testing.T) {
	tr := New()
	if _, _, ok := tr.Extremes(); ok {
		t.Error("empty trie extremes ok = true")
	}
	words := []string{"世界", "Zebra", "apple", "zebra", "🍎", "banana", "éclair"}
	for _, w := range words {
		tr.Insert(w)
	}
	min, max, ok := tr.Extremes()
	if !ok {
		t.Fatal("extremes ok = false")
	}
	// rune order: 'Z'(0x5A) < 'a'(0x61) < 'b'(0x62) < 'é'(0xE9) <
	// 'z'(0x7A) < '世'(0x4E16) < '🍎'(0x1F34E)
	if min != "Zebra" {
		t.Errorf("min = %q, want Zebra", min)
	}
	if max != "🍎" {
		t.Errorf("max = %q, want 🍎", max)
	}

	tr2 := New()
	tr2.Insert("za")
	tr2.Insert("zb")
	mn, mx, _ := tr2.Extremes()
	if mn != "za" || mx != "zb" {
		t.Errorf("tr2 extremes = (%q, %q), want (za, zb)", mn, mx)
	}

	tr3 := New()
	tr3.Insert("")
	if mn, mx, ok := tr3.Extremes(); !ok || mn != "" || mx != "" {
		t.Errorf("lone empty extremes = (%q, %q, %v)", mn, mx, ok)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	words := []string{"hello", "世界", "🍎", "with space", "caf\u00e9", ""}
	tr := New()
	for _, w := range words {
		tr.Insert(w)
	}
	var buf bytes.Buffer
	if err := tr.Persist(&buf); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	tr2, err := Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tr2.Count() != len(words) {
		t.Errorf("loaded Count() = %d, want %d", tr2.Count(), len(words))
	}
	for _, w := range words {
		mustContain(t, tr2, w)
	}
	b1, _ := tr.MarshalJSON()
	b2, _ := tr2.MarshalJSON()
	if !bytes.Equal(b1, b2) {
		t.Errorf("round trip mismatch:\n got  %s\n want %s", b2, b1)
	}
}

func TestPersistenceEmptyTrie(t *testing.T) {
	tr := New()
	var buf bytes.Buffer
	if err := tr.Persist(&buf); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != `{"word":false,"children":{}}` {
		t.Errorf("empty trie JSON = %s", got)
	}
	tr2, err := Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !tr2.IsEmpty() {
		t.Error("loaded empty trie is not empty")
	}
	if tr2.Count() != 0 {
		t.Error("loaded empty trie has count != 0")
	}
}

func TestPersistencePrefixOnlyNodes(t *testing.T) {
	tr := New()
	tr.Insert("abcd")
	tr.Insert("ab")
	if !tr.Delete("ab") {
		t.Fatal("delete ab failed")
	}
	// node "ab" is now a non-word prefix-only node.
	var buf bytes.Buffer
	if err := tr.Persist(&buf); err != nil {
		t.Fatal(err)
	}
	tr2, err := Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, tr2, "abcd")
	mustNotContain(t, tr2, "ab")
	if tr2.PrefixCount("ab") != 1 {
		t.Error("prefix count over prefix-only nodes wrong after load")
	}
}

func TestLoadGoldenJSON(t *testing.T) {
	const golden = `{
	  "word": true,
	  "children": {
	    "h": {
	      "word": false,
	      "children": {
	        "i": {
	          "word": true,
	          "children": {
	            "世界": {"word": true, "children": {}},
	            "x": {"word": false, "children": {}}
	          }
	        }
	      }
	    }
	  }
	}`
	tr, err := Load(strings.NewReader(golden))
	if err != nil {
		t.Fatalf("Load(golden): %v", err)
	}
	if tr.Count() != 3 {
		t.Errorf("golden Count() = %d, want 3", tr.Count())
	}
	mustContain(t, tr, "")
	mustContain(t, tr, "hi")
	mustContain(t, tr, "hi世界")
	mustNotContain(t, tr, "h")
	mustNotContain(t, tr, "hix")
	if tr.PrefixCount("hi") != 2 {
		t.Errorf("golden PrefixCount(hi) = %d, want 2", tr.PrefixCount("hi"))
	}
}

func TestUnmarshalJSONMethod(t *testing.T) {
	tr := New()
	tr.Insert("hello")
	tr.Insert("世界")
	raw, err := tr.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var tr2 Trie
	if err := tr2.UnmarshalJSON(raw); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if tr2.Count() != 2 {
		t.Errorf("UnmarshalJSON Count() = %d, want 2", tr2.Count())
	}
	mustContain(t, &tr2, "hello")
	mustContain(t, &tr2, "世界")
}

func TestConcurrentInsertContains(t *testing.T) {
	tr := New()
	const n = 300
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := fmt.Sprintf("word-%d", i)
			tr.Insert(w)
			if !tr.Contains(w) {
				t.Errorf("Contains(%s) = false after Insert", w)
			}
		}(i)
	}
	wg.Wait()
	if tr.Count() != n {
		t.Errorf("Count() = %d, want %d", tr.Count(), n)
	}
	for i := 0; i < n; i++ {
		mustContain(t, tr, fmt.Sprintf("word-%d", i))
	}
}