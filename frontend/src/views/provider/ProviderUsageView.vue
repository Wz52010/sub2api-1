<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { providerAPI, type ProviderUsageRow } from '@/api/provider'

const rows = ref<ProviderUsageRow[]>([])
const loading = ref(false)
const err = ref('')
const win = ref<'day' | 'week' | 'month'>('day')

async function load() {
  loading.value = true; err.value = ''
  try { rows.value = await providerAPI.usage(win.value) }
  catch (e: unknown) { err.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '加载失败' }
  finally { loading.value = false }
}
function setWin(w: 'day' | 'week' | 'month') { win.value = w; load() }
onMounted(load)
</script>

<template>
  <div>
    <h1 class="pv-h1">使用记录</h1>
    <p class="pv-note">仅你名下账号的用量(token/请求数),不含任何客户/用户信息与金额。</p>
    <div class="pv-tabs">
      <button :class="{ on: win === 'day' }" @click="setWin('day')">今日</button>
      <button :class="{ on: win === 'week' }" @click="setWin('week')">近7天</button>
      <button :class="{ on: win === 'month' }" @click="setWin('month')">近30天</button>
    </div>
    <p v-if="err" class="pv-err">{{ err }}</p>
    <div class="pv-panel">
      <table class="pv-table">
        <thead><tr><th>账号</th><th>请求数</th><th>输入 token</th><th>输出 token</th></tr></thead>
        <tbody>
          <tr v-for="r in rows" :key="r.account_id"><td>{{ r.account_name }}<span class="pv-dim"> #{{ r.account_id }}</span></td><td>{{ r.requests.toLocaleString() }}</td><td>{{ r.input_tokens.toLocaleString() }}</td><td>{{ r.output_tokens.toLocaleString() }}</td></tr>
          <tr v-if="!loading && rows.length === 0"><td colspan="4" class="pv-empty">该时段暂无用量。</td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.pv-h1 { font-size: 20px; margin: 0 0 8px; }
.pv-note { color: #94a3b8; font-size: 13px; margin: 0 0 14px; }
.pv-tabs { display: flex; gap: 8px; margin-bottom: 14px; }
.pv-tabs button { padding: 6px 14px; border: 1px solid #475569; border-radius: 7px; background: transparent; color: #cbd5e1; cursor: pointer; font-size: 13px; }
.pv-tabs button.on { background: #6366f1; color: #fff; border-color: #6366f1; }
.pv-panel { background: #1e293b; border: 1px solid #334155; border-radius: 12px; padding: 18px; overflow-x: auto; }
.pv-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.pv-table th, .pv-table td { text-align: left; padding: 8px 10px; border-bottom: 1px solid #334155; white-space: nowrap; }
.pv-table th { color: #94a3b8; font-weight: 500; }
.pv-dim { color: #94a3b8; }
.pv-empty { color: #94a3b8; text-align: center; padding: 18px; }
.pv-err { color: #f87171; }
</style>
