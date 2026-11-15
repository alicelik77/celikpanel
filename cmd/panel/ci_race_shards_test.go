package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The panel package's race tests run in the "Panel race" matrix of
// .github/workflows/ci.yml, one shard per -run/-skip pair. This test holds the
// matrix to its promise: every Test function of the package (every file,
// whatever its build tags) is run by exactly one shard, so a new test can
// never fall outside every shard, and none runs twice. It reads the workflow
// as text and matches names with Go's own regexp package, the one go test
// uses for -run and -skip.
//
// Panel yarış matrisi: paketin her Test işlevi tam bir dilimde koşar; yeni
// bir test hiçbir dilimin dışında kalamaz, hiçbiri iki kez koşmaz.

type ciRaceShard struct {
	name, pattern, skip string
	run, skipRE         *regexp.Regexp
}

func ciRaceShards(t *testing.T) []ciRaceShard {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "\n  panel-race:\n")
	if start < 0 {
		t.Fatal("the panel-race job is not in ci.yml")
	}
	block := text[start+1:]
	// The job ends at the next job (two-space indented key).
	if end := regexp.MustCompile(`\n  [a-z0-9-]+:\n`).FindStringIndex(block[1:]); end != nil {
		block = block[:end[0]+1]
	}
	field := regexp.MustCompile(`(?m)^\s+-?\s*(shard|pattern|skip):\s*(.*)$`)
	var shards []ciRaceShard
	for _, m := range field.FindAllStringSubmatch(block, -1) {
		value := strings.TrimSpace(m[2])
		if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2 {
			value = strings.ReplaceAll(value[1:len(value)-1], "''", "'")
		}
		switch m[1] {
		case "shard":
			shards = append(shards, ciRaceShard{name: value})
		case "pattern":
			shards[len(shards)-1].pattern = value
		case "skip":
			shards[len(shards)-1].skip = value
		}
	}
	if len(shards) < 10 {
		t.Fatalf("only %d race shards read from ci.yml", len(shards))
	}
	for i := range shards {
		if shards[i].pattern == "" {
			t.Fatalf("shard %q has no pattern", shards[i].name)
		}
		shards[i].run = regexp.MustCompile(shards[i].pattern)
		if shards[i].skip != "" {
			shards[i].skipRE = regexp.MustCompile(shards[i].skip)
		}
	}
	return shards
}

func ciPanelTestNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	fset := token.NewFileSet()
	for _, name := range files {
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "TestMain" {
				continue
			}
			// go test runs Test, or Test followed by a non-lowercase rune.
			if rest := strings.TrimPrefix(fn.Name.Name, "Test"); rest != "" && rest[0] >= 'a' && rest[0] <= 'z' {
				continue
			}
			seen[fn.Name.Name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) < 1000 {
		t.Fatalf("only %d tests found", len(names))
	}
	return names
}

func TestCIRaceShardsRunEveryPanelTestExactlyOnce(t *testing.T) {
	shards := ciRaceShards(t)
	// Every test the package has today, and the names a future test could
	// take that no current test has (an exact "Test", a digit, an underscore,
	// every capital letter).
	names := ciPanelTestNames(t)
	names = append(names, "Test", "Test0", "Test_x")
	for c := 'A'; c <= 'Z'; c++ {
		names = append(names, "Test"+string(c)+"zzFuture")
	}
	for _, name := range names {
		var in []string
		for _, s := range shards {
			if s.run.MatchString(name) && (s.skipRE == nil || !s.skipRE.MatchString(name)) {
				in = append(in, s.name)
			}
		}
		if len(in) != 1 {
			t.Errorf("%s is run by %d race shards %q; it must be run by exactly one", name, len(in), in)
		}
	}
}
