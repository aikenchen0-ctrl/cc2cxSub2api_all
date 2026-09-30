import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import DataTable from './DataTable.vue'

describe('copied Sub2API table', () => {
  afterEach(() => vi.restoreAllMocks())

  it('renders desktop rows and the caller-owned action slot', () => {
    const wrapper = mount(DataTable, {
      props: { columns: [{ key: 'name', label: '名称' }, { key: 'actions', label: '操作' }], data: [{ id: 7, name: 'tenant key' }], rowKey: 'id' },
      slots: { 'cell-actions': '<button>撤销</button>' },
    })
    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.text()).toContain('tenant key')
    expect(wrapper.get('button').text()).toBe('撤销')
    wrapper.unmount()
  })

  it('keeps row values and actions available in the mobile card layout', () => {
    const original = window.matchMedia
    vi.spyOn(window, 'matchMedia').mockImplementation(query => ({ ...original(query), matches: false }))
    const wrapper = mount(DataTable, {
      props: { columns: [{ key: 'name', label: '名称' }, { key: 'actions', label: '操作' }], data: [{ id: 8, name: 'mobile key' }], rowKey: 'id' },
      slots: { 'cell-actions': '<button>撤销</button>' },
    })
    expect(wrapper.find('table').exists()).toBe(false)
    expect(wrapper.text()).toContain('mobile key')
    expect(wrapper.get('button').text()).toBe('撤销')
    wrapper.unmount()
  })

  it('uses the supplied empty state without inventing row data', () => {
    const wrapper = mount(DataTable, { props: { columns: [{ key: 'name', label: '名称' }], data: [] }, slots: { empty: '暂无本站记录' } })
    expect(wrapper.text()).toContain('暂无本站记录')
    expect(wrapper.findAll('[data-row-id]')).toHaveLength(0)
    wrapper.unmount()
  })
})
