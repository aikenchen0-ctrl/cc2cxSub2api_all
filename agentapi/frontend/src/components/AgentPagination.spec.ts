import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentPagination from './AgentPagination.vue'

describe('AgentPagination', () => {
  it('renders the main-site page window and an accurate result range', () => {
    const wrapper = mount(AgentPagination, {
      props: { total: 240, page: 6, pageSize: 20, itemLabel: '条记录' },
    })

    expect(wrapper.text()).toContain('显示 101 至 120 项，共 240 条记录')
    expect(wrapper.get('[aria-current="page"]').text()).toBe('6')
    expect(wrapper.text()).toContain('…')
    expect(wrapper.find('[aria-label="前往第 12 页"]').exists()).toBe(true)
  })

  it('emits guarded previous, next, and numbered page changes', async () => {
    const wrapper = mount(AgentPagination, {
      props: { total: 80, page: 2, pageSize: 20 },
    })

    await wrapper.findAll('[aria-label="上一页"]')[0].trigger('click')
    await wrapper.findAll('[aria-label="下一页"]')[0].trigger('click')
    await wrapper.get('[aria-label="前往第 4 页"]').trigger('click')
    await wrapper.get('[aria-current="page"]').trigger('click')

    expect(wrapper.emitted('update:page')).toEqual([[1], [3], [4]])
  })

  it('keeps the active page size selectable and emits valid changes', async () => {
    const wrapper = mount(AgentPagination, {
      props: { total: 100, page: 1, pageSize: 30, pageSizeOptions: [10, 20, 50] },
    })

    await wrapper.get('[aria-label="每页显示条数"]').trigger('click')
    await flushPromises()
    expect([...document.body.querySelectorAll<HTMLElement>('[role="option"]')].map(option => option.textContent?.trim())).toEqual(['10', '20', '30', '50'])
    ;[...document.body.querySelectorAll<HTMLElement>('[role="option"]')].find(option => option.textContent?.trim() === '50')!.click()
    await flushPromises()
    expect(wrapper.emitted('update:pageSize')).toEqual([[50]])
  })

  it('supports optional page jump and clamps it to the available range', async () => {
    const wrapper = mount(AgentPagination, {
      props: { total: 95, page: 1, pageSize: 10, showJump: true },
    })

    const input = wrapper.get('input[placeholder="页码"]')
    await input.setValue('999')
    await wrapper.get('form[aria-label="跳转页码"]').trigger('submit')

    expect(wrapper.emitted('update:page')).toEqual([[10]])
    expect((input.element as HTMLInputElement).value).toBe('')
  })

  it('reports an empty collection without invalid page math', () => {
    const wrapper = mount(AgentPagination, {
      props: { total: 0, page: 1, pageSize: 20 },
    })

    expect(wrapper.text()).toContain('显示 0 至 0 项，共 0 条记录')
    expect(wrapper.text()).toContain('第 1 / 1 页')
    expect(wrapper.findAll('[aria-label="上一页"]').every((button) => button.attributes('disabled') !== undefined)).toBe(true)
    expect(wrapper.findAll('[aria-label="下一页"]').every((button) => button.attributes('disabled') !== undefined)).toBe(true)
  })

  it('requests the last valid page when the result count shrinks', async () => {
    const wrapper = mount(AgentPagination, {
      props: { total: 100, page: 5, pageSize: 20 },
    })

    await wrapper.setProps({ total: 35 })
    expect(wrapper.emitted('update:page')).toEqual([[2]])
  })
})
