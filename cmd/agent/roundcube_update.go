package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/openpgp"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Roundcube's update path (alpha.107, D-038 §9). Roundcube is installed by
// the panel from its own release, so the operating system never updates it:
// reporting a newer release on the installed line and applying it with the
// administrator's confirmation is the panel's job. The release list is read
// from Roundcube's own GitHub releases; a release is installed only when its
// archive carries a valid OpenPGP signature by Roundcube's signing key,
// pinned here by its primary fingerprint (F3E4 C04B … 41C4 F7D5; the archive
// is signed with its signing subkey 8970 E37A … 09CD 56B4, as roundcube.net
// publishes). The update keeps the configuration and the database, runs
// Roundcube's own database update, swaps the trees atomically, proves that
// webmail answers through its socket and puts the previous tree back byte
// for byte when it does not. Never automatic.
//
// Roundcube güncelleme yolu: yayın listesi Roundcube'un kendi
// yayınlarından; arşiv, sabitlenmiş imza anahtarıyla imzalıysa kurulur.
// Yapılandırma ve veritabanı korunur, ağaçlar atomik değişir, web postası
// kanıtlanır; olmazsa önceki ağaç bayt bayt geri gelir. Kendiliğinden olmaz.

//go:embed roundcube-signing-key.asc
var roundcubeSigningKey string

const (
	roundcubeSigningPrimary = "F3E4C04BB3DB5D4215C45F7F5AB2BAA141C4F7D5"
	roundcubeReleasesURL    = "https://api.github.com/repos/roundcube/roundcubemail/releases?per_page=40"
	roundcubeReleaseCache   = 6 * time.Hour
	roundcubeReleaseBound   = 30 * time.Second
	roundcubeSignatureMax   = 64 << 10
)

var roundcubeReleaseVersion = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)$`)

// roundcubeReleaseURL is the one place an archive is fetched from: the
// project's own release download, built from a validated version.
func roundcubeReleaseURL(version, suffix string) string {
	return fmt.Sprintf("https://github.com/roundcube/roundcubemail/releases/download/%s/roundcubemail-%s-complete.tar.gz%s", version, version, suffix)
}

func roundcubeVersionParts(version string) ([3]int, bool) {
	match := roundcubeReleaseVersion.FindStringSubmatch(version)
	if match == nil {
		return [3]int{}, false
	}
	var parts [3]int
	for i := range parts {
		parts[i], _ = strconv.Atoi(match[i+1])
	}
	return parts, true
}

func roundcubeVersionLess(a, b string) bool {
	pa, okA := roundcubeVersionParts(a)
	pb, okB := roundcubeVersionParts(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func roundcubeLine(version string) string {
	if parts, ok := roundcubeVersionParts(version); ok {
		return fmt.Sprintf("%d.%d", parts[0], parts[1])
	}
	return ""
}

// roundcubeReleaseChoice reads the release list: the newest stable release on
// the installed line and the newest stable release of all.
func roundcubeReleaseChoice(body []byte, installed string) (onLine, overall string, err error) {
	var releases []struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", "", fmt.Errorf("the release list could not be read: %v", err)
	}
	line := roundcubeLine(installed)
	for _, release := range releases {
		if release.Draft || release.Prerelease {
			continue
		}
		if _, ok := roundcubeVersionParts(release.Tag); !ok {
			continue
		}
		if overall == "" || roundcubeVersionLess(overall, release.Tag) {
			overall = release.Tag
		}
		if roundcubeLine(release.Tag) == line && (onLine == "" || roundcubeVersionLess(onLine, release.Tag)) {
			onLine = release.Tag
		}
	}
	return onLine, overall, nil
}

var roundcubeReleaseState = struct {
	sync.Mutex
	at   time.Time
	body []byte
}{}

// roundcubeFetchReleases reads the release list, kept for six hours (the
// list is public and rate-limited); force reads it again.
var roundcubeFetchReleases = func(ctx context.Context, force bool) ([]byte, time.Time, error) {
	roundcubeReleaseState.Lock()
	defer roundcubeReleaseState.Unlock()
	if !force && roundcubeReleaseState.body != nil && time.Since(roundcubeReleaseState.at) < roundcubeReleaseCache {
		return roundcubeReleaseState.body, roundcubeReleaseState.at, nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, roundcubeReleaseBound)
	defer cancel()
	request, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, roundcubeReleasesURL, nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := (&http.Client{Timeout: roundcubeReleaseBound}).Do(request)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("Roundcube's release list could not be reached: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("Roundcube's release list answered HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, time.Time{}, err
	}
	roundcubeReleaseState.at, roundcubeReleaseState.body = time.Now(), body
	return body, roundcubeReleaseState.at, nil
}

func roundcubeUpdateCheck(ctx context.Context, force bool) transport.RoundcubeUpdateCheckResponse {
	resp := transport.RoundcubeUpdateCheckResponse{}
	if !roundcubeInstalled() {
		return resp
	}
	resp.Installed = true
	resp.Version = roundcubeInstalledVersion()
	resp.Line = roundcubeLine(resp.Version)
	if resp.Version == "" {
		resp.Error = "the installed Roundcube does not say its version"
		return resp
	}
	body, at, err := roundcubeFetchReleases(ctx, force)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	resp.CheckedAt = at.UTC().Format(time.RFC3339)
	onLine, overall, err := roundcubeReleaseChoice(body, resp.Version)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	resp.LatestOnLine, resp.LatestOverall = onLine, overall
	resp.UpdateAvailable = onLine != "" && roundcubeVersionLess(resp.Version, onLine)
	return resp
}

// RoundcubeUpdateCheckV1 answers, read only, whether a newer Roundcube
// release exists on the installed line.
func (a *Agent) RoundcubeUpdateCheckV1(_ *transport.Empty, resp *transport.RoundcubeUpdateCheckResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), roundcubeReleaseBound+5*time.Second)
	defer cancel()
	*resp = roundcubeUpdateCheck(ctx, false)
	return nil
}

// verifyRoundcubeSignature accepts the archive only when the detached
// signature is valid and made by the pinned key (any of its subkeys).
func verifyRoundcubeSignature(archive io.Reader, signature []byte, armoredKey, primary string) error {
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armoredKey))
	if err != nil || len(keyring) == 0 {
		return fmt.Errorf("the pinned Roundcube signing key could not be read")
	}
	if got := strings.ToUpper(hex.EncodeToString(keyring[0].PrimaryKey.Fingerprint[:])); got != primary {
		return fmt.Errorf("the embedded Roundcube signing key is not the pinned one")
	}
	signer, err := openpgp.CheckArmoredDetachedSignature(keyring, archive, bytes.NewReader(signature))
	if err != nil {
		return fmt.Errorf("the archive's signature is not valid: %v", err)
	}
	if got := strings.ToUpper(hex.EncodeToString(signer.PrimaryKey.Fingerprint[:])); got != primary {
		return fmt.Errorf("the archive is signed by another key")
	}
	return nil
}

// UpdateRoundcubeV1 updates Roundcube to the newest release on its line at
// the administrator's confirmation (alpha.107).
func (a *Agent) UpdateRoundcubeV1(req *transport.RoundcubeUpdateRequest, resp *transport.RoundcubeUpdateResponse) error {
	*resp = transport.RoundcubeUpdateResponse{}
	if req == nil {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "missing request"
		return nil
	}
	version := strings.TrimSpace(req.Version)
	if _, ok := roundcubeVersionParts(version); !ok {
		resp.ErrorCode, resp.Error = "INVALID_REQUEST", "not a Roundcube release version"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepUpdateRoundcube, "roundcube", version, "update"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = "MUTATION_REFUSED", err.Error()
		return nil
	}
	defer finishStep()
	*resp = updateRoundcubeTo(ctx, version, hostRoundcubeUpdateOps())
	return nil
}
