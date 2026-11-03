package main

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/alicelik/celikpanel/internal/appcatalog"
)

// Unpacking a verified release into the root-private staging folder. The
// rules are WordPress's (catalog limits, no absolute or escaping path, no
// duplicate, no device, no hard link) with three additions the catalog's
// releases need: a top folder of any name (or none), a zip inside the zip
// (PrestaShop), and relative symbolic links that stay inside the release,
// also through other links (Nextcloud's licence files), which are written as
// copies of the file they end at so the published tree holds no link at all.
// An absolute link, or one that leaves the release, is refused.
//
// Doğrulanmış sürüm, kök kullanıcıya özel hazırlık klasörüne açılır. Kurallar
// WordPress'inkilerdir; ek olarak üst klasör adı serbesttir, zip içinde zip
// açılır ve sürüm ağacının içinde kalan göreli bağlantılar (başka bağlantı
// üzerinden de) gösterdikleri dosyanın kopyası olarak yazılır. Mutlak ya da
// ağaçtan çıkan bağlantı reddedilir.

type catalogExtractLimits struct {
	maxBytes   int64
	maxEntries int
	maxFile    int64
}

func limitsFor(app appcatalog.App) catalogExtractLimits {
	limits := catalogExtractLimits{maxBytes: app.Archive.MaxExtractedBytes, maxEntries: app.Archive.MaxEntries, maxFile: 256 << 20}
	if limits.maxBytes <= 0 {
		limits.maxBytes = 512 << 20
	}
	if limits.maxEntries <= 0 {
		limits.maxEntries = 50_000
	}
	return limits
}

// catalogExtractor writes entries below destination.
type catalogExtractor struct {
	destination string
	prefix      string
	limits      catalogExtractLimits
	seen        map[string]bool
	links       map[string]string // relative path -> relative target
	total       int64
	count       int
}

// entryPath validates an archive name and returns the path relative to the
// application root ("" for the top folder itself).
func (x *catalogExtractor) entryPath(name string) (string, error) {
	if name == "" || len(name) > wpMaxArchivePathBytes || strings.ContainsRune(name, 0) || strings.ContainsRune(name, '\\') {
		return "", errors.New("the archive contains an invalid path")
	}
	clean := path.Clean(name)
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("archive entry %q leaves the application folder", name)
	}
	if clean == "." {
		return "", nil
	}
	if x.prefix != "" {
		if clean == x.prefix {
			return "", nil
		}
		rest, ok := strings.CutPrefix(clean, x.prefix+"/")
		if !ok {
			return "", fmt.Errorf("archive entry %q is outside the release's folder %q", name, x.prefix)
		}
		clean = rest
	}
	if x.seen[clean] {
		return "", fmt.Errorf("the archive contains %q twice", clean)
	}
	x.seen[clean] = true
	x.count++
	if x.count > x.limits.maxEntries {
		return "", errors.New("the archive contains too many entries")
	}
	return clean, nil
}

func (x *catalogExtractor) target(rel string) (string, error) {
	target := filepath.Join(x.destination, filepath.FromSlash(rel))
	check, err := filepath.Rel(x.destination, target)
	if err != nil || check == ".." || strings.HasPrefix(check, ".."+string(filepath.Separator)) {
		return "", errors.New("archive entry escapes the staging folder")
	}
	return target, nil
}

func (x *catalogExtractor) dir(rel string) error {
	if rel == "" {
		return nil
	}
	target, err := x.target(rel)
	if err != nil {
		return err
	}
	return os.MkdirAll(target, 0o750)
}

func (x *catalogExtractor) file(rel string, size int64, body io.Reader) error {
	if rel == "" {
		return errors.New("the release's top folder is a file")
	}
	if size < 0 || size > x.limits.maxFile || x.total > x.limits.maxBytes-size {
		return errors.New("the archive exceeds the unpacking limits")
	}
	x.total += size
	target, err := x.target(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(body, size+1))
	closeErr := out.Close()
	if copyErr == nil && written != size {
		copyErr = fmt.Errorf("archive entry %q is not the size it declares", rel)
	}
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// maxCatalogLinkHops bounds a chain of links (link -> link -> file).
const maxCatalogLinkHops = 16

// catalogLinkTarget resolves a link's text against the folder that holds
// the link and returns the path relative to the application root. Only a
// relative target that stays inside the root is accepted: an absolute target,
// one that climbs out of the release, or one with a backslash or NUL is
// refused (alpha.118 lab: Nextcloud 35 links its .license files to files in
// other folders of its own tree, and to other links).
//
// Bağlantının hedefi, bağlantının bulunduğu klasöre göre çözülür; yalnız
// sürüm ağacının içinde kalan göreli hedef kabul edilir.
func catalogLinkTarget(rel, target string) (string, error) {
	if rel == "" || target == "" || strings.ContainsAny(target, "\\\x00") || path.IsAbs(target) {
		return "", fmt.Errorf("archive entry %q is a link outside the release", rel)
	}
	resolved := path.Clean(path.Join(path.Dir(rel), target))
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") || path.IsAbs(resolved) {
		return "", fmt.Errorf("archive entry %q is a link outside the release", rel)
	}
	return resolved, nil
}

// link records a symbolic link whose target stays inside the release
// (materialized after every entry is written).
func (x *catalogExtractor) link(rel, target string) error {
	resolved, err := catalogLinkTarget(rel, target)
	if err != nil {
		return err
	}
	x.links[rel] = resolved
	return nil
}

// finalLinkTarget follows a chain of links of the release to the entry that
// is not a link; a loop or an overly long chain is refused.
func (x *catalogExtractor) finalLinkTarget(rel string) (string, error) {
	current := x.links[rel]
	for hops := 0; ; hops++ {
		next, isLink := x.links[current]
		if !isLink {
			return current, nil
		}
		if hops >= maxCatalogLinkHops || current == rel {
			return "", fmt.Errorf("archive link %q is part of a loop or a too long chain of links", rel)
		}
		current = next
	}
}

// materializeLinks writes each link as a copy of the regular file it names
// (through any chain of links of the release). No link is ever written to
// disk, so a path can never be followed outside the staging folder.
func (x *catalogExtractor) materializeLinks() error {
	for rel := range x.links {
		targetRel, err := x.finalLinkTarget(rel)
		if err != nil {
			return err
		}
		source, err := x.target(targetRel)
		if err != nil {
			return err
		}
		info, err := os.Lstat(source)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("archive link %q points at %q, which is not a file of the release", rel, targetRel)
		}
		in, err := os.Open(source)
		if err != nil {
			return err
		}
		err = x.file(rel, info.Size(), in)
		in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// extractCatalogArchive unpacks the verified archive of app into the empty
// destination; scratch is a root-private folder for an inner archive.
func extractCatalogArchive(app appcatalog.App, archivePath, destination, scratch string) error {
	entries, err := os.ReadDir(destination)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("the staging folder is not empty")
	}
	x := &catalogExtractor{destination: destination, prefix: app.Archive.StripPrefix, limits: limitsFor(app),
		seen: map[string]bool{}, links: map[string]string{}}
	switch app.Archive.Format {
	case appcatalog.FormatTarGz, appcatalog.FormatTarBz2:
		err = x.extractTar(archivePath, app.Archive.Format)
	case appcatalog.FormatZip:
		source := archivePath
		if app.Archive.Inner != "" {
			source, err = extractInnerZip(archivePath, app.Archive.Inner, scratch, app.Archive.MaxDownloadBytes*2)
			if err != nil {
				return err
			}
			defer os.Remove(source)
		}
		err = x.extractZip(source)
	default:
		return fmt.Errorf("unsupported archive format %q", app.Archive.Format)
	}
	if err != nil {
		return err
	}
	if err := x.materializeLinks(); err != nil {
		return err
	}
	for _, marker := range app.Archive.Markers {
		dir := strings.HasSuffix(marker, "/")
		info, err := os.Lstat(filepath.Join(destination, filepath.FromSlash(strings.TrimSuffix(marker, "/"))))
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != dir {
			return fmt.Errorf("the release is missing %s, which every %s release has", marker, app.Name)
		}
	}
	return nil
}

func (x *catalogExtractor) extractTar(archivePath, format string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	var stream io.Reader
	if format == appcatalog.FormatTarGz {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		stream = gz
	} else {
		stream = bzip2.NewReader(f)
	}
	reader := tar.NewReader(stream)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue
		}
		rel, err := x.entryPath(header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = x.dir(rel)
		case tar.TypeReg, tar.TypeRegA:
			err = x.file(rel, header.Size, reader)
		case tar.TypeSymlink:
			err = x.link(rel, header.Linkname)
		default:
			err = fmt.Errorf("archive entry %q has an unsupported type", rel)
		}
		if err != nil {
			return err
		}
	}
}

func (x *catalogExtractor) extractZip(archivePath string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		rel, err := x.entryPath(entry.Name)
		if err != nil {
			return err
		}
		mode := entry.Mode()
		switch {
		case mode.IsDir() || strings.HasSuffix(entry.Name, "/"):
			err = x.dir(rel)
		case mode&os.ModeSymlink != 0:
			err = fmt.Errorf("archive entry %q is a link; zip releases may not contain links", rel)
		case mode.IsRegular():
			if entry.UncompressedSize64 > uint64(x.limits.maxFile) {
				return errors.New("the archive exceeds the unpacking limits")
			}
			body, openErr := entry.Open()
			if openErr != nil {
				return openErr
			}
			err = x.file(rel, int64(entry.UncompressedSize64), body)
			body.Close()
		default:
			err = fmt.Errorf("archive entry %q has an unsupported type", rel)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// extractInnerZip copies the named member of the outer zip to a file in
// scratch and returns its path.
func extractInnerZip(outer, name, scratch string, limit int64) (string, error) {
	reader, err := zip.OpenReader(outer)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.Name != name {
			continue
		}
		if !entry.Mode().IsRegular() || limit <= 0 || entry.UncompressedSize64 > uint64(limit) {
			return "", fmt.Errorf("the release's %s is not a plain file within the limits", name)
		}
		body, err := entry.Open()
		if err != nil {
			return "", err
		}
		defer body.Close()
		target := filepath.Join(scratch, "inner.zip")
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return "", err
		}
		written, copyErr := io.Copy(out, io.LimitReader(body, int64(entry.UncompressedSize64)+1))
		closeErr := out.Close()
		if copyErr == nil && written != int64(entry.UncompressedSize64) {
			copyErr = fmt.Errorf("the release's %s is not the size it declares", name)
		}
		if copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			_ = os.Remove(target)
			return "", copyErr
		}
		return target, nil
	}
	return "", fmt.Errorf("the release does not contain %s", name)
}
