import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { menuGroups } from '../constants/menu'
import type { AppUser, SystemConfig } from '../types/modules'

const TOKEN_KEY = 'modern-dns-token'
const USER_KEY = 'modern-dns-user'

const defaultUser: AppUser = {
  name: 'admin',
  role: '超级管理员',
  avatar: '',
}

const safeParse = <T>(value: string | null, fallback: T): T => {
  try {
    const parsed = JSON.parse(value || 'null')
    return parsed || fallback
  } catch (_error) {
    return fallback
  }
}

const applyTheme = (): void => {
  document.documentElement.classList.remove('dark')
}

const isValidJwt = (token: string): boolean =>
  typeof token === 'string' && token.trim().length > 0

export const useAppStore = defineStore('app', () => {
  const rawToken = localStorage.getItem(TOKEN_KEY) || ''
  if (rawToken && !isValidJwt(rawToken)) {
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(USER_KEY)
    localStorage.removeItem('modern-dns-refresh-token')
  }
  const savedToken = isValidJwt(rawToken) ? rawToken : ''
  const savedUser = safeParse(localStorage.getItem(USER_KEY), defaultUser)
  const theme = ref('light')
  const collapsed = ref(false)
  const activeMenu = ref('/dashboard/overview')
  const loading = ref(false)
  const token = ref(savedToken)
  const user = ref(savedUser)
  const savedLang = localStorage.getItem('modern-dns-lang')
  const systemConfig = ref<SystemConfig>({
    timezone: 'Asia/Shanghai',
    language: savedLang === 'en-US' ? 'en-US' : 'zh-CN',
    autoBackup: true,
    dnssecGlobal: true,
  })

  applyTheme()

  const menuTree = computed(() => menuGroups)
  const isAuthenticated = computed(() => Boolean(token.value))

  const setActiveMenu = (path: string): void => {
    activeMenu.value = path
  }

  const toggleCollapse = () => {
    collapsed.value = !collapsed.value
  }

  const setLoading = (value: boolean): void => {
    loading.value = value
  }

  const updateSystemConfig = (payload: Partial<SystemConfig>): void => {
    systemConfig.value = { ...systemConfig.value, ...payload }
  }

  const login = (data: { token: string; user: AppUser } | null): void => {
    if (!data?.token) return
    token.value = data.token
    user.value = data.user || defaultUser
    localStorage.setItem(TOKEN_KEY, token.value)
    localStorage.setItem(USER_KEY, JSON.stringify(user.value))
  }

  const logout = (): void => {
    token.value = ''
    user.value = defaultUser
    loading.value = false
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(USER_KEY)
  }

  return {
    theme,
    collapsed,
    activeMenu,
    loading,
    token,
    user,
    systemConfig,
    menuTree,
    isAuthenticated,
    setActiveMenu,
    toggleCollapse,
    setLoading,
    updateSystemConfig,
    login,
    logout,
  }
})