package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/openpgp/armor"
	pgperrors "golang.org/x/crypto/openpgp/errors"
	"golang.org/x/crypto/openpgp/packet"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The repository card (alpha.107, D-038 §12): who enabled the repository and
// when, its signing key's fingerprint and expiry, what apt last downloaded
// from it, apt's own error lines that name it, the confinement, and the
// distribution's moving "default version" packages installed beside it.
// Read only: the Agent reads files it already has, starts no command and no
// network access.
//
// Depo kartı: kim ne zaman açtı, imza anahtarı ve süresi, apt'ın son indirdiği,
// depoyu adlandıran apt hata satırları, sınırlama ve kurulu "varsayılan sürüm"
// paketleri. Salt okunur: komut ve ağ erişimi yok.

const repoKeyExpiringWithin = 60 * 24 * time.Hour

var repoDescribeNow = time.Now

func describeEnabledRepo(repo *core.ManagedRepo, resp *RepoStatusResponse) {
	if loc, err := parseAptSourceLine(resp.Source); err == nil {
		confinement := repoConfinementOf(repo, loc)
		resp.Confinement = &confinement
		resp.Lists = repoListsInfo(loc)
	}
	resp.Key = repoKeyInfo(repo)
	entry := readRepoRecord().Repos[repo.ID]
	resp.EnabledAt = entry.EnabledAt
	resp.AptErrors, resp.AptErrorsAt = entry.AptErrors, entry.AptErrorsAt
	resp.MovingPackages = repoMovingPackages(repo)
}

func repoListsInfo(loc aptSourceLocation) *transport.RepoListsInfo {
	release, ok := readRepoReleaseInfo(loc)
	if !ok {
		return &transport.RepoListsInfo{State: "missing"}
	}
	info := &transport.RepoListsInfo{State: "fresh", Date: release.Date, ValidUntil: release.ValidUntil, Origin: release.Origin, Suite: release.Suite}
	if release.ValidUntil != "" {
		if until, err := time.Parse(time.RFC1123, release.ValidUntil); err == nil && repoDescribeNow().After(until) {
			info.State = "expired"
		} else if err != nil {
			if until, err := time.Parse("Mon, 2 Jan 2006 15:04:05 MST", release.ValidUntil); err == nil && repoDescribeNow().After(until) {
				info.State = "expired"
			}
		}
	}
	return info
}

func repoKeyInfo(repo *core.ManagedRepo) *transport.RepoKeyInfo {
	info := &transport.RepoKeyInfo{Fingerprint: repo.KeyFingerprint, State: "unknown"}
	for _, armored := range []bool{true, false} {
		data, err := readSecureRepoFile(repoKeyringPath(repo.ID, armored), repoManagedFileMode)
		if err != nil {
			continue
		}
		created, expires, ok := openPGPKeyValidity(data, armored)
		if !ok {
			return info
		}
		info.CreatedAt = created.UTC().Format(time.RFC3339)
		info.State = repoKeyState(expires, repoDescribeNow())
		if !expires.IsZero() {
			info.ExpiresAt = expires.UTC().Format(time.RFC3339)
		}
		return info
	}
	return info
}

func repoKeyState(expires, now time.Time) string {
	switch {
	case expires.IsZero():
		return "valid"
	case !now.Before(expires):
		return "expired"
	case expires.Sub(now) <= repoKeyExpiringWithin:
		return "expiring"
	default:
		return "valid"
	}
}

// openPGPKeyValidity reads the key's creation time and the moment after which
// it can no longer verify a signature: the latest expiry among the keys that
// may sign (the primary key and its signing subkeys); zero means it does not
// expire.
//
// alpha.108 (owner finding, Boston): a vendor extends its key by issuing a
// NEWER self-signature with a longer lifetime and keeps the older ones in the
// same file. Sury's key carries four (2019, 2021, 2024, 2025), newest first;
// the openpgp package keeps the LAST self-signature it reads and the FIRST
// binding of a subkey, so the card said "expired on 17 March 2021" while apt,
// which uses the newest valid self-signature as gpg does, accepted the
// repository. The packets are therefore read here: for each user ID and each
// subkey the newest signature that verifies against the primary key counts.
//
// alpha.108: üretici anahtarını daha YENİ bir öz-imzayla uzatır; eskiler
// dosyada kalır. openpgp paketi sonuncuyu tuttuğu için kart 2021'de doldu
// diyordu. Burada her kimlik ve alt anahtar için doğrulanan en yeni imza sayılır.
func openPGPKeyValidity(data []byte, armored bool) (created, expires time.Time, ok bool) {
	raw := data
	if armored {
		block, err := armor.Decode(bytes.NewReader(data))
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		raw, err = io.ReadAll(io.LimitReader(block.Body, 1<<20))
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
	}
	var primary *packet.PublicKey
	var uid *packet.UserId
	var sub *packet.PublicKey
	var newestSelf *packet.Signature
	type subkeyBinding struct {
		key *packet.PublicKey
		sig *packet.Signature
	}
	var subkeys []*subkeyBinding
	newer := func(have, candidate *packet.Signature) bool {
		return have == nil || candidate.CreationTime.After(have.CreationTime)
	}
	reader := packet.NewReader(bytes.NewReader(raw))
read:
	for {
		p, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			var unsupported pgperrors.UnsupportedError
			if errors.As(err, &unsupported) {
				continue
			}
			return time.Time{}, time.Time{}, false
		}
		switch pkt := p.(type) {
		case *packet.PublicKey:
			if !pkt.IsSubkey {
				if primary != nil {
					// A second primary key: only the first entity counts.
					break read
				}
				primary = pkt
				continue
			}
			sub, uid = pkt, nil
			subkeys = append(subkeys, &subkeyBinding{key: pkt})
		case *packet.UserId:
			uid, sub = pkt, nil
		case *packet.Signature:
			if primary == nil {
				continue
			}
			switch {
			case sub != nil && pkt.SigType == packet.SigTypeSubkeyBinding:
				current := subkeys[len(subkeys)-1]
				if newer(current.sig, pkt) && primary.VerifyKeySignature(sub, pkt) == nil {
					current.sig = pkt
				}
			case uid != nil && sub == nil && isUserIDCertification(pkt.SigType) &&
				pkt.IssuerKeyId != nil && *pkt.IssuerKeyId == primary.KeyId:
				if newer(newestSelf, pkt) && primary.VerifyUserIdSignature(uid.Id, primary, pkt) == nil {
					newestSelf = pkt
				}
			}
		}
	}
	if primary == nil {
		return time.Time{}, time.Time{}, false
	}
	created = primary.CreationTime
	var signers []time.Time
	never := false
	consider := func(sig *packet.Signature, key *packet.PublicKey) {
		if sig.KeyLifetimeSecs == nil || *sig.KeyLifetimeSecs == 0 {
			never = true
			return
		}
		signers = append(signers, key.CreationTime.Add(time.Duration(*sig.KeyLifetimeSecs)*time.Second))
	}
	if newestSelf != nil && (!newestSelf.FlagsValid || newestSelf.FlagSign) {
		consider(newestSelf, primary)
	}
	for _, binding := range subkeys {
		if binding.sig != nil && binding.sig.FlagsValid && binding.sig.FlagSign {
			consider(binding.sig, binding.key)
		}
	}
	if never || len(signers) == 0 {
		// A primary key whose self-signature carries no lifetime does not
		// expire; with no signing key at all, nothing is claimed beyond it.
		if newestSelf != nil && newestSelf.KeyLifetimeSecs != nil && *newestSelf.KeyLifetimeSecs > 0 && !never {
			return created, created.Add(time.Duration(*newestSelf.KeyLifetimeSecs) * time.Second), true
		}
		return created, time.Time{}, true
	}
	sort.Slice(signers, func(i, j int) bool { return signers[i].After(signers[j]) })
	return created, signers[0], true
}

func isUserIDCertification(t packet.SignatureType) bool {
	return t == packet.SigTypePositiveCert || t == packet.SigTypeGenericCert ||
		t == packet.SigTypePersonaCert || t == packet.SigTypeCasualCert
}

// repoMovingPackages are the installed distribution packages that follow
// whichever version is the default (php-fpm, postgresql), with the exact
// package each depends on today (from dpkg's own record).
func repoMovingPackages(repo *core.ManagedRepo) []transport.RepoMovingPackage {
	pattern := strings.TrimSpace(repo.MovingPackagePattern)
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	file, err := os.Open(dpkgStatusPath)
	if err != nil {
		return nil
	}
	defer file.Close()
	return movingPackagesFromStatus(io.LimitReader(file, repoListsReadCap), re, repo.PackagePattern)
}

func movingPackagesFromStatus(r io.Reader, moving *regexp.Regexp, versionedPattern string) []transport.RepoMovingPackage {
	versioned, _ := regexp.Compile(strings.TrimSpace(versionedPattern))
	versionPrefix := regexp.MustCompile(`^(php[0-9]+\.[0-9]+|postgresql-[0-9]+)`)
	var out []transport.RepoMovingPackage
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var name, status, depends string
	flush := func() {
		if name != "" && moving.MatchString(name) && strings.HasSuffix(status, "install ok installed") {
			item := transport.RepoMovingPackage{Package: name}
			for _, dep := range strings.Split(depends, ",") {
				dep = strings.TrimSpace(dep)
				if i := strings.IndexAny(dep, " (|"); i >= 0 {
					dep = dep[:i]
				}
				if dep == "" {
					continue
				}
				if (versioned != nil && versioned.MatchString(dep)) || versionPrefix.MatchString(dep) {
					item.Exact = dep
					break
				}
			}
			out = append(out, item)
		}
		name, status, depends = "", "", ""
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "Package: "):
			name = strings.TrimSpace(strings.TrimPrefix(line, "Package: "))
		case strings.HasPrefix(line, "Status: "):
			status = strings.TrimSpace(strings.TrimPrefix(line, "Status: "))
		case strings.HasPrefix(line, "Depends: "):
			depends = strings.TrimSpace(strings.TrimPrefix(line, "Depends: "))
		}
	}
	flush()
	sort.Slice(out, func(i, j int) bool { return out[i].Package < out[j].Package })
	return out
}

// ConfineRepoV1 confines a repository that was enabled before alpha.107
// (D-038 §12): the Panel calls it once after it starts on this release, in
// its own repo_confine job, for an enabled repository whose confinement was
// not written. It writes nothing when CelikPanel's file exists (as written or
// edited) or the owner's own preferences file sets a rule for the repository.
// ConfineRepoV1, alpha.107 öncesi açılmış bir depoyu sınırlar.
func (a *Agent) ConfineRepoV1(req *EnableRepoRequest, resp *RepoStatusResponse) error {
	*resp = RepoStatusResponse{}
	if req == nil || !validRepoID.MatchString(req.RepoID) {
		resp.ErrorCode, resp.Error = repoErrInvalidRequest, "invalid repo id"
		return nil
	}
	repo := repoFromCatalogue(req.RepoID)
	if repo == nil {
		resp.ErrorCode, resp.Error = repoErrInvalidRequest, "unknown repository"
		return nil
	}
	_, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepConfineRepo, req.RepoID, "", "confine"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = repoErrEnableFailed, err.Error()
		return nil
	}
	defer finishStep()
	if _, err := verifiedHostProfile("apt"); err != nil {
		resp.ErrorCode, resp.Error = repoErrUnsupportedSystem, err.Error()
		return nil
	}
	err = runRepoPackageMutation(func() error {
		data, err := readSecureRepoFile(repoSourcePath(req.RepoID), repoManagedFileMode)
		if err != nil {
			return err
		}
		resp.Source = strings.TrimSpace(string(data))
		loc, err := parseAptSourceLine(firstLine(resp.Source))
		if err != nil {
			return err
		}
		state := confineRepoLocked(repo, loc, strings.TrimSpace(req.ExpectedSHA256))
		resp.Confinement = &state
		resp.Enabled = true
		return nil
	})
	if err != nil {
		resp.ErrorCode, resp.Error = repoErrConfigurationInvalid, err.Error()
	}
	return nil
}
