import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentModelConsoleView from './AgentModelConsoleView.vue'
import { agentAPI } from '@/agent/api'

describe('AgentModelConsoleView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('sends the selected public model through the AgentAPI endpoint', async () => {
    const chat = vi.spyOn(agentAPI.model, 'chat').mockResolvedValue({
      id: 'chat-1',
      choices: [{ message: { role: 'assistant', content: 'response from main site' } }],
    })
    const wrapper = mount(AgentModelConsoleView)

    await wrapper.get('#agent-prompt').setValue('Say hello')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(chat).toHaveBeenCalledWith('gpt-5.5', [{ role: 'user', content: 'Say hello' }])
    expect(wrapper.text()).toContain('response from main site')
    expect(wrapper.text()).not.toContain('sk-super-')
  })
})
