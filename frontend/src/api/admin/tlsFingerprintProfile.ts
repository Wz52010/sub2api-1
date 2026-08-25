/**
 * Admin TLS Fingerprint Profile API endpoints
 * Handles TLS fingerprint profile CRUD for administrators
 */

import { apiClient } from '../client'

/**
 * TLS fingerprint profile interface
 */
export interface TLSFingerprintProfile {
  id: number
  name: string
  description: string | null
  enable_grease: boolean
  cipher_suites: number[]
  curves: number[]
  point_formats: number[]
  signature_algorithms: number[]
  alpn_protocols: string[]
  supported_versions: number[]
  key_share_groups: number[]
  psk_modes: number[]
  extensions: number[]
  created_at: string
  updated_at: string
  metadata?: TLSFingerprintProfileMetadata
}

export interface TLSFingerprintProfileMetadata {
  client_type: string
  client_version_range: string
  tls_version_range: string
  alpn_preference: string
  fingerprint_key: string
}

/**
 * Create profile request
 */
export interface CreateProfileRequest {
  name: string
  description?: string | null
  enable_grease?: boolean
  cipher_suites?: number[]
  curves?: number[]
  point_formats?: number[]
  signature_algorithms?: number[]
  alpn_protocols?: string[]
  supported_versions?: number[]
  key_share_groups?: number[]
  psk_modes?: number[]
  extensions?: number[]
}

/**
 * Update profile request
 */
export interface UpdateProfileRequest {
  name?: string
  description?: string | null
  enable_grease?: boolean
  cipher_suites?: number[]
  curves?: number[]
  point_formats?: number[]
  signature_algorithms?: number[]
  alpn_protocols?: string[]
  supported_versions?: number[]
  key_share_groups?: number[]
  psk_modes?: number[]
  extensions?: number[]
}

export async function list(): Promise<TLSFingerprintProfile[]> {
  const { data } = await apiClient.get<TLSFingerprintProfile[]>('/admin/tls-fingerprint-profiles')
  return data
}

export async function getById(id: number): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.get<TLSFingerprintProfile>(`/admin/tls-fingerprint-profiles/${id}`)
  return data
}

export async function create(profileData: CreateProfileRequest): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.post<TLSFingerprintProfile>('/admin/tls-fingerprint-profiles', profileData)
  return data
}

export async function update(id: number, updates: UpdateProfileRequest): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.put<TLSFingerprintProfile>(`/admin/tls-fingerprint-profiles/${id}`, updates)
  return data
}

export async function deleteProfile(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/tls-fingerprint-profiles/${id}`)
  return data
}

export interface FingerprintProbeResult {
  echo_url: string
  profile_name: string
  protocol_mode: string
  http_version: string
  ja3: string
  ja3_hash: string
  ja4: string
  peetprint_hash: string
  h2_akamai_fingerprint: string
  h2_akamai_fingerprint_hash: string
  expected_h2_akamai: string
  h2_match: boolean
  // false 表示该模板是 h1-only 客户端(Node/undici,真实 Claude Code),H2 维度不适用,
  // 验证以 TLS 层(JA3/JA4)为准;true 表示真正走 H2(如 Codex Rust),才比对 Akamai。
  h2_applicable: boolean
}

// verify 用该 Profile 的真实指纹 transport 打 tls.peet.ws，回传实测 JA3/JA4/H2 与目标比对（只读诊断）。
export async function verify(id: number): Promise<FingerprintProbeResult> {
  const { data } = await apiClient.post<FingerprintProbeResult>(`/admin/tls-fingerprint-profiles/${id}/verify`)
  return data
}

export const tlsFingerprintProfileAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteProfile,
  verify
}

export default tlsFingerprintProfileAPI
