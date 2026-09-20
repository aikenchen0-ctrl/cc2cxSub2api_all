/** One-line JSON audit. Never log passwords, JWT, or sk. */

export function audit(event: string, fields: Record<string, string | number | boolean | undefined>): void {
  const payload: Record<string, string | number | boolean> = { ts: new Date().toISOString(), event };
  for (const [key, value] of Object.entries(fields)) {
    if (value === undefined) continue;
    payload[key] = value;
  }
  console.log(`[occ-audit] ${JSON.stringify(payload)}`);
}
