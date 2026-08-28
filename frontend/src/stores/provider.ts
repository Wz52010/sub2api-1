/**
 * 供货商门户认证 store。独立于 useAuthStore,使用 provider_token,互不干扰。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { providerAPI, PROVIDER_TOKEN_KEY, type ProviderInfo } from '@/api/provider'

const PROVIDER_INFO_KEY = 'provider_info'

export const useProviderStore = defineStore('provider', () => {
  const token = ref<string | null>(null)
  const provider = ref<ProviderInfo | null>(null)

  const isAuthenticated = computed(() => !!token.value)

  function restore(): void {
    token.value = localStorage.getItem(PROVIDER_TOKEN_KEY)
    const raw = localStorage.getItem(PROVIDER_INFO_KEY)
    if (raw) {
      try {
        provider.value = JSON.parse(raw) as ProviderInfo
      } catch {
        provider.value = null
      }
    }
  }

  async function login(email: string, password: string): Promise<void> {
    const res = await providerAPI.login(email, password)
    token.value = res.token
    provider.value = res.provider
    localStorage.setItem(PROVIDER_TOKEN_KEY, res.token)
    localStorage.setItem(PROVIDER_INFO_KEY, JSON.stringify(res.provider))
  }

  function logout(): void {
    token.value = null
    provider.value = null
    localStorage.removeItem(PROVIDER_TOKEN_KEY)
    localStorage.removeItem(PROVIDER_INFO_KEY)
  }

  return { token, provider, isAuthenticated, restore, login, logout }
})
