import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'
import { createVuetify } from 'vuetify'
import * as components from 'vuetify/components'
import * as directives from 'vuetify/directives'
import { aliases, mdi } from 'vuetify/iconsets/mdi'
import { zhHans } from 'vuetify/locale'

const savedTheme = typeof window !== 'undefined' ? localStorage.getItem('sra.theme') : null
const initialTheme = savedTheme === 'analysisLight' ? 'analysisLight' : 'analysisDark'

export default createVuetify({
  components,
  directives,
  icons: { defaultSet: 'mdi', aliases, sets: { mdi } },
  locale: { locale: 'zhHans', fallback: 'en', messages: { zhHans } },
  theme: {
    defaultTheme: initialTheme,
    themes: {
      analysisDark: {
        dark: true,
        colors: {
          background: '#101419',
          surface: '#171c22',
          'surface-variant': '#252d36',
          primary: '#8ab4f8',
          secondary: '#9dd5c0',
          info: '#8ab4f8',
          success: '#9dd5c0',
          warning: '#f3c778',
          error: '#f28b82',
        },
      },
      analysisLight: {
        dark: false,
        colors: {
          background: '#f1f5f9',
          surface: '#ffffff',
          'surface-variant': '#e2e8f0',
          primary: '#2563eb',
          secondary: '#0d9488',
          info: '#2563eb',
          success: '#16a34a',
          warning: '#d97706',
          error: '#dc2626',
        },
      },
    },
  },
  defaults: {
    VBtn: { variant: 'flat', rounded: 'sm' },
    VCard: { rounded: 'sm', elevation: 1 },
    VTextField: { variant: 'outlined', density: 'comfortable' },
    VSelect: { variant: 'outlined', density: 'comfortable' },
  },
})
