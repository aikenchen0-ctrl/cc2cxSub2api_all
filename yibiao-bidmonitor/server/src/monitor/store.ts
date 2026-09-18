export interface MonitorBidInput {
  fingerprint: string;
  title: string;
  url: string;
  source: string;
  publishDate?: string | null;
  purchaser?: string | null;
  content?: string | null;
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
  monitorLog: {
    findMany(args: unknown): Promise<any[]>;
    deleteMany(args: unknown): Promise<{ count: number }>;
  };
  monitorNotification?: { deleteMany(args: unknown): Promise<{ count: number }> };
  monitorRun?: { deleteMany(args: unknown): Promise<{ count: number }> };
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

  async function clearHistory(userId: number): Promise<void> {
    await prisma.monitorBid.deleteMany({ where: { userId } });
    await prisma.monitorLog.deleteMany({ where: { userId } });
    await prisma.monitorNotification?.deleteMany({ where: { userId } });
    await prisma.monitorRun?.deleteMany({ where: { userId } });
  }

  return { getProfile, updateProfile, saveBids, listBids, listLogs, clearHistory };
}

export type MonitorStore = ReturnType<typeof createMonitorStore>;
