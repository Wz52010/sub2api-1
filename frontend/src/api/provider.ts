/**
 * 供货商二级门户 API 客户端。
 * 独立 axios 实例 + 独立 token(provider_token),与用户/管理端的 auth_token 完全隔离。
 */
import axios, { type AxiosInstance, type InternalAxiosRequestConfig } from 'axios'
import { getAPIBaseURL } from './url'

export const PROVIDER_TOKEN_KEY = 'provider_token'

const providerClient: AxiosInstance = axios.create({
  baseURL: getAPIBaseURL(),
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
})

providerClient.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = localStorage.getItem(PROVIDER_TOKEN_KEY)
  if (token && config.headers) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 后端信封 {code,message,data};code=0 时解出 data。
providerClient.interceptors.response.use(
  (response) => {
    const env = response.data as { code?: number; data?: unknown }
    if (env && typeof env === 'object' && 'code' in env && env.code === 0) {
      response.data = env.data
    }
    return response
  },
  (error) => Promise.reject(error),
)

export interface ProviderInfo {
  id: number
  name: string
  email: string
  allowed_group_ids: number[]
}

export interface ProviderOwnedAccount {
  id: number
  name: string
  platform: string
  type: string
  status: string
  error_message?: string
  last_used_at?: string
  rate_limited_at?: string
  rate_limit_reset_at?: string
  expires_at?: string
  created_at: string
}

export interface ProviderAccountUsage {
  requests: number
  input_tokens: number
  output_tokens: number
}

export interface ProviderDashboard {
  total_accounts: number
  by_status: Record<string, number>
  today_requests: number
  today_input_tokens: number
  today_output_tokens: number
  proxy_count: number
}

export interface ProviderGroupView {
  id: number
  name: string
  platform: string
  status: string
}

export interface ProviderProxy {
  id: number
  name: string
  protocol: string
  host: string
  port: number
  status: string
  created_at: string
}

export interface ProviderUsageRow {
  account_id: number
  account_name: string
  requests: number
  input_tokens: number
  output_tokens: number
}

export interface ProviderAuditRow {
  id: number
  action: string
  detail: string
  client_ip: string
  created_at: string
}

export const providerAPI = {
  async login(email: string, password: string): Promise<{ token: string; provider: ProviderInfo }> {
    const { data } = await providerClient.post('/provider/auth/login', { email, password })
    return data
  },
  async listAccounts(): Promise<ProviderOwnedAccount[]> {
    const { data } = await providerClient.get('/provider/accounts')
    return (data?.accounts ?? []) as ProviderOwnedAccount[]
  },
  async createAccount(payload: {
    name: string
    platform: string
    type: string
    credentials: Record<string, unknown>
    group_id: number
  }): Promise<{ id: number; status: string }> {
    const { data } = await providerClient.post('/provider/accounts', payload)
    return data
  },
  async accountUsage(id: number, window: 'day' | 'week' | 'month' = 'day'): Promise<ProviderAccountUsage> {
    const { data } = await providerClient.get(`/provider/accounts/${id}/usage`, { params: { window } })
    return data
  },
  async deleteAccount(id: number): Promise<void> {
    await providerClient.delete(`/provider/accounts/${id}`)
  },
  async dashboard(): Promise<ProviderDashboard> {
    const { data } = await providerClient.get('/provider/dashboard')
    return data
  },
  async groups(): Promise<ProviderGroupView[]> {
    const { data } = await providerClient.get('/provider/groups')
    return (data?.groups ?? []) as ProviderGroupView[]
  },
  async proxies(): Promise<ProviderProxy[]> {
    const { data } = await providerClient.get('/provider/proxies')
    return (data?.proxies ?? []) as ProviderProxy[]
  },
  async createProxy(payload: {
    name: string
    protocol: string
    host: string
    port: number
    username?: string
    password?: string
  }): Promise<{ id: number }> {
    const { data } = await providerClient.post('/provider/proxies', payload)
    return data
  },
  async deleteProxy(id: number): Promise<void> {
    await providerClient.delete(`/provider/proxies/${id}`)
  },
  async usage(window: 'day' | 'week' | 'month' = 'day'): Promise<ProviderUsageRow[]> {
    const { data } = await providerClient.get('/provider/usage', { params: { window } })
    return (data?.usage ?? []) as ProviderUsageRow[]
  },
  async audit(limit = 200): Promise<ProviderAuditRow[]> {
    const { data } = await providerClient.get('/provider/audit', { params: { limit } })
    return (data?.logs ?? []) as ProviderAuditRow[]
  },
}
