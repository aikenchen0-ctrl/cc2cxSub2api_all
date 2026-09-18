import type { FastifyInstance, FastifyPluginOptions, FastifyRequest } from 'fastify';
import type { PrismaClient } from '@prisma/client';

const PROVIDER = 'sub2api';
const SUBJECT_PATTERN = /^[^\s/\\]{1,256}$/;

interface IdentityBody {
  userId?: unknown;
  subject?: unknown;
  email?: unknown;
  displayName?: unknown;
  avatarUrl?: unknown;
}

function text(value: unknown, maxLength: number): string | undefined {
  if (typeof value !== 'string') return undefined;
  const trimmed = value.trim();
  return trimmed ? trimmed.slice(0, maxLength) : undefined;
}

function subjectFrom(req: FastifyRequest): string {
  const raw = (req.params as { subject?: string }).subject || '';
  return decodeURIComponent(raw);
}

export async function ssoIdentityRoutes(app: FastifyInstance, _opts: FastifyPluginOptions): Promise<void> {
  const prisma = (app as unknown as { prisma: PrismaClient }).prisma;

  app.get('/sso/identities', async () => {
    const items = await (prisma as any).userIdentity.findMany({
      where: { provider: PROVIDER },
      orderBy: { createdAt: 'desc' },
      include: {
        user: {
          select: { id: true, username: true, displayName: true, status: true },
        },
      },
    });
    return { items };
  });

  app.post('/sso/identities', async (req, reply) => {
    const body = ((req.body || {}) as IdentityBody);
    const userId = Number(body.userId);
    const subject = text(body.subject, 256) || '';
    if (!Number.isSafeInteger(userId) || userId <= 0 || !SUBJECT_PATTERN.test(subject)) {
      return reply.code(400).send({ error: 'userId and subject are required' });
    }

    const user = await prisma.user.findUnique({ where: { id: userId } });
    if (!user) return reply.code(404).send({ error: 'user not found' });
    if (user.status !== 'active') return reply.code(409).send({ error: 'user is not active' });

    const where = { provider_subject: { provider: PROVIDER, subject } };
    const existing = await (prisma as any).userIdentity.findUnique({ where });
    const metadata = {
      email: text(body.email, 512),
      displayName: text(body.displayName, 512),
      avatarUrl: text(body.avatarUrl, 2048),
    };
    if (existing && existing.userId !== userId) {
      return reply.code(409).send({ error: 'subject is already bound to another user' });
    }
    const identity = existing
      ? await (prisma as any).userIdentity.update({ where, data: metadata, include: { user: true } })
      : await (prisma as any).userIdentity.create({
        data: { provider: PROVIDER, subject, userId, ...metadata },
        include: { user: true },
      });
    return reply.code(existing ? 200 : 201).send({ identity });
  });

  app.delete('/sso/identities/:subject', async (req, reply) => {
    const subject = subjectFrom(req);
    if (!SUBJECT_PATTERN.test(subject)) return reply.code(400).send({ error: 'invalid subject' });
    try {
      await (prisma as any).userIdentity.delete({
        where: { provider_subject: { provider: PROVIDER, subject } },
      });
      return { success: true };
    } catch {
      return reply.code(404).send({ error: 'identity not found' });
    }
  });
}
