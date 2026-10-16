"""set18b: build web/tools/browser-inspect/cold-load-set18b.mjs from cold-load-set18.mjs and the parts in this folder."""
from pathlib import Path
import sys

here = Path(__file__).resolve().parent
repo = Path(sys.argv[1])
src = (repo / "web/tools/browser-inspect/cold-load-set18.mjs").read_text(encoding="utf-8").replace("\r\n", "\n").split("\n")
head = (here / "head.txt").read_text(encoding="utf-8").replace("\r\n", "\n").rstrip("\n").split("\n")
follow = (here / "follow.txt").read_text(encoding="utf-8").replace("\r\n", "\n").rstrip("\n").split("\n")
cell = (here / "cell.txt").read_text(encoding="utf-8").replace("\r\n", "\n").rstrip("\n").split("\n")


def index(prefix):
    hits = [i for i, line in enumerate(src) if line.startswith(prefix)]
    assert len(hits) == 1, (prefix, hits)
    return hits[0]


i_import = index("import { createHash")
i_sig = index("const signature = ")
i_review = index("async function reviewBlocks")
i_cell = index("// -- the cell")
middle = "\n".join(src[i_import:i_sig])
reviewblocks = src[i_review:i_cell]


def sub(text, old, new, count=1):
    assert text.count(old) == count, (old, text.count(old))
    return text.replace(old, new)


middle = sub(middle, "import { createHash, randomBytes } from 'node:crypto';", "import { createHash } from 'node:crypto';")
middle = sub(middle, "usage: node cold-load-set18.mjs", "usage: node cold-load-set18b.mjs")
middle = sub(middle, "function leafOf() {", "// servername null: no SNI (an IP address in the URL), set18 O2.\nfunction leafOf(servername = 'localhost') {")
middle = sub(middle, "rejectUnauthorized: false, servername: 'localhost' }", "rejectUnauthorized: false, ...(servername ? { servername } : {}) }")
middle = sub(middle, "const leafRecord = async label => { try { const leaf = await leafOf(); await jsonl(join(dir, 'leaf.jsonl'), { at: iso(), label, ...leaf,",
             "const leafRecord = async (label, servername = 'localhost') => { try { const leaf = await leafOf(servername); await jsonl(join(dir, 'leaf.jsonl'), { at: iso(), label, sni: servername, ...leaf,")
middle = sub(middle, "await leafRecord('start');", "await leafRecord('start'); await leafRecord('start-no-sni', null);")
middle = sub(middle, "sessionStorage.getItem('set18.lang')", "sessionStorage.getItem('set18b.lang')")
middle = sub(middle, "const latest = { operation: null, setup: null };",
             "const latest = { operation: null, setup: null };\n// Every POST of the mail enrollment continuation the page sends (set18 O3): counted from the page's own requests.\nconst enrollPosts = [];")
middle = sub(middle, "        await jsonl(join(dir, 'requests.jsonl'), entry);\n",
             "        if (entry.method === 'POST' && path0.startsWith('/api/v1/setup/mail-enrollment/')) enrollPosts.push({ at: entry.start, url: entry.url, status: entry.status, body: entry.body ? entry.body.slice(0, 600) : null, request_body: entry.request_body || null });\n        await jsonl(join(dir, 'requests.jsonl'), entry);\n")
cell_text = sub("\n".join(cell), "        summary.c2_review_tr = await page.evaluate(() => null);\n", "")
out = "\n".join(head) + "\n" + middle + "\n" + "\n".join(follow) + "\n" + "\n".join(reviewblocks) + "\n" + cell_text + "\n"
assert "set18.lang" not in out and "cellResume" not in out
target = repo / "web/tools/browser-inspect/cold-load-set18b.mjs"
target.write_text(out, encoding="utf-8", newline="\n")
print("written", target.name, len(out.split("\n")), "lines")
