# set14: shared names (sourced)
R=/var/tmp/cp-set14-run
L=$R/logs
J=<redacted>
REPO='<redacted> PROJECTS/celikpanel'
C=a5830dd3e
G() { git -c safe.directory='*' -C "$REPO" "$@"; }
