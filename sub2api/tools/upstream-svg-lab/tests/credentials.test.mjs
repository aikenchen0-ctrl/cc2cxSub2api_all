import test from 'node:test'
import assert from 'node:assert/strict'

import {
  cleanSourceURL,
  parseCredentials,
  responsesEndpoint,
} from '../lib/credentials.mjs'

test('parseCredentials accepts one explicit URL and a labelled Bearer key', () => {
  const parsed = parseCredentials(
    'endpoint: https://api.example.test/v1/ \nAuthorization: Bearer sk-live_1234567890'
  )

  assert.deepEqual(parsed, {
    url: 'https://api.example.test/v1',
    key: 'sk-live_1234567890',
  })
})

test('parseCredentials accepts JSON api_key and does not treat SSH text as credentials', () => {
  const parsed = parseCredentials(
    '{"base_url":"https://relay.example.test","api_key":"key_live_1234567890"}'
  )

  assert.equal(parsed.url, 'https://relay.example.test')
  assert.equal(parsed.key, 'key_live_1234567890')

  assert.throws(
    () => parseCredentials('192.0.2.10 fixture-ssh-password root'),
    /明确的 http\(s\) URL/
  )
})

test('parseCredentials accepts an sk key only when it is clearly delimited', () => {
  const parsed = parseCredentials(
    'https://api.example.test/v1\nsk-live_1234567890'
  )

  assert.equal(parsed.key, 'sk-live_1234567890')
})

test('parseCredentials rejects missing, ambiguous, or unlabelled secrets', () => {
  assert.throws(
    () => parseCredentials('https://api.example.test/v1'),
    /未检测到 API Key/
  )
  assert.throws(
    () => parseCredentials('https://one.example.test\nhttps://two.example.test\napi_key=key_1234567890'),
    /多个 URL/
  )
  assert.throws(
    () => parseCredentials('https://api.example.test\napi_key=key_1234567890\nBearer key_abcdefghijk'),
    /多个 Key/
  )
  assert.throws(
    () => parseCredentials('https://api.example.test\nfixture-ssh-password'),
    /未检测到 API Key/
  )
})

test('parseCredentials rejects example or redacted keys', () => {
  assert.throws(
    () => parseCredentials('https://api.example.test\napi_key=your-key'),
    /示例或已脱敏/
  )
  assert.throws(
    () => parseCredentials('https://api.example.test\napi_key=key_live_***'),
    /未检测到 API Key|示例或已脱敏|脱敏符号/
  )
  assert.throws(
    () => parseCredentials('https://api.example.test\napi_key=key_live_*'),
    /示例或已脱敏|脱敏符号/
  )
})

test('cleanSourceURL rejects credentials, query secrets, fragments, and non-http URLs', () => {
  assert.equal(
    cleanSourceURL(' https://api.example.test/v1///, '),
    'https://api.example.test/v1'
  )
  assert.throws(
    () => cleanSourceURL('ssh://root@example.test:22'),
    /http:\/\/ 或 https:\/\//
  )
  assert.throws(
    () => cleanSourceURL('https://user:password@example.test/v1'),
    /不能包含用户名或密码/
  )
  assert.throws(
    () => cleanSourceURL('https://api.example.test/v1?api_key=secret'),
    /密钥移到独立的 Key 字段/
  )
  assert.throws(
    () => cleanSourceURL('https://api.example.test/v1#fragment'),
    /查询参数或片段/
  )
})

test('responsesEndpoint normalizes compatible base and endpoint paths', () => {
  assert.equal(
    responsesEndpoint('https://api.example.test'),
    'https://api.example.test/v1/responses'
  )
  assert.equal(
    responsesEndpoint('https://api.example.test/v1/'),
    'https://api.example.test/v1/responses'
  )
  assert.equal(
    responsesEndpoint('https://api.example.test/v1/chat/completions'),
    'https://api.example.test/v1/responses'
  )
  assert.equal(
    responsesEndpoint('https://api.example.test/v1/models'),
    'https://api.example.test/v1/responses'
  )
  assert.equal(
    responsesEndpoint('https://api.example.test/custom/responses'),
    'https://api.example.test/custom/responses'
  )
})

test('credential helpers never return a key as part of a URL', () => {
  const source = 'https://api.example.test/v1?key=key_live_1234567890'
  assert.throws(() => cleanSourceURL(source), /密钥移到独立的 Key 字段/)
  assert.throws(() => parseCredentials(`${source}\napi_key=key_live_1234567890`))
})

test('密钥在前、空行、HTTP IP 加端口和末尾不换行空格均可解析', () => {
  const key = `sk-${'a'.repeat(61)}`
  for (const newline of ['\n', '\r\n']) {
    const text = `${key}${newline}${newline}http://23.251.32.58:8080/v1\u00a0`
    assert.deepEqual(parseCredentials(text), { url: 'http://23.251.32.58:8080/v1', key })
  }
  assert.equal(responsesEndpoint('http://23.251.32.58:8080/v1\u00a0'), 'http://23.251.32.58:8080/v1/responses')
})

test('只清理复制文本行边界的零宽字符，不更改密钥内部字节', () => {
  const key = `sk-${'b'.repeat(61)}`
  assert.deepEqual(
    parseCredentials(`\u200B${key}\u2060\n\n\uFEFFhttp://192.0.2.7:8080/v1\u200B\u00a0`),
    { url: 'http://192.0.2.7:8080/v1', key }
  )
  assert.equal(cleanSourceURL('\u200Bhttp://192.0.2.7:8080/v1\u2060'), 'http://192.0.2.7:8080/v1')
})
