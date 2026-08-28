<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { providerAPI, type ProviderAuditRow } from '@/api/provider'

const rows = ref<ProviderAuditRow[]>([])
const loading = ref(false)
const err = ref('')

const actionLabel: Record<string, string> = {
  login: '登录',
  'account.create': '添加账号',
  'account.delete': '撤下账号',
  'proxy.create': '添加代理',
  'proxy.delete': '撤下代理',
}

async function load() {
  loading.value = true; err.value = ''
  try { rows.value = await providerAPI.audit(200) }
  catch (e: unknown) { err.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '加载失败' }
  finally { loading.value = false }
}
function fmt(t: string): string { const d = new Date(t); return isNaN(d.getTime()) ? t : d.toLocaleString() }
onMounted(load)
</script>

<template>
  <div>
    <h1 class="pv-h1">操作日志</h1>
    <p class="pv-note">仅记录你自己的操作(登录 / 添加撤下账号、代理),不含运营方或其他供货商的动作。</p>
    <p v-if="err" class="pv-err">{{ err }}</p>
    <div class="pv-panel">
      <table class="pv-table">
        <thead><tr><th>时间</th><th>操作</th><th>详情</th><th>IP</th></tr></thead>
        <tbody>
          <tr v-for="r in rows" :key="r.id"><td>{{ fmt(r.created_at) }}</td><td>{{ actionLabel[r.action] || r.action }}</td><td class="pv-dim">{{ r.detail || '—' }}</td><td class="pv-dim">{{ r.client_ip || '—' }}</td></tr>
          <tr v-if="!loading && rows.length === 0"><td colspan="4" class="pv-empty">暂无操作记录。</td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.pv-h1 { font-size: 20px; margin: 0 0 8px; }
.pv-note { color: #94a3b8; font-size: 13px; margin: 0 0 18px; }
.pv-panel { background: #1e293b; border: 1px solid #334155; border-radius: 12px; padding: 18px; overflow-x: auto; }
.pv-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.pv-table th, .pv-table td { text-align: left; padding: 8px 10px; border-bottom: 1px solid #334155; white-space: nowrap; }
.pv-table th { color: #94a3b8; font-weight: 500; }
.pv-dim { color: #94a3b8; }
.pv-empty { color: #94a3b8; text-align: center; padding: 18px; }
.pv-err { color: #f87171; }
</style>
