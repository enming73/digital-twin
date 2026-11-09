import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { isAllowedOrigin } from './src/twin/proxyOrigin'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true },
      '/ws': {
        target: 'http://127.0.0.1:8080', changeOrigin: true, ws: true,
        configure(proxy) {
          const events = proxy as unknown as { on: (name: string, handler: (upstream: { destroy(): void; setHeader(name: string, value: string): void }, request: { headers: { origin?: string } }, socket: { destroy(): void }) => void) => void }
          events.on('proxyReqWs', (upstream, request, socket) => {
            if (!isAllowedOrigin(request.headers.origin)) { upstream.destroy(); socket.destroy(); return }
            upstream.setHeader('Origin', 'http://127.0.0.1:8080')
          })
        },
      },
    },
  },
})
