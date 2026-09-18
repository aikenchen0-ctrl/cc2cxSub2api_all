import { occKeyNamePrefix } from './config.ts';
import { createUserKey, listKeys } from './sub2api-client.ts';
import { loadVault, saveVault } from './vault.ts';

export async function provisionUserApiKey(input: {
  userId: string;
  email: string;
  accessToken: string;
  refreshToken: string;
}): Promise<string> {
  const existing = await loadVault(input.userId);
  if (existing?.userApiKey) {
    await saveVault({
      ...existing,
      email: input.email,
      refreshToken: input.refreshToken || existing.refreshToken,
    });
    return existing.userApiKey;
  }
  const prefix = occKeyNamePrefix();
  const keys = await listKeys(input.accessToken);
  const named = keys.find((key) => key.name.startsWith(prefix) && key.key);
  if (named?.key) {
    await saveVault({
      userId: input.userId,
      email: input.email,
      refreshToken: input.refreshToken,
      userApiKey: named.key,
    });
    return named.key;
  }
  const userApiKey = await createUserKey(input.accessToken, `${prefix}-${input.userId}`.slice(0, 64));
  await saveVault({
    userId: input.userId,
    email: input.email,
    refreshToken: input.refreshToken,
    userApiKey,
  });
  return userApiKey;
}
