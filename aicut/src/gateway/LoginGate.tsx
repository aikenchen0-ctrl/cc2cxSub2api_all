import { FormEvent, ReactNode, useEffect, useState } from 'react';
import { theme } from '../theme';

interface AuthUser {
  id: string;
  email: string;
}

async function fetchMe(): Promise<AuthUser | null> {
  const response = await fetch('/api/auth/me', { credentials: 'same-origin' });
  if (response.status === 401) return null;
  if (!response.ok) throw new Error('无法读取登录状态');
  const body = await response.json() as { user?: AuthUser };
  return body.user ?? null;
}

async function fetchAuthConfig(): Promise<{ openRegister: boolean }> {
  const response = await fetch('/api/auth/config', { credentials: 'same-origin' });
  if (!response.ok) return { openRegister: false };
  return await response.json() as { openRegister: boolean };
}

export function LoginGate({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null | undefined>(undefined);
  const [openRegister, setOpenRegister] = useState(false);
  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void fetchMe().then(setUser).catch(() => setUser(null));
    void fetchAuthConfig().then((config) => setOpenRegister(Boolean(config.openRegister))).catch(() => {});
  }, []);

  if (user === undefined) {
    return (
      <div style={{
        height: '100vh', display: 'grid', placeItems: 'center',
        background: theme.bg, color: theme.textDim, fontFamily: 'Geist, system-ui, sans-serif',
      }}>
        正在检查登录…
      </div>
    );
  }

  if (user) return <>{children}</>;

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const path = mode === 'register' ? '/api/auth/register' : '/api/auth/login';
      const response = await fetch(path, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });
      const body = await response.json() as { error?: string; user?: AuthUser };
      if (!response.ok) throw new Error(body.error || (mode === 'register' ? '注册失败' : '登录失败'));
      if (!body.user) throw new Error('响应缺少用户');
      setUser(body.user);
    } catch (err) {
      setError(err instanceof Error ? err.message : '请求失败');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={{
      height: '100vh', display: 'grid', placeItems: 'center',
      background: theme.bg, color: theme.text, fontFamily: 'Geist, system-ui, sans-serif',
    }}>
      <form onSubmit={onSubmit} style={{
        width: 360, display: 'grid', gap: 12, padding: 28, borderRadius: 12,
        background: theme.panel, border: `0.5px solid ${theme.border}`,
      }}>
        <div style={{ fontSize: 16, fontWeight: 500 }}>AI剪辑</div>
        <div style={{ fontSize: 13, color: theme.textDim }}>
          {mode === 'register' ? '注册 Sub2API 账号后进入工程。' : '使用 Sub2API 账号登录后才能打开工程。'}
        </div>
        <input
          type="email"
          required
          placeholder="邮箱"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          style={{
            padding: '10px 12px', borderRadius: 8, border: `0.5px solid ${theme.border}`,
            background: theme.inset, color: theme.text, fontSize: 13,
          }}
        />
        <input
          type="password"
          required
          minLength={mode === 'register' ? 8 : undefined}
          placeholder={mode === 'register' ? '密码（至少 8 位）' : '密码'}
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          style={{
            padding: '10px 12px', borderRadius: 8, border: `0.5px solid ${theme.border}`,
            background: theme.inset, color: theme.text, fontSize: 13,
          }}
        />
        {error ? <div style={{ color: theme.danger, fontSize: 12 }}>{error}</div> : null}
        <button
          type="submit"
          disabled={busy}
          style={{
            padding: '10px 12px', borderRadius: 8, border: 0, cursor: 'pointer',
            background: theme.accent, color: theme.onAccent, fontSize: 13,
          }}
        >
          {busy ? (mode === 'register' ? '注册中…' : '登录中…') : (mode === 'register' ? '注册并进入' : '登录')}
        </button>
        {openRegister ? (
          <button
            type="button"
            onClick={() => {
              setMode(mode === 'login' ? 'register' : 'login');
              setError('');
            }}
            style={{
              padding: 0, border: 0, background: 'transparent', cursor: 'pointer',
              color: theme.textDim, fontSize: 12, textAlign: 'left',
            }}
          >
            {mode === 'login' ? '没有账号？注册' : '已有账号？登录'}
          </button>
        ) : null}
      </form>
    </div>
  );
}
