import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentConsoleView from './AgentConsoleView.vue'
import { agentAPI } from '@/agent/api'

const context = {
  agent: {
    agent_id: 'agent-1',
    domain: 'agent.example.com',
    name: 'Example Agent',
    site_name: 'Example Agent',
    status: 'active',
    billing_mode: 'owner_upstream',
    main_balance_cents: 10000,
    billing_status: 'ok',
    wallet_available_cents: 8000,
    wallet_allocated_cents: 2000,
  },
  authenticated: true,
  is_agent_admin: true,
}

describe('AgentConsoleView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('shows mapped users and pending settlements and can reconcile them', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({
      total: 1,
      page: 1,
      page_size: 25,
      items: [{
        agent_id: 'agent-1',
        main_user_id: 'user-1',
        email: 'user@example.com',
        status: 'active',
        balance_cents: 250,
        created_at: '2026-09-22T00:00:00Z',
        updated_at: '2026-09-22T00:00:00Z',
      }],
    })
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({
      total: 1,
      items: [{ request_id: 'req-pending', reserved_cents: 25, actual_cents: 0, status: 'pending' }],
    })
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({
      enabled: false,
      provider: 'manual',
      items: [],
      total: 0,
    })
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue({ catalog: ['gpt-5.5'], enabled: ['gpt-5.5'], customized: false })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 1, items: [{ id: 1, actor_type: 'agent_admin', actor_id: 'owner-1', agent_id: 'agent-1', operation: 'wallet.sync', target_type: 'agent_wallet', request_id: 'req-audit', result: 'success', created_at: '2026-09-22T00:00:00Z' }] })
    const reconcile = vi.spyOn(agentAPI, 'reconcileSettlements').mockResolvedValue({ total: 1, items: [] })

    const wrapper = mount(AgentConsoleView)
    await flushPromises()

    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('req-pending')
    expect(wrapper.text()).toContain('同步主账户余额')

    const button = wrapper.findAll('button').find((item) => item.text().includes('核对待结算记录'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()
    expect(reconcile).toHaveBeenCalledOnce()
  })

  it('keeps healthy console sections available when one panel fails to load', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({
      total: 1,
      page: 1,
      page_size: 25,
      items: [{
        agent_id: 'agent-1',
        main_user_id: 'user-1',
        email: 'user@example.com',
        status: 'active',
        balance_cents: 250,
        created_at: '2026-09-22T00:00:00Z',
        updated_at: '2026-09-22T00:00:00Z',
      }],
    })
    vi.spyOn(agentAPI, 'getSettlements').mockRejectedValue(new Error('settlement endpoint unavailable'))
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({ enabled: false, provider: 'manual', items: [], total: 0 })
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue({ catalog: ['gpt-5.5'], enabled: ['gpt-5.5'], customized: false })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 0, items: [] })

    const wrapper = mount(AgentConsoleView)
    await flushPromises()

    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('Example Agent')
    expect(wrapper.text()).toContain('加载待结算记录失败，请刷新重试。')
    expect(wrapper.text()).toContain('以下管理信息加载失败：待结算记录。请刷新重试。')
    expect(wrapper.text()).toContain('暂无充值订单')
  })

  it('paginates mapped users through the server-backed Agent endpoint', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const getUsers = vi.spyOn(agentAPI, 'getUsers').mockImplementation(async (page = 1, pageSize = 25) => ({
      total: 26,
      page,
      page_size: pageSize,
      items: [{
        agent_id: 'agent-1',
        main_user_id: page === 1 ? 'user-first-page' : 'user-second-page',
        email: page === 1 ? 'first@example.com' : 'second@example.com',
        status: 'active',
        balance_cents: 250,
        created_at: '2026-09-22T00:00:00Z',
        updated_at: '2026-09-22T00:00:00Z',
      }],
    }))
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ total: 0, items: [] })
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({ enabled: false, provider: 'manual', items: [], total: 0 })
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue({ catalog: ['gpt-5.5'], enabled: ['gpt-5.5'], customized: false })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 0, items: [] })

    const wrapper = mount(AgentConsoleView)
    await flushPromises()
    expect(getUsers).toHaveBeenCalledWith(1, 25, '')
    expect(wrapper.text()).toContain('first@example.com')
    expect(wrapper.text()).toContain('显示 1–25，共 26 名用户')

    await wrapper.get('[aria-label="下一页"]').trigger('click')
    await flushPromises()

    expect(getUsers).toHaveBeenLastCalledWith(2, 25, '')
    expect(wrapper.text()).toContain('second@example.com')
    expect(wrapper.text()).not.toContain('first@example.com')
  })

  it('debounces mapped-user search and resets the page before querying', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    const getUsers = vi.spyOn(agentAPI, 'getUsers').mockImplementation(async (page = 1, pageSize = 25, search = '') => ({
      total: search ? 1 : 50,
      page,
      page_size: pageSize,
      items: [{
        agent_id: 'agent-1',
        main_user_id: search ? 'matched-user' : `page-${page}-user`,
        email: search ? 'target@example.com' : `page-${page}@example.com`,
        status: 'active',
        balance_cents: 250,
        created_at: '2026-09-22T00:00:00Z',
        updated_at: '2026-09-22T00:00:00Z',
      }],
    }))
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ total: 0, items: [] })
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({ enabled: false, provider: 'manual', items: [], total: 0 })
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue({ catalog: ['gpt-5.5'], enabled: ['gpt-5.5'], customized: false })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 0, items: [] })

    const wrapper = mount(AgentConsoleView)
    await flushPromises()
    await wrapper.get('[aria-label="下一页"]').trigger('click')
    await flushPromises()
    expect(getUsers).toHaveBeenLastCalledWith(2, 25, '')

    vi.useFakeTimers()
    await wrapper.get('[aria-label="搜索关联用户"]').setValue('  target@example.com  ')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(getUsers).toHaveBeenLastCalledWith(1, 25, 'target@example.com')
    expect(wrapper.text()).toContain('target@example.com')
  })

  it('does not let a slower previous refresh overwrite newer console data', async () => {
    let resolveInitialContext!: (value: typeof context) => void
    const initialContext = new Promise<typeof context>((resolve) => { resolveInitialContext = resolve })
    const latestContext = { ...context, agent: { ...context.agent, name: 'Latest Agent' } }
    vi.spyOn(agentAPI, 'getContext')
      .mockReturnValueOnce(initialContext)
      .mockResolvedValueOnce(latestContext)
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ total: 0, page: 1, page_size: 25, items: [] })
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ total: 0, items: [] })
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({ enabled: false, provider: 'manual', items: [], total: 0 })
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue({ catalog: ['gpt-5.5'], enabled: ['gpt-5.5'], customized: false })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 0, items: [] })

    const wrapper = mount(AgentConsoleView)
    const refresh = wrapper.findAll('button').find((button) => button.text() === '刷新')
    expect(refresh).toBeDefined()
    await refresh!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Latest Agent')

    resolveInitialContext(context)
    await flushPromises()
    expect(wrapper.text()).toContain('Latest Agent')
    expect(wrapper.text()).not.toContain('Example Agent')
  })

  it('saves branding through the administrator API', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ total: 0, page: 1, page_size: 25, items: [] })
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ total: 0, items: [] })
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({ enabled: false, provider: 'manual', items: [], total: 0 })
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue({ catalog: ['gpt-5.5'], enabled: ['gpt-5.5'], customized: false })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 0, items: [] })
    const update = vi.spyOn(agentAPI, 'updateBranding').mockResolvedValue({ name: 'Edited', site_name: 'Edited Site', site_logo: '/edited.svg' })

    const wrapper = mount(AgentConsoleView)
    await flushPromises()
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('Edited')
    await inputs[1].setValue('Edited Site')
    await inputs[2].setValue('/edited.svg')
    const button = wrapper.findAll('button').find((item) => item.text().includes('保存品牌信息'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith({ name: 'Edited', site_name: 'Edited Site', site_logo: '/edited.svg' })
    expect(wrapper.text()).toContain('品牌信息已保存')
  })

  it('saves an allowlist from the public catalog without exposing private model names', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue(context)
    vi.spyOn(agentAPI, 'getUsers').mockResolvedValue({ total: 0, page: 1, page_size: 25, items: [] })
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ total: 0, items: [] })
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({ enabled: false, provider: 'manual', items: [], total: 0 })
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue({ catalog: ['gpt-5.5', 'gpt-image-2'], enabled: ['gpt-5.5'], customized: true })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 0, items: [] })
    const update = vi.spyOn(agentAPI, 'updateAdminModelPolicy').mockResolvedValue({ catalog: ['gpt-5.5', 'gpt-image-2'], enabled: ['gpt-5.5', 'gpt-image-2'], customized: true })

    const wrapper = mount(AgentConsoleView)
    await flushPromises()
    expect(wrapper.text()).toContain('停用的模型会从')
    await wrapper.find('input[type="checkbox"][value="gpt-image-2"]').setValue(true)
    const button = wrapper.findAll('button').find((item) => item.text().includes('保存模型权限'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(['gpt-5.5', 'gpt-image-2'])
    expect(wrapper.text()).toContain('模型权限已保存，已启用 2 个模型')
  })

  it('updates mapped account status and reflects the local access state', async () => {
    vi.spyOn(agentAPI, 'getContext').mockResolvedValue({
      ...context,
      agent: { ...context.agent, owner_main_user_id: 'owner-1' },
    })
    let userStatus: 'active' | 'disabled' = 'active'
    const mappedUser = () => ({
      agent_id: 'agent-1',
      main_user_id: 'user-1',
      email: 'user@example.com',
      status: userStatus,
      balance_cents: 250,
      created_at: '2026-09-22T00:00:00Z',
      updated_at: '2026-09-22T00:00:00Z',
    })
    vi.spyOn(agentAPI, 'getUsers').mockImplementation(async (page = 1, pageSize = 25) => ({ total: 1, page, page_size: pageSize, items: [mappedUser()] }))
    vi.spyOn(agentAPI, 'getSettlements').mockResolvedValue({ total: 0, items: [] })
    vi.spyOn(agentAPI, 'getAdminRechargeOrders').mockResolvedValue({ enabled: false, provider: 'manual', items: [], total: 0 })
    vi.spyOn(agentAPI, 'getAdminModelPolicy').mockResolvedValue({ catalog: ['gpt-5.5'], enabled: ['gpt-5.5'], customized: false })
    vi.spyOn(agentAPI, 'getAuditEvents').mockResolvedValue({ total: 0, items: [] })
    const update = vi.spyOn(agentAPI, 'setMappedUserStatus').mockImplementation(async (_mainUserID, status) => {
      userStatus = status
      return mappedUser()
    })
    const wrapper = mount(AgentConsoleView)
    await flushPromises()
    const disable = wrapper.findAll('button').find((button) => button.text() === '停用账号')
    expect(disable).toBeDefined()
    await disable!.trigger('click')
    await flushPromises()

    expect(update).not.toHaveBeenCalled()
    expect(document.body.querySelector('[role="alertdialog"]')?.textContent).toContain('确定停用 user@example.com 关联的 Sub2API 账号吗？')
    const confirm = document.body.querySelector<HTMLButtonElement>('[data-testid="agent-confirm-action"]')
    expect(confirm?.textContent).toBe('确认停用')
    confirm?.click()
    await flushPromises()

    expect(update).toHaveBeenCalledWith('user-1', 'disabled')
    expect(wrapper.text()).toContain('已停用')
    expect(wrapper.text()).toContain('Sub2API 账号已停用')
    expect(wrapper.findAll('button').some((button) => button.text() === '启用账号')).toBe(true)
    wrapper.unmount()
  })
})
