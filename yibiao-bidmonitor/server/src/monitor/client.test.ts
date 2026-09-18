import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { BidMonitorClient, MonitorClientError } from './client';

function response(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

describe('BidMonitorClient', () => {
  it('sends the internal token and a scoped user path', async () => {
    const requests: Array<{ url: string; init?: RequestInit }> = [];
    const client = new BidMonitorClient({
      baseUrl: 'http://127.0.0.1:8080',
      serviceToken: 'internal-token',
      fetchImpl: async (url, init) => {
        requests.push({ url: String(url), init });
        return response(200, { user_id: '7', is_running: false });
      },
    });

    const result = await client.status(7);
    assert.equal(result.user_id, '7');
    assert.equal(requests[0].url, 'http://127.0.0.1:8080/internal/users/7/status');
    assert.equal(new Headers(requests[0].init?.headers).get('X-BidMonitor-Service-Token'), 'internal-token');
  });

  it('marks service failures as retryable when appropriate', async () => {
    const client = new BidMonitorClient({
      baseUrl: 'http://127.0.0.1:8080/',
      serviceToken: 'internal-token',
      fetchImpl: async () => response(503, { detail: 'unavailable' }),
    });

    await assert.rejects(
      () => client.runOnce(7),
      (error: unknown) => {
        assert.ok(error instanceof MonitorClientError);
        assert.equal(error.status, 503);
        assert.equal(error.retryable, true);
        return true;
      },
    );
  });

  it('rejects invalid user ids before making a request', async () => {
    let called = false;
    const client = new BidMonitorClient({
      baseUrl: 'http://127.0.0.1:8080',
      serviceToken: 'internal-token',
      fetchImpl: async () => {
        called = true;
        return response(200, {});
      },
    });

    await assert.rejects(() => client.status('../7'), /positive integer/);
    assert.equal(called, false);
  });
});
