<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { providerAPI, type ProviderProxy } from '@/api/provider'

const proxies = ref<ProviderProxy[]>([])
const loading = ref(false)
const listError = ref('')
const form = ref({ name: '', protocol: 'socks5', host: '', port: 1080, username: '', password: '' })
const submitting = ref(false)
const formError = ref('')

async function refresh() {
  loading.value = true; listError.value = ''
  try { proxies.value = await providerAPI.proxies() }
  catch (e: unknown) { listError.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '加载失败' }
  finally { loading.value = false }
}
async function submit() {
  formError.value = ''
  if (!form.value.name.trim() || !form.value.host.trim() || !form.value.port) { formError.value = '名称/主机/端口必填'; return }
  submitting.value = true
  try {
    await providerAPI.createProxy({ name: form.value.name.trim(), protocol: form.value.protocol, host: form.value.host.trim(), port: Number(form.value.port), username: form.value.username || undefined, password: form.value.password || undefined })
    form.value.name = ''; form.value.host = ''; form.value.username = ''; form.value.password = ''
    await refresh()
  } catch (e: unknown) { formError.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '创建失败' }
  finally { submitting.value = false }
}
async function remove(p: ProviderProxy) {
  if (!window.confirm(`撤下代理「${p.name}」?`)) return
  try { await providerAPI.deleteProxy(p.id); await refresh() }
  catch (e: unknown) { listError.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '删除失败' }
}
onMounted(refresh)
</script>

<template>
  <div>
    <h1 class="pv-h1">IP 管理</h1>
    <p class="pv-note">这里只管理你自己提供的代理 IP,与运营方及其他供货商完全隔离。</p>
    <section class="pv-panel">
      <div class="pv-head"><h2 class="pv-h2">我的代理</h2><button class="pv-ghost" :disabled="loading" @click="refresh">{{ loading ? '刷新中…' : '刷新' }}</button></div>
      <p v-if="listError" class="pv-err">{{ listError }}</p>
      <div class="pv-tw">
        <table class="pv-table">
          <thead><tr><th>名称</th><th>协议</th><th>主机:端口</th><th>状态</th><th></th></tr></thead>
          <tbody>
            <tr v-for="p in proxies" :key="p.id"><td>{{ p.name }}</td><td>{{ p.protocol }}</td><td>{{ p.host }}:{{ p.port }}</td><td>{{ p.status || '—' }}</td><td><button class="pv-danger" @click="remove(p)">撤下</button></td></tr>
            <tr v-if="!loading && proxies.length === 0"><td colspan="5" class="pv-empty">暂无代理,下方添加。</td></tr>
          </tbody>
        </table>
      </div>
    </section>
    <section class="pv-panel">
      <h2 class="pv-h2">添加代理</h2>
      <div class="pv-row two"><div><label>名称</label><input v-model="form.name" class="pv-in" /></div><div><label>协议</label><select v-model="form.protocol" class="pv-in"><option>socks5</option><option>http</option><option>https</option></select></div></div>
      <div class="pv-row two"><div><label>主机</label><input v-model="form.host" class="pv-in" placeholder="1.2.3.4" /></div><div><label>端口</label><input v-model.number="form.port" type="number" class="pv-in" /></div></div>
      <div class="pv-row two"><div><label>用户名(可选)</label><input v-model="form.username" class="pv-in" /></div><div><label>密码(可选)</label><input v-model="form.password" type="password" class="pv-in" /></div></div>
      <p v-if="formError" class="pv-err">{{ formError }}</p>
      <button class="pv-primary" :disabled="submitting" @click="submit">{{ submitting ? '提交中…' : '添加' }}</button>
    </section>
  </div>
</template>

<style scoped>
.pv-h1 { font-size: 20px; margin: 0 0 8px; } .pv-h2 { font-size: 15px; margin: 0 0 12px; }
.pv-note { color: #94a3b8; font-size: 13px; margin: 0 0 18px; }
.pv-panel { background: #1e293b; border: 1px solid #334155; border-radius: 12px; padding: 18px; margin-bottom: 18px; }
.pv-head { display: flex; justify-content: space-between; align-items: center; }
.pv-tw { overflow-x: auto; }
.pv-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.pv-table th, .pv-table td { text-align: left; padding: 8px 10px; border-bottom: 1px solid #334155; white-space: nowrap; }
.pv-table th { color: #94a3b8; font-weight: 500; }
.pv-empty { color: #94a3b8; text-align: center; padding: 18px; }
label { display: block; font-size: 12px; color: #cbd5e1; margin-bottom: 5px; }
.pv-row { margin-bottom: 12px; } .pv-row.two { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.pv-in { width: 100%; box-sizing: border-box; padding: 9px 11px; border-radius: 8px; border: 1px solid #475569; background: #0f172a; color: #f1f5f9; font-size: 13px; }
.pv-primary { padding: 10px 18px; border: none; border-radius: 8px; background: #6366f1; color: #fff; font-weight: 600; cursor: pointer; }
.pv-primary:disabled { opacity: .6; }
.pv-ghost { padding: 6px 12px; border: 1px solid #475569; border-radius: 7px; background: transparent; color: #cbd5e1; cursor: pointer; font-size: 13px; }
.pv-danger { border: none; background: transparent; color: #f87171; cursor: pointer; font-size: 13px; }
.pv-err { color: #f87171; font-size: 13px; }
</style>
