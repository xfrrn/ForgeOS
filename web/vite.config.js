import { defineConfig } from 'vite'

// The control plane compares the browser's Origin with the forwarded Host.
const controlPlane = {
  target: process.env.FORGEOS_API_URL || 'http://127.0.0.1:8080',
  changeOrigin: false,
}

export default defineConfig({
  base: '/ui/',
  server: {
    host: '127.0.0.1',
    proxy: {
      '/auth': controlPlane,
      '/tasks': controlPlane,
      '/projects': controlPlane,
      '/workers': controlPlane,
      '/executions': controlPlane,
      '/global-rules': controlPlane,
      '/native-approvals': controlPlane,
      '/role-processes': controlPlane,
    },
  },
})
