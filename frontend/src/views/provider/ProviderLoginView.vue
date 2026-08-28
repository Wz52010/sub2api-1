<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useProviderStore } from '@/stores/provider'

const router = useRouter()
const providerStore = useProviderStore()

const email = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  error.value = ''
  if (!email.value || !password.value) {
    error.value = '请输入邮箱和密码'
    return
  }
  loading.value = true
  try {
    await providerStore.login(email.value.trim(), password.value)
    router.replace('/provider')
  } catch (e: unknown) {
    const detail = (e as { response?: { data?: { message?: string } } })?.response?.data?.message
    error.value = detail || '登录失败,请检查邮箱/密码'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="pv-login">
    <div class="pv-card">
      <h1 class="pv-title">供货商门户</h1>
      <p class="pv-sub">登录后可授权账号并查看账号实况</p>
      <form @submit.prevent="submit">
        <label class="pv-label">邮箱</label>
        <input v-model="email" type="email" class="pv-input" autocomplete="username" placeholder="you@example.com" />
        <label class="pv-label">密码</label>
        <input v-model="password" type="password" class="pv-input" autocomplete="current-password" placeholder="••••••••" />
        <p v-if="error" class="pv-error">{{ error }}</p>
        <button type="submit" class="pv-btn" :disabled="loading">
          {{ loading ? '登录中…' : '登录' }}
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.pv-login {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #0f172a;
  padding: 24px;
}
.pv-card {
  width: 100%;
  max-width: 380px;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 14px;
  padding: 32px 28px;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.35);
}
.pv-title {
  margin: 0 0 4px;
  font-size: 22px;
  font-weight: 700;
  color: #f1f5f9;
}
.pv-sub {
  margin: 0 0 24px;
  font-size: 13px;
  color: #94a3b8;
}
.pv-label {
  display: block;
  font-size: 13px;
  color: #cbd5e1;
  margin: 14px 0 6px;
}
.pv-input {
  width: 100%;
  box-sizing: border-box;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid #475569;
  background: #0f172a;
  color: #f1f5f9;
  font-size: 14px;
  outline: none;
}
.pv-input:focus {
  border-color: #6366f1;
}
.pv-error {
  color: #f87171;
  font-size: 13px;
  margin: 12px 0 0;
}
.pv-btn {
  width: 100%;
  margin-top: 22px;
  padding: 11px;
  border: none;
  border-radius: 8px;
  background: #6366f1;
  color: #fff;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
}
.pv-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
