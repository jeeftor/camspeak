import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  resolve: {
    conditions: ['browser'],
    alias: {
      $lib: '/src/lib',
    },
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
