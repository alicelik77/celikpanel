# set15: shared names (sourced)
R=/var/tmp/cp-set15-run
L=$R/logs
J=<redacted>
REPO='<redacted> PROJECTS/celikpanel'
C=1636b73f0
G() { git -c safe.directory='*' -C "$REPO" "$@"; }
