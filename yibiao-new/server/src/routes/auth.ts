import type { FastifyInstance, FastifyPluginOptions, FastifyRequest, FastifyReply } from 'fastify';
import type { PrismaClient } from '@prisma/client';
import bcrypt from 'bcryptjs';
import { randomBytes } from 'node:crypto';
import {
  signInitialPasswordChangeToken,
  signToken,
  TokenPurposeError,
  verifyInitialPasswordChangeToken,
} from '../auth/middleware';
import type { JwtPayload } from '../auth/middleware';
import { validateInitialPassword } from '../auth/initialPassword';
import { parseModules } from '../auth/permissions';
import { consumeSsoHandoff, createSsoHandoff, verifySub2ApiTicket, type SsoUser } from '../auth/sso';

// 公开路由（无需登录）：POST /api/login（手机号登录）、POST /api/register（手机号注册，待审批）。
const PHONE_RE = /^1[3-9]\d{9}$/;

interface AuthUserSource {
  id: number;
  username: string;
  displayName: string | null;
  role: string;
  status: string;
  phone: string | null;
  department: string | null;
  modules: string;
}

function toAuthUser(user: AuthUserSource) {
  return {
    id: user.id,
    username: user.username,
    displayName: user.displayName,
    role: user.role,
    status: user.status,
    phone: user.phone,
    department: user.department,
    modules: parseModules(user.modules),
  };
}

async function resolveSsoUser(prisma: PrismaClient, external: SsoUser): Promise<AuthUserSource | null> {
  const identity = await (prisma as any).userIdentity.findUnique({
    where: { provider_subject: { provider: 'sub2api', subject: external.sub } },
    include: { user: true },
  });
  if (identity?.user) {
    return identity.user.status === 'active' ? identity.user : null;
  }
  if (process.env.SUB2API_SSO_AUTO_PROVISION !== 'true') return null;

  const username = `sso:${external.sub}`.slice(0, 180);
  const existing = await prisma.user.findUnique({ where: { username } });
  if (existing) return null;
  const password = await bcrypt.hash(randomBytes(32).toString('hex'), 10);
  try {
    return await prisma.user.create({
      data: {
        username,
        password,
        displayName: external.displayName || external.username || external.email || username,
        role: 'user',
        status: 'active',
        modules: JSON.stringify(['bid-monitor']),
        identities: {
          create: {
            provider: 'sub2api',
            subject: external.sub,
            email: external.email,
            displayName: external.displayName,
            avatarUrl: external.avatarUrl,
          },
        },
      },
    });
  } catch {
    return null;
  }
}

export async function authRoutes(app: FastifyInstance, _opts: FastifyPluginOptions): Promise<void> {
  const prisma = (app as unknown as { prisma: PrismaClient }).prisma;

  app.get('/auth/sso/callback', async (req, reply) => {
    const query = (req.query || {}) as { ticket?: string };
    const verified = verifySub2ApiTicket(query.ticket || '');
    reply.header('Cache-Control', 'no-store');
    reply.header('Referrer-Policy', 'no-referrer');
    if (!verified) return reply.redirect('/?sso_error=invalid');
    const handoff = createSsoHandoff(verified.user);
    const params = new URLSearchParams({ sso_handoff: handoff, redirect: verified.next });
    return reply.redirect(`/?${params.toString()}`);
  });

  app.post('/auth/sso/exchange', async (req, reply) => {
    const body = (req.body || {}) as { handoff?: unknown };
    const handoff = typeof body.handoff === 'string' ? body.handoff : '';
    const external = consumeSsoHandoff(handoff);
    if (!external) return reply.code(401).send({ error: 'SSO handoff expired' });
    const user = await resolveSsoUser(prisma, external);
    if (!user) return reply.code(403).send({ error: 'SSO user is not provisioned or is disabled' });
    return {
      token: signToken({ id: user.id, username: user.username, role: user.role }),
      user: toAuthUser(user),
    };
  });

  // POST /login { username(手机号), password } → { token, user }。
  // username 列对手机号注册用户存的就是手机号；seed 的 admin 同理。
  app.post('/login', async (req: FastifyRequest, reply: FastifyReply) => {
    const { username, password } = ((req.body ?? {}) as { username?: string; password?: string });
    if (!username || !password) {
      return reply.code(400).send({ error: '手机号和密码必填' });
    }
    const user = await prisma.user.findUnique({ where: { username } });
    if (!user) {
      return reply.code(401).send({ error: '手机号或密码错误' });
    }
    const ok = await bcrypt.compare(password, user.password);
    if (!ok) {
      return reply.code(401).send({ error: '手机号或密码错误' });
    }
    // status 门禁：pending→审批中，disabled→已停用。非 active 一律拦截在前端登录之外。
    if (user.status === 'pending') {
      return reply.code(403).send({ error: '账号待管理员审批' });
    }
    if (user.status === 'disabled') {
      return reply.code(403).send({ error: '账号已停用，请联系管理员' });
    }
    if (user.mustChangePassword) {
      if (user.role !== 'admin') {
        return reply.code(403).send({ error: '账号状态异常，请联系管理员' });
      }
      return {
        password_change_required: true,
        password_change_token: signInitialPasswordChangeToken({
          id: user.id,
          username: user.username,
          role: user.role,
        }),
        expires_in: 600,
      };
    }
    const token = signToken({ id: user.id, username: user.username, role: user.role });
    return {
      token,
      user: toAuthUser(user),
    };
  });

  app.post('/change-initial-password', async (req: FastifyRequest, reply: FastifyReply) => {
    const header = req.headers.authorization;
    if (!header || !header.startsWith('Bearer ') || !header.slice(7)) {
      return reply.code(401).send({ error: '改密凭证无效或已过期，请重新登录' });
    }

    let payload: JwtPayload;
    try {
      payload = verifyInitialPasswordChangeToken(header.slice(7));
    } catch (error) {
      if (error instanceof TokenPurposeError && error.code === 'access-token-not-allowed') {
        return reply.code(403).send({ error: '该凭证不能用于初始密码修改' });
      }
      return reply.code(401).send({ error: '改密凭证无效或已过期，请重新登录' });
    }

    const user = await prisma.user.findUnique({ where: { id: payload.id } });
    if (!user || user.status !== 'active') {
      return reply.code(401).send({ error: '账号不可用或已停用' });
    }
    if (user.role !== 'admin') {
      return reply.code(403).send({ error: '只有管理员可修改初始密码' });
    }
    if (!user.mustChangePassword) {
      return reply.code(409).send({ error: '初始密码已经修改，请使用新密码登录' });
    }

    const { newPassword, confirmPassword } = ((req.body ?? {}) as {
      newPassword?: unknown;
      confirmPassword?: unknown;
    });
    if (typeof newPassword !== 'string' || typeof confirmPassword !== 'string') {
      return reply.code(400).send({ error: '新密码和确认密码必填' });
    }
    if (newPassword !== confirmPassword) {
      return reply.code(400).send({ error: '两次输入的密码不一致' });
    }
    const [passwordError] = validateInitialPassword(newPassword);
    if (passwordError) {
      return reply.code(400).send({ error: passwordError });
    }
    if (await bcrypt.compare(newPassword, user.password)) {
      return reply.code(400).send({ error: '新密码不能与当前密码相同' });
    }

    const hashed = await bcrypt.hash(newPassword, 10);
    const updated = await prisma.user.updateMany({
      where: { id: user.id, mustChangePassword: true },
      data: { password: hashed, mustChangePassword: false },
    });
    if (updated.count === 0) {
      return reply.code(409).send({ error: '初始密码已经修改，请使用新密码登录' });
    }

    const changedUser = await prisma.user.findUnique({ where: { id: user.id } });
    if (!changedUser || changedUser.status !== 'active') {
      return reply.code(401).send({ error: '账号不可用或已停用' });
    }
    if (changedUser.role !== 'admin') {
      return reply.code(403).send({ error: '只有管理员可修改初始密码' });
    }
    return {
      token: signToken({ id: changedUser.id, username: changedUser.username, role: changedUser.role }),
      user: toAuthUser(changedUser),
    };
  });

  // POST /register { phone, password, displayName, department } → 建 status=pending / role=user 账号（username=手机号）。
  app.post('/register', async (req: FastifyRequest, reply: FastifyReply) => {
    const { phone, password, displayName, department } = ((req.body ?? {}) as {
      phone?: string;
      password?: string;
      displayName?: string;
      department?: string;
    });
    if (!phone || !PHONE_RE.test(phone)) {
      return reply.code(400).send({ error: '手机号格式不正确' });
    }
    if (!displayName || !displayName.trim()) {
      return reply.code(400).send({ error: '请输入姓名' });
    }
    if (!password || password.length < 8) {
      return reply.code(400).send({ error: '密码至少 8 位' });
    }
    const exists = await prisma.user.findUnique({ where: { phone } });
    if (exists) {
      return reply.code(409).send({ error: '该手机号已注册' });
    }
    const hashed = await bcrypt.hash(password, 10);
    await prisma.user.create({
      data: {
        username: phone,
        phone,
        password: hashed,
        displayName: displayName.trim(),
        department: department?.trim() || null,
        role: 'user',
        status: 'pending',
      },
    });
    return { success: true, message: '注册成功，请等待管理员审批后登录' };
  });
}

// 受保护路由（需登录）：GET /api/me → 返回当前用户最新信息（权限即时生效的拉取端点）。
// 挂在 protectedApp 块（verifyToken 已注入 req.user），不进公开 authRoutes。
// 返回形状与 /login 的 user 完全一致；status 非 active → 401（前端据此 logout）。
export async function meRoutes(app: FastifyInstance, _opts: FastifyPluginOptions): Promise<void> {
  const prisma = (app as unknown as { prisma: PrismaClient }).prisma;

  app.get('/me', async (req: FastifyRequest, reply: FastifyReply) => {
    const payload = (req as FastifyRequest & { user: JwtPayload }).user;
    const user = await prisma.user.findUnique({ where: { id: payload.id } });
    if (!user || user.status === 'disabled') {
      return reply.code(401).send({ error: '账号不可用或已停用' });
    }
    return {
      user: toAuthUser(user),
    };
  });
}
