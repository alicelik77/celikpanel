| Cell | Debian 13 | Ubuntu 24.04 | Arch | Debian 13 (run-b) | raw (`<platform>/<run>/driver/` + the step's `section.json`) |
| --- | --- | --- | --- | --- | --- |
| S0 | PASS | PASS | PASS | PASS | `steps/09-s0-sites/section.json` |
| h | PASS | PASS | PASS | PASS | `steps/10-h-i-unchanged-start/section.json` |
| i | PASS | PASS | PASS | PASS | `steps/10-h-i-unchanged-start/section.json` |
| C | PASS | PASS | PASS | PASS | `steps/11-c-directory-modes/section.json` |
| A1 | PASS (re-judged) | PASS | not run | PASS (re-judged) | `steps/12-a1-probe-ready/section.json` |
| A4 | PASS | PASS | not run | PASS | `steps/13-a4-nginx-stopped/section.json` |
| A2 | PASS (re-judged) | PASS | PASS | PASS | `steps/14-a2-include-missing/section.json` |
| A5 | NOT-MEASURED | NOT-MEASURED | not run | NOT-MEASURED | `steps/15-a5-include-back/section.json` |
| A3 | PASS | PASS | not run | PASS (re-judged) | `steps/16-a3-mail-name/section.json` |
| A6 | PASS | PASS | not run | PASS | `steps/17-a6-bound-address/section.json` |
| B1 | PASS | PASS | PASS | PASS | `steps/18-b1-locked-start/section.json` |
| B2 | PASS | PASS | PASS | PASS | `steps/19-b2-unlocked-recheck/section.json` |
| B3 | PASS | PASS | PASS | PASS | `steps/20-b3-save-writes/section.json` |
| B4 | FAIL | FAIL | FAIL | FAIL | `steps/21-b4-take-refused/section.json` |
| B-before | PASS | not run | not run | not run | `steps/09-set10-sites-before-the-update/section.json` |
| B-return | PASS | not run | not run | not run | `steps/17-set10-site-files-after-the-update/section.json` |

Driver verdicts recomputed without the dropped expectation:
- Debian 13 A1: driver FAIL, recomputed PASS (`debian13/run-c/driver/steps/12-a1-probe-ready/section.json`)
- Debian 13 A2: driver FAIL, recomputed PASS (`debian13/run-c/driver/steps/14-a2-include-missing/section.json`)
- Debian 13 (run-b) A1: driver FAIL, recomputed PASS (`debian13/run-b/driver/steps/12-a1-probe-ready/section.json`)
- Debian 13 (run-b) A3: driver FAIL, recomputed PASS (`debian13/run-b/driver/steps/16-a3-mail-name/section.json`)

- Debian 13 A5 NOT-MEASURED: the dashboard entry for the site clears (`debian13/run-c/driver/steps/15-a5-include-back/section.json`)
- Debian 13 B4 FAIL: the page and the Domains row name the same state after the refusal (`debian13/run-c/driver/steps/21-b4-take-refused/section.json`)
- Ubuntu 24.04 A5 NOT-MEASURED: the dashboard entry for the site clears (`ubuntu/run-a/driver/steps/15-a5-include-back/section.json`)
- Ubuntu 24.04 B4 FAIL: the page and the Domains row name the same state after the refusal (`ubuntu/run-a/driver/steps/21-b4-take-refused/section.json`)
- Arch B4 FAIL: the page and the Domains row name the same state after the refusal (`arch/run-a/driver/steps/16-b4-take-refused/section.json`)
- Debian 13 (run-b) A5 NOT-MEASURED: the dashboard entry for the site clears (`debian13/run-b/driver/steps/15-a5-include-back/section.json`)
- Debian 13 (run-b) B4 FAIL: the page and the Domains row name the same state after the refusal (`debian13/run-b/driver/steps/21-b4-take-refused/section.json`)
