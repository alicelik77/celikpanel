#!/bin/bash
# peek.sh FILE [N] : tail of a run log
tail -n ${2:-40} /var/tmp/a114r2/$1 | grep -v DeprecationWarning | grep -v 'utcnow()' | cut -c1-${W:-500}
