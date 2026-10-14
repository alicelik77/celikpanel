#!/usr/bin/env bash
# set16: the archives of the pre-publication measurement of the v0.1.0-alpha.85 candidate, built from one exact source
# commit in a disposable clone. The same steps as build-set15-artifacts.sh (that script is not changed), in two modes:
#
#   --baseline-ref v0.1.0-alpha.84 SOURCE   (the update cells)
#     B  baseline    the v0.1.0-alpha.84 tag's own commit (f0157e6c), unpatched: it carries the D-027 acceptance-
#                    license seam and its guard (refused otherwise); its own policy (84, previous 83), its own web/.
#     G  good        SOURCE + a fixture commit that changes ONLY deploy/release-sequence-policy to v0.1.0-alpha.85 / 85,
#                    previous 84 = the tag commit (the source's own policy still reads alpha.84; refused unless it does)
#     D  defective   G + the fixture defect of set5: cmd/panel --migrate-only exits 1 after migrating the isolated copy
#   --fresh UPDATE_ARTIFACTS_JSON           (the fresh-install cell; run after the first mode)
#     B  baseline    the update document's G commit itself (fetched from that build's clone; its dist is reused by
#                    name), labelled by its own policy v0.1.0-alpha.85 / 85: the installed candidate is one archive
#     G  good        B + release policy v0.1.0-alpha.86 / 86 (previous = B): the origin's candidate only, never installed
#     D  defective   G + the same fixture defect (never installed; the artifacts document needs it)
#
# The fixture driver is set16_fixture.py. Each commit is built with dns-pair-acceptance/scripts/build-dist.sh
# --acceptance-license and CELIKPANEL_DIST_VERSION set to the committed policy's version. The disposable clone lives
# under ${SET16_BUILD_ROOT:-/var/tmp/cp-set16/cp-upd1-build}/<stamp> (the fixture driver edits only a path that
# contains "cp-upd1-build"); build-dist.sh writes its dist under /var/tmp/cp-pair-accept/dist/ as before. Output:
# <stamp>/upd1-artifacts.json. None of these archives is a release.
#
# usage: build-set16-artifacts.sh --baseline-ref v0.1.0-alpha.84 SOURCE_COMMIT
#        build-set16-artifacts.sh --fresh UPDATE_ARTIFACTS_JSON
set -euo pipefail
shopt -s inherit_errexit
umask 022

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO=${CELIKPANEL_REPO:-'/mnt/c/CELIKBROS PROJECTS/celikpanel'}
usage() { echo "usage: build-set16-artifacts.sh --baseline-ref v0.1.0-alpha.84 SOURCE_COMMIT | --fresh UPDATE_ARTIFACTS_JSON" >&2; exit 2; }
case ${1:-} in
    --baseline-ref) [[ ${2:-} == v0.1.0-alpha.84 && $# -eq 3 ]] || usage; MODE=update; BASELINE_REF=$2; SOURCE=$3 ;;
    --fresh) [[ $# -eq 2 && -s ${2:-} ]] || usage; MODE=fresh; BASELINE_REF=v0.1.0-alpha.85; UPDATE_DOC=$2 ;;
    *) usage ;;
esac
BUILD_DIST="$HERE/../dns-pair-acceptance/scripts/build-dist.sh"
DRIVER="$HERE/set16_fixture.py"
GUARD="$HERE/../../../deploy/release-acceptance-license-guard.sh"
GO=${CELIKPANEL_GO:-/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go}
TAG84=f0157e6c8f1614cff176821172f8a9067493e625
[[ -f $BUILD_DIST && -f $DRIVER ]] || { echo "harness files missing" >&2; exit 2; }
command -v git >/dev/null && command -v python3 >/dev/null || { echo "git and python3 are required" >&2; exit 2; }

if [[ $MODE == fresh ]]; then
    read -r SOURCE update_clone installed < <(python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1]));
assert d["baseline_ref"]["ref"] == "v0.1.0-alpha.84", "not an update document"
print(d["source_head"], d["clone"], d["good"]["commit"])' "$UPDATE_DOC")
fi
source_commit=$(git -c safe.directory='*' -C "$REPO" rev-parse "${SOURCE}^{commit}")
stamp=$(date -u +%Y%m%dt%H%M%Sz)
work=${SET16_BUILD_ROOT:-/var/tmp/cp-set16/cp-upd1-build}/$stamp
[[ ! -e $work ]] || { echo "refusing to reuse $work" >&2; exit 2; }
mkdir -p "$work"
clone=$work/repo
git -c safe.directory='*' clone --quiet --no-hardlinks "$REPO" "$clone"
git -C "$clone" checkout --quiet --detach "$source_commit"
git -C "$clone" config user.name "upd1 disposable fixture"
git -C "$clone" config user.email "upd1-fixture@example.invalid"

commit_fixture() {
    local kind=$1 message=$2 previous=${3:-} commit
    python3 "$DRIVER" fixture-source --repo "$clone" --kind "$kind" ${previous:+--previous-commit "$previous"} \
        --baseline-ref "$BASELINE_REF" >&2
    git -C "$clone" commit --quiet --allow-empty -am "$message"
    commit=$(git -C "$clone" rev-parse HEAD)
    git -C "$clone" update-ref "refs/upd1/$kind" "$commit"
    echo "$commit"
}

build_web() {   # build web/ of the checked-out tree into $1
    ( cd "$clone/web" && rm -rf dist && npm ci --no-audit --no-fund && npm run build ) >&2
    [[ -f $clone/web/dist/index.html ]] || { echo "web build produced no index.html" >&2; exit 1; }
    rm -rf -- "$1"
    cp -a "$clone/web/dist" "$1"
}

use_web() {
    rm -rf -- "$clone/web/dist"
    cp -a "$1" "$clone/web/dist"
}

build() {
    local commit=$1 version=$2 dist_json
    dist_json=/var/tmp/cp-pair-accept/dist/$commit-acceptance-license/dist.json
    if [[ -n ${UPD1_REUSE_DIST_OF_COMMIT:-} && ${UPD1_REUSE_DIST_OF_COMMIT} == "$commit" && -s $dist_json ]]; then
        echo "BUILD-UPD1-REUSED: the dist of $commit ($version) at ${dist_json%/dist.json}, left by an earlier build, is" \
            "used unchanged because UPD1_REUSE_DIST_OF_COMMIT names exactly this commit; it was NOT built by this run." >&2
        echo "$dist_json"
        return 0
    fi
    if ! CELIKPANEL_REPO=$clone CELIKPANEL_DIST_VERSION=$version CELIKPANEL_ACCEPTANCE_GUARD=$GUARD \
        bash "$BUILD_DIST" --acceptance-license "$commit" >&2; then
        echo "BUILD-UPD1-FAILED: the dist build of $commit ($version) failed or was refused." >&2
        return 1
    fi
    [[ -s $dist_json ]] || { echo "BUILD-UPD1-FAILED: the dist build of $commit ($version) wrote no $dist_json" >&2; return 1; }
    echo "$dist_json"
}

expected_policy() {   # version current previous previous_version previous_commit
    printf 'format=celikpanel-release-sequence-policy-v1\nversion=%s\ncurrent=%s\nprevious=%s\nprevious_version=%s\nprevious_commit=%s\n' "$@"
}

agent_closure() {   # the Agent package closure of the checked-out tree must not include the licensing seam
    ( cd "$clone" && GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$GO" list -deps ./cmd/agent ) > "$work/agent-deps.txt"
    echo "== Agent package closure at the baseline (go list -deps ./cmd/agent, $(wc -l < "$work/agent-deps.txt") packages): licensing packages"
    grep -c '/internal/licensing' "$work/agent-deps.txt" || true
}

only_policy_changed() {   # $1 $2: the two commits differ in deploy/release-sequence-policy only
    [[ "$(git -C "$clone" diff --name-only "$1" "$2")" == deploy/release-sequence-policy ]]
}

proof=$work/baseline-ref-proof.txt
if [[ $MODE == update ]]; then
    tag_commit=$(git -C "$clone" rev-parse "${BASELINE_REF}^{commit}")
    [[ $tag_commit == "$TAG84" ]] || { echo "$BASELINE_REF is not f0157e6c" >&2; exit 1; }
    for needed in internal/licensing/acceptance_fixture.go internal/licensing/acceptance_off.go \
            deploy/release-acceptance-license-guard.sh; do
        git -C "$clone" cat-file -e "$tag_commit:$needed" \
            || { echo "$BASELINE_REF lacks $needed; it cannot be built unpatched" >&2; exit 1; }
    done
    [[ "$(git -C "$clone" show "$source_commit:deploy/release-sequence-policy")" == \
       "$(git -C "$clone" show "$tag_commit:deploy/release-sequence-policy")" ]] \
        || { echo "the source's release policy is not the alpha.84 tag's; this script labels it alpha.85 itself; refusing" >&2; exit 1; }
    baseline=$tag_commit
    git -C "$clone" update-ref refs/upd1/baseline "$baseline"
    build_web "$work/web-dist-candidate"
    good=$(commit_fixture good "test(fixture): set16 candidate labelled v0.1.0-alpha.85 (release policy only; unpublished, disposable)" "$baseline")
    only_policy_changed "$source_commit" "$good" || { echo "the good fixture changed more than the release policy" >&2; exit 1; }
    [[ "$(git -C "$clone" show "$good:deploy/release-sequence-policy")" == \
       "$(expected_policy v0.1.0-alpha.85 85 84 v0.1.0-alpha.84 "$tag_commit")" ]] \
        || { echo "the good fixture's policy is not v0.1.0-alpha.85 / 85 after the tag" >&2; exit 1; }
    defective=$(commit_fixture defective "test(fixture): set16 defective candidate - migrate-only fails (unpublished, disposable)")
    [[ -z $(git -C "$clone" diff --stat "$source_commit" "$defective" -- web) ]] || { echo "fixture commit changed web/" >&2; exit 1; }
    {
        echo "baseline-ref $BASELINE_REF tag_commit=$tag_commit baseline_fixture=$baseline (the tag commit itself) source=$source_commit good=$good defective=$defective"
        echo "== release policy at the tag"
        git -C "$clone" show "$tag_commit:deploy/release-sequence-policy"
        echo "== release policy at the source (unchanged since the tag; the source is not labelled by itself)"
        git -C "$clone" show "$source_commit:deploy/release-sequence-policy"
        echo "== release policy at the good fixture commit (git diff --name-only source good: $(git -C "$clone" diff --name-only "$source_commit" "$good" | tr '\n' ' '))"
        git -C "$clone" show "$good:deploy/release-sequence-policy"
        echo "== the defective fixture commit changes: $(git -C "$clone" diff --name-only "$good" "$defective" | tr '\n' ' ')"
        git -C "$clone" checkout --quiet --detach "$tag_commit"
        agent_closure
    } > "$proof"
    [[ $(tail -n 1 "$proof") == 0 ]] || { echo "the Agent package closure includes the licensing seam; see $proof" >&2; exit 1; }
    build_web "$work/web-dist-baseline"
    git -C "$clone" checkout --quiet --detach "$source_commit"
    use_web "$work/web-dist-baseline"
    b_json=$(build "$baseline" "$BASELINE_REF")
    use_web "$work/web-dist-candidate"
    g_json=$(build "$good" v0.1.0-alpha.85)
    d_json=$(build "$defective" v0.1.0-alpha.85)
    seq_b=84 seq_c=85 cand_version=v0.1.0-alpha.85
else
    git -C "$clone" fetch --quiet "$update_clone" refs/upd1/good:refs/upd1/installed
    [[ $(git -C "$clone" rev-parse refs/upd1/installed) == "$installed" ]] || { echo "the update clone's good ref is not $installed" >&2; exit 1; }
    only_policy_changed "$source_commit" "$installed" || { echo "the installed commit differs from the source in more than the policy" >&2; exit 1; }
    tag_commit=$installed
    baseline=$installed
    git -C "$clone" checkout --quiet --detach "$baseline"
    git -C "$clone" update-ref refs/upd1/baseline "$baseline"
    {
        echo "fresh: baseline = the update document's good commit $baseline itself (source $source_commit + its release policy, below); not a tag"
        git -C "$clone" show "$baseline:deploy/release-sequence-policy"
        echo "== git diff --name-only source baseline: $(git -C "$clone" diff --name-only "$source_commit" "$baseline" | tr '\n' ' ')"
        agent_closure
    } > "$proof"
    [[ $(tail -n 1 "$proof") == 0 ]] || { echo "the Agent package closure includes the licensing seam; see $proof" >&2; exit 1; }
    build_web "$work/web-dist-candidate"
    good=$(commit_fixture good "test(fixture): set16 origin candidate labelled v0.1.0-alpha.86 (unpublished, disposable)" "$baseline")
    defective=$(commit_fixture defective "test(fixture): set16 defective over the alpha.86 label - migrate-only fails (unpublished, disposable)")
    [[ -z $(git -C "$clone" diff --stat "$source_commit" "$defective" -- web) ]] || { echo "fixture commit changed web/" >&2; exit 1; }
    use_web "$work/web-dist-candidate"
    b_json=$(build "$baseline" v0.1.0-alpha.85)
    g_json=$(build "$good" v0.1.0-alpha.86)
    d_json=$(build "$defective" v0.1.0-alpha.86)
    seq_b=85 seq_c=86 cand_version=v0.1.0-alpha.86
fi

python3 - "$work/upd1-artifacts.json" "$source_commit" "$clone" "$b_json" "$g_json" "$d_json" \
    "$baseline" "$good" "$BASELINE_REF" "$tag_commit" "$proof" "$seq_b" "$seq_c" "$cand_version" "$MODE" <<'PY'
import json, os, sys
out, head, clone, b, g, d, baseline, good, ref, tag, proof, seq_b, seq_c, cand, mode = sys.argv[1:]
def item(path, sequence, parent=None):
    value = json.load(open(path))
    entry = {k: value[k] for k in ("archive", "sha256", "commit", "tree", "version", "product_web_src")}
    entry.update(sequence=sequence, license_mode=value.get("license_mode"), dist_json=path)
    if parent:
        entry["parent"] = parent
    return entry
document = {"schema": "celikpanel/upd1-artifacts/v1", "source_head": head, "clone": clone,
            "baseline": item(b, int(seq_b)), "good": item(g, int(seq_c), baseline), "defective": item(d, int(seq_c), good)}
document["defective"]["defect"] = "cmd/panel --migrate-only exits 1 after migrating the isolated copy"
for role in ("baseline", "good", "defective"):
    if os.environ.get("UPD1_REUSE_DIST_OF_COMMIT") == document[role]["commit"]:
        document[role]["reused_dist"] = "built by an earlier run, used unchanged (UPD1_REUSE_DIST_OF_COMMIT)"
document["baseline_ref"] = {"ref": ref, "tag_commit": tag, "patched_files": [], "proof": proof}
if mode == "update":
    document["provenance"] = (f"set16 update: baseline the {ref} tag's own commit ({tag}) unchanged (it carries the "
                              f"acceptance-license seam), rebuilt locally (not the published archive); good: a disposable "
                              f"fixture commit over the source {head} that changes only the release policy, labelled {cand}; "
                              "defective: an unpublished disposable fixture commit over it; acceptance-license panels; "
                              "signed only by the per-lab fixture key at run time; not a release")
else:
    document["provenance"] = (f"set16 fresh: baseline (installed) the update document's good commit {baseline} labelled "
                              f"{ref} by its own policy (source {head} + the policy; the profile name is a label, not a "
                              f"published tag); good/defective: unpublished disposable fixture commits labelled {cand}, "
                              "served by the origin only; acceptance-license panels; not a release")
with open(out, "x") as handle:
    json.dump(document, handle, indent=2, sort_keys=True)
    handle.write("\n")
print(out)
PY
echo UPD1-ARTIFACTS-OK
