import assert from 'node:assert/strict';
import { truncateText, extractTags, formatPublishedAt } from './app.js';

function test(name, fn) {
  try {
    fn();
    console.log(`ok - ${name}`);
  } catch (err) {
    console.error(`fail - ${name}`);
    console.error(err);
    process.exit(1);
  }
}

test('truncateText keeps short texts intact', () => {
  const input = 'short text';
  const result = truncateText(input, 20);
  assert.equal(result.text, input);
  assert.equal(result.tooltip, input);
});

test('truncateText truncates long text and preserves tooltip', () => {
  const input = 'this is a very long summary that should be truncated in the UI table for better readability';
  const result = truncateText(input, 30);
  assert.ok(result.text.endsWith('…'));
  assert.equal(result.tooltip, input);
  assert.ok(result.text.length <= 31);
});

test('extractTags returns sorted tag keys from JSON map', () => {
  const map = { Remote: true, PartTime: true };
  const tags = extractTags(map);
  assert.deepEqual(tags, ['PartTime', 'Remote']);
});

test('extractTags handles empty or invalid inputs', () => {
  assert.deepEqual(extractTags(null), []);
  assert.deepEqual(extractTags({}), []);
});

test('formatPublishedAt formats valid ISO time', () => {
  const text = formatPublishedAt('2024-06-11T08:00:00Z');
  assert.ok(text.includes('2024'));
});

test('formatPublishedAt handles bad input', () => {
  assert.equal(formatPublishedAt(''), '');
  assert.equal(formatPublishedAt('invalid'), '');
});
