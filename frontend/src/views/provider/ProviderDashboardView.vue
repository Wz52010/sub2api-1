<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { providerAPI, type ProviderDashboard } from '@/api/provider'

const d = ref<ProviderDashboard | null>(null)
const loading = ref(false)
const err = ref('')

async function load() {
  loading.value = true
  err.value = ''
  try {
    d.value = await providerAPI.dashboard()
  } catch (e: unknown) {
    err.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '加载失败'
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <div>
    <h1 class="pv-h1">仪表盘</h1>
    <p v-if="err" class="pv-err">{{ err }}</p>
    <div v-if="d" class="pv-cards">
      <div class="pv-card"><div class="pv-num">{{ d.total_accounts }}</div><div class="pv-cap">我的账号总数</div></div>
      <div class="pv-card"><div class="pv-num">{{ d.proxy_count }}</div><div class="pv-cap">我的代理数</div></div>
      <div class="pv-card"><div class="pv-num">{{ d.today_requests }}</div><div class="pv-cap">今日请求数</div></div>
      <div class="pv-card"><div class="pv-num">{{ (d.today_input_tokens + d.today_output_tokens).toLocaleString() }}</div><div class="pv-cap">今日 token(入+出)</div></div>
    </div>
    <div v-if="d" class="pv-panel">
      <h2 class="pv-h2">账号状态分布</h2>
      <div class="pv-status">
        <span v-for="(n, st) in d.by_status" :key="st" class="pv-chip">{{ st || '未知' }}: <b>{{ n }}</b></span>
        <span v-if="Object.keys(d.by_status).length === 0" class="pv-dim">暂无账号</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pv-h1 { font-size: 20px; margin: 0 0 20px; }
.pv-h2 { font-size: 15px; margin: 0 0 12px; }
.pv-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 14px; margin-bottom: 20px; }
.pv-card { background: #1e293b; border: 1px solid #334155; border-radius: 12px; padding: 20px; }
.pv-num { font-size: 30px; font-weight: 700; color: #f1f5f9; }
.pv-cap { font-size: 13px; color: #94a3b8; margin-top: 6px; }
.pv-panel { background: #1e293b; border: 1px solid #334155; border-radius: 12px; padding: 18px; }
.pv-status { display: flex; flex-wrap: wrap; gap: 10px; }
.pv-chip { background: #334155; padding: 6px 12px; border-radius: 999px; font-size: 13px; }
.pv-dim { color: #94a3b8; }
.pv-err { color: #f87171; }
</style>
