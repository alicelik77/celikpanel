// alpha.128 (G30): the connections one sending server may hold at once. The
// field is shown and sent only when the server said its value, so an older
// Agent's policy is never saved with a made-up number.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('../src/components/PostfixManagement.tsx', import.meta.url), 'utf8');

test('the connection limit is read from the server and sent back only when it was read', () => {
  assert.match(source, /connections: typeof policy\.connection_limit === 'number' \? policy\.connection_limit : null/);
  assert.match(source, /connection_limit: values\.connections \?\? undefined/);
  assert.match(source, /\{form\.connections !== null && \(/);
});
