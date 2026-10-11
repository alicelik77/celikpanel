#!/bin/bash
# set12: one cell on one new lab. Environment from the caller's wrapper: CELL ART LAB PORT LPORT COPY [HAND]
export PYTHONDONTWRITEBYTECODE=1
export SET12_RUN_ROOT=/var/tmp/cp-set12-run
export SET12_NEIGHBOUR_GATE=<scratchpad>/set12/tools/neighbour.sh
export SET12_DISK_GATE=<scratchpad>/set12/tools/gate.sh
export CELIKPANEL_LAB_LINK_BASE_IMAGES=1
