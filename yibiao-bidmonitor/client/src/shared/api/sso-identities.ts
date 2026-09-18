import { http } from './http';

export interface SsoIdentityUser {
  id: number;
  username: string;
  displayName: string | null;
  status: string;
}

export interface SsoIdentity {
  id: number;
  userId: number;
  provider: string;
  subject: string;
  email: string | null;
  displayName: string | null;
  avatarUrl: string | null;
  createdAt: string;
  updatedAt: string;
  user?: SsoIdentityUser | null;
}

export async function fetchSsoIdentities(): Promise<SsoIdentity[]> {
  const { data } = await http.get<{ items: SsoIdentity[] }>('/sso/identities');
  return data.items;
}

export async function bindSsoIdentity(input: {
  userId: number;
  subject: string;
  email?: string;
  displayName?: string;
  avatarUrl?: string;
}): Promise<SsoIdentity> {
  const { data } = await http.post<{ identity: SsoIdentity }>('/sso/identities', input);
  return data.identity;
}

export async function removeSsoIdentity(subject: string): Promise<void> {
  await http.delete(`/sso/identities/${encodeURIComponent(subject)}`);
}
