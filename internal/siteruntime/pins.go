package siteruntime

import (
	"fmt"
	"sort"
	"strings"
)

// The app servers CelikPanel installs into every Python app's environment
// before the customer's requirements, with pip --require-hashes --no-deps:
// gunicorn for WSGI and uvicorn for ASGI (without its [standard] extra, so
// nothing compiled), with uvicorn's pure-Python dependencies. Every file is
// a py3-none-any wheel, so the same pins serve every supported Python minor;
// typing_extensions is needed only below 3.11 (uvicorn's marker).
//
// Source of every version and hash: PyPI's JSON API,
// https://pypi.org/pypi/<name>/<version>/json, read on 2026-11-04 (the
// "digests.sha256" of the bdist_wheel file). Each version was at least two
// weeks old then and not yanked:
//
//	gunicorn 26.2.0           uploaded 2026-08-24  requires_python >=3.10, no dependencies
//	uvicorn 0.54.0            uploaded 2026-09-25  requires click>=7.0, h11>=0.8, typing-extensions>=4.0 (python < 3.11)
//	click 8.5.0               uploaded 2026-08-26  no dependencies (colorama only on Windows)
//	h11 0.16.0                uploaded 2025-04-24  no dependencies
//	typing_extensions 4.16.0  uploaded 2026-07-02  no dependencies
//
// A new pin is a reviewed change of this file with the same record.
//
// Uygulama sunucuları: gunicorn (WSGI) ve uvicorn (ASGI), PyPI'ın JSON
// arayüzünden okunan sha256 özetleriyle sabitlenir ve müşterinin paketlerinden
// önce özet denetimiyle kurulur.

// PythonPin is one pinned wheel.
type PythonPin struct {
	Name    string
	Version string
	Wheel   string
	SHA256  string
	// BelowMinor: installed only on Python minors below this one ("" = all).
	BelowMinor string
}

// PythonPinsRead is when the pins were read from PyPI.
const PythonPinsRead = "2026-11-04"

var appServerPins = []PythonPin{
	{Name: "gunicorn", Version: "26.2.0", Wheel: "gunicorn-26.2.0-py3-none-any.whl", SHA256: "bd249d0b3f7972f7432f0a6b6ff3b3ee2d129f70cd1ff6c09a9dd9e29a2b88e3"},
	{Name: "uvicorn", Version: "0.54.0", Wheel: "uvicorn-0.54.0-py3-none-any.whl", SHA256: "505bdb0f318731d45f1f712071fc781a8981f6847a31c902c9f5e652d4f67faf"},
	{Name: "click", Version: "8.5.0", Wheel: "click-8.5.0-py3-none-any.whl", SHA256: "255bc9599cf7748b4b1a446ccc735421bd08a2ae529a8b88597d3de5664ee360"},
	{Name: "h11", Version: "0.16.0", Wheel: "h11-0.16.0-py3-none-any.whl", SHA256: "63cf8bbe7522de3bf65932fda1d9c2772064ffb3dae62d55932da54b31cb6c86"},
	{Name: "typing_extensions", Version: "4.16.0", Wheel: "typing_extensions-4.16.0-py3-none-any.whl", SHA256: "481caa481374e813c1b176ada14e97f1f67a4539ce9cfeb3f350d78d6370c2e8", BelowMinor: "3.11"},
}

// PinnedMinors are the Python minors this release has app-server pins for:
// every supported system's Python (DistroPythons) and nothing else.
var PinnedMinors = []string{"3.10", "3.11", "3.12", "3.13", "3.14"}

// PinnedMinor reports whether this release can serve apps on a minor.
func PinnedMinor(minor string) bool {
	for _, m := range PinnedMinors {
		if m == minor {
			return true
		}
	}
	return false
}

func minorNumber(minor string) int {
	var major, n int
	if _, err := fmt.Sscanf(minor, "%d.%d", &major, &n); err != nil {
		return -1
	}
	return n
}

// AppServerPins are the pins of one Python minor.
func AppServerPins(minor string) ([]PythonPin, error) {
	if !PinnedMinor(minor) {
		return nil, fmt.Errorf("this release has no app-server pins for Python %s; a CelikPanel update adds them", minor)
	}
	out := []PythonPin{}
	for _, p := range appServerPins {
		if p.BelowMinor != "" && minorNumber(minor) >= minorNumber(p.BelowMinor) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// AppServerRequirements is the root-owned requirements file of the pins.
func AppServerRequirements(minor string) (string, error) {
	pins, err := AppServerPins(minor)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# CelikPanel's pinned app servers (PyPI JSON API, " + PythonPinsRead + "); installed with --require-hashes --no-deps.\n")
	for _, p := range pins {
		fmt.Fprintf(&b, "%s==%s --hash=sha256:%s\n", p.Name, p.Version, p.SHA256)
	}
	return b.String(), nil
}
