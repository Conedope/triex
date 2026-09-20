// Command triex is a Unicode-aware prefix tree (trie) CLI built on the triex
// library. It offers insert/delete/contains, prefix enumeration and
// autocomplete, longest-prefix lookup, prefix-count aggregation, extremes, and
// JSON persistence. Without --file the database is ephemeral (in-memory) and
// disappears when the process exits.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Conedope/triex"
)

const version = "0.1.0"

const usage = `triex - Unicode-aware prefix tree CLI (` + version + `)

Usage:
  triex add WORD... [--file DB]      insert words (mutates DB)
  triex del WORD... [--file DB]      delete words (mutates DB)
  triex contains WORD [--file DB]    prints true (exit 0) or false (exit 1)
  triex count [--file DB]            number of stored words
  triex prefix PREFIX [--limit N] [--file DB]   enumerate words under PREFIX
  triex complete PREFIX [--limit N] [--file DB] autocomplete; errors when the prefix is absent
  triex longest INPUT [--file DB]    longest stored word that is a prefix of INPUT
  triex extremes [--file DB]         lexicographic min and max stored words
  triex load WORDFILE [--file DB]    add words from file (one per line)
  triex save FILE [--file DB]        persist the current database to FILE
  triex version                      print the version and exit
  triex --help                       show this help and exit

Global flags:
  --file PATH / --db PATH  JSON database file; default is in-memory only
  --limit N                cap results (0 or absent = unlimited)
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type options struct {
	db    string
	limit int
}

// parseFlags splits flags out of args wherever they appear. A help request
// returns showHelp=true.
func parseFlags(args []string) (o options, rest []string, showHelp bool, err error) {
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--help" || a == "-h":
			return o, nil, true, nil
		case a == "--file" || a == "--db":
			if i+1 >= len(args) {
				return o, nil, false, fmt.Errorf("flag %s requires a value", a)
			}
			o.db = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--file=") || strings.HasPrefix(a, "--db="):
			o.db = a[strings.IndexByte(a, '=')+1:]
			i++
		case a == "--limit":
			if i+1 >= len(args) {
				return o, nil, false, fmt.Errorf("flag --limit requires a value")
			}
			if o.limit, err = strconv.Atoi(args[i+1]); err != nil {
				return o, nil, false, fmt.Errorf("invalid --limit value %q", args[i+1])
			}
			i += 2
		case strings.HasPrefix(a, "--limit="):
			if o.limit, err = strconv.Atoi(a[len("--limit="):]); err != nil {
				return o, nil, false, fmt.Errorf("invalid --limit value %q", a[len("--limit="):])
			}
			i++
		default:
			rest = append(rest, a)
			i++
		}
	}
	return o, rest, false, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	o, rest, showHelp, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	if showHelp {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if len(rest) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	cmd := rest[0]
	words := rest[1:]

	tr, err := loadDB(o.db)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}

	switch cmd {
	case "add":
		if len(words) == 0 {
			return arityError(stderr)
		}
		for _, w := range words {
			if tr.Insert(w) {
				fmt.Fprintf(stdout, "added: %s\n", w)
			} else {
				fmt.Fprintf(stdout, "exists: %s\n", w)
			}
		}
		fmt.Fprintf(stdout, "total: %d\n", tr.Count())
		return finishMutation(stdout, stderr, o.db, tr)

	case "del":
		if len(words) == 0 {
			return arityError(stderr)
		}
		for _, w := range words {
			if tr.Delete(w) {
				fmt.Fprintf(stdout, "deleted: %s\n", w)
			} else {
				fmt.Fprintf(stdout, "missing: %s\n", w)
			}
		}
		fmt.Fprintf(stdout, "total: %d\n", tr.Count())
		return finishMutation(stdout, stderr, o.db, tr)

	case "contains":
		if len(words) != 1 {
			return arityError(stderr)
		}
		if tr.Contains(words[0]) {
			fmt.Fprintln(stdout, "true")
			return 0
		}
		fmt.Fprintln(stdout, "false")
		return 1

	case "count":
		if len(words) != 0 {
			return arityError(stderr)
		}
		fmt.Fprintln(stdout, tr.Count())
		return 0

	case "prefix":
		if len(words) != 1 {
			return arityError(stderr)
		}
		for _, w := range tr.WordsWithPrefix(words[0], o.limit) {
			fmt.Fprintln(stdout, w)
		}
		return 0

	case "complete":
		if len(words) != 1 {
			return arityError(stderr)
		}
		matches, err := tr.Autocomplete(words[0], o.limit)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		for _, w := range matches {
			fmt.Fprintln(stdout, w)
		}
		return 0

	case "longest":
		if len(words) != 1 {
			return arityError(stderr)
		}
		w, ok := tr.LongestPrefix(words[0])
		if !ok {
			fmt.Fprintln(stdout, "none")
			return 1
		}
		fmt.Fprintln(stdout, w)
		return 0

	case "extremes":
		if len(words) != 0 {
			return arityError(stderr)
		}
		min, max, ok := tr.Extremes()
		if !ok {
			fmt.Fprintln(stdout, "min: none")
			fmt.Fprintln(stdout, "max: none")
			return 0
		}
		fmt.Fprintf(stdout, "min: %s\n", min)
		fmt.Fprintf(stdout, "max: %s\n", max)
		return 0

	case "load":
		if len(words) != 1 {
			return arityError(stderr)
		}
		n, err := loadWordFile(tr, words[0])
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
		fmt.Fprintf(stdout, "loaded: %d\n", n)
		return finishMutation(stdout, stderr, o.db, tr)

	case "save":
		if len(words) != 1 {
			return arityError(stderr)
		}
		if err := writeDB(words[0], tr); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
		fmt.Fprintf(stdout, "saved: %s\n", words[0])
		return 0

	case "version":
		if len(words) != 0 {
			return arityError(stderr)
		}
		fmt.Fprintf(stdout, "triex %s\n", version)
		return 0

	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}

func arityError(stderr io.Writer) int {
	fmt.Fprintln(stderr, "error: wrong number of arguments")
	return 2
}

// finishMutation persists the database after a mutating command and reports
// whether that persistence succeeded.
func finishMutation(stdout, stderr io.Writer, db string, tr *triex.Trie) int {
	if db == "" {
		return 0
	}
	if err := writeDB(db, tr); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	return 0
}

// loadDB opens db (creating an empty in-memory trie when db is empty or the
// file does not yet exist).
func loadDB(db string) (*triex.Trie, error) {
	if db == "" {
		return triex.New(), nil
	}
	f, err := os.Open(db)
	if err != nil {
		if os.IsNotExist(err) {
			return triex.New(), nil
		}
		return nil, err
	}
	defer f.Close()
	return triex.Load(f)
}

// writeDB persists tr to db, creating the file as needed.
func writeDB(db string, tr *triex.Trie) error {
	f, err := os.Create(db)
	if err != nil {
		return err
	}
	defer f.Close()
	return tr.Persist(f)
}

// loadWordFile inserts every non-blank line of path into tr and reports how
// many words were added.
func loadWordFile(tr *triex.Trie, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	n := 0
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimSuffix(sc.Text(), "\r"))
		if line == "" {
			continue
		}
		tr.Insert(line)
		n++
	}
	return n, sc.Err()
}