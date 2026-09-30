import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { agentAPI } from '@/agent/api'
import AgentBatchImageView from './AgentBatchImageView.vue'

const global = {
  stubs: {
    TablePageLayout: { template: '<main><slot name="actions"/><slot name="filters"/><slot name="table"/><slot name="pagination"/></main>' },
    DataTable: {
      props: ['data', 'loading'],
      template: '<section><slot v-if="!loading && data.length === 0" name="empty"/><div v-for="row in data" :key="row.task_id"><slot name="cell-task_id" :row="row"/><slot name="cell-status" :row="row"/><slot name="cell-settlement_status" :row="row"/><slot name="cell-actions" :row="row"/></div></section>',
    },
    BaseDialog: { props: ['show', 'title'], emits: ['close'], template: '<section v-if="show"><h3>{{ title }}</h3><slot/><footer><slot name="footer"/></footer></section>' },
    EmptyState: { props: ['title', 'description', 'actionText'], emits: ['action'], template: '<div><p>{{ title }}</p><p>{{ description }}</p><button v-if="actionText" @click="$emit(\'action\')">{{ actionText }}</button></div>' },
    AgentPagination: true,
    Icon: true,
  },
}

describe('AgentBatchImageView', () => {
  beforeEach(() => {
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['gpt-5.5', 'gpt-image-2', 'grok-imagine-video-1.5'])
    vi.spyOn(agentAPI, 'getTasks').mockResolvedValue({ items: [], total: 0, page: 1, page_size: 25 })
  })

  afterEach(() => vi.restoreAllMocks())

  it('loads only image history and enabled image models', async () => {
    const wrapper = mount(AgentBatchImageView, { global })
    await flushPromises()

    expect(agentAPI.getTasks).toHaveBeenCalledWith(1, 25, 'image')
    await wrapper.get('button.btn-primary').trigger('click')
    expect(wrapper.get('#batch-image-model').text()).toContain('gpt-image-2')
  })

  it('submits each prompt once and keeps failed prompts for deliberate retry', async () => {
    const generate = vi.spyOn(agentAPI.model, 'generateImageAsync').mockImplementation(async (_model, prompt) => {
      if (prompt === '失败提示词') throw new Error('upstream failed')
      return { id: `task-${prompt}`, status: 'queued' }
    })
    const wrapper = mount(AgentBatchImageView, { global })
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    await wrapper.get('#batch-image-prompts').setValue('第一张图\n失败提示词\n第三张图')
    await wrapper.findAll('button.btn-primary').at(-1)!.trigger('click')
    await flushPromises()

    expect(generate).toHaveBeenCalledTimes(3)
    expect(generate.mock.calls).toEqual([
      ['gpt-image-2', '第一张图'],
      ['gpt-image-2', '失败提示词'],
      ['gpt-image-2', '第三张图'],
    ])
    expect(wrapper.text()).toContain('2 已提交 / 1 失败')
    expect((wrapper.get('#batch-image-prompts').element as HTMLTextAreaElement).value).toBe('失败提示词')
    expect(wrapper.text()).toContain('失败项不会自动重试')
  })

  it('queries a persisted task and never renders an unsafe result URL', async () => {
    vi.mocked(agentAPI.getTasks).mockResolvedValue({
      items: [{
        task_type: 'image', task_id: 'image-task-1', request_id: 'request-1', model: 'gpt-image-2',
        status: 'processing', settlement_status: 'pending', reserved_cents: 10, actual_cents: 0,
        created_at: '2026-09-29T00:00:00Z', updated_at: '2026-09-29T00:00:01Z',
      }],
      total: 1, page: 1, page_size: 25,
    })
    const getTask = vi.spyOn(agentAPI.model, 'getImageTask').mockResolvedValue({
      id: 'image-task-1', status: 'completed', result: { data: [
        { url: 'javascript:alert(1)' },
        { url: 'https://media.example/result.png' },
      ] },
    })
    const wrapper = mount(AgentBatchImageView, { global })
    await flushPromises()
    expect(wrapper.get('[data-status="processing"]').text()).toBe('处理中')
    expect(wrapper.get('[data-status="pending"]').text()).toBe('待处理')
    await wrapper.findAll('button').find((button) => button.text().includes('查询结果'))!.trigger('click')
    await flushPromises()

    expect(getTask).toHaveBeenCalledWith('image-task-1')
    expect(wrapper.findAll('img')).toHaveLength(1)
    expect(wrapper.get('img').attributes('src')).toBe('https://media.example/result.png')
    expect(wrapper.get('[data-status="completed"]').text()).toBe('已完成')
  })
})
