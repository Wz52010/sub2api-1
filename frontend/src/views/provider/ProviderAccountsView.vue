<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { providerAPI, type ProviderGroupView, type ProviderOwnedAccount } from '@/api/provider'

const accounts = ref<ProviderOwnedAccount[]>([])
const groups = ref<ProviderGroupView[]>([])
const loading = ref(false)
const listError = ref('')

function msg(e: unknown): string {
  return (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '操作失败'
}

async function refresh() {
  loading.value = true
  listError.value = ''
  try {
    accounts.value = await providerAPI.listAccounts()
    groups.value = await providerAPI.groups()
  } catch (e: unknown) {
    listError.value = msg(e)
  } finally {
    loading.value = false
  }
}

// ============ 添加账号:OAuth 授权(主) / 手工凭据(次) ============
const mode = ref<'oauth' | 'manual'>('oauth')

const PLATFORMS = [
  { value: 'anthropic', label: 'Claude', hint: 'Anthropic OAuth' },
  { value: 'openai', label: 'ChatGPT · Codex', hint: 'OpenAI OAuth' },
  { value: 'gemini', label: 'Gemini', hint: 'Google OAuth' },
]

// ---- OAuth 授权流 ----
const oauth = ref({
  platform: 'anthropic',
  name: '',
  group_id: 0,
  gemini_oauth_type: 'code_assist',
  gemini_tier_id: '',
  gemini_project_id: '',
})
const oauthUrl = ref('')
const oauthSessionId = ref('')
const oauthState = ref('')
const oauthCode = ref('')
const oauthBusy = ref(false)
const oauthErr = ref('')
const oauthOk = ref('')

function resetOAuthFlow() {
  oauthUrl.value = ''
  oauthSessionId.value = ''
  oauthState.value = ''
  oauthCode.value = ''
}

// 切换平台时重置已生成的链接(不同平台会话不通用)。
watch(() => oauth.value.platform, () => { resetOAuthFlow(); oauthErr.value = ''; oauthOk.value = '' })

// 从粘贴的回调 URL 里自动抽取 code + state(OpenAI/Gemini 回调是 localhost?code=...&state=...)。
watch(oauthCode, (v) => {
  const t = (v || '').trim()
  if (!t.includes('code=')) return
  try {
    const u = t.includes('?') ? new URL(t) : new URL('http://localhost/cb?' + t.replace(/^\?/, ''))
    const code = u.searchParams.get('code')
    const st = u.searchParams.get('state')
    if (st) oauthState.value = st
    if (code && code !== t) oauthCode.value = code
  } catch { /* 不是 URL 就当成裸 code,忽略 */ }
})

async function genAuthURL() {
  oauthErr.value = ''; oauthOk.value = ''
  if (!oauth.value.name.trim()) { oauthErr.value = '请先填账号名称'; return }
  if (!oauth.value.group_id) { oauthErr.value = '请选择授权分组'; return }
  oauthBusy.value = true
  try {
    const r = await providerAPI.oauthAuthURL({
      platform: oauth.value.platform,
      gemini_oauth_type: oauth.value.gemini_oauth_type,
      gemini_tier_id: oauth.value.gemini_tier_id || undefined,
      gemini_project_id: oauth.value.gemini_project_id || undefined,
    })
    oauthUrl.value = r.auth_url
    oauthSessionId.value = r.session_id
    oauthState.value = r.state || ''
    oauthCode.value = ''
  } catch (e: unknown) { oauthErr.value = msg(e) }
  finally { oauthBusy.value = false }
}

async function copyURL() {
  try { await navigator.clipboard.writeText(oauthUrl.value); oauthOk.value = '链接已复制'; setTimeout(() => (oauthOk.value = ''), 1500) }
  catch { /* 剪贴板不可用时用户可手动选中复制 */ }
}
function openURL() { if (oauthUrl.value) window.open(oauthUrl.value, '_blank', 'noopener') }

async function completeOAuth() {
  oauthErr.value = ''; oauthOk.value = ''
  if (!oauthSessionId.value) { oauthErr.value = '请先生成授权链接'; return }
  if (!oauthCode.value.trim()) { oauthErr.value = '请粘贴授权码 / 回调链接'; return }
  oauthBusy.value = true
  try {
    const res = await providerAPI.oauthExchange({
      platform: oauth.value.platform,
      session_id: oauthSessionId.value,
      code: oauthCode.value.trim(),
      state: oauthState.value || undefined,
      name: oauth.value.name.trim(),
      group_id: Number(oauth.value.group_id),
      gemini_oauth_type: oauth.value.gemini_oauth_type,
      gemini_tier_id: oauth.value.gemini_tier_id || undefined,
    })
    oauthOk.value = `授权成功,账号 #${res.id}(${res.status || '待校验'})`
    oauth.value.name = ''
    resetOAuthFlow()
    await refresh()
  } catch (e: unknown) { oauthErr.value = msg(e) }
  finally { oauthBusy.value = false }
}

// ---- 手工凭据(apikey/upstream 等) ----
const form = ref({ name: '', platform: 'anthropic', type: 'apikey', credentialsJSON: '', group_id: 0 })
const submitting = ref(false)
const formError = ref('')
const formOk = ref('')

async function submitAccount() {
  formError.value = ''; formOk.value = ''
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
    formError.value = msg(e)
  } finally {
    submitting.value = false
  }
}

async function removeAccount(a: ProviderOwnedAccount) {
  if (!window.confirm(`确认撤下账号「${a.name}」?`)) return
  try { await providerAPI.deleteAccount(a.id); await refresh() }
  catch (e: unknown) { listError.value = msg(e) }
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
      <div class="pv-tabs">
        <button :class="{ on: mode === 'oauth' }" @click="mode = 'oauth'">OAuth 授权</button>
        <button :class="{ on: mode === 'manual' }" @click="mode = 'manual'">手工凭据</button>
      </div>

      <!-- ============ OAuth 授权 ============ -->
      <div v-if="mode === 'oauth'">
        <p class="pv-note">登录授权即可:授权成功后账号直接进入调度池,原始账号密码/令牌不经手你我的浏览器,也不会生成 API Key。</p>

        <div class="pv-row"><label>平台</label>
          <div class="pv-seg">
            <button v-for="pf in PLATFORMS" :key="pf.value" type="button" :class="{ on: oauth.platform === pf.value }" @click="oauth.platform = pf.value">
              {{ pf.label }}<span class="pv-seg-hint">{{ pf.hint }}</span>
            </button>
          </div>
        </div>

        <div class="pv-row two">
          <div><label>账号名称</label><input v-model="oauth.name" class="pv-in" placeholder="给这个账号起个名" /></div>
          <div><label>授权分组</label>
            <select v-model="oauth.group_id" class="pv-in">
              <option :value="0" disabled>选择分组</option>
              <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}(#{{ g.id }})</option>
            </select>
          </div>
        </div>

        <!-- Gemini 专属 -->
        <div v-if="oauth.platform === 'gemini'" class="pv-row three">
          <div><label>OAuth 类型</label>
            <select v-model="oauth.gemini_oauth_type" class="pv-in">
              <option value="code_assist">code_assist</option>
              <option value="google_one">google_one</option>
            </select>
          </div>
          <div><label>Tier(可选)</label><input v-model="oauth.gemini_tier_id" class="pv-in" placeholder="留空自动检测" /></div>
          <div><label>Project ID(code_assist)</label><input v-model="oauth.gemini_project_id" class="pv-in" placeholder="可选" /></div>
        </div>

        <!-- Step 1 -->
        <div class="pv-step">
          <div class="pv-step-n">1</div>
          <div class="pv-step-b">
            <div class="pv-step-t">生成授权链接</div>
            <button v-if="!oauthUrl" class="pv-primary" :disabled="oauthBusy" @click="genAuthURL">{{ oauthBusy ? '生成中…' : '生成授权链接' }}</button>
            <div v-else class="pv-urlbox">
              <input :value="oauthUrl" readonly class="pv-in pv-mono" />
              <button class="pv-ghost" @click="copyURL">复制</button>
              <button class="pv-ghost" @click="openURL">打开</button>
              <button class="pv-ghost" @click="genAuthURL">重新生成</button>
            </div>
          </div>
        </div>

        <!-- Step 2 -->
        <div class="pv-step">
          <div class="pv-step-n">2</div>
          <div class="pv-step-b">
            <div class="pv-step-t">打开链接并授权</div>
            <p class="pv-dim pv-step-d">在新标签打开上面的链接,用要提供的账号完成授权。</p>
            <p v-if="oauth.platform !== 'anthropic'" class="pv-hintbox">授权后浏览器会跳到一个 <b>localhost 打不开</b>的页面——这是正常的,直接把<b>地址栏里整条链接</b>复制粘贴到下面即可(会自动提取授权码)。</p>
            <p v-else class="pv-hintbox">授权后页面会显示一段<b>授权码</b>,复制粘贴到下面。</p>
          </div>
        </div>

        <!-- Step 3 -->
        <div class="pv-step">
          <div class="pv-step-n">3</div>
          <div class="pv-step-b">
            <div class="pv-step-t">粘贴授权码 / 回调链接</div>
            <textarea v-model="oauthCode" rows="3" class="pv-in pv-mono" placeholder="粘贴授权码,或整条 localhost 回调链接"></textarea>
            <p v-if="oauthErr" class="pv-err">{{ oauthErr }}</p>
            <p v-if="oauthOk" class="pv-ok">{{ oauthOk }}</p>
            <button class="pv-primary" :disabled="oauthBusy || !oauthSessionId" @click="completeOAuth">{{ oauthBusy ? '提交中…' : '完成授权' }}</button>
          </div>
        </div>
      </div>

      <!-- ============ 手工凭据 ============ -->
      <div v-else>
        <p class="pv-note">用于 apikey / upstream 等已有凭据的账号(把中转的 base_url + api_key 交给主站当上游)。OAuth 账号请走上面的授权流。</p>
        <div class="pv-row"><label>名称</label><input v-model="form.name" class="pv-in" placeholder="给这个账号起个名" /></div>
        <div class="pv-row two">
          <div><label>平台</label><select v-model="form.platform" class="pv-in"><option>anthropic</option><option>openai</option><option>gemini</option><option>grok</option></select></div>
          <div><label>类型</label><select v-model="form.type" class="pv-in"><option>apikey</option><option>upstream</option></select></div>
        </div>
        <div class="pv-row"><label>授权分组</label>
          <select v-model="form.group_id" class="pv-in">
            <option :value="0" disabled>选择分组</option>
            <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}(#{{ g.id }})</option>
          </select>
        </div>
        <div class="pv-row"><label>凭据(JSON)</label><textarea v-model="form.credentialsJSON" rows="4" class="pv-in pv-mono" placeholder='{"api_key":"..."} 或 {"api_key":"...","base_url":"https://..."}'></textarea></div>
        <p v-if="formError" class="pv-err">{{ formError }}</p>
        <p v-if="formOk" class="pv-ok">{{ formOk }}</p>
        <button class="pv-primary" :disabled="submitting" @click="submitAccount">{{ submitting ? '提交中…' : '提交账号' }}</button>
      </div>
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
.pv-note { color: #94a3b8; font-size: 13px; margin: 0 0 16px; }
label { display: block; font-size: 12px; color: #cbd5e1; margin-bottom: 5px; }
.pv-row { margin-bottom: 12px; }
.pv-row.two { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.pv-row.three { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 12px; }
.pv-in { width: 100%; box-sizing: border-box; padding: 9px 11px; border-radius: 8px; border: 1px solid #475569; background: #0f172a; color: #f1f5f9; font-size: 13px; }
.pv-mono { font-family: ui-monospace, monospace; resize: vertical; }
.pv-primary { padding: 10px 18px; border: none; border-radius: 8px; background: #6366f1; color: #fff; font-weight: 600; cursor: pointer; }
.pv-primary:disabled { opacity: .6; cursor: not-allowed; }
.pv-ghost { padding: 8px 12px; border: 1px solid #475569; border-radius: 7px; background: transparent; color: #cbd5e1; cursor: pointer; font-size: 13px; }
.pv-danger { border: none; background: transparent; color: #f87171; cursor: pointer; font-size: 13px; }
.pv-err { color: #f87171; font-size: 13px; } .pv-ok { color: #6ee7b7; font-size: 13px; }
/* tabs */
.pv-tabs { display: flex; gap: 8px; margin-bottom: 16px; }
.pv-tabs button { padding: 7px 16px; border: 1px solid #475569; border-radius: 8px; background: transparent; color: #cbd5e1; cursor: pointer; font-size: 13px; }
.pv-tabs button.on { background: #6366f1; color: #fff; border-color: #6366f1; }
/* segmented platform picker */
.pv-seg { display: flex; gap: 8px; flex-wrap: wrap; }
.pv-seg button { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 9px 14px; border: 1px solid #475569; border-radius: 8px; background: #0f172a; color: #e2e8f0; cursor: pointer; font-size: 13px; font-weight: 600; }
.pv-seg button.on { border-color: #6366f1; background: #312e81; }
.pv-seg-hint { font-size: 10px; font-weight: 400; color: #94a3b8; }
/* steps */
.pv-step { display: flex; gap: 12px; padding: 12px 0; border-top: 1px solid #273449; }
.pv-step:first-of-type { border-top: none; }
.pv-step-n { flex: 0 0 24px; height: 24px; border-radius: 999px; background: #6366f1; color: #fff; font-size: 12px; font-weight: 700; display: flex; align-items: center; justify-content: center; }
.pv-step-b { flex: 1; min-width: 0; }
.pv-step-t { font-size: 13px; font-weight: 600; margin-bottom: 8px; }
.pv-step-d { margin: 0 0 8px; font-size: 12px; }
.pv-urlbox { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.pv-urlbox .pv-in { flex: 1; min-width: 220px; }
.pv-hintbox { font-size: 12px; color: #fcd34d; background: #422006; border: 1px solid #78350f; border-radius: 8px; padding: 8px 10px; margin: 4px 0 0; }
.pv-hintbox b { color: #fde68a; }
</style>
