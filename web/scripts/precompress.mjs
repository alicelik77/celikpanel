// Build-time gzip copies of the web assets (v0.1.0-alpha.125, "faster panel"
// P3). The Panel serves `<file>.gz` with Content-Encoding: gzip to a browser
// that accepts it (cmd/panel/compression.go) and the file itself otherwise, so
// no CPU is spent per request. Runs last in `npm run build`, after the bundle
// budget (which measures the gzip size itself).
//
// The copies are reproducible: maximum compression, no file name or time in
// the gzip header, and the header's operating-system byte fixed to "unknown"
// (255), so two builds of the same source give the same bytes on any host.
//
// Derleme anında gzip kopyaları: Panel `<dosya>.gz`yi gzip kabul eden
// tarayıcıya sunar, diğerlerine dosyanın kendisini. Kopyalar tekrarlanabilir.
import { readdir, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { constants, gzipSync } from 'node:zlib'

const distDir = fileURLToPath(new URL('../dist/', import.meta.url))
const compressible = /\.(js|mjs|css|html|svg|json|txt|webmanifest)$/
const minimumBytes = 1024

async function* files(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    // .vite holds the build manifest, which nothing serves.
    if (entry.name.startsWith('.')) continue
    const path = join(dir, entry.name)
    if (entry.isDirectory()) yield* files(path)
    else if (entry.isFile()) yield path
  }
}

let written = 0
let rawTotal = 0
let gzipTotal = 0
for await (const path of files(distDir)) {
  if (!compressible.test(path)) continue
  const source = await readFile(path)
  if (source.length < minimumBytes) continue
  const packed = gzipSync(source, { level: constants.Z_BEST_COMPRESSION })
  packed[9] = 255 // OS: unknown (RFC 1952), the same on every build host
  // A copy that saves less than a tenth is not worth a second file.
  if (packed.length > source.length * 0.9) continue
  await writeFile(`${path}.gz`, packed)
  written += 1
  rawTotal += source.length
  gzipTotal += packed.length
}
const kib = (n) => (n / 1024).toFixed(1)
console.log(`precompress: ${written} files, ${kib(rawTotal)} KiB -> ${kib(gzipTotal)} KiB gzip`)
