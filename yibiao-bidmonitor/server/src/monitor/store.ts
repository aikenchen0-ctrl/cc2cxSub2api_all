export interface MonitorBidInput {
  fingerprint: string;
  title: string;
  url: string;
  source: string;
  publishDate?: string | null;
  purchaser?: string | null;
  content?: string | null;
}

export interface MonitorRunInput {
  userId: number;
  profileId?: number | null;
  status: string;
}

export interface MonitorNotificationInput {
  fingerprint: string;
  channel: string;
  status: string;
  lastError?: string | null;
}

interface MonitorPrismaLike {
  monitorProfile: {
    findFirst(args: unknown): Promise<any>;
    create(args: unknown): Promise<any>;
    update(args: unknown): Promise<any>;
  };
  monitorBid: {
    createMany(args: unknown): Promise<{ count: number }>;
    findMany(args: unknown): Promise<any[]>;
    deleteMany(args: unknown): Promise<{ count: number }>;
  };
  monitorRun?: {
    create(args: unknown): Promise<any>;
    update(args: unknown): Promise<any>;
    findMany(args: unknown): Promise<any[]>;
    deleteMany(args: unknown): Promise<{ count: number }>;
  };
  monitorLog: {
    createMany?(args: unknown): Promise<{ count: number }>;
    findMany(args: unknown): Promise<any[]>;
    deleteMany(args: unknown): Promise<{ count: number }>;
  };
  monitorContact: {
    findMany(args: unknown): Promise<any[]>;
    deleteMany(args: unknown): Promise<{ count: number }>;
    createMany(args: unknown): Promise<{ count: number }>;
  };
  monitorNotification?: {
    deleteMany(args: unknown): Promise<{ count: number }>;
    createMany(args: unknown): Promise<{ count: number }>;
  };
}

const MAX_TITLE_LENGTH = 1000;
const MAX_URL_LENGTH = 2000;
const MAX_SOURCE_LENGTH = 200;
const MAX_CONTENT_LENGTH = 20000;

function text(value: unknown, maxLength: number): string | null {
  if (value === null || value === undefined) return null;
  return String(value).slice(0, maxLength);
}

export function createMonitorStore(prisma: MonitorPrismaLike) {
  async function getProfile(userId: number): Promise<any> {
    const current = await prisma.monitorProfile.findFirst({
      where: { userId },
      orderBy: { id: 'asc' },
    });
    if (current) return current;
    return prisma.monitorProfile.create({
      data: {
        userId,
        name: 'Default profile',
        enabled: false,
        intervalMinutes: 30,
        config: {},
      },
    });
  }

  async function updateProfile(userId: number, config: Record<string, unknown>): Promise<any> {
    const profile = await getProfile(userId);
    return prisma.monitorProfile.update({
      where: { id: profile.id },
      data: { config },
    });
  }

  async function saveBids(userId: number, runId: number | null, bids: MonitorBidInput[]): Promise<number> {
    if (!bids.length) return 0;
    const result = await prisma.monitorBid.createMany({
      data: bids.map((bid) => ({
        userId,
        runId,
        fingerprint: text(bid.fingerprint, 128) || '',
        title: text(bid.title, MAX_TITLE_LENGTH) || '',
        url: text(bid.url, MAX_URL_LENGTH) || '',
        source: text(bid.source, MAX_SOURCE_LENGTH) || '',
        publishDate: text(bid.publishDate, 100),
        purchaser: text(bid.purchaser, 500),
        content: text(bid.content, MAX_CONTENT_LENGTH),
      })),
      skipDuplicates: true,
    });
    return result.count;
  }

  async function createRun(input: MonitorRunInput): Promise<any> {
    if (!prisma.monitorRun) return { id: null };
    return prisma.monitorRun.create({
      data: {
        userId: input.userId,
        profileId: input.profileId ?? null,
        status: input.status,
      },
    });
  }

  async function finishRun(
    runId: number,
    status: string,
    counts: Record<string, unknown> | null,
    error: string | null,
  ): Promise<any> {
    if (!prisma.monitorRun) return null;
    return prisma.monitorRun.update({
      where: { id: runId },
      data: {
        status,
        counts,
        error,
        finishedAt: new Date(),
      },
    });
  }

  async function saveLogs(userId: number, runId: number, logs: string[]): Promise<number> {
    if (!logs.length || !prisma.monitorLog.createMany) return 0;
    const result = await prisma.monitorLog.createMany({
      data: logs.slice(-300).map((message) => ({ userId, runId, level: 'info', message: String(message).slice(0, 4000) })),
    });
    return result.count;
  }

  async function saveNotifications(userId: number, notifications: MonitorNotificationInput[]): Promise<number> {
    if (!prisma.monitorNotification) return 0;
    if (!notifications.length) return 0;
    const fingerprints = [...new Set(notifications.map((item) => item.fingerprint).filter(Boolean))];
    if (!fingerprints.length) return 0;
    const bids = await prisma.monitorBid.findMany({ where: { userId, fingerprint: { in: fingerprints } }, select: { id: true, fingerprint: true } });
    const bidIds = new Map(bids.map((bid: any) => [String(bid.fingerprint), bid.id]));
    const rows = notifications
      .map((item) => ({
        userId,
        bidId: bidIds.get(item.fingerprint),
        channel: item.channel,
        status: item.status,
        attempts: 1,
        lastError: item.lastError ?? null,
      }))
      .filter((item) => item.bidId != null);
    if (!rows.length) return 0;
    const result = await prisma.monitorNotification.createMany({ data: rows, skipDuplicates: true });
    return result.count;
  }

  async function listRuns(userId: number, limit = 20): Promise<any[]> {
    if (!prisma.monitorRun) return [];
    return prisma.monitorRun.findMany({
      where: { userId },
      orderBy: { startedAt: 'desc' },
      take: Math.max(1, Math.min(100, Math.trunc(limit))),
    });
  }

  async function listBids(userId: number, limit = 50, offset = 0): Promise<any[]> {
    return prisma.monitorBid.findMany({
      where: { userId },
      orderBy: { matchedAt: 'desc' },
      skip: Math.max(0, Math.trunc(offset)),
      take: Math.max(1, Math.min(200, Math.trunc(limit))),
    });
  }

  async function listLogs(userId: number, limit = 100): Promise<any[]> {
    return prisma.monitorLog.findMany({
      where: { userId },
      orderBy: { createdAt: 'desc' },
      take: Math.max(1, Math.min(300, Math.trunc(limit))),
    });
  }

  async function listContacts(userId: number): Promise<any[]> {
    return prisma.monitorContact.findMany({
      where: { userId, enabled: true },
      orderBy: { createdAt: 'asc' },
      select: { id: true, channel: true, target: true, enabled: true, config: true },
    });
  }

  async function replaceContacts(userId: number, contacts: Array<Record<string, unknown>>): Promise<void> {
    const allowedChannels = new Set(['email', 'sms', 'wechat', 'voice']);
    const rows = contacts
      .filter((item) => item && typeof item === 'object')
      .map((item) => ({
        channel: String(item.channel || '').trim().toLowerCase(),
        target: String(item.target || '').trim().slice(0, 512),
        enabled: item.enabled !== false,
      }))
      .filter((item) => allowedChannels.has(item.channel) && item.target.length > 0)
      .slice(0, 20);
    await prisma.monitorContact.deleteMany({ where: { userId } });
    if (rows.length) await prisma.monitorContact.createMany({ data: rows.map((row) => ({ userId, ...row })) });
  }

  async function clearHistory(userId: number): Promise<void> {
    await prisma.monitorBid.deleteMany({ where: { userId } });
    await prisma.monitorLog.deleteMany({ where: { userId } });
    await prisma.monitorNotification?.deleteMany({ where: { userId } });
    await prisma.monitorRun?.deleteMany({ where: { userId } });
  }

  return {
    getProfile,
    updateProfile,
    saveBids,
    createRun,
    finishRun,
    saveLogs,
    saveNotifications,
    listRuns,
    listBids,
    listLogs,
    listContacts,
    replaceContacts,
    clearHistory,
  };
}

export type MonitorStore = ReturnType<typeof createMonitorStore>;
