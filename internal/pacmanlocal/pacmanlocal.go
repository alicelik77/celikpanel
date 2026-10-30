// Package pacmanlocal reads pacman's own local package database
// (/var/lib/pacman/local) without running pacman: which version of a package
// is installed, and the SHA-256 digest the package recorded for one of its
// files (the gzip-compressed mtree). It is the Arch counterpart of dpkg's
// status and md5sums: a match proves the file on disk is the installed
// package's own, unmodified file. Like dpkg's records it is an identity check,
// not a defence against root, who can change both the file and the record.
//
// pacmanlocal, pacman'ın yerel paket veritabanını pacman çalıştırmadan okur:
// kurulu sürüm ve paketin bir dosya için kaydettiği SHA-256 özeti. Eşleşme,
// diskteki dosyanın kurulu paketin kendi, değişmemiş dosyası olduğunu gösterir.
package pacmanlocal

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DBDir is pacman's local package database.
const DBDir = "/var/lib/pacman/local"

const (
	descLimit  = 64 << 10
	mtreeLimit = 8 << 20
	plainLimit = 32 << 20
)

// Package is one installed package entry of the local database.
type Package struct {
	Name    string
	Version string
	Dir     string
}

var (
	namePattern    = regexp.MustCompile(`^[a-z0-9@._+][a-z0-9@._+-]*$`)
	versionPattern = regexp.MustCompile(`^([0-9]+:)?[0-9A-Za-z._+~]+-[0-9]+(\.[0-9]+)?$`)
	digestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidVersion reports whether value is a canonical pacman package version
// ("[epoch:]pkgver-pkgrel").
func ValidVersion(value string) bool {
	return versionPattern.MatchString(value) && !strings.Contains(value, "..")
}

// ParseDesc returns the %NAME% and %VERSION% of a local database desc file.
func ParseDesc(desc []byte) (string, string, error) {
	values := map[string]string{}
	lines := strings.Split(string(desc), "\n")
	for i := 0; i < len(lines); i++ {
		key := strings.TrimSpace(lines[i])
		if key != "%NAME%" && key != "%VERSION%" {
			continue
		}
		if _, seen := values[key]; seen || i+1 >= len(lines) {
			return "", "", errors.New("pacman package description is not canonical")
		}
		values[key] = strings.TrimSpace(lines[i+1])
		i++
	}
	name, version := values["%NAME%"], values["%VERSION%"]
	if !namePattern.MatchString(name) || !ValidVersion(version) {
		return "", "", errors.New("pacman package description has no canonical name and version")
	}
	return name, version, nil
}

// Installed finds the one local database entry of an installed package. A
// missing package is os.ErrNotExist; more than one entry is refused.
func Installed(dbDir, name string) (Package, error) {
	if !namePattern.MatchString(name) {
		return Package{}, fmt.Errorf("invalid pacman package name %q", name)
	}
	if err := secureDirectory(dbDir); err != nil {
		return Package{}, err
	}
	entries, err := os.ReadDir(dbDir)
	if err != nil {
		return Package{}, fmt.Errorf("read pacman local database: %w", err)
	}
	found := Package{}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), name+"-") {
			continue
		}
		dir := filepath.Join(dbDir, entry.Name())
		desc, err := ReadSecureFile(filepath.Join(dir, "desc"), descLimit)
		if err != nil {
			return Package{}, err
		}
		gotName, version, err := ParseDesc(desc)
		if err != nil {
			return Package{}, err
		}
		if gotName != name {
			continue
		}
		if entry.Name() != name+"-"+version {
			return Package{}, errors.New("pacman package entry name differs from its description")
		}
		if found.Name != "" {
			return Package{}, fmt.Errorf("pacman local database names package %s more than once", name)
		}
		found = Package{Name: name, Version: version, Dir: dir}
	}
	if found.Name == "" {
		return Package{}, fmt.Errorf("pacman package %s is not installed: %w", name, os.ErrNotExist)
	}
	return found, nil
}

// MtreeSHA256 returns the sha256digest a package's gzip-compressed mtree
// recorded for exactly one entry of an absolute path. A missing entry, a
// duplicate or an entry without a canonical digest is refused.
func MtreeSHA256(mtreeGzip []byte, path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, " \t\\") {
		return "", errors.New("mtree lookup path is not canonical")
	}
	reader, err := gzip.NewReader(bytes.NewReader(mtreeGzip))
	if err != nil {
		return "", fmt.Errorf("read pacman mtree: %w", err)
	}
	defer reader.Close()
	plain, err := io.ReadAll(io.LimitReader(reader, plainLimit+1))
	if err != nil {
		return "", fmt.Errorf("read pacman mtree: %w", err)
	}
	if len(plain) > plainLimit {
		return "", errors.New("pacman mtree exceeds its bound")
	}
	want := "." + path
	scanner := bufio.NewScanner(bytes.NewReader(plain))
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	digest := ""
	seen := false
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || fields[0] != want {
			continue
		}
		if seen {
			return "", errors.New("pacman mtree names the file more than once")
		}
		seen = true
		for _, field := range fields[1:] {
			if value, ok := strings.CutPrefix(field, "sha256digest="); ok {
				if digest != "" || !digestPattern.MatchString(value) {
					return "", errors.New("pacman mtree digest is not canonical")
				}
				digest = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read pacman mtree: %w", err)
	}
	if digest == "" {
		return "", errors.New("pacman mtree records no digest for the file")
	}
	return digest, nil
}

// PackageFileSHA256 returns the digest the installed package recorded for
// path.
func PackageFileSHA256(pkg Package, path string) (string, error) {
	if pkg.Dir == "" {
		return "", errors.New("pacman package entry is unknown")
	}
	mtree, err := ReadSecureFile(filepath.Join(pkg.Dir, "mtree"), mtreeLimit)
	if err != nil {
		return "", err
	}
	return MtreeSHA256(mtree, path)
}

// FileSHA256 hashes one regular file opened without following links.
func FileSHA256(path string) (string, error) {
	file, err := openNoFollow(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("hashed file is not a regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// VerifyPackageFile proves that the file at path is the installed package's
// own, unmodified file: its SHA-256 equals the package's mtree record.
func VerifyPackageFile(pkg Package, path string) error {
	want, err := PackageFileSHA256(pkg, path)
	if err != nil {
		return err
	}
	got, err := FileSHA256(path)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s differs from the file the installed %s %s package recorded", path, pkg.Name, pkg.Version)
	}
	return nil
}
