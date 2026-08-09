/**
 * 将 tools/models 下的 ONNX 映射到 Vite dev / production dist。
 */
import fs from 'fs'
import path from 'path'
import type { Plugin } from 'vite'

const MODEL_SUBDIRS = ['speaker', 'audio', 'vad', 'face'] as const

function resolveModelsRoot(configDir: string): string {
  return path.resolve(configDir, '../tools/models')
}

function resolveModelFile(modelsRoot: string, urlPath: string): string | null {
  const clean = urlPath.replace(/^\/+/, '').replace(/\.\./g, '')
  const parts = clean.split('/').filter(Boolean)
  if (parts.length < 2) return null
  const [subdir, ...rest] = parts
  if (!MODEL_SUBDIRS.includes(subdir as (typeof MODEL_SUBDIRS)[number])) return null
  const filePath = path.join(modelsRoot, subdir, ...rest)
  const normalizedRoot = path.join(modelsRoot, subdir)
  if (!filePath.startsWith(normalizedRoot)) return null
  if (!fs.existsSync(filePath) || fs.statSync(filePath).isDirectory()) return null
  return filePath
}

function copyModelTree(modelsRoot: string, outRoot: string) {
  for (const sub of MODEL_SUBDIRS) {
    const src = path.join(modelsRoot, sub)
    if (!fs.existsSync(src)) continue
    const dest = path.join(outRoot, sub)
    fs.mkdirSync(dest, { recursive: true })
    for (const name of fs.readdirSync(src)) {
      if (!name.endsWith('.onnx')) continue
      fs.copyFileSync(path.join(src, name), path.join(dest, name))
    }
  }
}

export function mochiToolsModelsPlugin(): Plugin {
  let modelsRoot = ''
  return {
    name: 'mochi-tools-models',
    configResolved(config) {
      modelsRoot = resolveModelsRoot(config.root)
    },
    configureServer(server) {
      server.middlewares.use('/models', (req, res, next) => {
        const urlPath = decodeURIComponent((req.url ?? '/').split('?')[0])
        const filePath = resolveModelFile(modelsRoot, urlPath)
        if (!filePath) return next()
        res.setHeader('Content-Type', 'application/octet-stream')
        res.setHeader('Cache-Control', 'no-cache')
        fs.createReadStream(filePath).pipe(res)
      })
    },
    writeBundle(options) {
      const outDir = path.join(options.dir, 'models')
      copyModelTree(modelsRoot, outDir)
    },
  }
}
