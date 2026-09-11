import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base: './',
  plugins: [vue()],
  resolve: {
    alias: {
      '@': '/src',
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8310',
        changeOrigin: true,
        ws: true,
      },
    },
  },
  build: {
    target: ['es2020', 'chrome87', 'safari14'],
    outDir: 'dist',
    chunkSizeWarningLimit: 1024,
  },
})
