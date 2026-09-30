import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  duplicateAccount,
  createSparkShadow,
  showSuccess,
  showError,
  startModelRefreshJob,
  getModelRefreshJob
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  duplicateAccount: vi.fn(),
  createSparkShadow: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  startModelRefreshJob: vi.fn(),
  getModelRefreshJob: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      startModelRefreshJob,
      getModelRefreshJob,
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      duplicate: duplicateAccount,
      getUpstreamBillingProbeSettings: async () => ({ enabled: true, interval_minutes: 30 }),
      createSparkShadow,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess, showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token' })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const mountView = () =>
  mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: true,
        Pagination: true,
        ConfirmDialog: true,
        BaseDialog: { props: ['show'], template: '<div v-if="show" data-test="refresh-progress"><slot /><slot name="footer" /></div>' },
        AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
        AccountTableFilters: { template: '<div></div>' },
        AccountBulkActionsBar: true,
        AccountActionMenu: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: true,
        AccountStatsModal: true,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: true,
        Icon: true
      }
    }
  })

const running = { id: 'models-1', status: 'running', total: 2, completed: 0, succeeded: 0, failed: 0, items: [] }
const completed = { ...running, status: 'completed', completed: 2, succeeded: 2 }
const refreshButton = (wrapper: ReturnType<typeof mountView>) => wrapper.findAll('button').find(b => b.text() === 'admin.accounts.refreshModels')!

async function start(wrapper: ReturnType<typeof mountView>) {
  await refreshButton(wrapper).trigger('click')
  wrapper.findAllComponents(ConfirmDialog).find(d => d.props('show'))!.vm.$emit('confirm')
  await flushPromises()
}

describe('account model refresh background progress', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    localStorage.clear()
    sessionStorage.clear()
    listAccounts.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    listWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    startModelRefreshJob.mockReset().mockResolvedValue(running)
    getModelRefreshJob.mockReset().mockResolvedValue(running)
    showError.mockReset()
  })
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

  it('hides progress while polling continues and reopens the same job without confirmation', async () => {
    const wrapper = mountView()
    await flushPromises()
    await start(wrapper)
    const background = wrapper.findAll('button').find(b => b.text() === 'admin.accounts.refreshModelsBackground')!
    expect(background.exists()).toBe(true)
    await background.trigger('click')
    expect(wrapper.find('[data-test="refresh-progress"]').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(500)
    expect(getModelRefreshJob).toHaveBeenCalledWith('models-1')
    await refreshButton(wrapper).trigger('click')
    expect(wrapper.find('[data-test="refresh-progress"]').exists()).toBe(true)
    expect(startModelRefreshJob).toHaveBeenCalledTimes(1)
    expect(wrapper.findAllComponents(ConfirmDialog).some(d => d.props('show'))).toBe(false)
    getModelRefreshJob.mockResolvedValue(completed)
    const loads = listAccounts.mock.calls.length
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    expect(listAccounts.mock.calls.length).toBeGreaterThan(loads)
    const polls = getModelRefreshJob.mock.calls.length
    await vi.advanceTimersByTimeAsync(4000)
    expect(getModelRefreshJob).toHaveBeenCalledTimes(polls)
    wrapper.unmount()
  })

  it('recovers from a transient polling failure', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    getModelRefreshJob.mockRejectedValueOnce(new Error('network unavailable')).mockResolvedValue(completed)
    const wrapper = mountView()
    await flushPromises()
    await start(wrapper)
    await vi.advanceTimersByTimeAsync(500)
    expect(getModelRefreshJob).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(3000)
    expect(getModelRefreshJob).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('does not duplicate a pending start or restart polling after unmount', async () => {
    let resolve!: (value: typeof running) => void
    startModelRefreshJob.mockImplementationOnce(() => new Promise(r => { resolve = r }))
    const wrapper = mountView()
    await flushPromises()
    await start(wrapper)
    await refreshButton(wrapper).trigger('click')
    expect(startModelRefreshJob).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    resolve(running)
    await flushPromises()
    await vi.advanceTimersByTimeAsync(5000)
    expect(getModelRefreshJob).not.toHaveBeenCalled()
  })

  it('stops polling an expired job and permits starting again', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    getModelRefreshJob.mockRejectedValue({ status: 404, message: 'Job not found' })
    const wrapper = mountView()
    await flushPromises()
    await start(wrapper)
    await vi.advanceTimersByTimeAsync(5000)
    expect(getModelRefreshJob).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledWith('Job not found')
    await refreshButton(wrapper).trigger('click')
    expect(wrapper.findAllComponents(ConfirmDialog).some(d => d.props('show'))).toBe(true)
    wrapper.unmount()
  })
})

