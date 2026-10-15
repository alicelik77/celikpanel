# set17: shared names (sourced)
R=/var/tmp/cp-set17
L=$R/logs
J=<redacted>
REPO='/mnt/c/CELIKBROS PROJECTS/celikpanel'
C=313f3eea2c9b89251588648b09484cd34b7aecb1
G() { git -c safe.directory='*' -C "$REPO" "$@"; }
