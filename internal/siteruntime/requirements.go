package siteruntime

import (
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

// requirements.txt. CelikPanel installs packages only from PyPI
// (https://pypi.org/simple), only as wheels unless the administrator allowed
// source builds for the offering, and with hash checking whenever the file
// carries hashes. Before a build the Agent reads the file (and the files it
// includes, all inside app/src, without following any link) and refuses a
// line that would install from somewhere else or change those rules; the
// refusal names the file, the line and why. What pip then reads is the
// checked text, flattened into one root-owned file, never the customer's
// file itself.
//
// The rule is the supply-chain policy of the offering, not the security
// boundary: the customer can upload any code into app/src anyway. What runs
// is held by the sandbox.
//
// requirements.txt: paketler yalnız PyPI'dan, kaynaktan derleme açılmadıkça
// yalnız hazır paket (wheel) olarak ve dosyada özet varsa özet denetimiyle
// kurulur. Başka yerden kuracak ya da bu kuralları değiştirecek satır, dosya ve
// satır numarasıyla reddedilir.

// Refusal codes of a requirements line (runtime.req.<code> on screen).
const (
	ReqEditable      = "editable"           // -e / --editable
	ReqVCS           = "vcs"                // git+https://... and the other VCS forms
	ReqURL           = "url"                // https://..., name @ https://...
	ReqPath          = "path"               // ./pkg, /tmp/x.whl, ../lib
	ReqIndexURL      = "index_url"          // -i / --index-url / --no-index
	ReqExtraIndex    = "extra_index_url"    // --extra-index-url
	ReqTrustedHost   = "trusted_host"       // --trusted-host
	ReqFindLinks     = "find_links"         // -f / --find-links
	ReqConstraintOut = "constraint_outside" // -c naming a file outside app/src
	ReqNestedOutside = "nested_outside"     // -r naming a file outside app/src
	ReqNestedMissing = "nested_missing"     // -r / -c naming a file that is not there
	ReqOption        = "option"             // any other pip option
	ReqSyntax        = "syntax"             // not a requirement CelikPanel can read
	ReqTooLarge      = "too_large"          // too many files, lines or bytes
	ReqNestedTooDeep = "nested_too_deep"    // -r more than four levels deep
)

// ReqRefusalCodes lists every code (the web test reads it).
var ReqRefusalCodes = []string{ReqEditable, ReqVCS, ReqURL, ReqPath, ReqIndexURL, ReqExtraIndex, ReqTrustedHost, ReqFindLinks,
	ReqConstraintOut, ReqNestedOutside, ReqNestedMissing, ReqOption, ReqSyntax, ReqTooLarge, ReqNestedTooDeep}

// RequirementsFile is the file a build looks for in app/src.
const RequirementsFile = "requirements.txt"

// Bounds of what is read.
const (
	maxRequirementsBytes = 256 << 10
	maxRequirementsFiles = 16
	maxRequirementsLines = 4000
	maxRequirementsDepth = 4
)

// ReqRefusal names the refused line. Text is the line with any credential
// hidden.
type ReqRefusal struct {
	File string
	Line int
	Code string
	Text string
}

func (r *ReqRefusal) Error() string {
	return fmt.Sprintf("%s:%d %s: %s", r.File, r.Line, r.Code, r.Text)
}

// Message is the operation message the screen reads back: "<file>:<line>
// <code>: <text>".
func (r *ReqRefusal) Message() string { return r.Error() }

// Requirements is the checked, flattened result.
type Requirements struct {
	// Text is every requirement line (nested files included), one per line,
	// with the global options that are allowed.
	Text string
	// Constraints is every constraint line of the -c files.
	Constraints string
	// Hashed: the files carry hashes (--hash or --require-hashes), so pip
	// runs with --require-hashes.
	Hashed bool
	// Packages counts the requirement lines.
	Packages int
}

// ReadSourceFile reads a file of app/src by its path relative to app/src.
// It returns an error wrapping os.ErrNotExist when the file is not there.
type ReadSourceFile func(rel string) ([]byte, error)

var (
	reqNamePattern    = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*[A-Za-z0-9]|[A-Za-z0-9])(\[[A-Za-z0-9._,\s-]*\])?`)
	reqSpecPattern    = regexp.MustCompile(`^[<>=!~0-9A-Za-z.*+,\s-]*$`)
	reqMarkerPattern  = regexp.MustCompile(`^[A-Za-z0-9_.<>=!~'"\s(),-]*$`)
	reqHashPattern    = regexp.MustCompile(`^(sha256:[0-9a-f]{64}|sha384:[0-9a-f]{96}|sha512:[0-9a-f]{128})$`)
	reqFilePattern    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,63}(/[A-Za-z0-9_][A-Za-z0-9._-]{0,63}){0,3}$`)
	reqCommentPattern = regexp.MustCompile(`(^|\s)#.*$`)
	vcsPrefixes       = []string{"git+", "hg+", "svn+", "bzr+"}
	archiveSuffixes   = []string{".whl", ".tar.gz", ".tgz", ".zip", ".tar.bz2", ".tar.xz", ".egg"}
	credentialPattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/\s@]+@`)
)

// hideCredentials removes "user:password@" from any URL in a line.
func hideCredentials(line string) string {
	return credentialPattern.ReplaceAllString(line, "${1}***@")
}

type reqParser struct {
	read   ReadSourceFile
	files  int
	lines  int
	out    Requirements
	reqs   []string
	cons   []string
	opened map[string]bool
}

// CheckRequirements reads app/src/requirements.txt and every file it
// includes. present is false when there is no requirements.txt (the app
// needs only the app server). A refused line is a *ReqRefusal.
func CheckRequirements(read ReadSourceFile) (req Requirements, present bool, err error) {
	p := &reqParser{read: read, opened: map[string]bool{}}
	data, err := read(RequirementsFile)
	if errors.Is(err, os.ErrNotExist) {
		return Requirements{}, false, nil
	}
	if err != nil {
		return Requirements{}, true, err
	}
	if err := p.parse(RequirementsFile, data, 0, false); err != nil {
		return Requirements{}, true, err
	}
	p.out.Text = strings.Join(p.reqs, "\n")
	if p.out.Text != "" {
		p.out.Text += "\n"
	}
	p.out.Constraints = strings.Join(p.cons, "\n")
	if p.out.Constraints != "" {
		p.out.Constraints += "\n"
	}
	return p.out, true, nil
}

// logicalLines joins "\" continuations and drops comments and blank lines,
// keeping the number of the line each starts on.
func logicalLines(text string) ([]string, []int) {
	var out []string
	var nums []int
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(raw); i++ {
		start := i + 1
		line := raw[i]
		for strings.HasSuffix(line, `\`) && i+1 < len(raw) {
			i++
			line = strings.TrimSuffix(line, `\`) + raw[i]
		}
		line = strings.TrimSpace(reqCommentPattern.ReplaceAllString(line, ""))
		if line == "" {
			continue
		}
		out = append(out, line)
		nums = append(nums, start)
	}
	return out, nums
}

func (p *reqParser) parse(file string, data []byte, depth int, constraint bool) error {
	refuse := func(line int, code, text string) error {
		return &ReqRefusal{File: file, Line: line, Code: code, Text: hideCredentials(text)}
	}
	p.files++
	if p.files > maxRequirementsFiles || len(data) > maxRequirementsBytes {
		return refuse(0, ReqTooLarge, fmt.Sprintf("at most %d files of %d KiB each", maxRequirementsFiles, maxRequirementsBytes>>10))
	}
	lines, nums := logicalLines(string(data))
	for i, line := range lines {
		n := nums[i]
		p.lines++
		if p.lines > maxRequirementsLines {
			return refuse(n, ReqTooLarge, fmt.Sprintf("at most %d lines", maxRequirementsLines))
		}
		// pip expands ${NAME} from the environment; nothing here may depend
		// on the build's environment.
		if strings.ContainsAny(line, "$`") {
			return refuse(n, ReqSyntax, line)
		}
		if strings.HasPrefix(line, "-") {
			if err := p.option(file, n, line, depth, constraint); err != nil {
				return err
			}
			continue
		}
		flat, hashed, err := requirementLine(line)
		if err != nil {
			return refuse(n, err.Error(), line)
		}
		if hashed {
			p.out.Hashed = true
		}
		if constraint {
			p.cons = append(p.cons, flat)
		} else {
			p.reqs = append(p.reqs, flat)
			p.out.Packages++
		}
	}
	return nil
}

// optionValue splits "-r x", "-rx", "--requirement=x", "--requirement x".
func optionValue(line string) (name, value string) {
	if strings.HasPrefix(line, "--") {
		if before, after, ok := strings.Cut(line, "="); ok && !strings.ContainsAny(before, " \t") {
			return before, strings.TrimSpace(after)
		}
		fields := strings.Fields(line)
		return fields[0], strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
	}
	if len(line) > 2 && line[2] != ' ' && line[2] != '\t' {
		return line[:2], strings.TrimSpace(line[2:])
	}
	fields := strings.Fields(line)
	return fields[0], strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
}

func (p *reqParser) option(file string, n int, line string, depth int, constraint bool) error {
	refuse := func(code string) error {
		return &ReqRefusal{File: file, Line: n, Code: code, Text: hideCredentials(line)}
	}
	name, value := optionValue(line)
	switch name {
	case "-e", "--editable":
		return refuse(ReqEditable)
	case "-i", "--index-url", "--no-index":
		return refuse(ReqIndexURL)
	case "--extra-index-url":
		return refuse(ReqExtraIndex)
	case "--trusted-host":
		return refuse(ReqTrustedHost)
	case "-f", "--find-links":
		return refuse(ReqFindLinks)
	case "--require-hashes":
		if value != "" {
			return refuse(ReqOption)
		}
		p.out.Hashed = true
		return nil
	case "--pre", "--prefer-binary":
		if value != "" {
			return refuse(ReqOption)
		}
		if !constraint {
			p.reqs = append(p.reqs, name)
		}
		return nil
	case "-r", "--requirement", "-c", "--constraint":
		nestedConstraint := name == "-c" || name == "--constraint"
		if constraint && !nestedConstraint {
			return refuse(ReqOption)
		}
		target, ok := nestedPath(file, value)
		if !ok {
			if nestedConstraint {
				return refuse(ReqConstraintOut)
			}
			return refuse(ReqNestedOutside)
		}
		if depth+1 > maxRequirementsDepth {
			return refuse(ReqNestedTooDeep)
		}
		if p.opened[target] {
			// A file included twice adds nothing the second time.
			return nil
		}
		p.opened[target] = true
		data, err := p.read(target)
		if errors.Is(err, os.ErrNotExist) {
			return refuse(ReqNestedMissing)
		}
		if err != nil {
			return err
		}
		return p.parse(target, data, depth+1, constraint || nestedConstraint)
	}
	return refuse(ReqOption)
}

// nestedPath resolves an included file relative to the including file's
// folder; it must stay inside app/src and be a plain relative name.
func nestedPath(from, value string) (string, bool) {
	if value == "" || strings.Contains(value, "://") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") || strings.Contains(value, `\`) {
		return "", false
	}
	joined := path.Clean(path.Join(path.Dir(from), value))
	if joined == "." || strings.HasPrefix(joined, "../") || joined == ".." || !reqFilePattern.MatchString(joined) {
		return "", false
	}
	for _, part := range strings.Split(joined, "/") {
		if strings.HasPrefix(part, "..") {
			return "", false
		}
	}
	return joined, true
}

// requirementLine checks one requirement and returns it flattened. An error's
// text is the refusal code.
func requirementLine(line string) (string, bool, error) {
	spec := line
	var opts []string
	if i := strings.Index(line, " --"); i >= 0 {
		spec, opts = strings.TrimSpace(line[:i]), strings.Fields(line[i:])
	} else if i := strings.Index(line, "\t--"); i >= 0 {
		spec, opts = strings.TrimSpace(line[:i]), strings.Fields(line[i:])
	}
	lower := strings.ToLower(spec)
	for _, prefix := range vcsPrefixes {
		if strings.HasPrefix(lower, prefix) || strings.Contains(lower, "@ "+prefix) || strings.Contains(lower, "@"+prefix) {
			return "", false, errors.New(ReqVCS)
		}
	}
	if strings.Contains(spec, "://") || strings.Contains(spec, "@") {
		return "", false, errors.New(ReqURL)
	}
	if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, "~") || strings.ContainsAny(spec, `/\`) {
		return "", false, errors.New(ReqPath)
	}
	for _, suffix := range archiveSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return "", false, errors.New(ReqPath)
		}
	}
	m := reqNamePattern.FindString(spec)
	if m == "" {
		return "", false, errors.New(ReqSyntax)
	}
	rest := spec[len(m):]
	versions, marker, _ := strings.Cut(rest, ";")
	if !reqSpecPattern.MatchString(versions) || !reqMarkerPattern.MatchString(marker) {
		return "", false, errors.New(ReqSyntax)
	}
	hashed := false
	flat := []string{spec}
	for i := 0; i < len(opts); i++ {
		name, value := opts[i], ""
		if before, after, ok := strings.Cut(name, "="); ok {
			name, value = before, after
		} else if i+1 < len(opts) {
			i++
			value = opts[i]
		}
		if name != "--hash" {
			return "", false, errors.New(ReqOption)
		}
		if !reqHashPattern.MatchString(value) {
			return "", false, errors.New(ReqSyntax)
		}
		hashed = true
		flat = append(flat, "--hash="+value)
	}
	return strings.Join(flat, " "), hashed, nil
}

// RequirementName is the package name of a requirement ("Django>=4" ->
// "Django"), "" when there is none.
func RequirementName(spec string) string {
	m := reqNamePattern.FindStringSubmatch(strings.TrimSpace(spec))
	if m == nil {
		return ""
	}
	return m[1]
}
