import { CSSProperties } from 'react';
import { theme } from '../theme';
import { gatewayLoginRequired } from './gateway-mode';

export function SessionBar() {
  if (!gatewayLoginRequired()) return null;

  async function logout() {
    await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' });
    window.location.reload();
  }

  return (
    <button
      type="button"
      onClick={() => { void logout(); }}
      aria-label="退出登录"
      data-tip="退出登录"
      className="cc-header-btn cc-tip cc-tip-r"
      style={logoutButton}
    >
      退出
    </button>
  );
}

const logoutButton: CSSProperties = {
  background: 'none',
  border: `0.5px solid ${theme.border}`,
  color: theme.textDim,
  cursor: 'pointer',
  padding: '2px 8px',
  borderRadius: 4,
  fontSize: 11,
  height: 22,
};
