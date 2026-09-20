# triex

A Unicode-aware prefix tree (trie) library and CLI for Go, written with only
the standard library. Pure, offline, and deterministic.

`triex` stores words as sequences of **runes**, so any Unicode input works —
ASCII, spaces, CJK, émoji, you name it. Words are always emitted in
lexicographic (code point) order.

## What is a trie good for?

- **Autocomplete / prefix enumeration** — every word that starts with `"app"` is
  found in `O(len(prefix))` plus the size of the answer, without scanning the
  whole dictionary. `WordsWithPrefix`, `Autocomplete`.
- **Prefix-sum aggregation** — "how many words start with `an`?" is one walk
  down the tree. `PrefixCount`.
- **Longest-prefix routing** — given an input string, find the longest stored
  word that is a prefix of it (think dictionary-based tokenization or route
  matching). `LongestPrefix`.
- **Ordered range queries** — the trie keeps words sorted, so min/max are cheap.
  `Extremes`.

A trie is a poor fit when you only ever do exact key lookups (a `map[string]`
is faster), or when your keys are huge but never share prefixes.

## Empty-string semantics

The empty string `""` is a **valid stored word**: the root may be terminal.
`Insert("")` returns `true` on first insert, `Contains("")` works, and
`WordsWithPrefix("")` returns every stored word. `LongestPrefix` may therefore
return `("", true)` when an empty string is stored and nothing longer matches.

## Library

```go
package main

import (
	"fmt"

	"github.com/Conedope/triex"
)

func main() {
	t := triex.New()
	t.Insert("apple")
	t.Insert("application")
	t.Insert("app")
	t.Insert("")

	fmt.Println(t.Count())                     // 4
	fmt.Println(t.PrefixCount("app"))          // 3
	fmt.Println(t.WordsWithPrefix("app", 2))   // [app apple]
	if ws, err := t.Autocomplete("app", 0); err == nil {
		fmt.Println(ws)                        // [app apple application]
	}
	if _, err := t.Autocomplete("zzz", 0); err != nil {
		fmt.Println(err)                        // triex: prefix not present
	}
	fmt.Println(t.LongestPrefix("appleseed"))  // apple true
	fmt.Println(t.Extremes())                  // "" application (min is the stored empty string)
}
```

### API

| Method | Description |
| --- | --- |
| `New() *Trie` | returns an empty trie |
| `Insert(word) bool` | stores `word`; `true` if newly added |
| `Delete(word) bool` | removes `word`; `true` if it was present |
| `Contains(word) bool` | reports whether `word` is stored |
| `Count() int` | number of stored words (`""` counts as one) |
| `IsEmpty() bool` | true when no words are stored |
| `PrefixCount(prefix) int` | number of stored words starting with `prefix` |
| `WordsWithPrefix(prefix, limit) []string` | up to `limit` words (lexicographic); `limit <= 0` means unlimited |
| `Autocomplete(prefix, limit) ([]string, error)` | like `WordsWithPrefix`, but errors `ErrPrefixNotPresent` when `prefix` is absent |
| `LongestPrefix(input) (string, bool)` | longest stored word that is a prefix of `input` (including `input` itself) |
| `Extremes() (min, max string, ok bool)` | lexicographic smallest and largest stored words |
| `Persist(w io.Writer) error` | writes a compact nested-JSON database |
| `Load(r io.Reader) (*Trie, error)` | reads a database back |
| `MarshalJSON` / `UnmarshalJSON` | `encoding/json` integration |

`Autocomplete` fails only when no node exists for the prefix (e.g. `"zzz"` over
`["apple"]`); a prefix whose node exists but that is not itself a stored word
(e.g. `"hell"` over `["hello"]`) succeeds and returns its descendants.

### Concurrency

`Trie` uses an internal `sync.RWMutex`: read methods (`Contains`, `Count`,
`WordsWithPrefix`, ...) run concurrently, while `Insert`/`Delete` take an
exclusive lock. Multiple concurrent writers are not allowed by design:
serialize mutations (or shard tries).

### Persistence format

A database is a single line of nested JSON. Node keys are single runes, `word`
marks a terminal node:

```json
{"word":true,"children":{"h":{"word":false,"children":{"i":{"word":true,"children":{}}}}}}
```

Empty trie: `{"word":false,"children":{}}`. Load accepts this format whether it
came from `Persist` or was written by hand, and even tolerates multi-rune keys
in hand-written files by expanding them into chains. Round-trips are exact.

## CLI

```
$ triex --help
```

All commands accept `--file DB` (alias `--db`) pointing at a JSON database. With
no `--file`, the database is ephemeral (in-memory only). Mutating commands
(`add`, `del`, `load`) rewrite the file afterwards.

| Command | Meaning |
| --- | --- |
| `add WORD...` | insert words (prints `added:` / `exists:`, then `total:`) |
| `del WORD...` | delete words (prints `deleted:` / `missing:`, then `total:`) |
| `contains WORD` | prints `true` (exit 0) or `false` (exit 1) |
| `count` | print the number of stored words |
| `prefix PREFIX [--limit N]` | enumerate words under `PREFIX` in order |
| `complete PREFIX [--limit N]` | autocomplete; exits 1 when `PREFIX` is absent |
| `longest INPUT` | print the longest stored word that is a prefix of `INPUT` (`none`, exit 1) |
| `extremes` | print the lexicographic `min:` and `max:` stored words |
| `load WORDFILE` | add every non-blank line of `WORDFILE` as a word |
| `save FILE` | persist the current database to `FILE` |
| `version` | print the version |
| `--help` | show usage |

### Real transcript

```
$ triex add cat cats catnip cattle dog dogma --file /tmp/triex-demo.json
added: cat
added: cats
added: catnip
added: cattle
added: dog
added: dogma
total: 6

$ triex count --file /tmp/triex-demo.json
6

$ triex prefix ca --file /tmp/triex-demo.json
cat
catnip
cats
cattle

$ triex complete cat --limit 3 --file /tmp/triex-demo.json
cat
catnip
cats

$ triex longest catnipz --file /tmp/triex-demo.json
catnip

$ triex load /tmp/triex-words.txt --file /tmp/triex-demo.json
loaded: 8

$ triex contains 世界 --file /tmp/triex-demo.json
true

$ triex extremes --file /tmp/triex-demo.json
min: cat
max: 世界语

$ cat /tmp/triex-demo.json
{"word":false,"children":{"c":{"word":false,"children":{"a":{"word":false,"children":{"t":{"word":true,"children":{...",...,"世":{"word":false,"children":{"界":{"word":true,"children":{"语":{"word":true,"children":{}}}}}}}}}}}
```

(Truncated for readability; the real file is one compact line.)

## Build and test

```
CGO_ENABLED=0 go build -o triex ./cmd/triex
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
```

Requires Go 1.22+. No dependencies outside the standard library.

## License

MIT — © 2026 Conedope. See [LICENSE](LICENSE).