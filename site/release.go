package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The release a page shows is not known when the pages are generated: it is
// the release the portal package is built for. Pages carry these
// placeholders and deploy/build-download-portal.sh replaces them with the
// published release's checked values (version, time, commit, SHA-256 and the
// file addresses of latest.json). The lines between the two signed markers
// name the signed manifest and its signature; an unsigned (test) build drops
// them.
//
// Sürüm bilgisi sayfalar üretilirken bilinmez; portal paketi kurulurken
// build-download-portal.sh bu yer tutucuları yayımlanan sürümle doldurur.
const (
	phVersion     = "@@CP_RELEASE_VERSION@@"
	phDateEN      = "@@CP_RELEASE_DATE_EN@@"
	phDateTR      = "@@CP_RELEASE_DATE_TR@@"
	phDateTime    = "@@CP_RELEASE_DATETIME@@"
	phCommit      = "@@CP_RELEASE_COMMIT@@"
	phCommitShort = "@@CP_RELEASE_COMMIT_SHORT@@"
	phSHA256      = "@@CP_RELEASE_SHA256@@"
	phArchive     = "@@CP_RELEASE_ARCHIVE_NAME@@"
	phArchiveURL  = "@@CP_RELEASE_ARCHIVE_URL@@"
	phChecksumURL = "@@CP_RELEASE_CHECKSUM_URL@@"
	phManifestURL = "@@CP_RELEASE_MANIFEST_URL@@"
	phSigURL      = "@@CP_RELEASE_SIGNATURE_URL@@"
	signedStart   = "<!-- cp:signed -->"
	signedEnd     = "<!-- cp:/signed -->"
)

var placeholder = regexp.MustCompile(`@@CP_[A-Z0-9_]+@@`)

type release struct {
	Version, Commit, CommitShort, SHA256 string
	Published                            time.Time
}

var monthsTR = [...]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

func dateEN(t time.Time) string { return fmt.Sprintf("%d %s %d", t.Day(), t.Month(), t.Year()) }
func dateTR(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), monthsTR[t.Month()-1], t.Year())
}

// fillRelease is the preview's version of the builder's substitution (signed
// layout, linux/amd64).
func fillRelease(page []byte, r release) []byte {
	prefix := "/releases/" + r.Version + "/linux/amd64/"
	archive := "celikpanel-" + r.Version + "-linux-amd64.tar.gz"
	replacer := strings.NewReplacer(
		phVersion, r.Version,
		phDateEN, dateEN(r.Published),
		phDateTR, dateTR(r.Published),
		phDateTime, r.Published.UTC().Format(time.RFC3339),
		phCommit, r.Commit,
		phCommitShort, r.CommitShort,
		phSHA256, r.SHA256,
		phArchive, archive,
		phArchiveURL, prefix+archive,
		phChecksumURL, prefix+archive+".sha256",
		phManifestURL, prefix+"release-manifest-v2",
		phSigURL, prefix+"release-manifest-v2.sig",
	)
	lines := strings.Split(replacer.Replace(string(page)), "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) == signedStart || strings.TrimSpace(line) == signedEnd {
			continue
		}
		kept = append(kept, line)
	}
	return []byte(strings.Join(kept, "\n"))
}

var (
	pinnedVersion = regexp.MustCompile(`(?m)^bootstrap_release_version=(v[0-9A-Za-z.-]+)$`)
	publishedLine = regexp.MustCompile("\\*\\*Published\\*\\* on ([0-9]{4}-[0-9]{2}-[0-9]{2}) \\(UTC\\): release commit `([0-9a-f]{7,40})`")
)

// currentRelease reads what the repository says about its latest release:
// the version download-portal/get.sh pins and the Published line of its
// release notes. The archive's SHA-256 is known only to the release build,
// so the preview says so in its place.
func currentRelease(portal string) (release, error) {
	raw, err := os.ReadFile(filepath.Join(portal, "get.sh"))
	if err != nil {
		return release{}, err
	}
	match := pinnedVersion.FindSubmatch(raw)
	if match == nil {
		return release{}, fmt.Errorf("%s/get.sh pins no release version", portal)
	}
	version := string(match[1])
	notes, err := os.ReadFile(filepath.Join(portal, "..", "docs", "RELEASE-NOTES-"+version+".md"))
	if err != nil {
		return release{}, err
	}
	line := publishedLine.FindSubmatch(notes)
	if line == nil {
		// Between a release's merge and the record of its publication the
		// notes have no Published line yet; the preview still renders.
		return release{
			Version: version, Commit: "(not yet published)", CommitShort: "unpublished",
			SHA256:    "(filled in when the portal package is built)",
			Published: time.Now().UTC(),
		}, nil
	}
	published, err := time.Parse("2006-01-02", string(line[1]))
	if err != nil {
		return release{}, err
	}
	commit := string(line[2])
	return release{
		Version: version, Commit: commit + " (preview: short form)", CommitShort: commit,
		SHA256:    "(filled in when the portal package is built)",
		Published: published,
	}, nil
}

// placeholdersIn lists the release placeholders a page carries.
func placeholdersIn(page []byte) []string {
	seen := map[string]bool{}
	var result []string
	for _, match := range placeholder.FindAll(page, -1) {
		if !seen[string(match)] {
			seen[string(match)] = true
			result = append(result, string(match))
		}
	}
	return result
}

var knownPlaceholders = map[string]bool{
	phVersion: true, phDateEN: true, phDateTR: true, phDateTime: true, phCommit: true,
	phCommitShort: true, phSHA256: true, phArchive: true, phArchiveURL: true,
	phChecksumURL: true, phManifestURL: true, phSigURL: true,
}

// signedBlocksWellFormed checks that every signed marker stands alone on its
// line and opens and closes in order, so the builder's line-based removal
// takes exactly the block.
func signedBlocksWellFormed(page []byte) error {
	open := false
	for number, line := range bytes.Split(page, []byte("\n")) {
		text := string(line)
		hasStart, hasEnd := strings.Contains(text, signedStart), strings.Contains(text, signedEnd)
		if (hasStart || hasEnd) && strings.TrimSpace(text) != signedStart && strings.TrimSpace(text) != signedEnd {
			return fmt.Errorf("line %d: a signed marker must stand alone on its line", number+1)
		}
		if hasStart {
			if open {
				return fmt.Errorf("line %d: nested signed block", number+1)
			}
			open = true
		}
		if hasEnd {
			if !open {
				return fmt.Errorf("line %d: signed block closed before it opened", number+1)
			}
			open = false
		}
	}
	if open {
		return fmt.Errorf("signed block not closed")
	}
	return nil
}
