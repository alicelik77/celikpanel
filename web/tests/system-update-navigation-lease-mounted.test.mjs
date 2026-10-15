import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ts from 'typescript';

const require = createRequire(import.meta.url);

function compileURL(relativePath, replacements = []) {
    const source = readFileSync(new URL(relativePath, import.meta.url), 'utf8');
    let compiled = ts.transpileModule(source, {
        compilerOptions: {
            module: ts.ModuleKind.ES2022,
            target: ts.ScriptTarget.ES2020,
            jsx: ts.JsxEmit.ReactJSX,
        },
    }).outputText;
    for (const [pattern, replacement] of replacements) compiled = compiled.replace(pattern, replacement);
    return 'data:text/javascript;base64,' + Buffer.from(compiled).toString('base64');
}

const watchdogURL = compileURL('../src/lib/systemUpdateWatchdog.ts');
const reactURL = pathToFileURL(require.resolve('react')).href;
const jsxRuntimeURL = pathToFileURL(require.resolve('react/jsx-runtime')).href;
const routerCoreURL = compileURL('../src/router-core.ts');
const routerHistoryURL = compileURL('../src/router-history.ts');
const routerURL = compileURL('../src/router.tsx', [
    [/from ['"]react['"]/, `from '${reactURL}'`],
    [/from ['"]react\/jsx-runtime['"]/, `from '${jsxRuntimeURL}'`],
    [/from ['"]\.\/router-core['"]/g, `from '${routerCoreURL}'`],
    [/from ['"]\.\/router-history['"]/g, `from '${routerHistoryURL}'`],
]);
const hookURL = compileURL('../src/lib/useSystemUpdateNavigationLease.ts', [
    [/from ['"]react['"]/, `from '${reactURL}'`],
    [/from ['"]\.\/systemUpdateWatchdog['"]/, `from '${watchdogURL}'`],
]);
const watchdog = await import(watchdogURL);
const { useSystemUpdateNavigationLease } = await import(hookURL);
const { isNavigationBlocked, useNavigationBlocker } = await import(routerURL);

class FakeLeaseRuntime {
    constructor(now) {
        this.time = now;
        this.monotonicTime = 0;
        this.nextTimer = 1;
        this.tasks = new Map();
        this.values = new Map();
        this.listeners = new Set();
        this.wakeListeners = new Set();
        this.storage = {
            getItem: (key) => this.values.get(key) ?? null,
            setItem: (key, value) => {
                this.values.set(key, value);
                if (key === watchdog.SYSTEM_UPDATE_NAVIGATION_RELEASE_KEY) {
                    for (const listener of [...this.listeners]) listener(value);
                }
            },
        };
        this.options = {
            now: () => this.time,
            monotonicNow: () => this.monotonicTime,
            schedule: (callback, delay) => {
                const id = this.nextTimer++;
                this.tasks.set(id, { at: this.time + delay, callback });
                return id;
            },
            cancel: (id) => this.tasks.delete(id),
            storage: this.storage,
            subscribe: (listener) => {
                this.listeners.add(listener);
                return () => this.listeners.delete(listener);
            },
            subscribeWake: (listener) => {
                this.wakeListeners.add(listener);
                return () => this.wakeListeners.delete(listener);
            },
        };
    }

    advance(milliseconds) {
        this.time += milliseconds;
        this.monotonicTime += milliseconds;
        while (true) {
            const due = [...this.tasks.entries()]
                .filter(([, task]) => task.at <= this.time)
                .sort((left, right) => left[1].at - right[1].at)[0];
            if (!due) return;
            this.tasks.delete(due[0]);
            due[1].callback();
        }
    }

    emit(identity) {
        for (const listener of [...this.listeners]) listener(identity);
    }

    elapseWhileThrottled(milliseconds, wallMilliseconds = milliseconds) {
        this.time += wallMilliseconds;
        this.monotonicTime += milliseconds;
    }

    wake() {
        for (const listener of [...this.wakeListeners]) listener();
    }
}

function Probe({ identity, createdAt, runtime, phase = 'active', requested = true }) {
    const lease = useSystemUpdateNavigationLease(identity, createdAt, requested, null, runtime.options);
    return React.createElement('output', {
        'data-blocked': lease.blocked,
        'data-released': lease.released,
        'data-phase': phase,
    });
}

function blocked(renderer) {
    return renderer.root.findByType('output').props['data-blocked'];
}

function GuardProbe({ identity, createdAt, runtime, requested, page }) {
    const lease = useSystemUpdateNavigationLease(identity, createdAt, requested, null, runtime.options);
    const navigationBlocked = watchdog.systemUpdateGlobalNavigationBlocked(requested, lease.blocked);
    const blocker = React.useRef(navigationBlocked);
    blocker.current = navigationBlocked;
    useNavigationBlocker(blocker);
    React.useLayoutEffect(() => navigationBlocked
        ? watchdog.acquireSystemUpdatePageLock(
            page.application,
            () => false,
            page.window,
            page.document,
        )
        : undefined, [navigationBlocked, page]);
    return React.createElement('output', { 'data-blocked': navigationBlocked });
}

function fakePage() {
    const attributes = new Map();
    const beforeUnload = new Set();
    return {
        attributes,
        beforeUnload,
        application: {
            inert: false,
            setAttribute: (key, value) => attributes.set(key, value),
            removeAttribute: (key) => attributes.delete(key),
        },
        document: { body: { style: { overflow: 'scroll' } } },
        window: {
            addEventListener: (type, listener) => {
                if (type === 'beforeunload') beforeUnload.add(listener);
            },
            removeEventListener: (type, listener) => {
                if (type === 'beforeunload') beforeUnload.delete(listener);
            },
        },
        dispatchBeforeUnload() {
            const event = {
                prevented: false,
                returnValue: undefined,
                preventDefault() { this.prevented = true; },
            };
            for (const listener of [...beforeUnload]) listener(event);
            return event;
        },
    };
}

test('mounted lease releases at exactly 120 seconds and never re-locks after clock rollback', () => {
    const runtime = new FakeLeaseRuntime(1_000_000);
    let renderer;
    act(() => {
        renderer = TestRenderer.create(React.createElement(Probe, {
            identity: 'operation-a', createdAt: 1_000_000, runtime,
        }));
    });
    assert.equal(blocked(renderer), true);
    act(() => runtime.advance(119_999));
    assert.equal(blocked(renderer), true);
    act(() => runtime.advance(1));
    assert.equal(blocked(renderer), false);

    runtime.time = 900_000;
    act(() => renderer.update(React.createElement(Probe, {
        identity: 'operation-a', createdAt: 1_000_000, runtime, phase: 'terminal',
    })));
    assert.equal(blocked(renderer), false);
    assert.equal(runtime.tasks.size, 0);
    act(() => renderer.unmount());
});

test('terminal, reload, and remount reuse the released exact-operation lease', () => {
    const runtime = new FakeLeaseRuntime(2_000_000);
    let renderer;
    act(() => {
        renderer = TestRenderer.create(React.createElement(Probe, {
            identity: 'operation-b', createdAt: 1_880_000, runtime, phase: 'terminal',
        }));
    });
    assert.equal(blocked(renderer), false);
    act(() => renderer.update(React.createElement(Probe, {
        identity: 'operation-b', createdAt: 1_880_000, runtime, phase: 'reload',
    })));
    assert.equal(blocked(renderer), false);
    act(() => renderer.unmount());

    runtime.time = 1_900_000;
    act(() => {
        renderer = TestRenderer.create(React.createElement(Probe, {
            identity: 'operation-b', createdAt: 1_880_000, runtime, phase: 'active',
        }));
    });
    assert.equal(blocked(renderer), false);
    act(() => renderer.unmount());
});

test('future clock releases fail-open across remount while a new exact operation gets a new lease', () => {
    const runtime = new FakeLeaseRuntime(3_000_000);
    let renderer;
    act(() => {
        renderer = TestRenderer.create(React.createElement(Probe, {
            identity: 'future-operation', createdAt: 3_100_000, runtime,
        }));
    });
    assert.equal(blocked(renderer), false);
    act(() => renderer.unmount());
    runtime.time = 3_100_001;
    act(() => {
        renderer = TestRenderer.create(React.createElement(Probe, {
            identity: 'future-operation', createdAt: 3_100_000, runtime,
        }));
    });
    assert.equal(blocked(renderer), false);
    act(() => renderer.update(React.createElement(Probe, {
        identity: 'new-operation', createdAt: 3_100_001, runtime,
    })));
    assert.equal(blocked(renderer), true);
    assert.equal(runtime.tasks.size, 1);
    act(() => renderer.unmount());
});

test('sibling-tab release and unmount clean up listeners and fake timers', () => {
    const runtime = new FakeLeaseRuntime(4_000_000);
    let renderer;
    act(() => {
        renderer = TestRenderer.create(React.createElement(Probe, {
            identity: 'operation-c', createdAt: 4_000_000, runtime,
        }));
    });
    assert.equal(runtime.tasks.size, 1);
    assert.equal(runtime.listeners.size, 1);
    assert.equal(runtime.wakeListeners.size, 1);
    act(() => runtime.emit('operation-c'));
    assert.equal(blocked(renderer), false);
    assert.equal(runtime.tasks.size, 0);
    assert.equal(runtime.listeners.size, 0);
    assert.equal(runtime.wakeListeners.size, 0);
    act(() => renderer.unmount());
    assert.equal(runtime.tasks.size, 0);
    assert.equal(runtime.listeners.size, 0);
    assert.equal(runtime.wakeListeners.size, 0);
});

test('background timer throttling cannot extend the lease after focus, visibility, or pageshow wake', () => {
    const runtime = new FakeLeaseRuntime(5_000_000);
    let renderer;
    act(() => {
        renderer = TestRenderer.create(React.createElement(Probe, {
            identity: 'operation-hidden', createdAt: 5_000_000, runtime,
        }));
    });
    assert.equal(blocked(renderer), true);
    runtime.elapseWhileThrottled(120_000);
    assert.equal(blocked(renderer), true, 'the deliberately throttled timer has not fired');
    act(() => runtime.wake());
    assert.equal(blocked(renderer), false, 'returning to the tab rechecks the absolute deadline');
    assert.equal(runtime.tasks.size, 0);
    assert.equal(runtime.wakeListeners.size, 0);
    assert.equal(
        runtime.storage.getItem(watchdog.SYSTEM_UPDATE_NAVIGATION_RELEASE_KEY),
        'operation-hidden',
    );
    act(() => renderer.unmount());
});

test('monotonic elapsed time releases on wake even after the wall clock rolls backward', () => {
    const runtime = new FakeLeaseRuntime(6_000_000);
    let renderer;
    act(() => {
        renderer = TestRenderer.create(React.createElement(Probe, {
            identity: 'operation-clock-skew', createdAt: 6_000_000, runtime,
        }));
    });
    runtime.elapseWhileThrottled(120_000, -60_000);
    act(() => runtime.wake());
    assert.equal(blocked(renderer), false);
    act(() => renderer.unmount());
});

test('an engaged lease released by auth or terminal suspension never re-locks', () => {
    const runtime = new FakeLeaseRuntime(7_000_000);
    let renderer;
    act(() => {
        renderer = TestRenderer.create(React.createElement(Probe, {
            identity: 'operation-auth', createdAt: 7_000_000, runtime, requested: true,
        }));
    });
    assert.equal(blocked(renderer), true);
    act(() => renderer.update(React.createElement(Probe, {
        identity: 'operation-auth', createdAt: 7_000_000, runtime,
        requested: false, phase: 'auth-paused',
    })));
    assert.equal(blocked(renderer), false);
    assert.equal(
        runtime.storage.getItem(watchdog.SYSTEM_UPDATE_NAVIGATION_RELEASE_KEY),
        'operation-auth',
    );
    act(() => renderer.update(React.createElement(Probe, {
        identity: 'operation-auth', createdAt: 7_000_000, runtime,
        requested: true, phase: 'active',
    })));
    assert.equal(blocked(renderer), false);
    act(() => renderer.unmount());
});

test('mounted lease removes inert, beforeunload, body lock, and router blocker at 120 seconds', () => {
    const runtime = new FakeLeaseRuntime(8_000_000);
    const page = fakePage();
    let renderer;
    act(() => {
        renderer = TestRenderer.create(React.createElement(GuardProbe, {
            identity: 'operation-page-lock', createdAt: 8_000_000,
            runtime, requested: true, page,
        }));
    });
    assert.equal(isNavigationBlocked(), true);
    assert.equal(page.application.inert, true);
    assert.equal(page.attributes.get('aria-busy'), 'true');
    assert.equal(page.document.body.style.overflow, 'hidden');
    assert.equal(page.beforeUnload.size, 1);
    assert.equal(page.dispatchBeforeUnload().prevented, true);

    act(() => runtime.advance(120_000));
    assert.equal(isNavigationBlocked(), false);
    assert.equal(page.application.inert, false);
    assert.equal(page.attributes.has('aria-busy'), false);
    assert.equal(page.document.body.style.overflow, 'scroll');
    assert.equal(page.beforeUnload.size, 0);
    assert.equal(page.dispatchBeforeUnload().prevented, false);

    act(() => renderer.update(React.createElement(GuardProbe, {
        identity: 'operation-page-lock', createdAt: 8_000_000,
        runtime, requested: true, page,
    })));
    assert.equal(isNavigationBlocked(), false, 'the same exact identity never restores the router blocker');
    assert.equal(page.beforeUnload.size, 0);
    act(() => renderer.unmount());
    assert.equal(isNavigationBlocked(), false);
});

test('navigation lease and mutation authority are independent executable policies', () => {
    assert.equal(watchdog.systemUpdateGlobalNavigationBlocked(true, true), true);
    assert.equal(watchdog.systemUpdateGlobalNavigationBlocked(true, false), false);
    assert.equal(watchdog.systemUpdateGlobalNavigationBlocked(false, true), false);
    assert.equal(watchdog.systemUpdateMutationLocked(false, false), true);
    assert.equal(watchdog.systemUpdateMutationLocked(true, true), true);
    assert.equal(watchdog.systemUpdateMutationLocked(true, false), false);
});

test('localStorage SecurityError is navigation-safe and never installs a broken storage listener', () => {
    const previousWindow = globalThis.window;
    let storageListenerCalls = 0;
    globalThis.window = {
        get localStorage() { throw new DOMException('denied', 'SecurityError'); },
        addEventListener() { storageListenerCalls += 1; },
        removeEventListener() {},
    };
    const runtime = new FakeLeaseRuntime(9_000_000);
    const options = {
        now: runtime.options.now,
        monotonicNow: runtime.options.monotonicNow,
        schedule: runtime.options.schedule,
        cancel: runtime.options.cancel,
        subscribeWake: runtime.options.subscribeWake,
    };
    let renderer;
    try {
        act(() => {
            renderer = TestRenderer.create(React.createElement(Probe, {
                identity: 'operation-storage-denied', createdAt: 9_000_000,
                runtime: { options },
            }));
        });
        assert.equal(blocked(renderer), true);
        assert.equal(storageListenerCalls, 0);
        act(() => runtime.advance(120_000));
        assert.equal(blocked(renderer), false);
    } finally {
        if (renderer) act(() => renderer.unmount());
        if (previousWindow === undefined) delete globalThis.window;
        else globalThis.window = previousWindow;
    }
});

// Native record set17b, finding F1 (2026-10-15): in both candidate runs the
// lock ended by its two-minute lease at T+120 s while the Panel was down until
// T+194 to 203 s. While the followed update's restart signal says its tracking
// has lost the Panel, the lock is kept until the tracking reaches the Panel
// again or the restart window ends, whichever comes first. The probe is wired
// as the provider wires it: the real restart signal, its selector and the lease.
const trackingURL = compileURL('../src/lib/systemUpdateTracking.ts');
const restartSignal = await import(compileURL('../src/lib/systemUpdateRestartSignal.ts', [
    [/from ['"]react['"]/, `from '${reactURL}'`],
    [/from ['"]\.\/systemUpdateTracking['"]/, `from '${trackingURL}'`],
]));
const tracking = await import(trackingURL);

const F1_REQUEST = 'f'.repeat(32);
function RestartProbe({ identity, createdAt, runtime, requested = true, wired = true }) {
    const published = restartSignal.useSystemUpdateRestart();
    const restart = restartSignal.systemUpdateNavigationRestart(published, identity, createdAt);
    const lease = useSystemUpdateNavigationLease(identity, createdAt, requested, wired ? restart : null, runtime.options);
    return React.createElement('output', { 'data-blocked': lease.blocked });
}
function mountRestart(runtime, props = {}) {
    let renderer;
    act(() => {
        renderer = TestRenderer.create(React.createElement(RestartProbe, {
            identity: F1_REQUEST, createdAt: runtime.time, runtime, ...props,
        }));
    });
    return renderer;
}
// What the provider publishes: the followed request, its start, the first lost
// read of the current outage (null while its reads answer).
const publish = (startedAt, lostSince) => act(() => restartSignal.publishSystemUpdateRestart({ requestID: F1_REQUEST, startedAt, lostSince }));
const endSignal = () => act(() => restartSignal.publishSystemUpdateRestart(null));
const at = (runtime, start, offset) => act(() => runtime.advance(start + offset - runtime.time));

test('set17b F1: the lock is kept at T+121 s through an outage that began at T+25 s and released at reconnection', () => {
    const start = 10_000_000;
    const runtime = new FakeLeaseRuntime(start);
    const renderer = mountRestart(runtime);
    try {
        publish(start, null);
        at(runtime, start, 25_000);
        publish(start, start + 25_000);
        at(runtime, start, 121_000);
        assert.equal(blocked(renderer), true, 'T+121 s: past the two-minute lease, the Panel still down');
        at(runtime, start, 193_000);
        assert.equal(blocked(renderer), true, 'T+193 s: still down (set17b run b reconnected at T+193.6 s)');
        assert.equal(runtime.values.get(watchdog.SYSTEM_UPDATE_NAVIGATION_RELEASE_KEY), undefined, 'nothing released yet');
        // The tracking reaches the Panel and reads the update's record.
        publish(start, null);
        assert.equal(blocked(renderer), false, 'released at reconnection');
        assert.equal(runtime.values.get(watchdog.SYSTEM_UPDATE_NAVIGATION_RELEASE_KEY), F1_REQUEST, 'released for good');
        // A second outage inside the window never brings it back.
        at(runtime, start, 200_000);
        publish(start, start + 200_000);
        assert.equal(blocked(renderer), false);
        assert.equal(runtime.tasks.size, 0);
    } finally { endSignal(); act(() => renderer.unmount()); }
});

test('set17b F1: without reconnection the lock ends with the restart window, 6 minutes from the start', () => {
    const start = 11_000_000;
    const runtime = new FakeLeaseRuntime(start);
    const renderer = mountRestart(runtime);
    try {
        at(runtime, start, 119_000);
        publish(start, start + 119_000);
        assert.equal(tracking.systemUpdateRestartDeadline(start, start + 119_000), start + tracking.SYSTEM_UPDATE_EXPECTED_RESTART_MS);
        at(runtime, start, 359_999);
        assert.equal(blocked(renderer), true, 'T+359.999 s: inside the window');
        at(runtime, start, 360_000);
        assert.equal(blocked(renderer), false, 'T+360 s: the window ended, the Panel never answered');
        assert.equal(runtime.values.get(watchdog.SYSTEM_UPDATE_NAVIGATION_RELEASE_KEY), F1_REQUEST);
    } finally { endSignal(); act(() => renderer.unmount()); }

    // A throttled background tab: the window's end is applied on wake.
    const runtime2 = new FakeLeaseRuntime(start);
    const renderer2 = mountRestart(runtime2);
    try {
        at(runtime2, start, 25_000);
        publish(start, start + 25_000);
        act(() => runtime2.elapseWhileThrottled(400_000));
        assert.equal(blocked(renderer2), true, 'no timer ran while throttled');
        act(() => runtime2.wake());
        assert.equal(blocked(renderer2), false, 'released on wake, past the window');
    } finally { endSignal(); act(() => renderer2.unmount()); }
});

test('set17b F1: without a restart the two-minute lease is unchanged', () => {
    const cases = [
        { name: 'no signal', signal: () => {} },
        { name: 'the server said active and its reads answer', signal: start => publish(start, null) },
        { name: 'another request is restarting', signal: start => act(() => restartSignal.publishSystemUpdateRestart({ requestID: 'e'.repeat(32), startedAt: start, lostSince: start + 25_000 })) },
        { name: 'another start of the same request', signal: start => publish(start + 1, start + 25_000) },
    ];
    for (const item of cases) {
        const start = 12_000_000;
        const runtime = new FakeLeaseRuntime(start);
        const renderer = mountRestart(runtime);
        try {
            at(runtime, start, 25_000);
            item.signal(start);
            at(runtime, start, 119_999);
            assert.equal(blocked(renderer), true, item.name);
            at(runtime, start, 120_000);
            assert.equal(blocked(renderer), false, `${item.name}: released at exactly 120 s`);
        } finally { endSignal(); act(() => renderer.unmount()); }
    }

    // A restart that begins after the lease ended does not bring the lock back,
    // and a lost read later than the lease does not stretch a late timer.
    const start = 13_000_000;
    const runtime = new FakeLeaseRuntime(start);
    const renderer = mountRestart(runtime);
    try {
        at(runtime, start, 120_000);
        assert.equal(blocked(renderer), false);
        publish(start, start + 121_000);
        assert.equal(blocked(renderer), false, 'released stays released');
    } finally { endSignal(); act(() => renderer.unmount()); }
    const late = new FakeLeaseRuntime(start);
    const lateRenderer = mountRestart(late);
    try {
        act(() => late.elapseWhileThrottled(130_000));
        publish(start, start + 130_000);
        act(() => late.wake());
        assert.equal(blocked(lateRenderer), false, 'a lost read after the lease is not a reason to keep it');
    } finally { endSignal(); act(() => lateRenderer.unmount()); }
});

test('set17b F1: the rule "released for good once no longer requested" holds inside the restart window', () => {
    const start = 14_000_000;
    const runtime = new FakeLeaseRuntime(start);
    const renderer = mountRestart(runtime);
    try {
        at(runtime, start, 25_000);
        publish(start, start + 25_000);
        at(runtime, start, 150_000);
        assert.equal(blocked(renderer), true);
        act(() => renderer.update(React.createElement(RestartProbe, { identity: F1_REQUEST, createdAt: start, runtime, requested: false })));
        assert.equal(blocked(renderer), false, 'no longer requested: released');
        act(() => renderer.update(React.createElement(RestartProbe, { identity: F1_REQUEST, createdAt: start, runtime, requested: true })));
        assert.equal(blocked(renderer), false, 'requested again inside the window: never re-locked');
    } finally { endSignal(); act(() => renderer.unmount()); }
});

test('set17b F1: a wall-clock rollback during the restart cannot lengthen the lock after reconnection', () => {
    const start = 15_000_000;
    const runtime = new FakeLeaseRuntime(start);
    const renderer = mountRestart(runtime);
    try {
        at(runtime, start, 25_000);
        publish(start, start + 25_000);
        at(runtime, start, 200_000);
        // The wall clock steps back 150 s (T+50 s on the wall, 200 s monotonic).
        runtime.time -= 150_000;
        publish(start, null);
        assert.equal(blocked(renderer), false, 'measured on the monotonic scale: past the lease');
    } finally { endSignal(); act(() => renderer.unmount()); }
});

test('set17b F1 control: the previous lease (no restart passed) ends at T+120 s in the middle of the outage', () => {
    // The same outage as the first F1 test with the restart withheld, which is
    // the lease before this change: the lock ends mid-outage.
    const start = 16_000_000;
    const runtime = new FakeLeaseRuntime(start);
    const renderer = mountRestart(runtime, { wired: false });
    try {
        at(runtime, start, 25_000);
        publish(start, start + 25_000);
        at(runtime, start, 119_999);
        assert.equal(blocked(renderer), true);
        at(runtime, start, 120_000);
        assert.equal(blocked(renderer), false, 'F1 as set17b measured it: the lock gone while the Panel is down');
    } finally { endSignal(); act(() => renderer.unmount()); }
});

test('set17b F1: the navigation deadline function and its bounds', () => {
    const start = 17_000_000;
    const lease = start + watchdog.SYSTEM_UPDATE_BLOCKING_DEADLINE_MS;
    const deadline = watchdog.systemUpdateNavigationDeadline;
    assert.equal(watchdog.SYSTEM_UPDATE_RESTART_NAVIGATION_LIMIT_MS, tracking.SYSTEM_UPDATE_EXPECTED_RESTART_MS, 'the limit is the restart window');
    assert.equal(deadline(start, null), lease);
    assert.equal(deadline(start, { lostSince: start + 25_000, until: start + 360_000 }), start + 360_000);
    assert.equal(deadline(start, { lostSince: start + 25_000, until: start + 9_999_999 }), start + 360_000, 'never past 6 minutes');
    assert.equal(deadline(start, { lostSince: start + 120_000, until: start + 360_000 }), lease, 'a lost read at or after the lease');
    assert.equal(deadline(start, { lostSince: start - 1, until: start + 360_000 }), lease, 'a lost read before the start');
    assert.equal(deadline(start, { lostSince: start + 25_000, until: start + 100_000 }), lease, 'never shorter than the lease');
    assert.equal(deadline(start, { lostSince: Number.NaN, until: start + 360_000 }), lease);
    assert.equal(deadline(start, { lostSince: start + 25_000, until: Number.POSITIVE_INFINITY }), lease);
    const select = restartSignal.systemUpdateNavigationRestart;
    const signal = { requestID: F1_REQUEST, startedAt: start, lostSince: start + 25_000 };
    assert.deepEqual(select(signal, F1_REQUEST, start), { lostSince: start + 25_000, until: start + 360_000 });
    assert.equal(select({ ...signal, lostSince: null }, F1_REQUEST, start), null, 'reads answer: no restart');
    assert.equal(select(signal, 'e'.repeat(32), start), null);
    assert.equal(select(signal, F1_REQUEST, start + 1), null);
    assert.equal(select(null, F1_REQUEST, start), null);
    assert.equal(select(signal, null, null), null);
});
