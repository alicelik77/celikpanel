#!/usr/bin/env bash
# set12: the archives of the update from the PUBLISHED v0.1.0-alpha.82, built from one exact source commit in a
# disposable clone. The same steps as build-upd1-artifacts.sh's --baseline-ref mode for v0.1.0-alpha.81 (that script
# is not changed; it accepts alpha.80 and alpha.81 only), with:
#
#   B  baseline    the published v0.1.0-alpha.82 tag's own commit (2a0af886), unpatched: it already carries the
#                  D-027 acceptance-license seam and its guard (refused otherwise); its own release policy
#                  (v0.1.0-alpha.82 / 82, previous 81) and its own web/ build.
#   G  good        SOURCE + release policy v0.1.0-alpha.83 / 83 (previous = B)
#   D  defective   G + the fixture defect of set5: cmd/panel --migrate-only exits 1 after migrating the isolated
#                  copy (owner_update_trial.apply_defect)
#
# The fixture driver is set12_fixture.py (owner_update_trial with the alpha.82 baseline profile added in its own
# process). Each commit is built with dns-pair-acceptance/scripts/build-dist.sh --acceptance-license and
# CELIKPANEL_DIST_VERSION set to the committed policy's version. Output: <stamp>/upd1-artifacts.json. None of these
# archives is a release. UPD1_REUSE_DIST_OF_COMMIT, as in build-upd1-artifacts.sh, may name the baseline's commit.
#
# usage: build-set12-artifacts.sh --baseline-ref v0.1.0-alpha.82 [SOURCE_COMMIT]
set -euo pipefail
shopt -s inherit_errexit
umask 022

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO=${CELIKPANEL_REPO:-'/mnt/c/CELIKBROS PROJECTS/celikpanel'}
[[ ${1:-} == --baseline-ref && ${2:-} == v0.1.0-alpha.82 ]] \
    || { echo "usage: build-set12-artifacts.sh --baseline-ref v0.1.0-alpha.82 [SOURCE_COMMIT]" >&2; exit 2; }
BASELINE_REF=$2
shift 2
SOURCE=${1:-HEAD}
BUILD_DIST="$HERE/../dns-pair-acceptance/scripts/build-dist.sh"
DRIVER="$HERE/set12_fixture.py"
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

tag_commit=$(git -C "$clone" rev-parse "${BASELINE_REF}^{commit}")
git -C "$clone" checkout --quiet --detach "$tag_commit"
for needed in internal/licensing/acceptance_fixture.go internal/licensing/acceptance_off.go \
        deploy/release-acceptance-license-guard.sh; do
    git -C "$clone" cat-file -e "$tag_commit:$needed" \
        || { echo "$BASELINE_REF lacks $needed; it cannot be built unpatched" >&2; exit 1; }
done
baseline=$tag_commit
git -C "$clone" update-ref refs/upd1/baseline "$baseline"
proof=$work/baseline-ref-proof.txt
{
    echo "baseline-ref $BASELINE_REF tag_commit=$tag_commit baseline_fixture=$baseline (the tag commit itself) source=$source_commit"
    echo "== release policy at the tag"
    git -C "$clone" show "$tag_commit:deploy/release-sequence-policy"
    ( cd "$clone" && GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$GO" list -deps ./cmd/agent ) > "$work/agent-deps.txt"
    echo "== Agent package closure at the baseline (go list -deps ./cmd/agent, $(wc -l < "$work/agent-deps.txt") packages): licensing packages"
    grep -c '/internal/licensing' "$work/agent-deps.txt" || true
} > "$proof"
[[ $(tail -n 1 "$proof") == 0 ]] || { echo "the Agent package closure includes the licensing seam; see $proof" >&2; exit 1; }
build_web "$work/web-dist-baseline"
git -C "$clone" checkout --quiet --detach "$source_commit"
good=$(commit_fixture good "test(fixture): set12 good candidate labelled as the release after the published $BASELINE_REF (unpublished, disposable)" "$baseline")
defective=$(commit_fixture defective "test(fixture): set12 defective candidate - migrate-only fails (unpublished, disposable)")
build_web "$work/web-dist-candidate"
[[ -z $(git -C "$clone" diff --stat "$source_commit" "$defective" -- web) ]] || { echo "fixture commit changed web/" >&2; exit 1; }
use_web "$work/web-dist-baseline"
b_json=$(build "$baseline" "$BASELINE_REF")
use_web "$work/web-dist-candidate"
g_json=$(build "$good" v0.1.0-alpha.83)
d_json=$(build "$defective" v0.1.0-alpha.83)

python3 - "$work/upd1-artifacts.json" "$source_commit" "$clone" "$b_json" "$g_json" "$d_json" \
    "$baseline" "$good" "$BASELINE_REF" "$tag_commit" "$proof" <<'PY'
import json, os, sys
out, head, clone, b, g, d, baseline, good, ref, tag, proof = sys.argv[1:]
def item(path, sequence, parent=None):
    value = json.load(open(path))
    entry = {k: value[k] for k in ("archive", "sha256", "commit", "tree", "version", "product_web_src")}
    entry.update(sequence=sequence, license_mode=value.get("license_mode"), dist_json=path)
    if parent:
        entry["parent"] = parent
    return entry
document = {"schema": "celikpanel/upd1-artifacts/v1", "source_head": head, "clone": clone,
            "baseline": item(b, 82), "good": item(g, 83, baseline), "defective": item(d, 83, good)}
document["defective"]["defect"] = "cmd/panel --migrate-only exits 1 after migrating the isolated copy"
for role in ("baseline", "good", "defective"):
    if os.environ.get("UPD1_REUSE_DIST_OF_COMMIT") == document[role]["commit"]:
        document[role]["reused_dist"] = "built by an earlier run, used unchanged (UPD1_REUSE_DIST_OF_COMMIT)"
document["baseline_ref"] = {"ref": ref, "tag_commit": tag, "patched_files": [], "proof": proof}
document["provenance"] = (f"baseline: the published {ref} tree ({tag}) unchanged (the tag commit itself; it carries "
                          "the acceptance-license seam); candidates: unpublished disposable fixture commits over the "
                          "source labelled v0.1.0-alpha.83; acceptance-license panels; signed only by the per-lab "
                          "fixture key at run time; not a release")
with open(out, "x") as handle:
    json.dump(document, handle, indent=2, sort_keys=True)
    handle.write("\n")
print(out)
PY
echo UPD1-ARTIFACTS-OK
