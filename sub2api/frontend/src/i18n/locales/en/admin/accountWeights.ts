export default {
  accountWeights: {
    title: 'Account Weights',
    description: 'View scheduler score snapshots and the runtime factors used to select accounts.',
    refresh: 'Refresh',
    searchPlaceholder: 'Search by account name or platform',
    sortDescending: 'Descending',
    sortAscending: 'Ascending',
    allStatuses: 'All statuses',
    status: { active: 'Active', inactive: 'Inactive', error: 'Error' },
    sort: {
      base: 'Base score', sticky: 'Sticky score', priority: 'Priority', priorityFactor: 'Priority factor',
      loadFactor: 'Load factor', queueFactor: 'Queue factor', errorRateFactor: 'Error-rate factor',
      ttftFactor: 'TTFT factor', resetFactor: 'Reset factor', quotaHeadroomFactor: 'Quota headroom factor',
      upstreamCostFactor: 'Upstream cost factor', concurrency: 'Current concurrency', capacity: 'Capacity',
      multiplier: 'Account multiplier', name: 'Name'
    },
    summary: { total: 'Total accounts', scored: 'Scored accounts', schedulable: 'Schedulable accounts', highestBase: 'Highest base score' },
    columns: {
      account: 'Account', platformType: 'Platform / Type', baseScore: 'Base score', stickyScore: 'Sticky score',
      priorityRaw: 'Priority (raw)', priorityFactor: 'Priority factor', concurrencyCapacity: 'Concurrency / Capacity',
      loadFactor: 'Load factor', queueFactor: 'Queue factor', errorRateFactor: 'Error-rate factor', ttftFactor: 'TTFT factor',
      resetFactor: 'Reset factor', quotaHeadroomFactor: 'Quota headroom factor', upstreamCostFactor: 'Upstream cost factor',
      multiplier: 'Account multiplier', status: 'Status', groups: 'Groups'
    },
    baseUrlMissing: 'Base URL is not configured',
    groupFallback: 'Group {id}',
    empty: 'No accounts match the current filters',
    factorNote: 'Factors are normalized scheduler values from 0 to 1; higher is better. A lower raw priority value has higher priority. Error-rate and TTFT factors in the admin snapshot currently use neutral scheduler defaults.'
  }
}
