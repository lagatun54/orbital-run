import { cp, mkdir, rm, writeFile } from 'node:fs/promises'

const backendURL = process.env.BACKEND_URL
let backendOrigin = process.env.STANDALONE_FRONTEND === 'true' ? null : ''
if (backendURL) {
  const url = new URL(backendURL)
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.href !== url.origin + '/') {
    throw new Error('BACKEND_URL must be an HTTP(S) origin without a path, query or credentials')
  }
  backendOrigin = url.origin
}

await rm('dist', { recursive: true, force: true })
await mkdir('dist', { recursive: true })
for (const file of ['index.html', 'style.css', 'game.js', 'scene.js']) {
  await cp(`web/${file}`, `dist/${file}`)
}
for (const file of ['pixi.min.mjs', 'pixi.min.mjs.map']) {
  await cp(`node_modules/pixi.js/dist/${file}`, `dist/${file}`)
}

await writeFile('dist/config.js', `export const backendOrigin = ${JSON.stringify(backendOrigin)}\n`)
