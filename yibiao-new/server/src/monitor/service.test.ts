import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { MonitorService } from './service';

function createFakes() {
  const calls: Array<{ name: string; args: unknown[] }> = [];
  const client = {
    status: async (userId: unknown) => {
      calls.push({ name: 'status', args: [userId] });
      return { user_id: String(userId), is_running: false, current_task_running: false };
    },
    start: async (userId: unknown, runtime: unknown) => {
      calls.push({ name: 'start', args: [userId, runtime] });
      return { accepted: true };
    },
    stop: async (userId: unknown) => {
      calls.push({ name: 'stop', args: [userId] });
      return { accepted: true };
    },
    runOnce: async (userId: unknown, runtime: unknown) => {
      calls.push({ name: 'runOnce', args: [userId, runtime] });
      return { accepted: true };
    },
    updateConfig: async (userId: unknown, config: any) => {
      calls.push({ name: 'updateConfig', args: [userId, config] });
      return { config };
    },
    results: async (userId: unknown) => {
      calls.push({ name: 'results', args: [userId] });
      return {
        total: 1,
        offset: 0,
        limit: 50,
        items: [{ title: 'Bid', url: 'https://example.test/bid/1', source: 'test' }],
      };
    },
    logs: async (userId: unknown) => {
      calls.push({ name: 'logs', args: [userId] });
      return { logs: ['one'] };
    },
    clearHistory: async (userId: unknown) => {
      calls.push({ name: 'clearHistory', args: [userId] });
      return { success: true };
    },
  };
  const store = {
    getProfile: async (userId: number) => {
      calls.push({ name: 'getProfile', args: [userId] });
      return { config: { keywords: ['alpha'] } };
    },
    updateProfile: async (userId: number, config: unknown) => {
      calls.push({ name: 'updateProfile', args: [userId, config] });
      return { config };
    },
    saveBids: async (userId: number, runId: number | null, bids: unknown[]) => {
      calls.push({ name: 'saveBids', args: [userId, runId, bids] });
      return bids.length;
    },
    listBids: async () => [],
    listLogs: async () => [],
    listContacts: async (userId: number) => {
      calls.push({ name: 'listContacts', args: [userId] });
      return [{ channel: 'email', target: 'user@example.test', enabled: true }];
    },
    replaceContacts: async (userId: number, contacts: unknown[]) => {
      calls.push({ name: 'replaceContacts', args: [userId, contacts] });
    },
    clearHistory: async (userId: number) => {
      calls.push({ name: 'storeClearHistory', args: [userId] });
    },
  };
  return { client, store, calls };
}

describe('MonitorService', () => {
  it('uses the authenticated user id for downstream calls', async () => {
    const { client, store, calls } = createFakes();
    const service = new MonitorService(client, store);
    await service.status(7);
    await service.start(7);
    await service.stop(7);
    await service.runOnce(7);
    assert.deepEqual(calls.slice(0, 4).map((call) => call.args[0]), [7, 7, 7, 7]);
  });

  it('injects server-side runtime AI configuration only when starting a run', async () => {
    const { client, store, calls } = createFakes();
    const service = new MonitorService(client, store, () => ({
      ai_config: {
        enable: true,
        api_key: 'server-secret',
        base_url: 'https://model.example.test/chat/completions',
        model: 'model-a',
      },
    }));
    await service.start(7);
    await service.runOnce(7);
    const startCall = calls.find((call) => call.name === 'start');
    const runCall = calls.find((call) => call.name === 'runOnce');
    assert.equal((startCall?.args[1] as any).ai_config.api_key, 'server-secret');
    assert.equal((runCall?.args[1] as any).ai_config.model, 'model-a');
  });

  it('imports result fingerprints into the user scoped store', async () => {
    const { client, store, calls } = createFakes();
    const service = new MonitorService(client, store);
    const result = await service.results(7, 50, 0);
    const save = calls.find((call) => call.name === 'saveBids');
    assert.ok(save);
    assert.equal(save.args[0], 7);
    assert.equal((save.args[2] as Array<Record<string, unknown>>)[0].url, 'https://example.test/bid/1');
    assert.equal(result.items.length, 1);
  });

  it('updates both the internal service and platform profile', async () => {
    const { client, store, calls } = createFakes();
    const service = new MonitorService(client, store);
    await service.updateConfig(7, { keywords: ['alpha'], api_key: 'secret' });
    assert.equal(calls.find((call) => call.name === 'updateConfig')?.args[0], 7);
    assert.equal(calls.find((call) => call.name === 'updateProfile')?.args[0], 7);
    assert.equal(JSON.stringify(calls.find((call) => call.name === 'updateConfig')?.args[1]).includes('secret'), false);
  });

  it('clears remote and platform history in the same user scope', async () => {
    const { client, store, calls } = createFakes();
    const service = new MonitorService(client, store);
    await service.clearHistory(7);
    assert.deepEqual(calls.map((call) => call.args[0]), [7, 7]);
  });

  it('isolates contact reads and writes by authenticated user', async () => {
    const { client, store, calls } = createFakes();
    const service = new MonitorService(client, store);
    await service.getContacts(7);
    await service.updateContacts(7, [{ channel: 'email', target: 'user@example.test' }]);
    assert.equal(calls.find((call) => call.name === 'listContacts')?.args[0], 7);
    assert.equal(calls.find((call) => call.name === 'replaceContacts')?.args[0], 7);
  });
});
