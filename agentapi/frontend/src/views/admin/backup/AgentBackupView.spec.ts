import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentTenantBackupRecord, type AgentTenantBackupSnapshot } from '@/agent/api'
import AgentBackupView from './AgentBackupView.vue'

const record: AgentTenantBackupRecord = {
  id: 7, status: 'completed', file_name: 'agentapi-agent-one-20260929-120000.json', size_bytes: 2048,
  parts: ['branding', 'model_policy'], triggered_by: 'manual', created_by: 'owner-1',
  started_at: '2026-09-29T12:00:00Z', restored_at: '',
}

const snapshot = {
  version: 1, source_agent_id: 'agent-one', created_at: '2026-09-29T12:00:00Z',
  branding: { name: 'Agent One', site_name: '代理一站' },
  model_policy: { customized: true, enabled: ['gpt-5.5'] },
  announcements: [], content_pages: [], plan_policies: [],
} satisfies AgentTenantBackupSnapshot

describe('AgentBackupView', () => {
  afterEach(() => vi.restoreAllMocks())

  function mockList(): void {
    vi.spyOn(agentAPI.adminBackups, 'list').mockResolvedValue({
      items: [record], total: 1,
      parts: ['branding', 'model_policy', 'announcements', 'content_pages', 'plan_policies'],
    })
  }

  it('copies the main backup workflow while showing the tenant-only data boundary', async () => {
    mockList()
    const create = vi.spyOn(agentAPI.adminBackups, 'create').mockResolvedValue(record)
    const download = vi.spyOn(agentAPI.adminBackups, 'download').mockResolvedValue({ record, snapshot })
    const restore = vi.spyOn(agentAPI.adminBackups, 'restore').mockResolvedValue({ ...record, restored_at: '2026-09-29T13:00:00Z' })
    const remove = vi.spyOn(agentAPI.adminBackups, 'remove').mockResolvedValue()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:backup') })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)

    const wrapper = mount(AgentBackupView)
    await flushPromises()

    expect(wrapper.text()).toContain('品牌信息')
    expect(wrapper.text()).toContain('支付密钥与主站全局设置')
    expect(wrapper.text()).toContain(record.file_name)
    expect(wrapper.text()).not.toContain('S3 密钥')
    expect(wrapper.text()).not.toContain('SuperKey 值')

    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(create).toHaveBeenCalledTimes(1)

    await wrapper.get(`[aria-label="下载 ${record.file_name}"]`).trigger('click')
    await flushPromises()
    expect(download).toHaveBeenCalledWith(7)

    await wrapper.get(`[aria-label="恢复 ${record.file_name}"]`).trigger('click')
    await flushPromises()
    let dialog = document.body.querySelector<HTMLElement>('[role="alertdialog"]')
    expect(dialog?.textContent).toContain('当前租户配置将被该快照事务性替换')
    ;(dialog?.querySelector('[data-testid="agent-confirm-action"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(restore).toHaveBeenCalledWith(7)

    await wrapper.get(`[aria-label="删除 ${record.file_name}"]`).trigger('click')
    await flushPromises()
    dialog = document.body.querySelector<HTMLElement>('[role="alertdialog"]')
    expect(dialog?.textContent).toContain('此操作不可撤销')
    ;(dialog?.querySelector('[data-testid="agent-confirm-action"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(remove).toHaveBeenCalledWith(7)
    wrapper.unmount()
  })

  it('imports a JSON snapshot without restoring it automatically', async () => {
    mockList()
    const importBackup = vi.spyOn(agentAPI.adminBackups, 'import').mockResolvedValue(record)
    const restore = vi.spyOn(agentAPI.adminBackups, 'restore').mockResolvedValue(record)
    const wrapper = mount(AgentBackupView)
    await flushPromises()

    const file = { name: 'tenant-backup.json', size: JSON.stringify(snapshot).length, text: async () => JSON.stringify(snapshot) }
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change')
    await flushPromises()

    expect(importBackup).toHaveBeenCalledWith(snapshot)
    expect(restore).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('导入不会自动覆盖当前配置')
    wrapper.unmount()
  })
})
