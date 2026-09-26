import axios, { type AxiosError, type AxiosInstance, type AxiosResponse } from 'axios'
import { errorMessage as localizeErrorMessage } from './locale'

const DEFAULT_API_BASE = '/api/v1'

function apiBaseURL(): string {
  const raw = String(import.meta.env.VITE_API_BASE_URL || DEFAULT_API_BASE).trim()
  return raw.replace(/\/+$/, '') || DEFAULT_API_BASE
}

export interface AgentAPIError {
  status: number
  code?: string | number
  reason?: string
  message: string
  metadata?: unknown
}

export const agentClient: AxiosInstance = axios.create({
  baseURL: apiBaseURL(),
  withCredentials: true,
  timeout: 30_000,
  headers: { 'Content-Type': 'application/json' },
})

function codeValue(value: unknown): string | number | undefined {
  return typeof value === 'string' || typeof value === 'number' ? value : undefined
}

function unwrap(response: AxiosResponse): AxiosResponse | Promise<never> {
  const payload = response.data as Record<string, unknown> | null
  if (!payload || typeof payload !== 'object' || !('code' in payload)) return response
  if (payload.code === 0) {
    response.data = payload.data
    return response
  }
  return Promise.reject({
    status: response.status,
    code: codeValue(payload.reason) ?? codeValue(payload.code),
    message: String(payload.message || '请求失败，请稍后重试。'),
    metadata: payload.metadata,
  } satisfies AgentAPIError)
}

agentClient.interceptors.response.use(unwrap, (error: AxiosError<Record<string, unknown>>) => {
  if (error.response) {
    const payload = error.response.data || {}
    return Promise.reject({
      status: error.response.status,
      code: codeValue(payload.reason) ?? codeValue(payload.code),
      message: String(payload.message || payload.detail || error.message || '请求失败，请稍后重试。'),
      metadata: payload.metadata,
    } satisfies AgentAPIError)
  }
  return Promise.reject({ status: 0, message: '网络连接失败，请检查与 AgentAPI 的连接。' } satisfies AgentAPIError)
})

export function errorMessage(error: unknown, fallback: string): string {
  return localizeErrorMessage(error, fallback)
}
