# set14c: shared names (sourced)
R=/var/tmp/cp-set14c-run
L=$R/logs
J=<redacted>
REPO='<redacted> PROJECTS/celikpanel'
C=68b79b372
G() { git -c safe.directory='*' -C "$REPO" "$@"; }
