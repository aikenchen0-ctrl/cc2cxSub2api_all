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
});
