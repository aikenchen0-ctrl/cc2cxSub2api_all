import Fastify from 'fastify';
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { ssoIdentityRoutes } from './sso-identities';

function createPrismaDouble() {
  const calls: Array<{ method: string; args: any }> = [];
  const users = new Map([
    [7, { id: 7, username: 'user-7', displayName: 'User 7', status: 'active' }],
    [8, { id: 8, username: 'user-8', displayName: 'User 8', status: 'active' }],
  ]);
  const identities = new Map<string, any>();
  const prisma = {
    user: {
      findUnique: async (args: any) => {
        calls.push({ method: 'user.findUnique', args });
        return users.get(args.where.id) ?? null;
      },
    },
    userIdentity: {
      findMany: async (args: any) => {
        calls.push({ method: 'userIdentity.findMany', args });
        return [...identities.values()];
      },
      findUnique: async (args: any) => {
        calls.push({ method: 'userIdentity.findUnique', args });
        return identities.get(`${args.where.provider_subject.provider}:${args.where.provider_subject.subject}`) ?? null;
      },
      create: async (args: any) => {
        calls.push({ method: 'userIdentity.create', args });
        const row = { id: identities.size + 1, ...args.data, user: users.get(args.data.userId) };
        identities.set(`${row.provider}:${row.subject}`, row);
        return row;
      },
      update: async (args: any) => {
        calls.push({ method: 'userIdentity.update', args });
        const key = `${args.where.provider_subject.provider}:${args.where.provider_subject.subject}`;
        const row = { ...identities.get(key), ...args.data };
        identities.set(key, row);
        return row;
      },
      delete: async (args: any) => {
        calls.push({ method: 'userIdentity.delete', args });
        identities.delete(`${args.where.provider_subject.provider}:${args.where.provider_subject.subject}`);
        return { id: 1 };
      },
    },
  };
  return { prisma, calls, identities };
}

describe('SSO identity admin routes', () => {
  it('lists identities and binds a Sub2API subject to an active local user', async () => {
    const { prisma, calls, identities } = createPrismaDouble();
    const app = Fastify();
    app.decorate('prisma', prisma);
    await app.register(ssoIdentityRoutes, { prefix: '/api' });

    const create = await app.inject({
      method: 'POST',
      url: '/api/sso/identities',
      payload: { userId: 7, subject: 'external-7', email: 'user@example.test', displayName: 'External User' },
    });
    assert.equal(create.statusCode, 201);
    assert.equal(create.json().identity.subject, 'external-7');
    assert.equal(identities.size, 1);

    const list = await app.inject({ method: 'GET', url: '/api/sso/identities' });
    assert.equal(list.statusCode, 200);
    assert.equal(list.json().items[0].userId, 7);
    assert.equal(calls.some((call) => call.method === 'userIdentity.findMany'), true);
    await app.close();
  });

  it('rejects a subject already bound to another user and invalid users', async () => {
    const { prisma } = createPrismaDouble();
    const app = Fastify();
    app.decorate('prisma', prisma);
    await app.register(ssoIdentityRoutes, { prefix: '/api' });

    const first = await app.inject({ method: 'POST', url: '/api/sso/identities', payload: { userId: 7, subject: 'external-7' } });
    assert.equal(first.statusCode, 201);
    const conflict = await app.inject({ method: 'POST', url: '/api/sso/identities', payload: { userId: 8, subject: 'external-7' } });
    assert.equal(conflict.statusCode, 409);
    const missing = await app.inject({ method: 'POST', url: '/api/sso/identities', payload: { userId: 9, subject: 'external-8' } });
    assert.equal(missing.statusCode, 404);
    await app.close();
  });

  it('removes an identity by provider and subject', async () => {
    const { prisma } = createPrismaDouble();
    const app = Fastify();
    app.decorate('prisma', prisma);
    await app.register(ssoIdentityRoutes, { prefix: '/api' });
    await app.inject({ method: 'POST', url: '/api/sso/identities', payload: { userId: 7, subject: 'external-7' } });
    const remove = await app.inject({ method: 'DELETE', url: '/api/sso/identities/external-7' });
    assert.equal(remove.statusCode, 200);
    await app.close();
  });
});
