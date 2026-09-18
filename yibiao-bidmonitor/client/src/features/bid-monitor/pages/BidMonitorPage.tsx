import { useEffect, useMemo, useState } from 'react';
import {
  useMonitorClearHistory,
  useMonitorConfig,
  useMonitorLogs,
  useMonitorResults,
  useMonitorRunOnce,
  useMonitorSaveConfig,
  useMonitorStart,
  useMonitorStatus,
  useMonitorStop,
  type MonitorConfig,
} from '../api/monitor';
import { useToast } from '../../../shared/ui';

const DEFAULT_CONFIG: MonitorConfig = {
  keywords: [],
  exclude_keywords: [],
  must_contain_keywords: [],
  notify_method: 'none',
  interval_minutes: 30,
  crawler: { enabled_sites: [], use_selenium: false },
};

function asLines(value: string[] | undefined): string {
  return (value ?? []).join('\n');
}

function parseLines(value: string): string[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
    .slice(0, 200);
}

function formatTime(value: string | null | undefined): string {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function BidMonitorPage() {
  const { showToast } = useToast();
  const status = useMonitorStatus();
  const configQuery = useMonitorConfig();
  const config = configQuery.data;
  const results = useMonitorResults();
  const logs = useMonitorLogs();
  const start = useMonitorStart();
  const stop = useMonitorStop();
  const runOnce = useMonitorRunOnce();
  const saveConfig = useMonitorSaveConfig();
  const clearHistory = useMonitorClearHistory();
  const [keywords, setKeywords] = useState('');
  const [excluded, setExcluded] = useState('');
  const [required, setRequired] = useState('');
  const [interval, setInterval] = useState('30');
  const [selenium, setSelenium] = useState(false);

  useEffect(() => {
    if (!config) return;
    setKeywords(asLines(config.keywords));
    setExcluded(asLines(config.exclude_keywords));
    setRequired(asLines(config.must_contain_keywords));
    setInterval(String(config.interval_minutes ?? 30));
    setSelenium(Boolean(config.crawler?.use_selenium));
  }, [config]);

  const currentStatus = status.data;
  const busy = start.isPending || stop.isPending || runOnce.isPending;
  const savedConfig = useMemo<MonitorConfig>(() => ({
    ...(config ?? DEFAULT_CONFIG),
    keywords: parseLines(keywords),
    exclude_keywords: parseLines(excluded),
    must_contain_keywords: parseLines(required),
    interval_minutes: Math.max(1, Math.min(1440, Number(interval) || 30)),
    crawler: {
      ...(config?.crawler ?? DEFAULT_CONFIG.crawler),
      use_selenium: selenium,
    },
  }), [config, excluded, interval, keywords, required, selenium]);

  const runAction = async (action: () => Promise<unknown>, successMessage: string) => {
    try {
      await action();
      showToast(successMessage, 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Request failed', 'error');
    }
  };

  const save = () => void runAction(() => saveConfig.mutateAsync(savedConfig), 'Configuration saved');
  const clear = () => {
    if (!window.confirm('Clear all stored monitor history?')) return;
    void runAction(() => clearHistory.mutateAsync(), 'History cleared');
  };

  return (
    <div className="bid-monitor-page">
      <div className="bid-monitor-shell">
        <header className="bid-monitor-head">
          <div>
            <span className="section-kicker">Bid intelligence</span>
            <h2>Bid Monitor</h2>
            <p>Collect notices, match keywords, and track notification runs.</p>
          </div>
          <div className="bid-monitor-actions">
            <button type="button" className="secondary-action" onClick={save} disabled={saveConfig.isPending}>Save config</button>
            <button type="button" className="secondary-action" onClick={() => void runAction(() => runOnce.mutateAsync(), 'Run queued')} disabled={busy}>Run once</button>
            {currentStatus?.is_running ? (
              <button type="button" className="primary-action" onClick={() => void runAction(() => stop.mutateAsync(), 'Monitor stopped')} disabled={busy}>Stop</button>
            ) : (
              <button type="button" className="primary-action" onClick={() => void runAction(() => start.mutateAsync(), 'Monitor started')} disabled={busy}>Start</button>
            )}
          </div>
        </header>

        <section className="bid-monitor-status" aria-label="Monitor status">
          <div><span>Status</span><strong className={currentStatus?.is_running ? 'is-online' : ''}>{currentStatus?.is_running ? 'Running' : 'Stopped'}</strong></div>
          <div><span>Current task</span><strong>{currentStatus?.current_task_running ? 'Collecting' : 'Idle'}</strong></div>
          <div><span>Last run</span><strong>{formatTime(currentStatus?.last_run_time)}</strong></div>
          <div><span>Matches</span><strong>{currentStatus?.last_result?.new_count == null ? '—' : String(currentStatus.last_result.new_count)}</strong></div>
        </section>

        <div className="bid-monitor-grid">
          <section className="bid-monitor-section bid-monitor-config">
            <div className="bid-monitor-section-head"><div><span className="section-kicker">Scope</span><h3>Matching rules</h3></div></div>
            <label className="bid-monitor-field"><span>Include keywords</span><textarea value={keywords} onChange={(event) => setKeywords(event.target.value)} placeholder="One keyword per line" /></label>
            <label className="bid-monitor-field"><span>Exclude keywords</span><textarea value={excluded} onChange={(event) => setExcluded(event.target.value)} placeholder="One keyword per line" /></label>
            <label className="bid-monitor-field"><span>Required keywords</span><textarea value={required} onChange={(event) => setRequired(event.target.value)} placeholder="All terms must match" /></label>
            <div className="bid-monitor-inline-fields">
              <label className="bid-monitor-field"><span>Interval (minutes)</span><input type="number" min="1" max="1440" value={interval} onChange={(event) => setInterval(event.target.value)} /></label>
              <label className="bid-monitor-toggle"><input type="checkbox" checked={selenium} onChange={(event) => setSelenium(event.target.checked)} /><span>Use browser mode</span></label>
            </div>
          </section>

          <section className="bid-monitor-section bid-monitor-results">
            <div className="bid-monitor-section-head"><div><span className="section-kicker">Feed</span><h3>Matched notices</h3></div><button type="button" className="text-action" onClick={clear} disabled={clearHistory.isPending}>Clear history</button></div>
            {results.isLoading ? <div className="bid-monitor-empty">Loading results...</div> : results.data?.items.length ? (
              <div className="bid-monitor-table-wrap"><table className="bid-monitor-table"><thead><tr><th>Title</th><th>Source</th><th>Date</th></tr></thead><tbody>{results.data.items.map((item) => <tr key={`${item.url}-${item.title}`}><td><a href={item.url} target="_blank" rel="noreferrer">{item.title || item.url}</a></td><td>{item.source || '—'}</td><td>{item.publish_date || '—'}</td></tr>)}</tbody></table></div>
            ) : <div className="bid-monitor-empty">No matched notices yet.</div>}
          </section>
        </div>

        <section className="bid-monitor-section bid-monitor-logs">
          <div className="bid-monitor-section-head"><div><span className="section-kicker">Trace</span><h3>Recent logs</h3></div></div>
          {logs.data?.length ? <pre>{logs.data.join('\n')}</pre> : <div className="bid-monitor-empty">No logs yet.</div>}
        </section>
      </div>
    </div>
  );
}

export default BidMonitorPage;
