// Automatic reads run only while the page is visible (D-033 rule 6, D-032;
// the open item of the alpha.85 release notes, 2026-10-15): stopped while
// hidden, one read at once when shown again, never two at once, reads only.
// Otomatik okumalar yalniz sayfa gorunurken calisir.
import assert from 'node:assert/strict';
import test from 'node:test';
import { startVisibleReads } from '../src/lib/visibleReads.ts';

function fakeTimers() {
    const timers = new Map();
    let next = 1;
    return {
        timers,
        setInterval: (fn, ms) => { const id = next++; timers.set(id, { fn, ms }); return id; },
        clearInterval: id => { timers.delete(id); },
        fire() { for (const item of [...timers.values()]) item.fn(); },
    };
}
function fakeDocument(state = 'visible') {
    const doc = new EventTarget();
    doc.visibilityState = state;
    doc.show = () => { doc.visibilityState = 'visible'; doc.dispatchEvent(new Event('visibilitychange')); };
    doc.hide = () => { doc.visibilityState = 'hidden'; doc.dispatchEvent(new Event('visibilitychange')); };
    return doc;
}
const flush = async () => { for (let i = 0; i < 4; i += 1) await Promise.resolve(); };

test('visible: reads on each tick; hidden: the interval stops and nothing is read', async () => {
    const original = globalThis.document;
    const doc = fakeDocument('visible');
    globalThis.document = doc;
    const timers = fakeTimers();
    let reads = 0;
    const stop = startVisibleReads(async () => { reads += 1; }, 3000, timers);
    try {
        assert.equal(reads, 0, 'no immediate read unless asked');
        assert.deepEqual([...timers.timers.values()].map(item => item.ms), [3000]);
        timers.fire(); await flush();
        assert.equal(reads, 1);
        doc.hide();
        assert.equal(timers.timers.size, 0, 'no timer while hidden');
        assert.equal(reads, 1);
        doc.show(); await flush();
        assert.equal(reads, 2, 'one read at once when shown again');
        assert.equal(timers.timers.size, 1, 'the interval resumes');
        timers.fire(); await flush();
        assert.equal(reads, 3);
    } finally {
        stop();
        if (original === undefined) delete globalThis.document; else globalThis.document = original;
    }
    assert.equal(timers.timers.size, 0, 'stopping clears the interval');
    doc.show(); await flush();
    assert.equal(reads, 3, 'a stopped reader does not answer visibility');
});

test('hidden from the start: no interval and no read until shown; immediate reads once regardless', async () => {
    const original = globalThis.document;
    const doc = fakeDocument('hidden');
    globalThis.document = doc;
    const timers = fakeTimers();
    let reads = 0;
    const stop = startVisibleReads(async () => { reads += 1; }, 5000, timers, { immediate: true });
    try {
        assert.equal(reads, 1, 'the opening read is the caller\'s own');
        assert.equal(timers.timers.size, 0);
        await flush();
        doc.show(); await flush();
        assert.equal(reads, 2);
        assert.equal(timers.timers.size, 1);
    } finally {
        stop();
        if (original === undefined) delete globalThis.document; else globalThis.document = original;
    }
});

test('never two reads at once: a tick during a read is skipped', async () => {
    const original = globalThis.document;
    delete globalThis.document;
    const timers = fakeTimers();
    let reads = 0;
    let release;
    const stop = startVisibleReads(() => { reads += 1; return new Promise(resolve => { release = resolve; }); }, 1500, timers);
    try {
        timers.fire();
        timers.fire();
        assert.equal(reads, 1, 'without a document the page counts as visible; the second tick waits');
        release(); await flush();
        timers.fire();
        assert.equal(reads, 2);
        release(); await flush();
    } finally {
        stop();
        if (original !== undefined) globalThis.document = original;
    }
});
