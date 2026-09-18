import type { FastifyInstance, FastifyPluginOptions, FastifyReply, FastifyRequest } from 'fastify';
import { getUserId } from '../auth/middleware';
import { MonitorService, monitorErrorStatus, type MonitorClientLike } from './service';
import type { MonitorStoreLike } from './service';
import type { MonitorConfig } from './types';

type MonitorRouteService = Pick<MonitorService, 'status' | 'start' | 'stop' | 'runOnce' | 'getConfig' | 'updateConfig' | 'results' | 'logs' | 'clearHistory'>;

interface MonitorRoutesOptions extends FastifyPluginOptions {
  service?: MonitorRouteService;
  monitorClient?: MonitorClientLike;
  monitorStore?: MonitorStoreLike;
}

type BodyRequest = FastifyRequest & { body: unknown };

function serviceFrom(app: FastifyInstance, opts: MonitorRoutesOptions): MonitorRouteService {
  if (opts.service) return opts.service;
  const decorated = app as FastifyInstance & { monitorService?: MonitorRouteService };
  if (decorated.monitorService) return decorated.monitorService;
  if (opts.monitorClient && opts.monitorStore) return new MonitorService(opts.monitorClient, opts.monitorStore);
  throw new Error('monitor service is not configured');
}

function sendMonitorError(reply: FastifyReply, error: unknown): void {
  reply.code(monitorErrorStatus(error)).send({ error: error instanceof Error ? error.message : 'monitor service request failed' });
}

export async function monitorRoutes(app: FastifyInstance, opts: MonitorRoutesOptions): Promise<void> {
  const service = serviceFrom(app, opts);

  app.get('/monitor/status', async (req, reply) => {
    try {
      return await service.status(getUserId(req));
    } catch (error) {
      sendMonitorError(reply, error);
    }
  });

  app.post('/monitor/start', async (req, reply) => {
    try {
      return await service.start(getUserId(req));
    } catch (error) {
      sendMonitorError(reply, error);
    }
  });

  app.post('/monitor/stop', async (req, reply) => {
    try {
      return await service.stop(getUserId(req));
    } catch (error) {
      sendMonitorError(reply, error);
    }
  });

  app.post('/monitor/run-once', async (req, reply) => {
    try {
      return await service.runOnce(getUserId(req));
    } catch (error) {
      sendMonitorError(reply, error);
    }
  });

  app.get('/monitor/config', async (req, reply) => {
    try {
      return { config: await service.getConfig(getUserId(req)) };
    } catch (error) {
      sendMonitorError(reply, error);
    }
  });

  app.put('/monitor/config', async (req, reply) => {
    try {
      const body = (req as BodyRequest).body;
      if (!body || typeof body !== 'object' || Array.isArray(body)) {
        reply.code(400).send({ error: 'config must be an object' });
        return;
      }
      return await service.updateConfig(getUserId(req), body as MonitorConfig);
    } catch (error) {
      sendMonitorError(reply, error);
    }
  });

  app.get('/monitor/results', async (req, reply) => {
    try {
      const query = (req.query || {}) as { limit?: string; offset?: string };
      const limit = Number(query.limit ?? 50);
      const offset = Number(query.offset ?? 0);
      if (!Number.isFinite(limit) || !Number.isFinite(offset)) {
        reply.code(400).send({ error: 'invalid pagination' });
        return;
      }
      return await service.results(getUserId(req), limit, offset);
    } catch (error) {
      sendMonitorError(reply, error);
    }
  });

  app.get('/monitor/logs', async (req, reply) => {
    try {
      const query = (req.query || {}) as { limit?: string };
      const limit = Number(query.limit ?? 100);
      if (!Number.isFinite(limit)) {
        reply.code(400).send({ error: 'invalid limit' });
        return;
      }
      return await service.logs(getUserId(req), limit);
    } catch (error) {
      sendMonitorError(reply, error);
    }
  });

  app.delete('/monitor/history', async (req, reply) => {
    try {
      return await service.clearHistory(getUserId(req));
    } catch (error) {
      sendMonitorError(reply, error);
    }
  });
}
