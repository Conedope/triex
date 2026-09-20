package triex_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "triex-cli")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binPath = filepath.Join(dir, "triex")
	out, err := exec.Command("go", "build", "-o", binPath, "./cmd/triex").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "building triex binary: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("running triex %v: %v", args, err)
		}
	}
	return buf.String(), code
}

func TestCLIEndToEnd(t *testing.T) {
	db := filepath.Join(t.TempDir(), "db.json")

	out, code := runCLI(t, "add", "apple", "app", "application", "banana", "band", "世界", "世界语", "--file", db)
	want := "added: apple\nadded: app\nadded: application\nadded: banana\nadded: band\nadded: 世界\nadded: 世界语\ntotal: 7\n"
	if out != want {
		t.Errorf("add output = %q, want %q", out, want)
	}
	if code != 0 {
		t.Errorf("add exit = %d, want 0", code)
	}

	if out, code = runCLI(t, "contains", "apple", "--file", db); out != "true\n" || code != 0 {
		t.Errorf("contains present = (%q, %d)", out, code)
	}
	if out, code = runCLI(t, "contains", "xyz", "--file", db); out != "false\n" || code != 1 {
		t.Errorf("contains absent = (%q, %d), want (\"false\\n\", 1)", out, code)
	}

	if out, code = runCLI(t, "count", "--file", db); out != "7\n" || code != 0 {
		t.Errorf("count = (%q, %d)", out, code)
	}

	if out, code = runCLI(t, "del", "apple", "--file", db); out != "deleted: apple\ntotal: 6\n" || code != 0 {
		t.Errorf("del = (%q, %d)", out, code)
	}
	if out, code = runCLI(t, "del", "apple", "--file", db); out != "missing: apple\ntotal: 6\n" || code != 0 {
		t.Errorf("del absent = (%q, %d)", out, code)
	}

	if out, code = runCLI(t, "prefix", "app", "--file", db); out != "app\napplication\n" || code != 0 {
		t.Errorf("prefix = (%q, %d)", out, code)
	}
	if out, code = runCLI(t, "complete", "app", "--limit", "1", "--file", db); out != "app\n" || code != 0 {
		t.Errorf("complete limit 1 = (%q, %d)", out, code)
	}
	if out, code = runCLI(t, "complete", "nope", "--file", db); code != 1 {
		t.Errorf("complete missing prefix = (%q, %d), want exit 1", out, code)
	} else if !bytes.Contains([]byte(out), []byte("prefix not present")) {
		t.Errorf("complete missing prefix output = %q", out)
	}

	if out, code = runCLI(t, "longest", "applicationzz", "--file", db); out != "application\n" || code != 0 {
		t.Errorf("longest = (%q, %d)", out, code)
	}
	if out, code = runCLI(t, "longest", "zzz", "--file", db); out != "none\n" || code != 1 {
		t.Errorf("longest none = (%q, %d)", out, code)
	}

	if out, code = runCLI(t, "extremes", "--file", db); out != "min: app\nmax: 世界语\n" || code != 0 {
		t.Errorf("extremes = (%q, %d)", out, code)
	}

	db2 := filepath.Join(t.TempDir(), "db2.json")
	if out, code = runCLI(t, "save", db2, "--file", db); out != "saved: "+db2+"\n" || code != 0 {
		t.Errorf("save = (%q, %d)", out, code)
	}

	// fresh subprocess re-loads the saved db
	if out, code = runCLI(t, "count", "--file", db2); out != "6\n" || code != 0 {
		t.Errorf("reload count = (%q, %d)", out, code)
	}
	if out, code = runCLI(t, "prefix", "世界", "--file", db2); out != "世界\n世界语\n" || code != 0 {
		t.Errorf("reload prefix = (%q, %d)", out, code)
	}
}

func TestCLIVersionHelpAndErrors(t *testing.T) {
	if out, code := runCLI(t, "version"); out != "triex 0.1.0\n" || code != 0 {
		t.Errorf("version = (%q, %d)", out, code)
	}
	if out, code := runCLI(t, "--help"); code != 0 || !bytes.Contains([]byte(out), []byte("Usage:")) {
		t.Errorf("--help = (%q, %d)", out, code)
	}
	if _, code := runCLI(t); code != 2 {
		t.Errorf("no args exit = %d, want 2", code)
	}
	if _, code := runCLI(t, "bogus"); code != 2 {
		t.Errorf("unknown command exit = %d, want 2", code)
	}
	if _, code := runCLI(t, "--limit", "x", "count"); code != 2 {
		t.Errorf("bad --limit exit = %d, want 2", code)
	}
}

func TestCLIWordloadFile(t *testing.T) {
	words := filepath.Join(t.TempDir(), "words.txt")
	if err := os.WriteFile(words, []byte("one\ntwo\nthree\n世界\n"), 0644); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "wl.json")
	out, code := runCLI(t, "load", words, "--file", db)
	if out != "loaded: 4\n" || code != 0 {
		t.Errorf("load = (%q, %d)", out, code)
	}
	if out, code := runCLI(t, "count", "--file", db); out != "4\n" || code != 0 {
		t.Errorf("count after load = (%q, %d)", out, code)
	}
	if out, code := runCLI(t, "contains", "世界", "--file", db); out != "true\n" || code != 0 {
		t.Errorf("contains loaded = (%q, %d)", out, code)
	}
}

func TestCLIEphemeralAndEmptyWord(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ephem.json")
	out, code := runCLI(t, "add", "", "hello", "--file", db)
	want := "added: \nadded: hello\ntotal: 2\n"
	if out != want || code != 0 {
		t.Errorf("add empty word = (%q, %d)", out, code)
	}
	if out, code := runCLI(t, "contains", "", "--file", db); out != "true\n" || code != 0 {
		t.Errorf("contains empty = (%q, %d)", out, code)
	}
	if out, code := runCLI(t, "count"); out != "0\n" || code != 0 {
		t.Errorf("ephemeral count = (%q, %d)", out, code)
	}
}