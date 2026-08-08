<template>
  <AppLayout>
    <div class="space-y-6">
      <header class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p class="text-sm font-medium text-primary-600 dark:text-primary-400">
            {{ t('admin.fingerprintIsolation.eyebrow') }}
          </p>
          <h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('admin.fingerprintIsolation.title') }}
          </h1>
          <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.fingerprintIsolation.description') }}
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="loadingProfiles || loadingAccounts"
            :title="t('common.refresh')"
            @click="loadPage"
          >
            <Icon name="refresh" size="sm" class="mr-1.5" :class="{ 'animate-spin': loadingProfiles || loadingAccounts }" />
            {{ t('common.refresh') }}
          </button>
          <button type="button" class="btn btn-primary" @click="showProfilesModal = true">
            <Icon name="lock" size="sm" class="mr-1.5" />
            {{ t('admin.fingerprintIsolation.manageProfiles') }}
          </button>
        </div>
      </header>

      <div
        v-if="errorMessage"
        class="flex items-start justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-800/60 dark:bg-red-950/30 dark:text-red-300"
      >
        <span>{{ errorMessage }}</span>
        <button type="button" class="shrink-0 text-xs font-medium underline" @click="loadPage">
          {{ t('common.refresh') }}
        </button>
      </div>

      <section class="grid grid-cols-2 gap-4 xl:grid-cols-4" aria-label="Fingerprint connection overview">
        <div v-for="stat in stats" :key="stat.label" class="card p-4">
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ stat.label }}</p>
              <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">{{ stat.value }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ stat.detail }}</p>
            </div>
            <span :class="['rounded-lg p-2', stat.iconClass]">
              <Icon :name="stat.icon" size="md" />
            </span>
          </div>
        </div>
      </section>

      <section class="card overflow-hidden">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-4 py-4 dark:border-dark-700 sm:px-6">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.fingerprintIsolation.accountsTitle') }}
            </h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.fingerprintIsolation.accountsDescription') }}
            </p>
          </div>
          <div class="flex w-full items-center gap-2 sm:w-auto">
            <div class="relative min-w-0 flex-1 sm:w-64">
              <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input
                v-model="searchQuery"
                type="search"
                class="input pl-9"
                :placeholder="t('admin.fingerprintIsolation.searchAccounts')"
                @keyup.enter="searchAccounts"
              />
            </div>
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="loadingAccounts"
              :title="t('common.search')"
              @click="searchAccounts"
            >
              <Icon name="search" size="sm" />
              <span class="sr-only">{{ t('common.search') }}</span>
            </button>
          </div>
        </div>

        <div v-if="loadingAccounts" class="flex items-center justify-center py-16">
          <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
        </div>
        <div v-else-if="accounts.length === 0" class="px-6 py-16 text-center">
          <Icon name="server" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
          <h3 class="mt-3 text-sm font-semibold text-gray-900 dark:text-white">
            {{ searchQuery ? t('admin.fingerprintIsolation.noSearchResults') : t('admin.fingerprintIsolation.noAccounts') }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ searchQuery ? t('admin.fingerprintIsolation.noSearchResultsHint') : t('admin.fingerprintIsolation.noAccountsHint') }}
          </p>
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800/80">
              <tr>
                <th v-for="column in columns" :key="column" class="whitespace-nowrap px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400 sm:px-6">
                  {{ t(`admin.fingerprintIsolation.columns.${column}`) }}
                </th>
                <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500 dark:text-gray-400 sm:px-6">
                  {{ t('admin.fingerprintIsolation.columns.actions') }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
              <tr v-for="account in accounts" :key="account.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                <td class="max-w-[220px] px-4 py-3 sm:px-6">
                  <div class="truncate font-medium text-gray-900 dark:text-white" :title="account.name">{{ account.name }}</div>
                  <div class="mt-0.5 text-xs text-gray-400">#{{ account.id }}</div>
                </td>
                <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                  <span class="font-medium">{{ account.platform }}</span>
                  <span class="ml-1 text-xs text-gray-400">{{ account.type }}</span>
                </td>
                <td class="px-4 py-3">
                  <span :class="statusClass(account.status)">{{ account.status }}</span>
                </td>
                <td class="px-4 py-3">
                  <div v-if="account.enable_tls_fingerprint && profileFor(account)" class="space-y-1">
                    <div class="flex items-center gap-1.5 text-sm font-medium text-gray-800 dark:text-gray-100">
                      <Icon name="lock" size="sm" class="text-primary-500" />
                      {{ profileFor(account)?.name }}
                    </div>
                    <code v-if="profileFor(account)?.metadata?.fingerprint_key" class="text-xs text-gray-400">
                      {{ profileFor(account)?.metadata?.fingerprint_key?.slice(0, 12) }}
                    </code>
                  </div>
                  <span v-else class="text-sm text-gray-400">{{ t('admin.fingerprintIsolation.unbound') }}</span>
                </td>
                <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                  <span v-if="account.proxy_id">#{{ account.proxy_id }}</span>
                  <span v-else class="text-gray-400">{{ t('admin.fingerprintIsolation.direct') }}</span>
                </td>
                <td class="px-4 py-3 text-right">
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm"
                    :title="t('admin.fingerprintIsolation.openDiagnostic')"
                    @click="openAccountTest(account)"
                  >
                    <Icon name="chart" size="sm" class="mr-1.5" />
                    {{ t('admin.fingerprintIsolation.diagnose') }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="pagination.total > 0" class="border-t border-gray-200 dark:border-dark-700">
          <Pagination
            :page="pagination.page"
            :total="pagination.total"
            :page-size="pagination.page_size"
            @update:page="handlePageChange"
            @update:page-size="handlePageSizeChange"
          />
        </div>
      </section>

      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.fingerprintIsolation.footerHint') }}
      </p>
    </div>

    <TLSFingerprintProfilesModal :show="showProfilesModal" @close="handleProfilesClosed" />
    <AccountTestModal :show="showAccountTestModal" :account="selectedAccount" @close="closeAccountTest" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { TLSFingerprintProfile } from '@/api/admin/tlsFingerprintProfile'
import type { Account } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import AccountTestModal from '@/components/account/AccountTestModal.vue'
import TLSFingerprintProfilesModal from '@/components/admin/TLSFingerprintProfilesModal.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const profiles = ref<TLSFingerprintProfile[]>([])
const accounts = ref<Account[]>([])
const loadingProfiles = ref(false)
const loadingAccounts = ref(false)
const errorMessage = ref('')
const searchQuery = ref('')
const showProfilesModal = ref(false)
const showAccountTestModal = ref(false)
const selectedAccount = ref<Account | null>(null)
const pagination = ref({ page: 1, page_size: 20, total: 0, pages: 0 })

const columns = ['account', 'platform', 'status', 'profile', 'proxy'] as const

const profileMap = computed(() => new Map(profiles.value.map((profile) => [profile.id, profile])))
const boundAccountCount = computed(() => accounts.value.filter((account) => account.enable_tls_fingerprint && profileFor(account)).length)
const configuredAccountCount = computed(() => accounts.value.filter((account) => account.enable_tls_fingerprint).length)

const stats = computed(() => [
  {
    label: t('admin.fingerprintIsolation.stats.profiles'),
    value: profiles.value.length,
    detail: t('admin.fingerprintIsolation.stats.profilesDetail'),
    icon: 'lock' as const,
    iconClass: 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-200'
  },
  {
    label: t('admin.fingerprintIsolation.stats.accounts'),
    value: pagination.value.total,
    detail: t('admin.fingerprintIsolation.stats.accountsDetail'),
    icon: 'server' as const,
    iconClass: 'bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300'
  },
  {
    label: t('admin.fingerprintIsolation.stats.bound'),
    value: boundAccountCount.value,
    detail: t('admin.fingerprintIsolation.stats.boundDetail'),
    icon: 'link' as const,
    iconClass: 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300'
  },
  {
    label: t('admin.fingerprintIsolation.stats.diagnostic'),
    value: configuredAccountCount.value,
    detail: t('admin.fingerprintIsolation.stats.diagnosticDetail'),
    icon: 'chart' as const,
    iconClass: 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-300'
  }
])

const profileFor = (account: Account) => {
  if (!account.tls_fingerprint_profile_id) return undefined
  return profileMap.value.get(account.tls_fingerprint_profile_id)
}

const statusClass = (status: Account['status']) => {
  if (status === 'active') return 'badge badge-success'
  if (status === 'error') return 'badge badge-danger'
  return 'badge badge-gray'
}

const loadProfiles = async () => {
  loadingProfiles.value = true
  try {
    profiles.value = await adminAPI.tlsFingerprintProfiles.list()
  } catch (error) {
    console.error('Failed to load TLS fingerprint profiles:', error)
    errorMessage.value = t('admin.fingerprintIsolation.loadProfilesFailed')
  } finally {
    loadingProfiles.value = false
  }
}

const loadAccounts = async () => {
  loadingAccounts.value = true
  try {
    const response = await adminAPI.accounts.list(pagination.value.page, pagination.value.page_size, {
      search: searchQuery.value.trim() || undefined,
      sort_by: 'id',
      sort_order: 'desc',
      lite: 'true',
      include_scheduler_score: 'false'
    })
    accounts.value = response.items
    pagination.value = {
      page: response.page,
      page_size: response.page_size,
      total: response.total,
      pages: response.pages
    }
  } catch (error) {
    console.error('Failed to load accounts:', error)
    errorMessage.value = t('admin.fingerprintIsolation.loadAccountsFailed')
  } finally {
    loadingAccounts.value = false
  }
}

const loadPage = async () => {
  errorMessage.value = ''
  await Promise.all([loadProfiles(), loadAccounts()])
}

const searchAccounts = () => {
  pagination.value.page = 1
  loadAccounts()
}

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadAccounts()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.page = 1
  pagination.value.page_size = pageSize
  loadAccounts()
}

const openAccountTest = (account: Account) => {
  selectedAccount.value = account
  showAccountTestModal.value = true
}

const closeAccountTest = () => {
  showAccountTestModal.value = false
  selectedAccount.value = null
}

const handleProfilesClosed = () => {
  showProfilesModal.value = false
  loadProfiles()
}

onMounted(loadPage)
</script>
