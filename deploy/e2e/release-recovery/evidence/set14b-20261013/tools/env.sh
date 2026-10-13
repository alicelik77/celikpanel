# set14b: shared names (sourced)
R=/var/tmp/cp-set14b-run
L=$R/logs
J=<redacted>
REPO='<redacted> PROJECTS/celikpanel'
C=1a74eeda5
G() { git -c safe.directory='*' -C "$REPO" "$@"; }
