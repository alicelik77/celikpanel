from pathlib import Path
here = Path(__file__).resolve().parent
p = Path(r"<redacted>")
t = p.read_text(encoding="utf-8")
part = (here / "resume_cell.mjs.part").read_text(encoding="utf-8")
t = t.replace("""//   cell "done": writes done.json""", """//   cell "resume" (set18 run, after D1): the extension wizard shows the COMPLETED first run instead of its steps
//     (GET /api/v1/setup/operation answers the finished run while the setup is an extension draft), so "Stop
//     extending" and the wizard's review and start are not reachable. In a new browser (no stored marker) the owner
//     opens /setup (D1 recorded again); the blocked UI actions are then made by the page's own session with the
//     wizard's own requests (POST /api/v1/setup/extend cancel; from the ready screen "Extend setup", then POST
//     /api/v1/setup/extend begin with Core Mail and the mail host name, POST /api/v1/setup/plan, POST
//     /api/v1/setup/start), each recorded as a STAND-IN for the blocked button; the run is then followed in the page
//     exactly as cell 2 (mail record, reverse DNS, "Check requirements again", "Finishing setup", ready).
//   cell "done": writes done.json""", 1)
t = t.replace("import { createHash } from 'node:crypto';", "import { createHash, randomBytes } from 'node:crypto';", 1)
t = t.replace("\nfor (const cell of cells) {", part + "\nfor (const cell of cells) {", 1)
t = t.replace("    if (cell === 'setup') await cellSetup();", "    if (cell === 'setup') await cellSetup();\n    else if (cell === 'resume') await cellResume();", 1)
p.write_text(t, encoding="utf-8")
print("ok", t.count("cellResume"))
