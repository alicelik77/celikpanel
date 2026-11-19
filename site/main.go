// Command site builds the public website of CelikPanel (celikpanel.net).
//
// The pages are static HTML generated from Go html/template templates
// (templates/) and one text catalogue per language (text/en.json,
// text/tr.json). The stylesheet and the two small scripts (assets/) are
// copied beside them. Nothing is needed at run time: every page reads without
// JavaScript, makes no outside request and keeps to the portal's strict
// Content-Security-Policy (no inline script or style).
//
// Usage, from the repository root:
//
//	go run ./site                    # write the pages into download-portal/
//	go run ./site -preview DIR       # a copy openable from disk, with the
//	                                 # current release filled in
//
// The generated files are committed under download-portal/ and listed with
// their SHA-256 in download-portal/site.sha256. deploy/build-download-portal.sh
// ships exactly those bytes and fills the release placeholders
// (@@CP_RELEASE_…@@) of the published release; TestCommittedSiteIsCurrent
// fails when a template, a catalogue or an asset changed without running
// `go run ./site` again.
//
// Adding a page: add its two output paths to `pages` in site.go, write
// templates/<key>.html defining "hero" and "main", add <key>.title and
// <key>.description (and its text) to both catalogues, link it from the
// navigation if it belongs there, then run `go run ./site` and the tests.
//
// Herkese açık sitenin üreticisi: sayfalar şablonlardan ve dil başına bir metin
// kataloğundan üretilir, download-portal/ altına yazılır ve site.sha256 ile
// listelenir. Şablon ya da metin değişince `go run ./site` yeniden çalıştırılır.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	out := flag.String("out", "download-portal", "directory the pages are written into")
	preview := flag.String("preview", "", "write a disk-openable preview with the current release into this directory instead")
	flag.Parse()
	var err error
	if *preview != "" {
		err = writePreview(*out, *preview)
	} else {
		err = writeSite(*out)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "site:", err)
		os.Exit(1)
	}
}
