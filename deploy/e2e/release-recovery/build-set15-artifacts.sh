#!/usr/bin/env bash
# set15: the archives of the pre-publication measurement of v0.1.0-alpha.84, built from one exact source commit in a
# disposable clone. The same steps as build-set12-artifacts.sh (that script is not changed), in two modes:
#
#   --baseline-ref v0.1.0-alpha.83 SOURCE   (the update cells)
#     B  baseline    the v0.1.0-alpha.83 tag's own commit (0447558e), unpatched: it carries the D-027 acceptance-
#                    license seam and its guard (refused otherwise); its own policy (83, previous 82), its own web/.
#     G  good        SOURCE ITSELF, no fixture commit: its committed release policy must already be exactly
#                    v0.1.0-alpha.84 / 84, previous 83 = the tag commit (refused otherwise).
#     D  defective   SOURCE + the fixture defect of set5: cmd/panel --migrate-only exits 1 after migrating the
#                    isolated copy (owner_update_trial.apply_defect)
#   --fresh SOURCE                          (the fresh-install cells)
#     B  baseline    SOURCE ITSELF (the build that is installed), labelled by its own policy v0.1.0-alpha.84 / 84
#     G  good        SOURCE + release policy v0.1.0-alpha.85 / 85 (previous = SOURCE): the origin's candidate only,
#                    never installed by a fresh cell
#     D  defective   G + the same fixture defect (never installed; the artifacts document needs it)
#
# The fixture driver is set15_fixture.py. Each commit is built with dns-pair-acceptance/scripts/build-dist.sh
# --acceptance-license and CELIKPANEL_DIST_VERSION set to the committed policy's version. Output:
# <stamp>/upd1-artifacts.json. None of these archives is a release. UPD1_REUSE_DIST_OF_COMMIT, as in
# build-upd1-artifacts.sh, may name one commit whose dist an earlier build left (SOURCE's, for the second mode).
#
# usage: build-set15-artifacts.sh --baseline-ref v0.1.0-alpha.83 SOURCE_COMMIT
#        build-set15-artifacts.sh --fresh SOURCE_COMMIT
set -euo pipefail
shopt -s inherit_errexit
umask 022

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO=${CELIKPANEL_REPO:-'/mnt/c/CELIKBROS PROJECTS/celikpanel'}
usage() { echo "usage: build-set15-artifacts.sh --baseline-ref v0.1.0-alpha.83 SOURCE_COMMIT | --fresh SOURCE_COMMIT" >&2; exit 2; }
case ${1:-} in
    --baseline-ref) [[ ${2:-} == v0.1.0-alpha.83 && $# -eq 3 ]] || usage; MODE=update; BASELINE_REF=$2; SOURCE=$3 ;;
    --fresh) [[ $# -eq 2 ]] || usage; MODE=fresh; BASELINE_REF=v0.1.0-alpha.84; SOURCE=$2 ;;
    *) usage ;;
esac
BUILD_DIST="$HERE/../dns-pair-acceptance/scripts/build-dist.sh"
DRIVER="$HERE/set15_fixture.py"
GUARD="$HERE/../../../deploy/release-acceptance-license-guard.sh"
GO=${CELIKPANEL_GO:-/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go}
[[ -f $BUILD_DIST && -f $DRIVER ]] || { echo "harness files missing" >&2; exit 2; }
command -v git >/dev/null && command -v python3 >/dev/null || { echo "git and python3 are required" >&2; exit 2; }

source_commit=$(git -c safe.directory='*' -C "$REPO" rev-parse "${SOURCE}^{commit}")
stamp=$(date -u +%Y%m%dt%H%M%Sz)
work=/var/tmp/cp-upd1-build/$stamp
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

proof=$work/baseline-ref-proof.txt
if [[ $MODE == update ]]; then
    tag_commit=$(git -C "$clone" rev-parse "${BASELINE_REF}^{commit}")
    [[ $tag_commit == 0447558e956d4c02c7bf06b368def0924856f8fc ]] || { echo "$BASELINE_REF is not 0447558e" >&2; exit 1; }
    for needed in internal/licensing/acceptance_fixture.go internal/licensing/acceptance_off.go \
            deploy/release-acceptance-license-guard.sh; do
        git -C "$clone" cat-file -e "$tag_commit:$needed" \
            || { echo "$BASELINE_REF lacks $needed; it cannot be built unpatched" >&2; exit 1; }
    done
    [[ "$(git -C "$clone" show "$source_commit:deploy/release-sequence-policy")" == \
       "$(expected_policy v0.1.0-alpha.84 84 83 v0.1.0-alpha.83 "$tag_commit")" ]] \
        || { echo "the source's release policy is not v0.1.0-alpha.84 / 84 after $tag_commit; refusing" >&2; exit 1; }
    baseline=$tag_commit
    git -C "$clone" update-ref refs/upd1/baseline "$baseline"
    git -C "$clone" update-ref refs/upd1/good "$source_commit"
    {
        echo "baseline-ref $BASELINE_REF tag_commit=$tag_commit baseline_fixture=$baseline (the tag commit itself) source=$source_commit"
        echo "== release policy at the tag"
        git -C "$clone" show "$tag_commit:deploy/release-sequence-policy"
        echo "== release policy at the source (the good candidate, no fixture commit)"
        git -C "$clone" show "$source_commit:deploy/release-sequence-policy"
        git -C "$clone" checkout --quiet --detach "$tag_commit"
        ( cd "$clone" && GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$GO" list -deps ./cmd/agent ) > "$work/agent-deps.txt"
        echo "== Agent package closure at the baseline (go list -deps ./cmd/agent, $(wc -l < "$work/agent-deps.txt") packages): licensing packages"
        grep -c '/internal/licensing' "$work/agent-deps.txt" || true
    } > "$proof"
    [[ $(tail -n 1 "$proof") == 0 ]] || { echo "the Agent package closure includes the licensing seam; see $proof" >&2; exit 1; }
    build_web "$work/web-dist-baseline"
    git -C "$clone" checkout --quiet --detach "$source_commit"
    build_web "$work/web-dist-candidate"
    good=$source_commit
    defective=$(commit_fixture defective "test(fixture): set15 defective candidate - migrate-only fails (unpublished, disposable)")
    [[ -z $(git -C "$clone" diff --stat "$source_commit" "$defective" -- web) ]] || { echo "fixture commit changed web/" >&2; exit 1; }
    use_web "$work/web-dist-baseline"
    b_json=$(build "$baseline" "$BASELINE_REF")
    use_web "$work/web-dist-candidate"
    g_json=$(build "$good" v0.1.0-alpha.84)
    d_json=$(build "$defective" v0.1.0-alpha.84)
    seq_b=83 seq_c=84 cand_version=v0.1.0-alpha.84
else
    previous_tag=$(git -C "$clone" rev-parse "v0.1.0-alpha.83^{commit}")
    [[ "$(git -C "$clone" show "$source_commit:deploy/release-sequence-policy")" == \
       "$(expected_policy v0.1.0-alpha.84 84 83 v0.1.0-alpha.83 "$previous_tag")" ]] \
        || { echo "the source's release policy is not v0.1.0-alpha.84 / 84 after alpha.83; refusing" >&2; exit 1; }
    tag_commit=$source_commit
    baseline=$source_commit
    git -C "$clone" update-ref refs/upd1/baseline "$baseline"
    {
        echo "fresh: baseline = the source commit itself $source_commit (its own policy, below); not a tag"
        git -C "$clone" show "$source_commit:deploy/release-sequence-policy"
        ( cd "$clone" && GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$GO" list -deps ./cmd/agent ) > "$work/agent-deps.txt"
        echo "== Agent package closure at the baseline (go list -deps ./cmd/agent, $(wc -l < "$work/agent-deps.txt") packages): licensing packages"
        grep -c '/internal/licensing' "$work/agent-deps.txt" || true
    } > "$proof"
    [[ $(tail -n 1 "$proof") == 0 ]] || { echo "the Agent package closure includes the licensing seam; see $proof" >&2; exit 1; }
    build_web "$work/web-dist-candidate"
    good=$(commit_fixture good "test(fixture): set15 origin candidate labelled v0.1.0-alpha.85 (unpublished, disposable)" "$baseline")
    defective=$(commit_fixture defective "test(fixture): set15 defective over the alpha.85 label - migrate-only fails (unpublished, disposable)")
    [[ -z $(git -C "$clone" diff --stat "$source_commit" "$defective" -- web) ]] || { echo "fixture commit changed web/" >&2; exit 1; }
    use_web "$work/web-dist-candidate"
    b_json=$(build "$baseline" v0.1.0-alpha.84)
    g_json=$(build "$good" v0.1.0-alpha.85)
    d_json=$(build "$defective" v0.1.0-alpha.85)
    seq_b=84 seq_c=85 cand_version=v0.1.0-alpha.85
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
    document["provenance"] = (f"set15 update: baseline the {ref} tag's own commit ({tag}) unchanged (it carries the "
                              f"acceptance-license seam); good: the source commit {head} itself, labelled {cand} by its "
                              "own policy; defective: an unpublished disposable fixture commit over it; acceptance-license "
                              "panels; signed only by the per-lab fixture key at run time; not a release")
else:
    document["provenance"] = (f"set15 fresh: baseline (installed) the source commit {head} itself labelled {ref} by its own "
                              f"policy (the profile name is a label, not a published tag); good/defective: unpublished "
                              f"disposable fixture commits labelled {cand}, served by the origin only; acceptance-license "
                              "panels; not a release")
with open(out, "x") as handle:
    json.dump(document, handle, indent=2, sort_keys=True)
    handle.write("\n")
print(out)
PY
echo UPD1-ARTIFACTS-OK
