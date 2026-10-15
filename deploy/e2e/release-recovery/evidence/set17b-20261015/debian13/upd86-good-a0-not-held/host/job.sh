#!/bin/bash
# set17b: one cell on one new lab (set17b-upd86-debian13-good), with the browser window "page" (SET12_HAND=h17bu86a).
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set17b
export SET17_NEIGHBOUR_GATE=<redacted>
export SET17_DISK_GATE=<redacted>
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
export SET12_HAND=h17bu86a
exec bash /var/tmp/cp-set17b/harness-c/deploy/e2e/release-recovery/run-set17b.sh cell set17b-upd86-debian13-good "$(cat /var/tmp/cp-set17b/artifacts-second.path)" s17b-u86-a 5281 19281
