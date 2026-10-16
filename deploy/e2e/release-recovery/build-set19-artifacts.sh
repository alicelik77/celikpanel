#!/usr/bin/env bash
# set19: the archives of the Certbot renewal measurement (2026-10-16), built from one exact source commit in a
# disposable clone. The steps of build-set17-artifacts.sh's update mode (that script is not changed):
#
#   B  baseline    the v0.1.0-alpha.90 tag's own commit (1743ac69), unpatched: it carries the D-027 acceptance-
#                  license seam and its guard (refused otherwise); its own policy (90, previous 89), its own web/.
#   G  good        SOURCE + a fixture commit that changes ONLY the release policy to v0.1.0-alpha.92 / 92 with the
#                  alpha.90 tag commit as its predecessor (the source's own policy is 92 after the alpha.91 tag
#                  commit 356109b5; the fixture origin serves exactly one transition from the installed baseline).
#   D  defective   G + the fixture defect of set5 (never installed; the artifacts document needs it).
#
# The fixture driver is set19_fixture.py. Each commit is built with dns-pair-acceptance/scripts/build-dist.sh
# --acceptance-license and CELIKPANEL_DIST_VERSION set to the committed policy's version. The disposable clone lives
# under ${SET19_BUILD_ROOT:-/var/tmp/cp-set19/cp-upd1-build}/<stamp>; build-dist.sh writes its dist under
# /var/tmp/cp-pair-accept/dist/. CELIKPANEL_REPO names the repository to clone (required: a clone made for this run,
# never a working checkout). Output: <stamp>/upd1-artifacts.json. None of these archives is a release.
#
# usage: CELIKPANEL_REPO=/var/tmp/cp-set19/repo build-set19-artifacts.sh SOURCE_COMMIT
set -euo pipefail
shopt -s inherit_errexit
umask 022

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
[[ $# -eq 1 && -n ${CELIKPANEL_REPO:-} ]] || { echo "usage: CELIKPANEL_REPO=REPO build-set19-artifacts.sh SOURCE_COMMIT" >&2; exit 2; }
REPO=$CELIKPANEL_REPO
SOURCE=$1
BASELINE_REF=v0.1.0-alpha.90
BUILD_DIST="$HERE/../dns-pair-acceptance/scripts/build-dist.sh"
DRIVER="$HERE/set19_fixture.py"
GUARD="$HERE/../../../deploy/release-acceptance-license-guard.sh"
GO=${CELIKPANEL_GO:-/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go}
TAG90=1743ac69edb0e5723dc60d992b53094dcc4b1431
TAG91=356109b532b9d0d0df5dd31fc19e9932918a9648
[[ -f $BUILD_DIST && -f $DRIVER ]] || { echo "harness files missing" >&2; exit 2; }
command -v git >/dev/null && command -v python3 >/dev/null || { echo "git and python3 are required" >&2; exit 2; }

source_commit=$(git -c safe.directory='*' -C "$REPO" rev-parse "${SOURCE}^{commit}")
stamp=$(date -u +%Y%m%dt%H%M%Sz)
work=${SET19_BUILD_ROOT:-/var/tmp/cp-set19/cp-upd1-build}/$stamp
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

proof=$work/baseline-ref-proof.txt
tag_commit=$(git -C "$clone" rev-parse "${BASELINE_REF}^{commit}")
[[ $tag_commit == "$TAG90" ]] || { echo "$BASELINE_REF is not 1743ac69" >&2; exit 1; }
for needed in internal/licensing/acceptance_fixture.go internal/licensing/acceptance_off.go \
        deploy/release-acceptance-license-guard.sh; do
    git -C "$clone" cat-file -e "$tag_commit:$needed" \
        || { echo "$BASELINE_REF lacks $needed; it cannot be built unpatched" >&2; exit 1; }
done
[[ "$(git -C "$clone" show "$source_commit:deploy/release-sequence-policy")" == \
   "$(expected_policy v0.1.0-alpha.92 92 91 v0.1.0-alpha.91 "$TAG91")" ]] \
    || { echo "the source's release policy is not v0.1.0-alpha.92 / 92 after the alpha.91 tag; refusing" >&2; exit 1; }
git -C "$clone" merge-base --is-ancestor "$tag_commit" "$source_commit" \
    || { echo "the alpha.90 tag commit is not an ancestor of the source; refusing" >&2; exit 1; }
baseline=$tag_commit
git -C "$clone" update-ref refs/upd1/baseline "$baseline"
build_web "$work/web-dist-candidate"
good=$(commit_fixture good "test(fixture): set19 candidate labelled v0.1.0-alpha.92 after the alpha.90 tag (release policy only; unpublished, disposable)" "$baseline")
[[ "$(git -C "$clone" diff --name-only "$source_commit" "$good")" == deploy/release-sequence-policy ]] \
    || { echo "the good fixture changed more than the release policy" >&2; exit 1; }
[[ "$(git -C "$clone" show "$good:deploy/release-sequence-policy")" == \
   "$(expected_policy v0.1.0-alpha.92 92 90 v0.1.0-alpha.90 "$baseline")" ]] \
    || { echo "the good fixture's policy is not v0.1.0-alpha.92 / 92 after the alpha.90 tag" >&2; exit 1; }
defective=$(commit_fixture defective "test(fixture): set19 defective candidate - migrate-only fails (unpublished, disposable)")
[[ -z $(git -C "$clone" diff --stat "$source_commit" "$defective" -- web) ]] || { echo "fixture commit changed web/" >&2; exit 1; }
{
    echo "baseline-ref $BASELINE_REF tag_commit=$tag_commit baseline=$baseline (the tag commit itself) source=$source_commit good=$good (policy-only fixture over the source) defective=$defective"
    echo "== release policy at the tag"
    git -C "$clone" show "$tag_commit:deploy/release-sequence-policy"
    echo "== release policy at the source (its own; previous_commit is the alpha.91 tag commit)"
    git -C "$clone" show "$source_commit:deploy/release-sequence-policy"
    echo "== release policy of the good fixture (previous_commit is the alpha.90 tag commit)"
    git -C "$clone" show "$good:deploy/release-sequence-policy"
    echo "== the defective fixture commit changes: $(git -C "$clone" diff --name-only "$good" "$defective" | tr '\n' ' ')"
    echo "== git diff --stat $tag_commit $source_commit -- internal/db/migrations (empty = no migration between them):"
    git -C "$clone" diff --stat "$tag_commit" "$source_commit" -- internal/db/migrations
    echo "== (end)"
    git -C "$clone" checkout --quiet --detach "$tag_commit"
    agent_closure
} > "$proof"
[[ $(tail -n 1 "$proof") == 0 ]] || { echo "the Agent package closure includes the licensing seam; see $proof" >&2; exit 1; }
build_web "$work/web-dist-baseline"
git -C "$clone" checkout --quiet --detach "$source_commit"
use_web "$work/web-dist-baseline"
b_json=$(build "$baseline" "$BASELINE_REF")
use_web "$work/web-dist-candidate"
g_json=$(build "$good" v0.1.0-alpha.92)
d_json=$(build "$defective" v0.1.0-alpha.92)

python3 - "$work/upd1-artifacts.json" "$source_commit" "$clone" "$b_json" "$g_json" "$d_json" \
    "$baseline" "$good" "$BASELINE_REF" "$tag_commit" "$proof" <<'PY'
import json, sys
out, head, clone, b, g, d, baseline, good, ref, tag, proof = sys.argv[1:]
def item(path, sequence, parent=None):
    value = json.load(open(path))
    entry = {k: value[k] for k in ("archive", "sha256", "commit", "tree", "version", "product_web_src")}
    entry.update(sequence=sequence, license_mode=value.get("license_mode"), dist_json=path)
    if parent:
        entry["parent"] = parent
    return entry
document = {"schema": "celikpanel/upd1-artifacts/v1", "source_head": head, "clone": clone,
            "baseline": item(b, 90), "good": item(g, 92, baseline), "defective": item(d, 92, good)}
document["defective"]["defect"] = "cmd/panel/main.go --migrate-only exits 1 after migrating the isolated copy"
document["baseline_ref"] = {"ref": ref, "tag_commit": tag, "patched_files": [], "proof": proof}
document["provenance"] = (f"set19: baseline the {ref} tag's own commit ({tag}) unchanged (it carries the acceptance-"
                          f"license seam), rebuilt locally (not the published archive); good: a policy-only fixture "
                          f"commit over the source {head} labelled v0.1.0-alpha.92 / 92 after the tag; defective: an "
                          "unpublished disposable fixture commit over it; acceptance-license panels; signed only by the "
                          "per-lab fixture key at run time; not a release")
with open(out, "x") as handle:
    json.dump(document, handle, indent=2, sort_keys=True)
    handle.write("\n")
print(out)
PY
echo UPD1-ARTIFACTS-OK
