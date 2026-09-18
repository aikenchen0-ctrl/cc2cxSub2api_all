import { createCipheriv, createDecipheriv, createHash, randomBytes } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { homedir } from 'node:os';
import { sessionSecret } from './config.ts';

interface VaultRecord {
  userId: string;
  email: string;
  refreshToken: string;
  userApiKey: string;
}

function vaultDir(): string {
  return join(homedir(), '.openchatcut', 'gateway-vault');
}

function vaultPath(userId: string): string {
  const safe = userId.replace(/[^a-zA-Z0-9_-]/g, '_').slice(0, 80);
  return join(vaultDir(), `${safe}.bin`);
}

function keyMaterial(): Buffer {
  return createHash('sha256').update(sessionSecret()).digest();
}

function encrypt(text: string): Buffer {
  const iv = randomBytes(12);
  const cipher = createCipheriv('aes-256-gcm', keyMaterial(), iv);
  const encrypted = Buffer.concat([cipher.update(text, 'utf8'), cipher.final()]);
  const tag = cipher.getAuthTag();
  return Buffer.concat([iv, tag, encrypted]);
}

function decrypt(buf: Buffer): string {
  const iv = buf.subarray(0, 12);
  const tag = buf.subarray(12, 28);
  const data = buf.subarray(28);
  const decipher = createDecipheriv('aes-256-gcm', keyMaterial(), iv);
  decipher.setAuthTag(tag);
  return Buffer.concat([decipher.update(data), decipher.final()]).toString('utf8');
}

export async function saveVault(record: VaultRecord): Promise<void> {
  await mkdir(vaultDir(), { recursive: true, mode: 0o700 });
  await writeFile(vaultPath(record.userId), encrypt(JSON.stringify(record)), { mode: 0o600 });
}

export async function loadVault(userId: string): Promise<VaultRecord | null> {
  try {
    const raw = await readFile(vaultPath(userId));
    const parsed = JSON.parse(decrypt(raw)) as VaultRecord;
    if (!parsed?.userId || !parsed.userApiKey) return null;
    return parsed;
  } catch {
    return null;
  }
}
