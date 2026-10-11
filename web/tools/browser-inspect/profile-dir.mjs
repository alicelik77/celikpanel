// The browser profile directory of the live tools (set11 cell E, 2026-10-11).
//
// The eleventh native record established that Chrome with its on-disk profile under a long Windows path could not
// open CacheStorage ("Failed to execute 'open' on 'CacheStorage': Unexpected internal error"), so the recovery service
// worker never installed, while the same Chrome with its profile under a short path registered it and showed the
// product's offline page. The profile directory sat 177 characters deep; an entry under its CacheStorage is about 316,
// past Windows' 260-character limit (the mechanism is inferred, not read from a Chrome line).
//
// So every live tool puts its profile under a short path: %TEMP%\cpb\<8 hex> on Windows, /tmp/cpb-<8 hex> elsewhere,
// or under LIVE_PROFILE_ROOT when that is set, logs the path's length, and refuses to start when the path is longer
// than PROFILE_PATH_LIMIT characters.
import { randomBytes } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

export const PROFILE_PATH_LIMIT = 120;

// shortProfileDir creates and returns the profile directory. outRoot, when given, receives browser-profile.json with
// the path's length and where it came from (not the path itself: it holds the user's name).
export function shortProfileDir(outRoot) {
    const id = randomBytes(4).toString('hex');
    const root = process.env.LIVE_PROFILE_ROOT;
    const dir = root ? join(root, `cpb-${id}`)
        : process.platform === 'win32' ? join(process.env.TEMP || tmpdir(), 'cpb', id) : `/tmp/cpb-${id}`;
    const record = { at: new Date().toISOString(), length: dir.length, limit: PROFILE_PATH_LIMIT, from: root ? 'LIVE_PROFILE_ROOT' : 'temp' };
    if (outRoot) writeFileSync(join(outRoot, `browser-profile-${id}.json`), JSON.stringify(record, null, 2) + '\n');
    if (dir.length > PROFILE_PATH_LIMIT) {
        console.error(`browser profile path is ${dir.length} characters, longer than ${PROFILE_PATH_LIMIT}: ${dir}`);
        console.error('Chrome cannot open CacheStorage under a long Windows path (set11 cell E). Set LIVE_PROFILE_ROOT or TEMP to a shorter directory.');
        process.exit(2);
    }
    mkdirSync(dir, { recursive: true });
    console.error(`browser profile: ${dir} (${dir.length} characters, limit ${PROFILE_PATH_LIMIT})`);
    return dir;
}
