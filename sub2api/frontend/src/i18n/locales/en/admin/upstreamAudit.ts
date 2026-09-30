export default {
  upstreamAudit: {
    title: 'Upstream Audit',
    description: 'Scan upstream account models and verify model identity with the ModelTrace fingerprint library.',
    eyebrow: 'MODELTRACE / UPSTREAM MODEL IDENTITY',
    heroTitle: 'Check whether upstream output matches the declared model',
    heroDescription: 'Sub2API will enumerate every upstream account and model, probe only models covered by ModelTrace, then compare outputs with reference fingerprints.',
    startScan: 'Scan account models',
    running: 'Audit running',
    loadFailed: 'Failed to load upstream audit data',
    refreshAccounts: 'Refresh account count',
    metrics: { accounts: 'Upstream accounts', supported: 'Fingerprint models', matched: 'Models to audit', lastRun: 'Latest audit', loading: 'Loading', pending: 'Generated after scan', never: 'Not run yet' },
    flowTitle: 'Audit workflow',
    flowDescription: 'Each run shows the account, declared model, predicted candidate, similarity, valid output count, and probe attempts.',
    flow: {
      syncTitle: 'Sync accounts and models', syncDescription: 'Read all Sub2API accounts and refresh each upstream model list.',
      matchTitle: 'Intersect fingerprint coverage', matchDescription: 'Keep GPT and Claude models covered by ModelTrace; mark the rest unsupported.',
      probeTitle: 'Send probe requests', probeDescription: 'Call the declared model with its own account credentials, parse numeric sequences, and retain diagnostic counts only.',
      scoreTitle: 'Score fingerprint similarity', scoreDescription: 'Compare with the unified bank and report the top candidate, closed-set weight, and evidence strength.'
    },
    tabs: { queue: 'Audit queue', library: 'Fingerprint library', settings: 'Audit policy' },
    queue: {
      title: 'Account model audit queue', description: 'Matched account models will be queued here after a scan.',
      emptyTitle: 'No audit jobs yet', emptyDescription: 'Scan account models to sync every account and automatically test models covered by the fingerprint library.',
      scanErrors: '{count} accounts could not load their model list',
      columns: { account: 'Upstream account', declared: 'Declared model', prediction: 'Fingerprint candidate', similarity: 'Similarity', status: 'Status', action: 'Action' }
    },
    progress: { title: 'Current audit progress', accounts: 'Accounts {done}/{total}', models: 'Models {done}/{total}', running: 'Running', completed: 'Completed' },
    status: { pending: 'Pending', probing: 'Probing', verified: 'Matched', mismatch: 'Mismatch', failed: 'Failed' },
    library: {
      title: 'ModelTrace fingerprint library', description: 'ModelTrace unified fingerprint snapshot built on 2026-09-23 with 16 candidate models.',
      gpt: 'GPT family', claude: 'Claude family', models: 'models', snapshot: 'Fingerprint SHA-256', calibration: 'This snapshot is not independently calibrated for same-context or multilingual use.'
    },
    settings: {
      title: 'Default audit policy', description: 'The audit currently uses these fixed controls and retains probe counts and scoring results.',
      scopeTitle: 'Scan scope', scopeDescription: 'Scan every Sub2API account and intersect all models reported for each upstream.',
      retriesTitle: 'Probe retries', retriesDescription: 'Try up to six requests per model, target three valid numeric sequences, and calibrate with the valid responses available.',
      languagesTitle: 'Probe language', languagesDescription: 'Use ModelTrace numeric-choice probes; the bank is not independently calibrated for multilingual use.',
      thresholdTitle: 'Result wording', thresholdDescription: 'Show candidate weights and evidence strength without presenting experimental similarity as proof of replacement.'
    },
    noticeTitle: 'Interpretation boundary',
    notice: 'ModelTrace is a closed-set fingerprint comparison. A high similarity score means the output resembles a library candidate; it does not prove provider substitution, degradation, or fraud. Confirm alerts with retesting, account logs, and upstream response metadata.'
  }
}
