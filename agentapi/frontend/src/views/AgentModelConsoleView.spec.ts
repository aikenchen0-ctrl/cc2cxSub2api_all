import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AgentModelConsoleView from './AgentModelConsoleView.vue'
import { agentAPI } from '@/agent/api'

describe('AgentModelConsoleView', () => {
  beforeEach(() => {
    vi.spyOn(agentAPI, 'getTasks').mockResolvedValue({ items: [], total: 0, page: 1, page_size: 25 })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('sends text requests with a model from the enabled text catalog', async () => {
    const list = vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['gpt-5.5', 'gpt-image-2'])
    const chat = vi.spyOn(agentAPI.model, 'chat').mockResolvedValue({
      id: 'chat-1',
      choices: [{ message: { role: 'assistant', content: 'response from main site' } }],
    })
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()

    expect(list).toHaveBeenCalledOnce()
    expect(wrapper.findAll('#agent-model option').map((option) => option.text())).toEqual(['gpt-5.5'])

    await wrapper.get('#agent-prompt').setValue('Say hello')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(chat).toHaveBeenCalledWith('gpt-5.5', [{ role: 'user', content: 'Say hello' }])
    expect(wrapper.text()).toContain('response from main site')
    expect(wrapper.text()).not.toContain('sk-super-')
  })

  it('generates an image and renders the returned public image URL', async () => {
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['gpt-5.5', 'gpt-image-2'])
    const generate = vi.spyOn(agentAPI.model, 'generateImage').mockResolvedValue({
      data: [{ url: 'https://media.example/generated.png', revised_prompt: 'a blue paper crane' }],
    })
    const edit = vi.spyOn(agentAPI.model, 'editImage')
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()
    await wrapper.findAll('nav button')[1].trigger('click')
    await wrapper.get('#agent-image-prompt').setValue('a paper crane')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(generate).toHaveBeenCalledWith('gpt-image-2', 'a paper crane')
    expect(edit).not.toHaveBeenCalled()
    expect(wrapper.get('img').attributes('src')).toBe('https://media.example/generated.png')
    expect(wrapper.text()).toContain('a blue paper crane')
  })

  it('uses the image edits endpoint when a reference image is selected', async () => {
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['gpt-image-2'])
    const edit = vi.spyOn(agentAPI.model, 'editImage').mockResolvedValue({ data: [{ b64_json: 'aW1hZ2U=' }] })
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()
    await wrapper.findAll('nav button')[1].trigger('click')
    await wrapper.get('#agent-image-prompt').setValue('make it blue')
    const file = new File(['image bytes'], 'reference.png', { type: 'image/png' })
    const input = wrapper.get('#agent-image-file')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(edit).toHaveBeenCalledWith('gpt-image-2', 'make it blue', file)
    expect(wrapper.get('img').attributes('src')).toBe('data:image/png;base64,aW1hZ2U=')
  })

  it('creates and polls an asynchronous image task once and renders its result', async () => {
    vi.useFakeTimers()
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['gpt-image-2'])
    const create = vi.spyOn(agentAPI.model, 'generateImageAsync').mockResolvedValue({ id: 'image-task-1', status: 'processing' })
    const poll = vi.spyOn(agentAPI.model, 'getImageTask').mockResolvedValue({
      id: 'image-task-1', status: 'done', result: { data: [{ url: 'https://media.example/async.png' }] },
    })
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()
    await wrapper.findAll('nav button')[1].trigger('click')
    await wrapper.get('#agent-image-prompt').setValue('a quiet lake')
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3_000)
    await flushPromises()

    expect(create).toHaveBeenCalledOnce()
    expect(create).toHaveBeenCalledWith('gpt-image-2', 'a quiet lake')
    expect(poll).toHaveBeenCalledWith('image-task-1')
    expect(wrapper.get('img').attributes('src')).toBe('https://media.example/async.png')
    expect(wrapper.text()).toContain('image-task-1')
  })

  it('confirms an already-terminal async image create response through the task route', async () => {
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['gpt-image-2'])
    const create = vi.spyOn(agentAPI.model, 'generateImageAsync').mockResolvedValue({
      id: 'image-terminal-1', status: 'failed',
    })
    const poll = vi.spyOn(agentAPI.model, 'getImageTask').mockResolvedValue({
      id: 'image-terminal-1', status: 'failed',
    })
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()
    await wrapper.findAll('nav button')[1].trigger('click')
    await wrapper.get('#agent-image-prompt').setValue('a failed image')
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(create).toHaveBeenCalledOnce()
    expect(poll).toHaveBeenCalledOnce()
    expect(poll).toHaveBeenCalledWith('image-terminal-1')
    expect(wrapper.text()).toContain('任务 image-terminal-1 · 失败')
  })

  it('restores a persisted user task after reopening the model console', async () => {
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['gpt-5.5', 'grok-imagine-video-1.5'])
    vi.mocked(agentAPI.getTasks).mockResolvedValue({
      items: [{
        task_type: 'video', task_id: 'video-after-refresh', request_id: 'request-after-refresh',
        model: 'grok-imagine-video-1.5', status: 'processing', settlement_status: 'pending',
        reserved_cents: 100, actual_cents: 0, created_at: '2026-09-25T00:00:00Z', updated_at: '2026-09-25T00:00:10Z',
      }],
      total: 1,
      page: 1,
      page_size: 25,
    })
    const poll = vi.spyOn(agentAPI.model, 'getVideo').mockResolvedValue({
      id: 'video-after-refresh', status: 'done', video: { url: 'https://media.example/restored.mp4' },
    })
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()

    expect(wrapper.text()).toContain('video-after-refresh')
    expect(wrapper.text()).toContain('继续查询')
    await wrapper.get('tbody button').trigger('click')
    await flushPromises()

    expect(poll).toHaveBeenCalledWith('video-after-refresh')
    expect(wrapper.get('video').attributes('src')).toBe('https://media.example/restored.mp4')
    expect(wrapper.text()).toContain('任务 video-after-refresh · 已完成')
  })

  it('restores an asynchronous image task without submitting another generation request', async () => {
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['gpt-image-2'])
    vi.mocked(agentAPI.getTasks).mockResolvedValue({
      items: [{
        task_type: 'image', task_id: 'image-after-refresh', request_id: 'image-request-after-refresh',
        model: 'gpt-image-2', status: 'processing', settlement_status: 'pending',
        reserved_cents: 100, actual_cents: 0, created_at: '2026-09-25T00:00:00Z', updated_at: '2026-09-25T00:00:10Z',
      }],
      total: 1,
      page: 1,
      page_size: 25,
    })
    const poll = vi.spyOn(agentAPI.model, 'getImageTask').mockResolvedValue({
      id: 'image-after-refresh', status: 'done', result: { data: [{ url: 'https://media.example/restored.png' }] },
    })
    const generate = vi.spyOn(agentAPI.model, 'generateImageAsync')
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()

    await wrapper.get('tbody button').trigger('click')
    await flushPromises()

    expect(poll).toHaveBeenCalledWith('image-after-refresh')
    expect(generate).not.toHaveBeenCalled()
    expect(wrapper.get('img').attributes('src')).toBe('https://media.example/restored.png')
  })

  it('creates a video once, polls its task ID, and displays the completed video', async () => {
    vi.useFakeTimers()
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['grok-imagine-video-1.5'])
    const create = vi.spyOn(agentAPI.model, 'createVideo').mockResolvedValue({ request_id: 'video-1', status: 'queued' })
    const poll = vi.spyOn(agentAPI.model, 'getVideo').mockResolvedValue({
      id: 'video-1', status: 'done', video: { url: 'https://media.example/video.mp4' },
    })
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()
    await wrapper.findAll('nav button')[2].trigger('click')
    await wrapper.get('#agent-video-prompt').setValue('a quiet lake')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3_000)
    await flushPromises()

    expect(create).toHaveBeenCalledOnce()
    expect(create).toHaveBeenCalledWith('grok-imagine-video-1.5', 'a quiet lake')
    expect(poll).toHaveBeenCalledWith('video-1')
    expect(wrapper.get('video').attributes('src')).toBe('https://media.example/video.mp4')
    expect(wrapper.text()).toContain('video-1')
  })

  it('checks an already-terminal video create response through the task route', async () => {
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue(['grok-imagine-video-1.5'])
    const create = vi.spyOn(agentAPI.model, 'createVideo').mockResolvedValue({
      request_id: 'video-terminal-1', status: 'completed', video: { url: 'https://media.example/initial.mp4' },
    })
    const poll = vi.spyOn(agentAPI.model, 'getVideo').mockResolvedValue({
      id: 'video-terminal-1', status: 'completed', video: { url: 'https://media.example/confirmed.mp4' },
    })
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()
    await wrapper.findAll('nav button')[2].trigger('click')
    await wrapper.get('#agent-video-prompt').setValue('a completed video')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(create).toHaveBeenCalledOnce()
    expect(poll).toHaveBeenCalledOnce()
    expect(poll).toHaveBeenCalledWith('video-terminal-1')
    expect(wrapper.get('video').attributes('src')).toBe('https://media.example/confirmed.mp4')
  })

  it('disables requests when the current capability has no enabled public model', async () => {
    vi.spyOn(agentAPI.model, 'list').mockResolvedValue([])
    const chat = vi.spyOn(agentAPI.model, 'chat')
    const wrapper = mount(AgentModelConsoleView)
    await flushPromises()
    await wrapper.get('#agent-prompt').setValue('Should not submit')
    expect(wrapper.get('form button[type="submit"]').attributes('disabled')).toBeDefined()

    await wrapper.get('form').trigger('submit')
    expect(chat).not.toHaveBeenCalled()

    await wrapper.findAll('nav button')[1].trigger('click')
    expect(wrapper.text()).toContain('当前代理站尚未启用任何模型')
  })
})
