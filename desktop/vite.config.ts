import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import fs from 'fs'
import path from 'path'

function readServerPort(): number {
  const candidates = [
    path.resolve(__dirname, '../config.yaml'),
    path.resolve(__dirname, '../../config.yaml'),
  ]
  for (const file of candidates) {
    if (!fs.existsSync(file)) continue
    const m = fs.readFileSync(file, 'utf-8').match(/^server:\s*\n(?:.*\n)*?\s*port:\s*(\d+)/m)
    if (m) return parseInt(m[1], 10)
  }
  return 8081
}

const backendPort = readServerPort()
const backendTarget = `http://localhost:${backendPort}`

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  clearScreen: false,
  server: {
    host: '0.0.0.0',
    port: 1420,
    strictPort: true,
    proxy: {
      '/api': {
        target: backendTarget,
        changeOrigin: true,
      },
      '/ws': {
        target: backendTarget,
        ws: true,
        changeOrigin: true,
      },
      '/health': {
        target: backendTarget,
        changeOrigin: true,
      },
    },
  },
  envPrefix: ['VITE_', 'TAURI_'],
  build: {
    target: 'esnext',
    minify: !process.env.TAURI_DEBUG ? 'esbuild' : false,
    sourcemap: !!process.env.TAURI_DEBUG,
  },
})
