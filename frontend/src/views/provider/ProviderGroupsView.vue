<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { providerAPI, type ProviderGroupView } from '@/api/provider'

const groups = ref<ProviderGroupView[]>([])
const err = ref('')
onMounted(async () => {
  try { groups.value = await providerAPI.groups() }
  catch (e: unknown) { err.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '加载失败' }
})
</script>

<template>
  <div>
    <h1 class="pv-h1">分组(只读)</h1>
    <p class="pv-note">这是运营方为你授权的分组,你添加的账号只能放进这些分组。此处仅供查看,不含任何用户/客户信息。</p>
    <p v-if="err" class="pv-err">{{ err }}</p>
    <div class="pv-panel">
      <table class="pv-table">
        <thead><tr><th>ID</th><th>名称</th><th>平台</th><th>状态</th></tr></thead>
        <tbody>
          <tr v-for="g in groups" :key="g.id"><td>#{{ g.id }}</td><td>{{ g.name }}</td><td>{{ g.platform || '—' }}</td><td>{{ g.status || '—' }}</td></tr>
          <tr v-if="groups.length === 0"><td colspan="4" class="pv-empty">尚未被授权任何分组,联系运营方。</td></tr>
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
.pv-table th, .pv-table td { text-align: left; padding: 8px 10px; border-bottom: 1px solid #334155; }
.pv-table th { color: #94a3b8; font-weight: 500; }
.pv-empty { color: #94a3b8; text-align: center; padding: 18px; }
.pv-err { color: #f87171; }
</style>
