package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The vendor lane (alpha.108, D-038 amendment "two lanes"; owner, 2026-10-25:
// "güvenlik ve stabilite her zaman daha önemli"). The default lane is the
// operating system's own packages. A vendor repository is the administrator's
// informed choice and is enabled only as ONE confirmed operation that writes,
// in this order, the confinement (CelikPanel's preferences file), the origin
// in CelikPanel's automatic-updates drop-in and, last, the repository's
// source: the source never exists without its confinement and its patches.
// If anything fails, what this operation wrote for a fresh repository is
// taken back. On a server where the repository is already enabled, the same
// plan names the missing steps and completes them; nothing the owner wrote is
// rewritten, and nothing already in place is switched off.
//
// Satıcı yolu: sınırlama, otomatik güncelleme kaynağı ve en son depo kaynağı
// tek onaylı işlemle yazılır; kaynak, sınırlaması ve yamaları olmadan hiç
// var olmaz. Açık depoda eksik adımlar tamamlanır; sahibin yazdığına dokunulmaz.

const (
	laneStepRepository  = "repository"
	laneStepConfinement = "confinement"
	laneStepUpdates     = "updates"

	laneStateDone    = "done"
	laneStateOwner   = "owner"
	laneStateTodo    = "todo"
	laneStateBlocked = "blocked"

	laneConfinedBy = "lane"

	laneBound = 10 * time.Minute
)

var errLanePlanChanged = errors.New("what would be written is not what was confirmed; nothing was written")

// vendorLaneAptConfig reads apt-config dump (a variable for tests).
var vendorLaneAptConfig = func(ctx context.Context) (map[string]string, map[string][]string, error) {
	if _, err := os.Stat(aptConfigPath); err != nil {
		return nil, nil, err
	}
	out, err := osUpdatesCommand(ctx, packageCommandEnv(), aptConfigPath, "dump")
	if err != nil {
		return nil, nil, err
	}
	scalars, lists := parseAptConfigDump(string(out))
	return scalars, lists, nil
}

// vendorLaneFetchKey downloads and checks the pinned key (a variable for tests).
var vendorLaneFetchKey = fetchRepoKey

// laneRepo is a catalogue repository that is a vendor lane.
func laneRepo(id string) *core.ManagedRepo {
	if !validRepoID.MatchString(id) {
		return nil
	}
	repo := repoFromCatalogue(id)
	if repo == nil || !validReleaseOrigin.MatchString(repo.ReleaseOrigin) {
		return nil
	}
	return repo
}

// laneRepositoryText is what the repository step writes, said once: the
// source line and the one key it trusts.
func laneRepositoryText(source, fingerprint string) string {
	return source + "\n# signed by the pinned OpenPGP key " + fingerprint + "\n"
}

// vendorLanePlan reads, without changing anything, where the lane stands and
// what confirming it would write.
func vendorLanePlan(ctx context.Context, repo *core.ManagedRepo) transport.VendorLanePlan {
	plan := transport.VendorLanePlan{RepoID: repo.ID, Name: repo.Name, Origin: repo.ReleaseOrigin, Steps: []transport.VendorLaneStep{}}
	loc, enabled := enabledRepoLocations()[repo.ID]
	plan.Enabled = enabled
	source, err := repoSourceForHost(repo)
	if err != nil {
		plan.Error = err.Error()
		return plan
	}
	want, err := parseAptSourceLine(source)
	if err != nil {
		plan.Error = err.Error()
		return plan
	}
	// alpha.108 review: the owner's own source for the same address.
	owners := ownerSourcesFor(repo, want)
	ownerSource := false

	// The repository.
	switch {
	case enabled && len(owners) > 0:
		// Two sources for one address: apt refuses updates when their keys
		// differ. The owner removes one; CelikPanel removes nothing.
		plan.Steps = append(plan.Steps, transport.VendorLaneStep{Step: laneStepRepository, State: laneStateBlocked, Reason: "duplicate_source", Path: repoSourcePath(repo.ID), Detail: ownerSourcePaths(owners)})
	case enabled:
		plan.Steps = append(plan.Steps, transport.VendorLaneStep{Step: laneStepRepository, State: laneStateDone, Path: repoSourcePath(repo.ID)})
	case len(owners) > 0:
		// The owner's source stays the only one; the lane offers its
		// confinement and its automatic updates.
		loc, ownerSource = owners[0].Loc, true
		plan.Steps = append(plan.Steps, transport.VendorLaneStep{Step: laneStepRepository, State: laneStateOwner, Reason: "owner_source", Path: owners[0].Path, Detail: ownerSourcePaths(owners)})
	default:
		loc = want
		text := laneRepositoryText(source, repo.KeyFingerprint)
		plan.Steps = append(plan.Steps, transport.VendorLaneStep{Step: laneStepRepository, State: laneStateTodo, Path: repoSourcePath(repo.ID), Text: text, SHA256: sha256Hex([]byte(text))})
	}

	// The confinement.
	confinement := transport.VendorLaneStep{Step: laneStepConfinement, Path: repoPinPath(repo.ID)}
	state := repoConfinementOf(repo, loc)
	switch state.State {
	case repoConfinementConfined:
		confinement.State = laneStateDone
	case repoConfinementOwnerRule:
		confinement.State, confinement.Reason, confinement.Detail = laneStateOwner, repoConfinementOwnerRule, strings.Join(state.OwnerFiles, ", ")
	case repoConfinementEdited:
		confinement.State, confinement.Reason = laneStateOwner, repoConfinementEdited
	default:
		text, kept, err := repoPinPlan(repo, loc)
		switch {
		case err == nil:
			plan.Kept = kept
		case !enabled && !ownerSource:
			// Not enabled and its list never downloaded: nothing can
			// have come from it, so nothing is kept. If the download
			// shows otherwise, the plan has changed and nothing is
			// written.
			text = renderRepoPin(repo, loc.Host, nil)
		default:
			confinement.State, confinement.Reason, confinement.Detail = laneStateBlocked, repoConfinementReasonListsUnreadable, err.Error()
		}
		if confinement.State == "" {
			confinement.State, confinement.Text, confinement.SHA256 = laneStateTodo, string(text), sha256Hex(text)
		}
	}
	plan.Steps = append(plan.Steps, confinement)

	// The operating system's automatic updates.
	plan.Steps = append(plan.Steps, laneUpdatesStep(ctx, repo, loc, enabled || ownerSource))

	todo, blocked := 0, false
	for _, step := range plan.Steps {
		switch step.State {
		case laneStateTodo:
			todo++
		case laneStateBlocked:
			blocked = true
		}
	}
	// A fresh repository is enabled only with all three in place; on an
	// enabled one each missing step that can be written is a gain.
	plan.Ready = todo > 0 && (enabled || ownerSource || !blocked)
	plan.SHA256 = laneSHA(plan.Steps)
	return plan
}

func laneUpdatesStep(ctx context.Context, repo *core.ManagedRepo, loc aptSourceLocation, enabled bool) transport.VendorLaneStep {
	step := transport.VendorLaneStep{Step: laneStepUpdates, Path: vendorOriginsDropInPath}
	if enabled {
		if release, ok := readRepoReleaseInfo(loc); ok && release.Origin != "" && !strings.EqualFold(release.Origin, repo.ReleaseOrigin) {
			step.State, step.Reason, step.Detail = laneStateBlocked, "origin_mismatch", release.Origin
			return step
		}
	}
	scalars, lists, err := vendorLaneAptConfig(ctx)
	if err != nil {
		step.State, step.Reason, step.Detail = laneStateBlocked, "apt_config_unreadable", err.Error()
		return step
	}
	installed, _ := readDpkgStatusVersions(dpkgStatusPath)
	switch {
	case len(installed["unattended-upgrades"]) == 0:
		step.State, step.Reason = laneStateBlocked, "automatic_not_installed"
		return step
	case scalars["APT::Periodic::Unattended-Upgrade"] == "" || scalars["APT::Periodic::Unattended-Upgrade"] == "0":
		step.State, step.Reason = laneStateBlocked, "automatic_off"
		return step
	}
	dropState, current := readVendorOriginsDropIn()
	if unattendedCovers(lists, repo.ReleaseOrigin) {
		step.State = laneStateDone
		if dropState != "present" || !containsFold(current, repo.ReleaseOrigin) {
			step.State, step.Reason = laneStateOwner, "owner_config"
		}
		return step
	}
	if dropState == "edited" {
		step.State, step.Reason, step.Detail = laneStateBlocked, "drop_in_edited", `"origin=`+repo.ReleaseOrigin+`";`
		return step
	}
	text := renderVendorOriginsDropIn(laneOrigins(current, repo.ReleaseOrigin))
	step.State, step.Text, step.SHA256 = laneStateTodo, text, sha256Hex([]byte(text))
	return step
}

func laneOrigins(current []string, origin string) []string {
	set := map[string]bool{origin: true}
	for _, item := range current {
		set[item] = true
	}
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func containsFold(list []string, value string) bool {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

// laneSHA is the digest the owner confirms: every step to be written and
// the digest of its text.
func laneSHA(steps []transport.VendorLaneStep) string {
	var b strings.Builder
	for _, step := range steps {
		if step.State == laneStateTodo {
			b.WriteString(step.Step + " " + step.SHA256 + "\n")
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return sha256Hex([]byte(b.String()))
}

func laneStepOf(plan transport.VendorLanePlan, name string) transport.VendorLaneStep {
	for _, step := range plan.Steps {
		if step.Step == name {
			return step
		}
	}
	return transport.VendorLaneStep{Step: name}
}

// VendorLanePlanV1 answers the confirmation: read only.
func (a *Agent) VendorLanePlanV1(req *EnableRepoRequest, resp *transport.VendorLanePlan) error {
	*resp = transport.VendorLanePlan{Steps: []transport.VendorLaneStep{}}
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	repo := laneRepo(req.RepoID)
	if repo == nil {
		resp.RepoID, resp.Error = req.RepoID, "this repository is not a vendor lane"
		return nil
	}
	if _, err := verifiedHostProfile("apt"); err != nil {
		resp.RepoID, resp.Error = repo.ID, "vendor repositories currently require a verified APT package ecosystem"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), osUpdatesBound)
	defer cancel()
	*resp = vendorLanePlan(ctx, repo)
	return nil
}

// EnableVendorLaneV1 writes the lane the owner confirmed (its SHA-256), in
// its own vendor_lane job, under the package operation lock.
func (a *Agent) EnableVendorLaneV1(req *EnableRepoRequest, resp *transport.VendorLaneResponse) error {
	*resp = transport.VendorLaneResponse{}
	if req == nil || len(strings.TrimSpace(req.ExpectedSHA256)) != 64 {
		resp.ErrorCode, resp.Error = "LANE_INVALID", "the confirmed plan is missing"
		return nil
	}
	repo := laneRepo(req.RepoID)
	if repo == nil {
		resp.ErrorCode, resp.Error = "LANE_INVALID", "this repository is not a vendor lane"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepVendorLane, repo.ID, "", "lane"),
	)
	if err != nil {
		resp.ErrorCode, resp.Error = "LANE_INVALID", err.Error()
		return nil
	}
	defer finishStep()
	if _, err := verifiedHostProfile("apt"); err != nil {
		resp.ErrorCode, resp.Error = "LANE_UNSUPPORTED", err.Error()
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, laneBound)
	defer cancel()
	_ = runRepoPackageMutation(func() error {
		enableVendorLaneLocked(ctx, repo, strings.TrimSpace(req.ExpectedSHA256), resp)
		return nil
	})
	after := vendorLanePlan(ctx, repo)
	resp.Steps, resp.Enabled = after.Steps, after.Enabled
	if _, lists, err := vendorLaneAptConfig(ctx); err == nil {
		resp.Effective = unattendedCovers(lists, repo.ReleaseOrigin)
	}
	return nil
}

func enableVendorLaneLocked(ctx context.Context, repo *core.ManagedRepo, expected string, resp *transport.VendorLaneResponse) {
	plan := vendorLanePlan(ctx, repo)
	switch {
	case plan.Error != "":
		resp.ErrorCode, resp.Error = "LANE_INVALID", plan.Error
		return
	case plan.SHA256 == "" || plan.SHA256 != expected:
		resp.ErrorCode, resp.Error = "LANE_PLAN_CHANGED", errLanePlanChanged.Error()
		return
	case !plan.Ready:
		resp.ErrorCode, resp.Error = "LANE_BLOCKED", "a step of this repository's lane cannot be written; the plan says which"
		return
	}
	if plan.Enabled {
		loc, ok := enabledRepoLocations()[repo.ID]
		if !ok {
			resp.ErrorCode, resp.Error = "LANE_PLAN_CHANGED", errLanePlanChanged.Error()
			return
		}
		completeVendorLane(repo, plan, loc, resp)
		return
	}
	// The owner's own source: never a second one, only its confinement and
	// automatic updates (alpha.108 review).
	if step := laneStepOf(plan, laneStepRepository); step.State == laneStateOwner {
		source, err := repoSourceForHost(repo)
		if err != nil {
			resp.ErrorCode, resp.Error = "LANE_INVALID", err.Error()
			return
		}
		want, err := parseAptSourceLine(source)
		if err != nil {
			resp.ErrorCode, resp.Error = "LANE_INVALID", err.Error()
			return
		}
		owners := ownerSourcesFor(repo, want)
		if len(owners) == 0 {
			resp.ErrorCode, resp.Error = "LANE_PLAN_CHANGED", errLanePlanChanged.Error()
			return
		}
		completeVendorLane(repo, plan, owners[0].Loc, resp)
		return
	}
	freshVendorLane(ctx, repo, plan, resp)
}

// laneUndo takes back what a failed lane wrote, in reverse.
type laneUndo struct {
	pin          string
	dropIn       bool
	dropInBefore []byte
	dropInExists bool
}

func (u *laneUndo) run(repoID string) {
	if u.dropIn {
		if u.dropInExists {
			_ = publishRepoPin(vendorOriginsDropInPath, u.dropInBefore)
		} else {
			_ = os.Remove(vendorOriginsDropInPath)
		}
	}
	if u.pin != "" {
		_ = removeRepoFileVerified(u.pin)
		_ = updateRepoRecord(repoID, func(entry *repoRecordEntry, ok bool) bool {
			entry.PinSHA256, entry.ConfinedAt, entry.ConfinedBy, entry.Kept, entry.KeptTotal = "", "", "", nil, 0
			return ok && (entry.EnabledAt != "" || len(entry.NoLongerUpdated) > 0)
		})
	}
}

// writeLanePin writes the confirmed preferences text and records it.
func writeLanePin(repo *core.ManagedRepo, text []byte, kept []string) error {
	if err := publishRepoPin(repoPinPath(repo.ID), text); err != nil {
		return err
	}
	now := repoConfinementNow().UTC().Format(time.RFC3339)
	shown := kept
	if len(shown) > repoKeptCap {
		shown = shown[:repoKeptCap]
	}
	if err := updateRepoRecord(repo.ID, func(entry *repoRecordEntry, _ bool) bool {
		entry.PinSHA256 = sha256Hex(text)
		entry.ConfinedAt, entry.ConfinedBy = now, laneConfinedBy
		entry.Kept, entry.KeptTotal = shown, len(kept)
		return true
	}); err != nil {
		_ = removeRepoFileVerified(repoPinPath(repo.ID))
		return fmt.Errorf("record the confinement: %w", err)
	}
	return nil
}

// writeLaneDropIn writes the confirmed drop-in text, remembering what was
// there for the undo.
func writeLaneDropIn(text string, undo *laneUndo) error {
	before, err := os.ReadFile(vendorOriginsDropInPath)
	undo.dropInExists = err == nil
	undo.dropInBefore = before
	if err := publishRepoPin(vendorOriginsDropInPath, []byte(text)); err != nil {
		return err
	}
	undo.dropIn = true
	return nil
}

func freshVendorLane(ctx context.Context, repo *core.ManagedRepo, plan transport.VendorLanePlan, resp *transport.VendorLaneResponse) {
	profile, err := verifiedHostProfile("apt")
	if err != nil {
		resp.ErrorCode, resp.Error = "LANE_UNSUPPORTED", err.Error()
		return
	}
	aptGet, err := executableForProfile(profile, "apt", "apt-get")
	if err != nil {
		resp.ErrorCode, resp.Error = "LANE_UNSUPPORTED", err.Error()
		return
	}
	source, err := repoSourceForHost(repo)
	if err != nil {
		resp.ErrorCode, resp.Error = "LANE_INVALID", err.Error()
		return
	}
	loc, err := parseAptSourceLine(source)
	if err != nil {
		resp.ErrorCode, resp.Error = "LANE_INVALID", err.Error()
		return
	}
	key, armored, err := vendorLaneFetchKey(repo.KeyURL, repo.KeyFingerprint)
	if err != nil {
		resp.ErrorCode, resp.Error = "LANE_KEY_UNTRUSTED", err.Error()
		return
	}
	keyring := repoKeyringPath(repo.ID, armored)
	paths := repoRecipePaths{Keyring: keyring, Source: repoSourcePath(repo.ID)}
	for _, candidate := range repoKeyringCandidates(repo.ID) {
		if candidate != keyring {
			paths.StaleKeyrings = append(paths.StaleKeyrings, candidate)
		}
	}
	confinement, updates := laneStepOf(plan, laneStepConfinement), laneStepOf(plan, laneStepUpdates)
	undo := &laneUndo{}
	completed := []string{}
	err = prepareAndPublishRepoRecipeThen(paths, key, source, func(stagedSource string) ([]byte, error) {
		// alpha.108 review: the staged source is read together with every
		// existing source, so a conflict with one of them ("Conflicting
		// values set for option Signed-By") stops the enable before
		// anything is written.
		parts, cleanup, err := stagedSourcePartsDir(stagedSource, repo.ID)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		cmd := serviceMutationCommand(ctx, aptGet, "update",
			"-o", "Dir::Etc::sourceparts="+parts,
			"-o", "APT::Get::List-Cleanup=0")
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		return cmd.CombinedOutput()
	}, func() error {
		// The list just downloaded is the catalogue's repository.
		if release, ok := readRepoReleaseInfo(loc); !ok || !strings.EqualFold(release.Origin, repo.ReleaseOrigin) {
			return fmt.Errorf("the downloaded repository does not name the expected origin %s", repo.ReleaseOrigin)
		}
		if confinement.State == laneStateTodo {
			text, kept, err := repoPinPlan(repo, loc)
			if err != nil {
				return fmt.Errorf("what came from the repository could not be read: %w", err)
			}
			if sha256Hex(text) != confinement.SHA256 {
				return errLanePlanChanged
			}
			if err := writeLanePin(repo, text, kept); err != nil {
				return err
			}
			undo.pin = repoPinPath(repo.ID)
			completed = append(completed, laneStepConfinement)
		}
		if updates.State == laneStateTodo {
			state, current := readVendorOriginsDropIn()
			text := renderVendorOriginsDropIn(laneOrigins(current, repo.ReleaseOrigin))
			if state == "edited" || sha256Hex([]byte(text)) != updates.SHA256 {
				return errLanePlanChanged
			}
			if err := writeLaneDropIn(text, undo); err != nil {
				return err
			}
			completed = append(completed, laneStepUpdates)
		}
		return nil
	})
	if err != nil {
		undo.run(repo.ID)
		resp.ErrorCode, resp.Error = "LANE_ENABLE_FAILED", err.Error()
		if errors.Is(err, errLanePlanChanged) {
			resp.ErrorCode = "LANE_PLAN_CHANGED"
		}
		return
	}
	resp.Completed = append([]string{laneStepRepository}, completed...)
	now := repoConfinementNow().UTC().Format(time.RFC3339)
	_ = updateRepoRecord(repo.ID, func(entry *repoRecordEntry, _ bool) bool {
		if entry.EnabledAt == "" {
			entry.EnabledAt = now
		}
		entry.NoLongerUpdated, entry.DisabledAt = nil, ""
		return true
	})
}

func completeVendorLane(repo *core.ManagedRepo, plan transport.VendorLanePlan, loc aptSourceLocation, resp *transport.VendorLaneResponse) {
	partial := func(code, detail string) {
		resp.ErrorCode, resp.Error = code, detail
		if len(resp.Completed) > 0 {
			resp.ErrorCode = "LANE_PARTIAL"
		}
	}
	if step := laneStepOf(plan, laneStepConfinement); step.State == laneStateTodo {
		state := confineRepoLocked(repo, loc, step.SHA256)
		if state.State != repoConfinementConfined {
			if state.Reason == repoConfinementReasonPlanChanged {
				partial("LANE_PLAN_CHANGED", errLanePlanChanged.Error())
			} else {
				partial("LANE_ENABLE_FAILED", "the confinement could not be written: "+state.Reason+" "+state.Detail)
			}
			return
		}
		resp.Completed = append(resp.Completed, laneStepConfinement)
	}
	if step := laneStepOf(plan, laneStepUpdates); step.State == laneStateTodo {
		state, current := readVendorOriginsDropIn()
		text := renderVendorOriginsDropIn(laneOrigins(current, repo.ReleaseOrigin))
		if state == "edited" || sha256Hex([]byte(text)) != step.SHA256 {
			partial("LANE_PLAN_CHANGED", errLanePlanChanged.Error())
			return
		}
		undo := &laneUndo{}
		if err := writeLaneDropIn(text, undo); err != nil {
			partial("LANE_ENABLE_FAILED", "the automatic updates drop-in could not be written: "+err.Error())
			return
		}
		resp.Completed = append(resp.Completed, laneStepUpdates)
	}
}

// laneNoLongerUpdated reads, for a disabled repository, the installed
// packages that came from it as recorded when it was disabled and still
// installed now.
func laneNoLongerUpdated(repoID string) ([]string, string) {
	entry := readRepoRecord().Repos[repoID]
	if len(entry.NoLongerUpdated) == 0 {
		return nil, ""
	}
	installed, err := readDpkgStatusVersions(dpkgStatusPath)
	if err != nil {
		return entry.NoLongerUpdated, entry.DisabledAt
	}
	var still []string
	for _, name := range entry.NoLongerUpdated {
		if len(installed[name]) > 0 {
			still = append(still, name)
		}
	}
	return still, entry.DisabledAt
}
