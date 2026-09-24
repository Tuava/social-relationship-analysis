import { defineStore } from 'pinia'
import { api } from '@/services/api'

export const useSessionStore = defineStore('session', {
  state: () => ({
    token: localStorage.getItem('sra_token') || '',
    username: '',
    loading: false,
  }),
  getters: { authenticated: (state) => Boolean(state.token) },
  actions: {
    async login(username: string, password: string) {
      this.loading = true
      try {
        const { data } = await api.post('/api/v1/auth/login', { username, password })
        this.token = data.token
        this.username = data.user.username
        localStorage.setItem('sra_token', this.token)
      } finally { this.loading = false }
    },
    logout() { this.token = ''; this.username = ''; localStorage.removeItem('sra_token') },
    async hydrate() {
      if (!this.token) return
      try { const { data } = await api.get('/api/v1/auth/me'); this.username = data.username }
      catch { this.logout() }
    },
  },
})
