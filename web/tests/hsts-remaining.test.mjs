import assert from 'node:assert/strict';
import test from 'node:test';
import { formatHSTSRemainingSeconds } from '../src/lib/hstsRemaining.ts';

// The time left of an HSTS retirement is a duration, not a rounded relative
// time: ninety minutes read "in 2 hours" (14 Oct 2026 screenshot round).
const MIN = 60;
const HOUR = 3_600;
const DAY = 86_400;

test('ninety minutes is hours and minutes, not "2 hours"', () => {
  assert.equal(formatHSTSRemainingSeconds(90 * MIN, 'en'), '1 h 30 min');
  assert.equal(formatHSTSRemainingSeconds(90 * MIN, 'tr'), '1 sa 30 dk');
});

test('under a day: hours and minutes; whole units drop the empty part', () => {
  assert.equal(formatHSTSRemainingSeconds(2 * HOUR, 'en'), '2 h');
  assert.equal(formatHSTSRemainingSeconds(45 * MIN, 'tr'), '45 dk');
  assert.equal(formatHSTSRemainingSeconds(23 * HOUR + 59 * MIN, 'en'), '23 h 59 min');
});

test('a day or more: days and hours, with singular and plural days in English', () => {
  assert.equal(formatHSTSRemainingSeconds(DAY, 'en'), '1 day');
  assert.equal(formatHSTSRemainingSeconds(2 * DAY + 3 * HOUR, 'en'), '2 days 3 h');
  assert.equal(formatHSTSRemainingSeconds(2 * DAY + 3 * HOUR, 'tr'), '2 gün 3 sa');
});

test('the time shown is never shorter than what is left', () => {
  assert.equal(formatHSTSRemainingSeconds(90 * MIN + 1, 'en'), '1 h 31 min');
  assert.equal(formatHSTSRemainingSeconds(DAY - 1, 'en'), '1 day');
  assert.equal(formatHSTSRemainingSeconds(3 * HOUR + DAY + 1, 'en'), '1 day 4 h');
});

test('under a minute counts seconds, and a spent or invalid value shows one second', () => {
  assert.equal(formatHSTSRemainingSeconds(59, 'tr'), '59 sn');
  assert.equal(formatHSTSRemainingSeconds(0, 'en'), '1 s');
  assert.equal(formatHSTSRemainingSeconds(Number.NaN, 'en'), '1 s');
});
