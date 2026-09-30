import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import AgentConfirmDialog from './AgentConfirmDialog.vue'

describe('AgentConfirmDialog', () => {
  afterEach(() => {
    document.body.classList.remove('modal-open')
  })

  it('uses the copied main-site BaseDialog structure and emits explicit actions', async () => {
    const wrapper = mount(AgentConfirmDialog, {
      props: {
        open: true,
        title: '删除记录',
        message: '此操作不可撤销。',
        confirmLabel: '删除',
        destructive: true,
      },
    })
    await flushPromises()

    const dialog = document.body.querySelector<HTMLElement>('[role="alertdialog"]')
    expect(dialog?.textContent).toContain('删除记录')
    expect(dialog?.textContent).toContain('此操作不可撤销。')
    expect(dialog?.getAttribute('aria-describedby')).toBeTruthy()

    ;(dialog?.querySelector('[data-testid="agent-confirm-action"]') as HTMLButtonElement).click()
    expect(wrapper.emitted('confirm')).toHaveLength(1)

    const cancel = [...dialog!.querySelectorAll('button')].find(button => button.textContent?.trim() === '取消')
    cancel?.click()
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    wrapper.unmount()
  })

  it('closes through Escape using the shared main-site dialog behavior', async () => {
    const wrapper = mount(AgentConfirmDialog, {
      props: { open: true, title: '确认操作', message: '继续吗？' },
    })
    await flushPromises()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    wrapper.unmount()
  })
})
