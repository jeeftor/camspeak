import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  resolve: {
    conditions: ['browser'],
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8585',
      '/mcp': 'http://localhost:8585',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
