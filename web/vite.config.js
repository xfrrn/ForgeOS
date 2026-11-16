import { defineConfig } from 'vite'

export default defineConfig({
  base: '/ui/',
  server: {
    host: '127.0.0.1',
    proxy: { '/tasks': process.env.FORGEOS_API_URL || 'http://127.0.0.1:8080' },
  },
})
