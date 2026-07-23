import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import fs from 'fs'
import path from 'path'

function readServerPort(): number {
  const cfg = path.resolve(__dirname, '../../config.yaml')
  if (!fs.existsSync(cfg)) return 8081
  const m = fs.readFileSync(cfg, 'utf-8').match(/^server:\s*\n(?:.*\n)*?\s*port:\s*(\d+)/m)
  return m ? parseInt(m[1], 10) : 8081
}

const backend = `http://localhost:${readServerPort()}`

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': resolve(__dirname, 'src') },
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: backend, changeOrigin: true },
      '/health': { target: backend, changeOrigin: true },
    },
  },
})
