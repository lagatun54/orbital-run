// Browser-only practice rules. Ranked matches use the Go simulation.
const WIDTH = 960
const HEIGHT = 540
const COUNTDOWN = 2000
const DURATION = 30000
const distance = (a, b) => Math.hypot(a.x - b.x, a.y - b.y)

export function createLocalMatch(started) {
  const player = { x: 224, y: 270, angle: 0, score: 0 }
  const bot = { x: 736, y: 270, angle: Math.PI, score: 0 }
  const crystals = []
  let previous = started + COUNTDOWN

  function respawn(index) {
    let point
    do {
      point = { x: 44 + Math.floor(Math.random() * (WIDTH - 88)), y: 44 + Math.floor(Math.random() * (HEIGHT - 88)) }
    } while (distance(point, player) < 65 || distance(point, bot) < 65 ||
      crystals.some((other, i) => i !== index && distance(point, other) < 45))
    crystals[index] = point
  }

  for (let i = 0; i < 7; i++) respawn(i)
  return (now, direction) => {
    const begins = started + COUNTDOWN
    const ends = begins + DURATION
    if (now < begins) return { phase: 'countdown', remainingMs: begins - now, player, bot, crystals }

    const step = Math.min(now, ends)
    const elapsed = Math.min(Math.max(0, step - previous) / 1000, 0.1)
    previous = step
    move(player, Number(Boolean(direction & 8)) - Number(Boolean(direction & 4)),
      Number(Boolean(direction & 2)) - Number(Boolean(direction & 1)), 265 * elapsed)
    const target = crystals.reduce((nearest, point) => distance(bot, point) < distance(bot, nearest) ? point : nearest)
    const dx = target.x - bot.x
    const dy = target.y - bot.y
    const length = Math.hypot(dx, dy)
    if (length > 0) move(bot, dx / length, dy / length, 143 * elapsed)

    for (const [i, crystal] of crystals.entries()) {
      const playerDistance = distance(player, crystal)
      const botDistance = distance(bot, crystal)
      const collector = playerDistance < 24 && playerDistance <= botDistance ? player : botDistance < 24 ? bot : null
      if (collector) {
        collector.score++
        respawn(i)
      }
    }

    return { phase: now >= ends ? 'finished' : 'playing', remainingMs: ends - step, player, bot, crystals }
  }
}

function move(ship, dx, dy, amount) {
  const length = Math.hypot(dx, dy)
  if (length === 0) return
  ship.angle = Math.atan2(dy, dx)
  ship.x = Math.min(WIDTH - 23, Math.max(23, ship.x + dx * amount / length))
  ship.y = Math.min(HEIGHT - 23, Math.max(23, ship.y + dy * amount / length))
}
