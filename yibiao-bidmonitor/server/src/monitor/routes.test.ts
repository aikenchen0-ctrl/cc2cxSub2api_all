import Fastify from 'fastify';
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { monitorRoutes } from './routes';

describe('monitor routes', () => {
  it('takes user scope from the authenticated request', async () => {
    const calls: number[] = [];
    const app = Fastify();
    await app.register(async (protectedApp) => {
      protectedApp.addHook('onRequest', async (req) => {
        (req as typeof req & { user: { id: number; role: string } }).user = { id: 7, role: 'user' };
      });
      await protectedApp.register(monitorRoutes, {
        prefix: '/api',
        service: {
          status: async (userId: number) => {
            calls.push(userId);
            return { user_id: String(userId), is_running: false, current_task_running: false };
          },
        } as any,
      });
    });

    const response = await app.inject({ method: 'GET', url: '/api/monitor/status' });
    assert.equal(response.statusCode, 200);
    assert.deepEqual(calls, [7]);
    await app.close();
  });

  it('takes contact scope from the authenticated request', async () => {
    const calls: Array<{ name: string; userId: number; contacts?: unknown }> = [];
    const app = Fastify();
    await app.register(async (protectedApp) => {
      protectedApp.addHook('onRequest', async (req) => {
        (req as typeof req & { user: { id: number; role: string } }).user = { id: 9, role: 'user' };
      });
      await protectedApp.register(monitorRoutes, {
        prefix: '/api',
        service: {
          getContacts: async (userId: number) => {
            calls.push({ name: 'get', userId });
            return [{ channel: 'email', target: 'user@example.test', enabled: true }];
          },
          updateContacts: async (userId: number, contacts: unknown[]) => {
            calls.push({ name: 'update', userId, contacts });
            return { success: true };
          },
        } as any,
      });
    });

    const list = await app.inject({ method: 'GET', url: '/api/monitor/contacts' });
    assert.equal(list.statusCode, 200);
    const update = await app.inject({
      method: 'PUT',
      url: '/api/monitor/contacts',
      payload: { contacts: [{ channel: 'email', target: 'new@example.test', enabled: true }] },
    });
    assert.equal(update.statusCode, 200);
    assert.deepEqual(calls.map((call) => call.userId), [9, 9]);
    await app.close();
  });
});
