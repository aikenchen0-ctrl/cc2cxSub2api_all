import { useEffect, useMemo, useState } from 'react';
import {
  useMonitorClearHistory,
  useMonitorConfig,
  useMonitorContacts,
  useMonitorSaveSites,
  useMonitorSites,
  useMonitorTestAi,
  useMonitorTestNotification,
  useMonitorLogs,
  useMonitorResults,
  useMonitorRunOnce,
  useMonitorRuns,
  useMonitorSaveConfig,
  useMonitorSaveContacts,
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
  const contactsQuery = useMonitorContacts();
  const sitesQuery = useMonitorSites();
  const results = useMonitorResults();
  const logs = useMonitorLogs();
  const runs = useMonitorRuns();
  const start = useMonitorStart();
  const stop = useMonitorStop();
  const runOnce = useMonitorRunOnce();
  const saveConfig = useMonitorSaveConfig();
  const saveContacts = useMonitorSaveContacts();
  const saveSites = useMonitorSaveSites();
  const testNotification = useMonitorTestNotification();
  const testAi = useMonitorTestAi();
  const clearHistory = useMonitorClearHistory();
  const [keywords, setKeywords] = useState('');
  const [excluded, setExcluded] = useState('');
  const [required, setRequired] = useState('');
  const [interval, setInterval] = useState('30');
  const [selenium, setSelenium] = useState(false);
  const [notificationEmail, setNotificationEmail] = useState('');
  const [notificationPhone, setNotificationPhone] = useState('');
  const [notificationWechat, setNotificationWechat] = useState('');
  const [notificationVoice, setNotificationVoice] = useState('');
  const [notificationMethods, setNotificationMethods] = useState<string[]>([]);
  const [enabledSites, setEnabledSites] = useState<string[]>([]);
  const [customSites, setCustomSites] = useState<Array<{ name: string; url: string }>>([]);
  const [customSiteName, setCustomSiteName] = useState('');
  const [customSiteUrl, setCustomSiteUrl] = useState('');
  const [aiEnabled, setAiEnabled] = useState(false);
  const [aiPrompt, setAiPrompt] = useState('');

  useEffect(() => {
    if (!config) return;
    setKeywords(asLines(config.keywords));
    setExcluded(asLines(config.exclude_keywords));
    setRequired(asLines(config.must_contain_keywords));
    setInterval(String(config.interval_minutes ?? 30));
    setSelenium(Boolean(config.crawler?.use_selenium));
    setNotificationMethods(parseMethods(config.notify_method));
    setAiEnabled(Boolean(config.ai_enabled));
    setAiPrompt(String(config.ai_prompt ?? ''));
  }, [config]);

  useEffect(() => {
    const contacts = contactsQuery.data ?? [];
    setNotificationEmail(contacts.find((item) => item.channel === 'email')?.target ?? '');
    setNotificationPhone(contacts.find((item) => item.channel === 'sms')?.target ?? '');
    setNotificationWechat(contacts.find((item) => item.channel === 'wechat')?.target ?? '');
    setNotificationVoice(contacts.find((item) => item.channel === 'voice')?.target ?? '');
  }, [contactsQuery.data]);

  useEffect(() => {
    if (!sitesQuery.data) return;
    setEnabledSites(sitesQuery.data.sites.filter((site) => site.enabled).map((site) => site.key));
    setCustomSites(sitesQuery.data.custom_sites ?? []);
  }, [sitesQuery.data]);

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
        enabled_sites: enabledSites,
        use_selenium: selenium,
      },
    notify_method: notificationMethods.length ? notificationMethods.join(',') : 'none',
    ai_enabled: aiEnabled,
    ai_prompt: aiPrompt.trim(),
  }), [aiEnabled, aiPrompt, config, enabledSites, excluded, interval, keywords, notificationMethods, required, selenium]);

  const runAction = async (action: () => Promise<unknown>, successMessage: string) => {
    try {
      await action();
      showToast(successMessage, 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Request failed', 'error');
    }
  };

  const save = () => void runAction(() => saveConfig.mutateAsync(savedConfig), 'Configuration saved');
  const saveNotificationTargets = () => void runAction(
    () => saveContacts.mutateAsync([
      ...(notificationEmail.trim() ? [{ channel: 'email', target: notificationEmail.trim(), enabled: true }] : []),
      ...(notificationPhone.trim() ? [{ channel: 'sms', target: notificationPhone.trim(), enabled: true }] : []),
      ...(notificationWechat.trim() ? [{ channel: 'wechat', target: notificationWechat.trim(), enabled: true }] : []),
      ...(notificationVoice.trim() ? [{ channel: 'voice', target: notificationVoice.trim(), enabled: true }] : []),
    ]),
    'Notification targets saved',
  );
  const saveSiteConfig = () => void runAction(
    () => saveSites.mutateAsync({ enabled_sites: enabledSites, custom_sites: customSites }),
    'Site configuration saved',
  );
  const addCustomSite = () => {
    const name = customSiteName.trim();
    const url = customSiteUrl.trim();
    if (!name || !url) return;
    setCustomSites((items) => [...items, { name, url }].slice(0, 50));
    setCustomSiteName('');
    setCustomSiteUrl('');
  };
  const testChannel = (channel: 'email' | 'sms' | 'wechat' | 'voice', target: string) => {
    if (!target.trim()) return;
    void runAction(() => testNotification.mutateAsync({ channel, target: target.trim() }), 'Notification test completed');
  };
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
            <div className="bid-monitor-notifications">
              <div className="bid-monitor-section-head"><div><span className="section-kicker">Notify</span><h3>Notification targets</h3></div><button type="button" className="text-action" onClick={saveNotificationTargets} disabled={saveContacts.isPending}>Save</button></div>
              <label className="bid-monitor-field"><span>Email address</span><input type="email" value={notificationEmail} onChange={(event) => setNotificationEmail(event.target.value)} placeholder="Optional" /></label>
              <label className="bid-monitor-field"><span>Phone number</span><input type="tel" value={notificationPhone} onChange={(event) => setNotificationPhone(event.target.value)} placeholder="Optional" /></label>
              <label className="bid-monitor-field"><span>WeChat target label</span><input value={notificationWechat} onChange={(event) => setNotificationWechat(event.target.value)} placeholder="Optional; credential stays on server" /></label>
              <label className="bid-monitor-field"><span>Voice phone number</span><input type="tel" value={notificationVoice} onChange={(event) => setNotificationVoice(event.target.value)} placeholder="Optional" /></label>
              <div className="bid-monitor-channel-list">
                {['email', 'sms', 'wechat', 'voice'].map((channel) => <label className="bid-monitor-toggle" key={channel}><input type="checkbox" checked={notificationMethods.includes(channel)} onChange={(event) => setNotificationMethods((items) => event.target.checked ? [...new Set([...items, channel])] : items.filter((item) => item !== channel))} /><span>{channel}</span></label>)}
              </div>
              <div className="bid-monitor-test-actions">
                <button type="button" className="text-action" onClick={() => testChannel('email', notificationEmail)}>Test email</button>
                <button type="button" className="text-action" onClick={() => testChannel('sms', notificationPhone)}>Test SMS</button>
                <button type="button" className="text-action" onClick={() => testChannel('wechat', notificationWechat)}>Test WeChat</button>
                <button type="button" className="text-action" onClick={() => testChannel('voice', notificationVoice)}>Test voice</button>
              </div>
              <p className="bid-monitor-note">Channel credentials are managed by the server.</p>
            </div>
            <div className="bid-monitor-ai">
              <div className="bid-monitor-section-head"><div><span className="section-kicker">Filter</span><h3>AI relevance filter</h3></div><button type="button" className="text-action" onClick={() => void runAction(() => testAi.mutateAsync(), 'AI test completed')} disabled={testAi.isPending}>Test AI</button></div>
              <label className="bid-monitor-toggle"><input type="checkbox" checked={aiEnabled} onChange={(event) => setAiEnabled(event.target.checked)} /><span>Enable AI second-pass filtering</span></label>
              <label className="bid-monitor-field"><span>Custom filter prompt</span><textarea value={aiPrompt} onChange={(event) => setAiPrompt(event.target.value)} placeholder="Optional" /></label>
            </div>
          </section>

          <section className="bid-monitor-section bid-monitor-sites">
            <div className="bid-monitor-section-head"><div><span className="section-kicker">Sources</span><h3>Monitored sites</h3></div><button type="button" className="text-action" onClick={saveSiteConfig} disabled={saveSites.isPending}>Save</button></div>
            <div className="bid-monitor-site-list">{(sitesQuery.data?.sites ?? []).map((site) => <label className="bid-monitor-site-row" key={site.key}><input type="checkbox" checked={enabledSites.includes(site.key)} onChange={(event) => setEnabledSites((items) => event.target.checked ? [...items, site.key] : items.filter((item) => item !== site.key))} /><span><strong>{site.name}</strong><small>{site.url}</small></span></label>)}</div>
            <div className="bid-monitor-custom-sites"><h4>Custom sites</h4>{customSites.map((site, index) => <div className="bid-monitor-custom-site" key={`${site.url}-${index}`}><span>{site.name}<small>{site.url}</small></span><button type="button" className="text-action" onClick={() => setCustomSites((items) => items.filter((_, itemIndex) => itemIndex !== index))}>Remove</button></div>)}<div className="bid-monitor-custom-form"><input value={customSiteName} onChange={(event) => setCustomSiteName(event.target.value)} placeholder="Site name" /><input value={customSiteUrl} onChange={(event) => setCustomSiteUrl(event.target.value)} placeholder="https://example.com" /><button type="button" className="secondary-action" onClick={addCustomSite}>Add site</button></div></div>
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
        <section className="bid-monitor-section bid-monitor-runs">
          <div className="bid-monitor-section-head"><div><span className="section-kicker">History</span><h3>Run history</h3></div></div>
          {runs.data?.length ? <div className="bid-monitor-table-wrap"><table className="bid-monitor-table"><thead><tr><th>Status</th><th>Started</th><th>Finished</th><th>Error</th></tr></thead><tbody>{runs.data.map((run) => <tr key={run.id}><td>{run.status}</td><td>{formatTime(run.startedAt)}</td><td>{formatTime(run.finishedAt)}</td><td>{run.error || '—'}</td></tr>)}</tbody></table></div> : <div className="bid-monitor-empty">No run history yet.</div>}
        </section>
      </div>
    </div>
  );
}

function parseMethods(value: string | undefined): string[] {
  const methods = String(value || '').split(',').map((item) => item.trim()).filter(Boolean);
  return methods.includes('both') ? ['email', 'sms'] : methods;
}

export default BidMonitorPage;
