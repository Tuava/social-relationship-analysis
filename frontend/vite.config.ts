import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueMcp from 'vue-mcp-next'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig(({ command, mode }) => ({
  // The local inspection server is opt-in and never runs in tests or builds.
  plugins: [
    vue(),
    ...(command !== 'serve' || mode !== 'development' || process.env.SRA_VUE_MCP !== '1'
      ? []
      : [
          vueMcp({
            port: 8890,
            mcpServerInfo: {
              name: 'social-relationship-analysis-vue',
              version: '1.0.0',
            },
          }),
        ]),
  ],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8000',
      '/health': 'http://127.0.0.1:8000',
    },
  },
}))
