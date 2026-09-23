import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { normalizeMonitorUserId, sanitizeMonitorConfig } from './scope';

describe('monitor scope helpers', () => {
  it('normalizes only positive integer user ids', () => {
    assert.equal(normalizeMonitorUserId(7), '7');
    assert.equal(normalizeMonitorUserId('12'), '12');
    assert.throws(() => normalizeMonitorUserId(0), /positive integer/);
    assert.throws(() => normalizeMonitorUserId('../12'), /positive integer/);
    assert.throws(() => normalizeMonitorUserId('user'), /positive integer/);
  });

  it('removes secrets recursively and limits large text values', () => {
    const input = {
      keywords: ['alpha'],
      ai_config: { api_key: 'secret', model: 'model-a' },
      contacts: [{ password: 'mail-secret', target: 'user@example.test' }],
      content: 'x'.repeat(5000),
    };
    const sanitized = sanitizeMonitorConfig(input) as Record<string, unknown>;
    assert.deepEqual(sanitized.ai_config, { model: 'model-a' });
    assert.deepEqual(sanitized.contacts, [{ target: 'user@example.test' }]);
    assert.equal((sanitized.content as string).length, 4000);
  });
});
