<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { providerAPI, type ProviderGroupView, type ProviderOwnedAccount } from '@/api/provider'

const accounts = ref<ProviderOwnedAccount[]>([])
const groups = ref<ProviderGroupView[]>([])
const loading = ref(false)
const listError = ref('')

const form = ref({ name: '', platform: 'anthropic', type: 'setup-token', credentialsJSON: '', group_id: 0 })
const submitting = ref(false)
const formError = ref('')
const formOk = ref('')

async function refresh() {
  loading.value = true
  listError.value = ''
  try {
    accounts.value = await providerAPI.listAccounts()
    groups.value = await providerAPI.groups()
  } catch (e: unknown) {
    listError.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

async function submitAccount() {
  formError.value = ''
  formOk.value = ''
  if (!form.value.name.trim()) { formError.value = '请填写名称'; return }
  if (!form.value.group_id) { formError.value = '请选择授权分组'; return }
  let credentials: Record<string, unknown>
  try { credentials = JSON.parse(form.value.credentialsJSON || '{}') } catch { formError.value = '凭据不是合法 JSON'; return }
  submitting.value = true
  try {
    const res = await providerAPI.createAccount({
      name: form.value.name.trim(), platform: form.value.platform, type: form.value.type,
      credentials, group_id: Number(form.value.group_id),
    })
    formOk.value = `已提交,账号 #${res.id}(${res.status || '待校验'})`
    form.value.name = ''; form.value.credentialsJSON = ''
    await refresh()
  } catch (e: unknown) {
    formError.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '提交失败'
  } finally {
    submitting.value = false
  }
}

async function removeAccount(a: ProviderOwnedAccount) {
  if (!window.confirm(`确认撤下账号「${a.name}」?`)) return
  try { await providerAPI.deleteAccount(a.id); await refresh() }
  catch (e: unknown) { listError.value = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '删除失败' }
}

function statusClass(s: string): string {
  const x = (s || '').toLowerCase()
  if (x.includes('active') || x.includes('normal') || x.includes('ok')) return 'ok'
  if (x.includes('rate') || x.includes('limit')) return 'warn'
  if (x.includes('disable') || x.includes('error') || x.includes('expired') || x.includes('invalid')) return 'bad'
  return 'muted'
}
function fmt(t?: string): string { if (!t) return '—'; const d = new Date(t); return isNaN(d.getTime()) ? '—' : d.toLocaleString() }

onMounted(refresh)
</script>

<template>
  <div>
    <h1 class="pv-h1">账户管理</h1>
    <section class="pv-panel">
      <div class="pv-head"><h2 class="pv-h2">我的账号</h2><button class="pv-ghost" :disabled="loading" @click="refresh">{{ loading ? '刷新中…' : '刷新' }}</button></div>
      <p v-if="listError" class="pv-err">{{ listError }}</p>
      <div class="pv-tw">
        <table class="pv-table">
          <thead><tr><th>名称</th><th>平台</th><th>状态</th><th>最后使用</th><th>过期</th><th></th></tr></thead>
          <tbody>
            <tr v-for="a in accounts" :key="a.id">
              <td>{{ a.name }}</td>
              <td>{{ a.platform }}<span class="pv-dim"> / {{ a.type }}</span></td>
              <td><span class="pv-badge" :class="statusClass(a.status)">{{ a.status || '—' }}</span><div v-if="a.error_message" class="pv-dim pv-em">{{ a.error_message }}</div></td>
              <td>{{ fmt(a.last_used_at) }}</td>
              <td>{{ fmt(a.expires_at) }}</td>
              <td><button class="pv-danger" @click="removeAccount(a)">撤下</button></td>
            </tr>
            <tr v-if="!loading && accounts.length === 0"><td colspan="6" class="pv-empty">暂无账号,下方授权一个。</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="pv-panel">
      <h2 class="pv-h2">授权 / 添加账号</h2>
      <div class="pv-row"><label>名称</label><input v-model="form.name" class="pv-in" placeholder="给这个账号起个名" /></div>
      <div class="pv-row two">
        <div><label>平台</label><select v-model="form.platform" class="pv-in"><option>anthropic</option><option>openai</option><option>gemini</option><option>grok</option></select></div>
        <div><label>类型</label><select v-model="form.type" class="pv-in"><option>setup-token</option><option>oauth</option><option>apikey</option><option>upstream</option></select></div>
      </div>
      <div class="pv-row"><label>授权分组</label>
        <select v-model="form.group_id" class="pv-in">
          <option :value="0" disabled>选择分组</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}(#{{ g.id }})</option>
        </select>
      </div>
      <div class="pv-row"><label>凭据(JSON)</label><textarea v-model="form.credentialsJSON" rows="5" class="pv-in pv-mono" placeholder='{"refresh_token":"..."} 或 {"api_key":"..."}'></textarea></div>
      <p v-if="formError" class="pv-err">{{ formError }}</p>
      <p v-if="formOk" class="pv-ok">{{ formOk }}</p>
      <button class="pv-primary" :disabled="submitting" @click="submitAccount">{{ submitting ? '提交中…' : '提交账号' }}</button>
    </section>
  </div>
</template>

<style scoped>
.pv-h1 { font-size: 20px; margin: 0 0 20px; }
.pv-h2 { font-size: 15px; margin: 0 0 12px; }
.pv-panel { background: #1e293b; border: 1px solid #334155; border-radius: 12px; padding: 18px; margin-bottom: 18px; }
.pv-head { display: flex; justify-content: space-between; align-items: center; }
.pv-tw { overflow-x: auto; }
.pv-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.pv-table th, .pv-table td { text-align: left; padding: 8px 10px; border-bottom: 1px solid #334155; white-space: nowrap; }
.pv-table th { color: #94a3b8; font-weight: 500; }
.pv-dim { color: #94a3b8; }
.pv-em { max-width: 260px; white-space: normal; font-size: 11px; }
.pv-badge { padding: 2px 8px; border-radius: 999px; font-size: 12px; }
.pv-badge.ok { background: #064e3b; color: #6ee7b7; } .pv-badge.warn { background: #78350f; color: #fcd34d; }
.pv-badge.bad { background: #7f1d1d; color: #fca5a5; } .pv-badge.muted { background: #334155; color: #cbd5e1; }
.pv-empty { color: #94a3b8; text-align: center; padding: 18px; }
label { display: block; font-size: 12px; color: #cbd5e1; margin-bottom: 5px; }
.pv-row { margin-bottom: 12px; } .pv-row.two { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.pv-in { width: 100%; box-sizing: border-box; padding: 9px 11px; border-radius: 8px; border: 1px solid #475569; background: #0f172a; color: #f1f5f9; font-size: 13px; }
.pv-mono { font-family: ui-monospace, monospace; resize: vertical; }
.pv-primary { padding: 10px 18px; border: none; border-radius: 8px; background: #6366f1; color: #fff; font-weight: 600; cursor: pointer; }
.pv-primary:disabled { opacity: .6; }
.pv-ghost { padding: 6px 12px; border: 1px solid #475569; border-radius: 7px; background: transparent; color: #cbd5e1; cursor: pointer; font-size: 13px; }
.pv-danger { border: none; background: transparent; color: #f87171; cursor: pointer; font-size: 13px; }
.pv-err { color: #f87171; font-size: 13px; } .pv-ok { color: #6ee7b7; font-size: 13px; }
</style>
