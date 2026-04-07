import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/auth/login':   'http://localhost:8083',
      '/auth/token':   'http://localhost:8083',
      '/auth/refresh': 'http://localhost:8083',
      '/api':          'http://localhost:8083',
    },
  },
})
