import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { agentAPI, type AgentProfile } from '@/agent/api'
import AgentProfileAvatarCard from './AgentProfileAvatarCard.vue'

const profile: AgentProfile = {
  id: '42',
  email: 'avatar@example.com',
  username: 'Avatar User',
  can_edit: true,
}

const originalFileReader = globalThis.FileReader
const originalImage = globalThis.Image
const originalCreateElement = document.createElement.bind(document)

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

function installCompressionMocks(blobSize = 8 * 1024): void {
  class MockFileReader {
    result: string | ArrayBuffer | null = null
    onload: ((this: FileReader, event: ProgressEvent<FileReader>) => unknown) | null = null
    onerror: ((this: FileReader, event: ProgressEvent<FileReader>) => unknown) | null = null
    error: DOMException | null = null

    readAsDataURL(blob: Blob): void {
      this.result = blob.type === 'image/webp'
        ? 'data:image/webp;base64,Y29tcHJlc3NlZC1hdmF0YXI='
        : 'data:image/png;base64,b3JpZ2luYWwtYXZhdGFy'
      this.onload?.call(this as unknown as FileReader, new ProgressEvent('load'))
    }
  }

  class MockImage {
    naturalWidth = 1200
    naturalHeight = 1200
    onload: (() => void) | null = null
    onerror: (() => void) | null = null
    set src(_value: string) { this.onload?.() }
  }

  globalThis.FileReader = MockFileReader as unknown as typeof FileReader
  globalThis.Image = MockImage as unknown as typeof Image
  vi.spyOn(document, 'createElement').mockImplementation(((tagName: string, options?: ElementCreationOptions) => {
    if (tagName === 'canvas') {
      return {
        width: 0,
        height: 0,
        getContext: () => ({ clearRect: vi.fn(), drawImage: vi.fn() }),
        toBlob: (callback: BlobCallback) => callback(new Blob([new Uint8Array(blobSize)], { type: 'image/webp' })),
      } as unknown as HTMLCanvasElement
    }
    return originalCreateElement(tagName, options)
  }) as typeof document.createElement)
}

describe('AgentProfileAvatarCard', () => {
  beforeEach(() => vi.restoreAllMocks())

  afterEach(() => {
    globalThis.FileReader = originalFileReader
    globalThis.Image = originalImage
    vi.restoreAllMocks()
  })

  it('compresses a large image, previews it and saves through the fixed AgentAPI endpoint', async () => {
    installCompressionMocks()
    const updated = { ...profile, avatar_url: 'data:image/webp;base64,Y29tcHJlc3NlZC1hdmF0YXI=' }
    const update = vi.spyOn(agentAPI.profile.avatar, 'update').mockResolvedValue(updated)
    const wrapper = mount(AgentProfileAvatarCard, { props: { profile } })
    const input = wrapper.get('[data-testid="profile-avatar-file-input"]')
    Object.defineProperty(input.element, 'files', {
      value: [new File([new Uint8Array(220 * 1024)], 'avatar.png', { type: 'image/png' })],
      configurable: true,
    })

    await input.trigger('change')
    await flushAsyncWork()
    expect(wrapper.get('[data-testid="profile-avatar-preview"]').attributes('src')).toBe(updated.avatar_url)
    await wrapper.get('[data-testid="profile-avatar-save"]').trigger('click')

    expect(update).toHaveBeenCalledWith(updated.avatar_url)
    expect(wrapper.emitted('updated')?.[0]).toEqual([updated])
    expect(wrapper.text()).toContain('头像已更新')
  })

  it('removes the current avatar and emits the authoritative profile', async () => {
    const current = { ...profile, avatar_url: 'data:image/png;base64,b2xk' }
    const updated = { ...profile, avatar_url: '' }
    const update = vi.spyOn(agentAPI.profile.avatar, 'update').mockResolvedValue(updated)
    const wrapper = mount(AgentProfileAvatarCard, { props: { profile: current } })

    await wrapper.get('[data-testid="profile-avatar-delete"]').trigger('click')

    expect(update).toHaveBeenCalledWith('')
    expect(wrapper.emitted('updated')?.[0]).toEqual([updated])
    expect(wrapper.text()).toContain('头像已删除')
  })

  it('rejects unsupported files without calling AgentAPI', async () => {
    const update = vi.spyOn(agentAPI.profile.avatar, 'update')
    const wrapper = mount(AgentProfileAvatarCard, { props: { profile } })
    const input = wrapper.get('[data-testid="profile-avatar-file-input"]')
    Object.defineProperty(input.element, 'files', {
      value: [new File(['<svg/>'], 'avatar.svg', { type: 'image/svg+xml' })],
      configurable: true,
    })

    await input.trigger('change')
    await flushAsyncWork()

    expect(wrapper.text()).toContain('请选择 PNG、JPEG、WebP 或 GIF')
    expect(update).not.toHaveBeenCalled()
  })
})
