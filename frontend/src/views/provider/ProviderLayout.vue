<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { useProviderStore } from '@/stores/provider'

const route = useRoute()
const router = useRouter()
const providerStore = useProviderStore()

const menu = [
  { path: '/provider/dashboard', label: '仪表盘', icon: '📊' },
  { path: '/provider/accounts', label: '账户管理', icon: '🔑' },
  { path: '/provider/groups', label: '分组(只读)', icon: '🗂️' },
  { path: '/provider/proxies', label: 'IP 管理', icon: '🌐' },
  { path: '/provider/usage', label: '使用记录', icon: '📈' },
  { path: '/provider/audit', label: '操作日志', icon: '📝' },
]

function logout() {
  providerStore.logout()
  router.replace('/provider/login')
}
</script>

<template>
  <div class="pl-shell">
    <aside class="pl-side">
      <div class="pl-brand">供货商门户</div>
      <nav class="pl-nav">
        <RouterLink
          v-for="m in menu"
          :key="m.path"
          :to="m.path"
          class="pl-item"
          :class="{ active: route.path === m.path }"
        >
          <span class="pl-ic">{{ m.icon }}</span>{{ m.label }}
        </RouterLink>
      </nav>
      <div class="pl-foot">
        <div class="pl-who">{{ providerStore.provider?.name }}</div>
        <div class="pl-mail">{{ providerStore.provider?.email }}</div>
        <button class="pl-logout" @click="logout">退出登录</button>
      </div>
    </aside>
    <main class="pl-main">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.pl-shell { display: flex; min-height: 100vh; background: #0f172a; color: #e2e8f0; }
.pl-side { width: 220px; flex-shrink: 0; background: #111827; border-right: 1px solid #1f2937; display: flex; flex-direction: column; }
.pl-brand { padding: 20px 18px; font-size: 17px; font-weight: 700; color: #f1f5f9; border-bottom: 1px solid #1f2937; }
.pl-nav { flex: 1; padding: 10px 8px; }
.pl-item { display: flex; align-items: center; gap: 10px; padding: 10px 12px; margin: 2px 0; border-radius: 8px; color: #cbd5e1; text-decoration: none; font-size: 14px; }
.pl-item:hover { background: #1f2937; }
.pl-item.active { background: #6366f1; color: #fff; }
.pl-ic { width: 20px; text-align: center; }
.pl-foot { padding: 14px 16px; border-top: 1px solid #1f2937; }
.pl-who { font-size: 13px; font-weight: 600; }
.pl-mail { font-size: 11px; color: #94a3b8; margin-bottom: 10px; word-break: break-all; }
.pl-logout { width: 100%; padding: 7px; border: 1px solid #475569; border-radius: 7px; background: transparent; color: #cbd5e1; cursor: pointer; font-size: 13px; }
.pl-main { flex: 1; padding: 26px 30px; overflow-x: auto; }
@media (max-width: 720px) {
  .pl-shell { flex-direction: column; }
  .pl-side { width: 100%; }
  .pl-nav { display: flex; flex-wrap: wrap; }
}
</style>
