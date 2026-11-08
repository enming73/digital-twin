// Run with the development server on 127.0.0.1:4173. This measures browser
// requestAnimationFrame cadence, not GPU frame time or a production benchmark.
import { chromium } from '@playwright/test'

const browser = await chromium.launch({ headless: true, args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] })
try {
  const page = await browser.newPage({ viewport: { width: 1366, height: 768 }, deviceScaleFactor: 1 })
  await page.goto('http://127.0.0.1:4173/', { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: 'Start simulation' }).click()
  const data = await page.evaluate(async () => {
    const canvas = document.createElement('canvas')
    const gl = canvas.getContext('webgl')
    const debug = gl?.getExtension('WEBGL_debug_renderer_info')
    const renderer = gl && debug ? gl.getParameter(debug.UNMASKED_RENDERER_WEBGL) : 'unavailable'
    const sample = await new Promise(resolve => {
      let start = 0, frames = 0
      const frame = time => { if (!start) start = time; frames++; if (time - start >= 5000) resolve({ frames, durationMs: time - start, fps: (frames - 1) * 1000 / (time - start) }); else requestAnimationFrame(frame) }
      requestAnimationFrame(frame)
    })
    return { renderer, userAgent: navigator.userAgent, viewport: [innerWidth, innerHeight], sample }
  })
  console.log(JSON.stringify(data, null, 2))
} finally { await browser.close() }
