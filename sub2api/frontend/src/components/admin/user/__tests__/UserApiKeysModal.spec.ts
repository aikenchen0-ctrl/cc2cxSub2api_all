import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminUser, ApiKey } from '@/types'
import UserApiKeysModal from '../UserApiKeysModal.vue'

const { getUserApiKeys, getAllGroups, updateApiKeyGroup } = vi.hoisted(() => ({
  getUserApiKeys: vi.fn(),
  getAllGroups: vi.fn(),
  updateApiKeyGroup: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { getUserApiKeys },
    groups: { getAll: getAllGroups },
    apiKeys: { updateApiKeyGroup },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const user = {
  id: 1,
  email: 'user@example.com',
  username: 'user',
} as AdminUser

const createKey = (name: string): ApiKey => ({
  id: name === 'Sub2API Super Key' ? 1 : 2,
  user_id: 1,
  key: `sk-${name}`,
  name,
  group_id: null,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  current_concurrency: 0,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
})

describe('UserApiKeysModal', () => {
  beforeEach(() => {
    getUserApiKeys.mockReset().mockResolvedValue({
      items: [createKey('Sub2API Super Key'), createKey('Regular Key')],
    })
    getAllGroups.mockReset().mockResolvedValue([])
    updateApiKeyGroup.mockReset()
  })

  it('hides group controls for the Super Key while keeping them for regular keys', async () => {
    const wrapper = mount(UserApiKeysModal, {
      props: { show: false, user },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          GroupBadge: true,
          GroupOptionItem: true,
          Teleport: true,
        },
      },
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const rows = wrapper.findAll('[data-test="api-key-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].find('[data-test="group-control"]').exists()).toBe(false)
    expect(rows[1].find('[data-test="group-control"]').exists()).toBe(true)
  })
})
