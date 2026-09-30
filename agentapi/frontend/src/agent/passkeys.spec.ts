import { describe, expect, it } from 'vitest'
import { base64URLToBuffer, bufferToBase64URL, creationOptionsFromJSON, requestOptionsFromJSON } from './passkeys'

describe('agent passkey WebAuthn conversion', () => {
  it('round-trips URL-safe base64 without padding', () => {
    const source = new Uint8Array([0, 1, 2, 250, 251, 252]).buffer
    const encoded = bufferToBase64URL(source)
    expect(encoded).toBe('AAEC-vv8')
    expect(Array.from(new Uint8Array(base64URLToBuffer(encoded!)))).toEqual([0, 1, 2, 250, 251, 252])
  })

  it('converts registration and assertion challenge fields to buffers', () => {
    const creation = creationOptionsFromJSON({
      challenge: 'AQID',
      rp: { id: 'example.test', name: 'Example' },
      user: { id: 'BAUG', name: 'u@example.com', displayName: 'User' },
      pubKeyCredParams: [],
      excludeCredentials: [{ type: 'public-key', id: 'BwgJ' }],
    })
    expect(Array.from(new Uint8Array(creation.challenge))).toEqual([1, 2, 3])
    expect(Array.from(new Uint8Array(creation.user.id))).toEqual([4, 5, 6])
    expect(Array.from(new Uint8Array(creation.excludeCredentials![0].id))).toEqual([7, 8, 9])

    const request = requestOptionsFromJSON({ challenge: 'AQID', allowCredentials: [{ type: 'public-key', id: 'BAUG' }] })
    expect(Array.from(new Uint8Array(request.challenge))).toEqual([1, 2, 3])
    expect(Array.from(new Uint8Array(request.allowCredentials![0].id))).toEqual([4, 5, 6])
  })
})
