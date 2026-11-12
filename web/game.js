import { createScene, standby } from './scene.js'
import { backendOrigin } from './config.js'
import { createLocalMatch } from './local-match.js'
import { forgetLocalResult, pendingLocalResults, saveLocalResult } from './local-results.js'

const endpointFor = path => new URL(path, backendOrigin || location.origin)

const get = id => document.getElementById(id)
const form = get('start-form')
const play = get('play')
const countdown = get('countdown')
const title = get('round-title')
const status = get('status')
const controls = [...document.querySelectorAll('[data-direction]')]
const keys = new Map([
  ['KeyW', 1], ['ArrowUp', 1], ['KeyS', 2], ['ArrowDown', 2],
  ['KeyA', 4], ['ArrowLeft', 4], ['KeyD', 8], ['ArrowRight', 8],
])
const pressed = new Set()
const pointers = new Map()
let socket
let localRound = false
let preferLocal = backendOrigin === null
let phase = 'ready'
let sentDirection = -1
let generation = 0
let savedID
let render
let syncingLocal = false
let latestLocal

if (backendOrigin === null) {
  get('leader-state').textContent = 'Рейтинг появится после подключения игрового сервера.'
} else {
  void refreshLeaderboard()
  void syncLocalResults()
  window.addEventListener('online', syncLocalResults)
  window.addEventListener('focus', syncLocalResults)
  setInterval(syncLocalResults, 20000)
}
try {
  render = await createScene(get('canvas-host'))
  render(standby, true)
  play.disabled = false
  announce(preferLocal ? 'Доступна локальная игра с ботом.' : 'Арена готова.')
} catch (error) {
  console.error(error)
  announce('Не удалось загрузить арену. Попробуйте открыть игру в браузере с WebGL.')
}

form.addEventListener('submit', event => {
  event.preventDefault()
  if (!render || socket || localRound) return
  const current = ++generation
  const name = get('pilot').value
  clearControls()
  sentDirection = -1
  phase = 'connecting'
  play.disabled = true
  if (preferLocal) {
    startLocalRound(name, current)
    return
  }
  play.textContent = 'Подключение…'
  announce('Подключаемся к арене.')
  const endpoint = endpointFor('/ws')
  endpoint.protocol = endpoint.protocol === 'https:' ? 'wss:' : 'ws:'
  const match = new WebSocket(endpoint)
  socket = match
  const timeout = setTimeout(() => match.close(), 6000)

  match.addEventListener('open', () => {
    match.send(JSON.stringify({ name }))
  })
  match.addEventListener('message', event => {
    clearTimeout(timeout)
    const frame = JSON.parse(event.data)
    if (frame.phase === 'finished') {
      phase = 'finished'
      clearControls()
      showFrame(frame)
      const { result } = frame
      get('last-result').hidden = true
      savedID = undefined
      get('leaders').querySelector('.current')?.classList.remove('current')
      const outcome = result.score > result.botScore ? 'Победа!' : result.score < result.botScore ? 'Бот победил' : 'Ничья'
      showStart(outcome, 'Играть ещё')
      announce(frame.queued ? 'Результат отправлен в таблицу рекордов.' : 'Сервис рекордов недоступен. Очки показаны на табло.')
      if (frame.queued) void waitForResult(result.id, current)
      return
    }
    renderRound(frame)
  })
  match.addEventListener('close', () => {
    clearTimeout(timeout)
    socket = undefined
    clearControls()
    if (phase === 'finished') {
      play.disabled = false
      return
    }
    preferLocal = true
    if (phase === 'connecting') {
      startLocalRound(name, current)
      return
    }
    phase = 'ready'
    showStart('Связь прервалась', 'Играть локально')
    announce('Сервер отключился. Можно начать новый раунд с ботом.')
  })
})

function startLocalRound(name, current) {
  localRound = true
  phase = 'connecting'
  get('last-result').hidden = true
  const step = createLocalMatch(performance.now())
  function tick(now) {
    const frame = step(now, currentDirection())
    if (frame.phase === 'finished') {
      phase = 'finished'
      localRound = false
      clearControls()
      showFrame(frame)
      const result = frame.player.score - frame.bot.score
      showStart(result > 0 ? 'Победа!' : result < 0 ? 'Бот победил' : 'Ничья', 'Играть ещё')
      const localResult = {
        id: 'local-' + crypto.randomUUID(),
        name,
        score: frame.player.score,
        botScore: frame.bot.score,
      }
      latestLocal = { id: localResult.id, generation: current }
      const stored = saveLocalResult(localResult)
      announce(stored
        ? 'Локальный результат сохранён. Отправим его в рейтинг, когда сервер ответит.'
        : 'Результат сохранён в этой вкладке. Оставьте её открытой до отправки.')
      void syncLocalResults()
      return
    }
    renderRound(frame)
    requestAnimationFrame(tick)
  }
  requestAnimationFrame(tick)
}

async function syncLocalResults() {
  if (backendOrigin === null || syncingLocal) return
  syncingLocal = true
  let changed = false
  try {
    for (const result of pendingLocalResults()) {
      const response = await fetch(endpointFor('/api/local-results'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(result),
        signal: AbortSignal.timeout(5000),
      })
      if (!response.ok) break
      forgetLocalResult(result.id)
      changed = true
      if (latestLocal?.id === result.id && latestLocal.generation === generation) {
        void waitForResult(result.id, generation)
      }
    }
  } catch {
    // The next timer or online event retries the unsent result.
  } finally {
    syncingLocal = false
    if (changed) void refreshLeaderboard()
  }
}

function renderRound(frame) {
  const previous = phase
  phase = frame.phase
  render(frame, previous === 'connecting')
  showFrame(frame)
  form.hidden = true
  countdown.hidden = phase !== 'countdown'
  if (phase === 'countdown') countdown.textContent = String(Math.max(1, Math.ceil(frame.remainingMs / 1000)))
  if (phase !== previous) {
    const message = phase === 'countdown' ? 'Готовьтесь к старту.' : 'Раунд начался. Собирайте кристаллы!'
    announce(localRound ? 'Локальная игра с ботом. ' + message : message)
    sendDirection()
  }
}

window.addEventListener('keydown', event => {
  if ((!socket && !localRound) || !keys.has(event.code) || event.target instanceof HTMLInputElement) return
  event.preventDefault()
  pressed.add(event.code)
  sendDirection()
})
window.addEventListener('keyup', event => {
  if (pressed.delete(event.code)) sendDirection()
})
window.addEventListener('blur', clearControls)
document.addEventListener('visibilitychange', () => {
  if (document.hidden) clearControls()
})

for (const button of controls) {
  button.setAttribute('aria-pressed', 'false')
  button.addEventListener('pointerdown', event => {
    if ((!socket && !localRound) || event.button !== 0) return
    button.setPointerCapture(event.pointerId)
    pointers.set(event.pointerId, Number(button.dataset.direction))
    sendDirection()
  })
  for (const name of ['pointerup', 'pointercancel', 'lostpointercapture']) {
    button.addEventListener(name, event => {
      if (pointers.delete(event.pointerId)) sendDirection()
    })
  }
}

function clearControls() {
  pressed.clear()
  pointers.clear()
  sendDirection()
}

function currentDirection() {
  let direction = 0
  for (const key of pressed) direction |= keys.get(key)
  for (const touch of pointers.values()) direction |= touch
  return direction
}

function sendDirection() {
  const direction = currentDirection()
  for (const button of controls) {
    button.setAttribute('aria-pressed', Boolean(direction & Number(button.dataset.direction)))
  }
  if (socket?.readyState === WebSocket.OPEN && phase !== 'connecting' && phase !== 'finished' && direction !== sentDirection) {
    socket.send(JSON.stringify({ direction }))
    sentDirection = direction
  }
}

function showFrame(frame) {
  const seconds = Math.ceil(frame.remainingMs / 1000)
  get('timer').textContent = '00:' + String(frame.phase === 'countdown' ? 30 : seconds).padStart(2, '0')
  get('player-score').textContent = String(frame.player.score).padStart(2, '0')
  get('bot-score').textContent = String(frame.bot.score).padStart(2, '0')
  if (frame.phase === 'finished') render(frame)
}

function showStart(label, action) {
  title.textContent = label
  form.hidden = false
  countdown.hidden = true
  play.disabled = Boolean(socket)
  play.textContent = action
}

function announce(message) {
  if (status.textContent !== message) status.textContent = message
}

async function refreshLeaderboard() {
  const state = get('leader-state')
  try {
    const response = await fetch(endpointFor('/api/leaderboard'), { signal: AbortSignal.timeout(4000) })
    if (!response.ok) throw new Error('leaderboard request failed')
    const results = await response.json()
    const rows = results.map((result, index) => {
      const row = document.createElement('tr')
      if (result.id === savedID) row.classList.add('current')
      const name = result.local ? result.name + ' (локально)' : result.name
      for (const value of [index + 1, name, result.score]) {
        row.insertCell().textContent = value
      }
      return row
    })
    get('leaders').replaceChildren(...rows)
    state.textContent = results.length ? '' : 'Пока без рекордов. Сыграйте первый раунд.'
    state.hidden = Boolean(results.length)
  } catch {
    state.hidden = false
    state.textContent = 'Таблица рекордов временно недоступна.'
  }
}

async function waitForResult(id, current) {
  const deadline = Date.now() + 12000
  while (Date.now() < deadline && current === generation) {
    try {
      const response = await fetch(endpointFor('/api/results/' + id), { signal: AbortSignal.timeout(3000) })
      if (current !== generation) return
      if (response.ok) {
        const result = await response.json()
        if (current !== generation) return
        savedID = id
        const rank = `Место ${result.rank}`
        get('last-rank').textContent = `${rank} · ${result.score} очк.`
        get('last-result').hidden = false
        announce(`Результат сохранён. ${rank} в рейтинге.`)
        void refreshLeaderboard()
        return
      }
    } catch {
      // The score service may still be restarting.
    }
    await new Promise(resolve => setTimeout(resolve, 750))
  }
  if (current === generation) announce('Результат ещё обрабатывается. Таблица обновится при следующем открытии.')
}
