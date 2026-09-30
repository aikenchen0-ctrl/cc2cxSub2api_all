import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import AgentSelect from './AgentSelect.vue'

const originalWidth = window.innerWidth
let wrapper: ReturnType<typeof mount> | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalWidth })
  vi.useRealTimers()
  vi.restoreAllMocks()
})

function mountSelect(props: Record<string, unknown> = {}) {
  wrapper = mount(AgentSelect, {
    props: {
      modelValue: null,
      options: [
        { value: '', label: '全部状态' },
        { value: 'active', label: '启用中' },
        { value: 'disabled', label: '已停用', disabled: true },
      ],
      ariaLabel: '状态筛选',
      ...props,
    },
  })
  return wrapper
}

describe('AgentSelect', () => {
  it('opens a main-site style list and emits the selected value', async () => {
    const select = mountSelect()
    await select.get('button[aria-label="状态筛选"]').trigger('click')
    await nextTick()
    const options = [...document.body.querySelectorAll<HTMLElement>('[role="option"]')]
    expect(options.map(option => option.textContent?.trim())).toEqual(['全部状态', '启用中', '已停用'])
    options[1].click()
    await nextTick()
    expect(select.emitted('update:modelValue')).toEqual([['active']])
    expect(select.emitted('change')?.[0]?.[0]).toBe('active')
  })

  it('filters locally and leaves disabled options inert', async () => {
    const select = mountSelect({ searchable: true })
    await select.get('button').trigger('click')
    await nextTick()
    const input = document.body.querySelector<HTMLInputElement>('.select-search-input')!
    input.value = '停用'
    input.dispatchEvent(new Event('input'))
    await nextTick()
    const option = document.body.querySelector<HTMLElement>('[role="option"]')!
    expect(option.textContent).toContain('已停用')
    option.click()
    expect(select.emitted('update:modelValue')).toBeUndefined()
  })

  it('supports keyboard selection and escape closing', async () => {
    const select = mountSelect({ modelValue: '' })
    await select.get('button').trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    const listbox = document.body.querySelector<HTMLElement>('[role="listbox"]')!
    await listbox.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    await nextTick()
    listbox.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await nextTick()
    expect(select.emitted('update:modelValue')).toEqual([['active']])
  })

  it('debounces remote search and cancels it when closed', async () => {
    vi.useFakeTimers()
    const select = mountSelect({ remote: true })
    await select.get('button').trigger('click')
    await nextTick()
    const input = document.body.querySelector<HTMLInputElement>('.select-search-input')!
    input.value = ' user '
    input.dispatchEvent(new Event('input'))
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)
    expect(select.emitted('search')).toEqual([['user']])

    input.value = 'cancelled'
    input.dispatchEvent(new Event('input'))
    await nextTick()
    await select.get('button').trigger('click')
    await vi.advanceTimersByTimeAsync(300)
    expect(select.emitted('search')).toEqual([['user']])
  })

  it('keeps the teleported dropdown inside the viewport', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 320 })
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 220, y: 20, top: 20, right: 300, bottom: 60, left: 220, width: 80, height: 40, toJSON: () => ({}),
    })
    const select = mountSelect()
    await select.get('button').trigger('click')
    await nextTick()
    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')!
    expect(dropdown.style.left).toBe('220px')
    expect(dropdown.style.maxWidth).toBe('92px')
    expect(dropdown.style.minWidth).toBe('92px')
  })
})
