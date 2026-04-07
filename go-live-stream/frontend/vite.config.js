import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')

  const backendUrl = env.VITE_BACKEND_URL || 'http://localhost:8080'
  const wsBackendUrl = backendUrl.replace(/^http/, 'ws')
  const devPort = parseInt(env.VITE_PORT || '5173', 10)

  return {
    plugins: [react()],
    server: {
      port: devPort,
      allowedHosts: 'all',
      proxy: {
        '/api': { target: backendUrl, changeOrigin: true },
        '/hls': { target: backendUrl, changeOrigin: true },
        '/ws':  { target: wsBackendUrl, ws: true, changeOrigin: true },
      },
    },
  }
})
