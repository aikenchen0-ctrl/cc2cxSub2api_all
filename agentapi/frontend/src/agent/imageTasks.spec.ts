import { describe, expect, it } from 'vitest'
import { imageResultsFrom, imageTaskID, imageTaskIsTerminal, safeImageURL } from './imageTasks'

describe('agent image task helpers', () => {
  it('reads task identities from direct and wrapped responses', () => {
    expect(imageTaskID({ id: 'direct-task' })).toBe('direct-task')
    expect(imageTaskID({ data: { task_id: 'wrapped-task' } })).toBe('wrapped-task')
  })

  it('accepts public and supported image data URLs only', () => {
    expect(safeImageURL('https://media.example/result.png')).toBe('https://media.example/result.png')
    expect(safeImageURL('data:image/png;base64,aW1hZ2U=')).toBe('data:image/png;base64,aW1hZ2U=')
    expect(safeImageURL('javascript:alert(1)')).toBe('')
    expect(safeImageURL('data:text/html;base64,PGgxPkJvb208L2gxPg==')).toBe('')
  })

  it('extracts nested image results and terminal states', () => {
    expect(imageResultsFrom({ result: { data: [{ b64_json: 'aW1hZ2U=', output_format: 'webp' }] } })).toEqual([
      { url: 'data:image/webp;base64,aW1hZ2U=', revisedPrompt: undefined },
    ])
    expect(imageTaskIsTerminal('completed')).toBe(true)
    expect(imageTaskIsTerminal('processing')).toBe(false)
  })
})
