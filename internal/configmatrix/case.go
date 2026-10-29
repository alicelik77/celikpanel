// Package configmatrix writes rendered configuration "cases" for the distro
// config matrix (deploy/test-distro-config-matrix.sh). It is used only by the
// render tests (TestRenderDistroConfigMatrix in cmd/agent, internal/services
// and internal/dnspeerenrollowner); no production binary imports it.
//
// A case is one program × variant. It is written as a directory:
//
//	<out>/<id>/case.env    shell-sourceable facts: PROGRAM VARIANT VALIDATOR SOURCE TARGET ...
//	<out>/<id>/root/...    the files the panel writes, at their absolute host paths
//	<out>/<id>/files.list  those absolute paths (the harness removes them afterwards)
//	<out>/<id>/setup.sh    commands the panel runs (postconf -e, postmap ...), replayed before validation
//
// The renders always come from the product's own generators; this package
// only stores what they return.
package configmatrix

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Case is one rendered configuration variant.
type Case struct {
	ID        string // unique, [a-z0-9.-]
	Program   string // nginx, php-fpm, postfix, dovecot, bind, powerdns, nftables, systemd, ...
	Variant   string // human-readable variant name
	Validator string // validator kind the harness knows (see deploy/test-distro-config-matrix/in-container/validate.sh)
	Source    string // generator file:line responsible for the bytes
	Target    string // primary file the validator looks at (absolute host path)
	Extra     map[string]string
	files     map[string]file
	setup     [][]string
}

type file struct {
	data    []byte
	mode    os.FileMode
	symlink string
}

// File adds a regular file at an absolute host path.
func (c *Case) File(path string, data []byte, mode os.FileMode) {
	if c.files == nil {
		c.files = map[string]file{}
	}
	c.files[filepath.Clean(path)] = file{data: append([]byte(nil), data...), mode: mode}
}

// Symlink adds a symbolic link at an absolute host path.
func (c *Case) Symlink(path, target string) {
	if c.files == nil {
		c.files = map[string]file{}
	}
	c.files[filepath.Clean(path)] = file{symlink: target}
}

// Command records a command the product runs to apply the configuration.
func (c *Case) Command(argv ...string) {
	c.setup = append(c.setup, append([]string(nil), argv...))
}

var caseID = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// Write stores the case under out/<ID>.
func (c *Case) Write(out string) error {
	if !caseID.MatchString(c.ID) {
		return fmt.Errorf("invalid case id %q", c.ID)
	}
	dir := filepath.Join(out, c.ID)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	root := filepath.Join(dir, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	paths := make([]string, 0, len(c.files))
	for path := range c.files {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("case %s: path %q is not absolute", c.ID, path)
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		f := c.files[path]
		dest := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if f.symlink != "" {
			if err := os.Symlink(f.symlink, dest); err != nil {
				return err
			}
			continue
		}
		mode := f.mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(dest, f.data, mode); err != nil {
			return err
		}
		if err := os.Chmod(dest, mode); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "files.list"), []byte(strings.Join(paths, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	var env strings.Builder
	for _, kv := range [][2]string{
		{"PROGRAM", c.Program}, {"VARIANT", c.Variant}, {"VALIDATOR", c.Validator},
		{"SOURCE", c.Source}, {"TARGET", c.Target},
	} {
		fmt.Fprintf(&env, "%s=%s\n", kv[0], ShellQuote(kv[1]))
	}
	keys := make([]string, 0, len(c.Extra))
	for k := range c.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`).MatchString(k) {
			return fmt.Errorf("case %s: invalid extra key %q", c.ID, k)
		}
		fmt.Fprintf(&env, "%s=%s\n", k, ShellQuote(c.Extra[k]))
	}
	if err := os.WriteFile(filepath.Join(dir, "case.env"), []byte(env.String()), 0o644); err != nil {
		return err
	}
	var setup strings.Builder
	setup.WriteString("set -e\n")
	for _, argv := range c.setup {
		quoted := make([]string, len(argv))
		for i, a := range argv {
			quoted[i] = ShellQuote(a)
		}
		setup.WriteString(strings.Join(quoted, " ") + "\n")
	}
	return os.WriteFile(filepath.Join(dir, "setup.sh"), []byte(setup.String()), 0o644)
}

// ShellQuote quotes s for POSIX sh.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// OutDir is the output directory requested by the harness, or "" when the
// render test should skip (ordinary go test runs).
func OutDir() string { return os.Getenv("CELIKPANEL_CONFIG_MATRIX_OUT") }
