import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { createMonitorStore } from './store';

function createPrismaDouble() {
  const calls: Array<{ model: string; method: string; args: any }> = [];
  const prisma = {
    monitorProfile: {
      findFirst: async (args: any) => {
        calls.push({ model: 'profile', method: 'findFirst', args });
        return null;
      },
      create: async (args: any) => {
        calls.push({ model: 'profile', method: 'create', args });
        return { id: 1, userId: args.data.userId, config: {} };
      },
      update: async (args: any) => {
        calls.push({ model: 'profile', method: 'update', args });
        return { id: 1, userId: args.where.id, config: args.data.config };
      },
    },
    monitorBid: {
      createMany: async (args: any) => {
        calls.push({ model: 'bid', method: 'createMany', args });
        return { count: args.data.length };
      },
      findMany: async (args: any) => {
        calls.push({ model: 'bid', method: 'findMany', args });
        return [];
      },
      deleteMany: async (args: any) => {
        calls.push({ model: 'bid', method: 'deleteMany', args });
        return { count: 1 };
      },
    },
    monitorLog: {
      findMany: async (args: any) => {
        calls.push({ model: 'log', method: 'findMany', args });
        return [];
      },
      deleteMany: async (args: any) => {
        calls.push({ model: 'log', method: 'deleteMany', args });
        return { count: 1 };
      },
    },
    monitorContact: {
      findMany: async (args: any) => {
        calls.push({ model: 'contact', method: 'findMany', args });
        return [];
      },
      deleteMany: async (args: any) => {
        calls.push({ model: 'contact', method: 'deleteMany', args });
        return { count: 1 };
      },
      createMany: async (args: any) => {
        calls.push({ model: 'contact', method: 'createMany', args });
        return { count: args.data.length };
      },
    },
  };
  return { prisma, calls };
}

describe('monitor store', () => {
  it('adds the authenticated user scope to every business query', async () => {
    const { prisma, calls } = createPrismaDouble();
    const store = createMonitorStore(prisma);

    await store.getProfile(7);
    await store.listBids(7, 10, 0);
    await store.listLogs(7, 10);
    await store.clearHistory(7);

    for (const call of calls) {
      if (call.model === 'profile' && call.method === 'create') {
        assert.equal(call.args.data.userId, 7);
      } else if (call.args.where) {
        assert.equal(call.args.where.userId, 7);
      }
    }
  });

  it('writes every imported bid with the requested user id', async () => {
    const { prisma, calls } = createPrismaDouble();
    const store = createMonitorStore(prisma);
    await store.saveBids(7, 9, [
      {
        fingerprint: 'fingerprint-1',
        title: 'Bid title',
        url: 'https://example.test/bid/1',
        source: 'test',
      },
    ]);

    const insert = calls.find((call) => call.model === 'bid' && call.method === 'createMany');
    assert.ok(insert);
    assert.deepEqual(insert.args.data[0], {
      userId: 7,
      runId: 9,
      fingerprint: 'fingerprint-1',
      title: 'Bid title',
      url: 'https://example.test/bid/1',
      source: 'test',
      publishDate: null,
      purchaser: null,
      content: null,
    });
    assert.equal(insert.args.skipDuplicates, true);
  });
});
