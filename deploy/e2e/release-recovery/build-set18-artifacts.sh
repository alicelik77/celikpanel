#!/usr/bin/env bash
# set18: the archives of the twenty-first native record (the alpha.87 candidate installed fresh and set up to ready),
# built from one exact source commit in a disposable clone, with build-set17-artifacts.sh's steps (that script is not
# changed):
#
#   build-set18-artifacts.sh SOURCE
#     B  baseline    SOURCE + a fixture commit that changes ONLY the release policy to v0.1.0-alpha.87 / 87, previous
#                    86 = the v0.1.0-alpha.86 tag commit (8d260f31): the installed candidate
#     G  good        B + a fixture commit that changes ONLY the release policy to v0.1.0-alpha.88 / 88 (previous = B):
#                    the origin's candidate; never installed (set18 starts no update)
#     D  defective   G + set5's fixture defect (never installed; the artifacts document needs it)
#
# The fixture driver is set18_fixture.py. Each commit is built with dns-pair-acceptance/scripts/build-dist.sh
# --acceptance-license and CELIKPANEL_DIST_VERSION set to the committed policy's version. The disposable clone lives
# under ${SET18_BUILD_ROOT:-/var/tmp/cp-set18/cp-upd1-build}/<stamp>; build-dist.sh writes its dist under
# /var/tmp/cp-pair-accept/dist/ as before. Output: <stamp>/upd1-artifacts.json. None of these archives is a release.
set -euo pipefail
shopt -s inherit_errexit
umask 022

HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO=${CELIKPANEL_REPO:-'/mnt/c/CELIKBROS PROJECTS/celikpanel'}
usage() { echo "usage: build-set18-artifacts.sh SOURCE_COMMIT" >&2; exit 2; }
[[ $# -eq 1 ]] || usage
SOURCE=$1
BUILD_DIST="$HERE/../dns-pair-acceptance/scripts/build-dist.sh"
DRIVER="$HERE/set18_fixture.py"
GUARD="$HERE/../../../deploy/release-acceptance-license-guard.sh"
GO=${CELIKPANEL_GO:-/opt/celikpanel-test-toolchains/go1.26.5/go/bin/go}
TAG86=8d260f317e58bed7fcd84c6cbb01823c4fe3f166
[[ -f $BUILD_DIST && -f $DRIVER ]] || { echo "harness files missing" >&2; exit 2; }

source_commit=$(git -c safe.directory='*' -C "$REPO" rev-parse "${SOURCE}^{commit}")
stamp=$(date -u +%Y%m%dt%H%M%Sz)
work=${SET18_BUILD_ROOT:-/var/tmp/cp-set18/cp-upd1-build}/$stamp
[[ ! -e $work ]] || { echo "refusing to reuse $work" >&2; exit 2; }
mkdir -p "$work"
clone=$work/repo
git -c safe.directory='*' clone --quiet --no-hardlinks "$REPO" "$clone"
git -C "$clone" checkout --quiet --detach "$source_commit"
git -C "$clone" config user.name "upd1 disposable fixture"
git -C "$clone" config user.email "upd1-fixture@example.invalid"

commit_fixture() {   # kind message baseline_ref [previous]
    local kind=$1 message=$2 ref=$3 previous=${4:-} commit
    python3 "$DRIVER" fixture-source --repo "$clone" --kind "$kind" ${previous:+--previous-commit "$previous"} \
        --baseline-ref "$ref" >&2
    git -C "$clone" commit --quiet --allow-empty -am "$message"
    commit=$(git -C "$clone" rev-parse HEAD)
    git -C "$clone" update-ref "refs/upd1/$kind" "$commit"
    echo "$commit"
}

build_web() {
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
        echo "BUILD-SET18-FAILED: the dist build of $commit ($version) failed or was refused." >&2
        return 1
    fi
    [[ -s $dist_json ]] || { echo "BUILD-SET18-FAILED: the dist build of $commit ($version) wrote no $dist_json" >&2; return 1; }
    echo "$dist_json"
}

expected_policy() {   # version current previous previous_version previous_commit
    printf 'format=celikpanel-release-sequence-policy-v1\nversion=%s\ncurrent=%s\nprevious=%s\nprevious_version=%s\nprevious_commit=%s\n' "$@"
}

only_policy_changed() {
    [[ "$(git -C "$clone" diff --name-only "$1" "$2")" == deploy/release-sequence-policy ]]
}

git -C "$clone" merge-base --is-ancestor "$TAG86" "$source_commit" \
    || { echo "the v0.1.0-alpha.86 tag commit is not an ancestor of the source; refusing" >&2; exit 1; }
[[ $(git -C "$clone" rev-parse 'v0.1.0-alpha.86^{commit}') == "$TAG86" ]] || { echo "v0.1.0-alpha.86 is not 8d260f31" >&2; exit 1; }
build_web "$work/web-dist-candidate"
# The installed candidate: SOURCE + policy 87 (previous 86 = the alpha.86 tag commit), through set17's alpha.86 label
# profile, whose candidate is v0.1.0-alpha.87 / 87 after v0.1.0-alpha.86 / 86.
baseline=$(commit_fixture good "test(fixture): set18 candidate labelled v0.1.0-alpha.87 (release policy only; unpublished, disposable)" v0.1.0-alpha.86 "$TAG86")
git -C "$clone" update-ref refs/upd1/baseline "$baseline"
only_policy_changed "$source_commit" "$baseline" || { echo "the baseline fixture changed more than the release policy" >&2; exit 1; }
[[ "$(git -C "$clone" show "$baseline:deploy/release-sequence-policy")" == \
   "$(expected_policy v0.1.0-alpha.87 87 86 v0.1.0-alpha.86 "$TAG86")" ]] \
    || { echo "the baseline fixture's policy is not v0.1.0-alpha.87 / 87 after the alpha.86 tag" >&2; exit 1; }
good=$(commit_fixture good "test(fixture): set18 origin candidate labelled v0.1.0-alpha.88 (release policy only; never installed)" v0.1.0-alpha.87 "$baseline")
only_policy_changed "$baseline" "$good" || { echo "the good fixture changed more than the release policy" >&2; exit 1; }
[[ "$(git -C "$clone" show "$good:deploy/release-sequence-policy")" == \
   "$(expected_policy v0.1.0-alpha.88 88 87 v0.1.0-alpha.87 "$baseline")" ]] \
    || { echo "the good fixture's policy is not v0.1.0-alpha.88 / 88 after the baseline" >&2; exit 1; }
defective=$(commit_fixture defective "test(fixture): set18 defective over the alpha.88 label - migrate-only fails (never installed)" v0.1.0-alpha.87)
[[ -z $(git -C "$clone" diff --stat "$source_commit" "$defective" -- web) ]] || { echo "fixture commit changed web/" >&2; exit 1; }
proof=$work/baseline-ref-proof.txt
{
    echo "set18: source=$source_commit baseline=$baseline (source + policy 87) good=$good (policy 88) defective=$defective"
    echo "== release policy at the source"; git -C "$clone" show "$source_commit:deploy/release-sequence-policy"
    echo "== release policy at the baseline (installed)"; git -C "$clone" show "$baseline:deploy/release-sequence-policy"
    echo "== release policy at the good fixture (never installed)"; git -C "$clone" show "$good:deploy/release-sequence-policy"
    echo "== git diff --stat $TAG86 $source_commit -- internal/db/migrations (empty = no migration since the alpha.86 tag):"
    git -C "$clone" diff --stat "$TAG86" "$source_commit" -- internal/db/migrations
    echo "== (end)"
    git -C "$clone" checkout --quiet --detach "$baseline"
    ( cd "$clone" && GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$GO" list -deps ./cmd/agent ) > "$work/agent-deps.txt"
    echo "== Agent package closure at the baseline ($(wc -l < "$work/agent-deps.txt") packages): licensing packages"
    grep -c '/internal/licensing' "$work/agent-deps.txt" || true
} > "$proof"
[[ $(tail -n 1 "$proof") == 0 ]] || { echo "the Agent package closure includes the licensing seam; see $proof" >&2; exit 1; }
use_web "$work/web-dist-candidate"
b_json=$(build "$baseline" v0.1.0-alpha.87)
g_json=$(build "$good" v0.1.0-alpha.88)
d_json=$(build "$defective" v0.1.0-alpha.88)

python3 - "$work/upd1-artifacts.json" "$source_commit" "$clone" "$b_json" "$g_json" "$d_json" "$baseline" "$good" "$proof" <<'PY'
import json, sys
out, head, clone, b, g, d, baseline, good, proof = sys.argv[1:]
def item(path, sequence, parent=None):
    value = json.load(open(path))
    entry = {k: value[k] for k in ("archive", "sha256", "commit", "tree", "version", "product_web_src")}
    entry.update(sequence=sequence, license_mode=value.get("license_mode"), dist_json=path)
    if parent:
        entry["parent"] = parent
    return entry
document = {"schema": "celikpanel/upd1-artifacts/v1", "source_head": head, "clone": clone,
            "baseline": item(b, 87), "good": item(g, 88, baseline), "defective": item(d, 88, good)}
document["defective"]["defect"] = "cmd/panel/main.go --migrate-only exits 1 after migrating the isolated copy"
document["baseline_ref"] = {"ref": "v0.1.0-alpha.87", "tag_commit": baseline, "patched_files": [], "proof": proof}
document["provenance"] = (f"set18: baseline (installed) an unpublished fixture commit {baseline} over the source {head} "
                          "that changes only the release policy to v0.1.0-alpha.87 / 87 (previous the alpha.86 tag); "
                          "good/defective: unpublished disposable fixture commits labelled v0.1.0-alpha.88, served by the "
                          "origin, never installed; acceptance-license panels; not a release")
with open(out, "x") as handle:
    json.dump(document, handle, indent=2, sort_keys=True)
    handle.write("\n")
print(out)
PY
echo SET18-ARTIFACTS-OK
