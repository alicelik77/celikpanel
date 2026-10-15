# set16: shared names (sourced)
R=/var/tmp/cp-set16
L=$R/logs
J=<redacted>
REPO='/mnt/c/CELIKBROS PROJECTS/celikpanel'
C=89cf883971800c39c72c9f8358c58d56aace4194
G() { git -c safe.directory='*' -C "$REPO" "$@"; }
