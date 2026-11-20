import { defineConfig } from 'vite'

const controlPlane = process.env.FORGEOS_API_URL || 'http://127.0.0.1:8080'

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
