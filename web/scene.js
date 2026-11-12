import { Application, Graphics, GraphicsContext } from './pixi.min.mjs'

const width = 960
const height = 540

export async function createScene(host) {
  const app = new Application()
  await app.init({
    width, height, background: 0x0b1827, antialias: true,
    preference: 'webgl', autoDensity: true, resolution: Math.min(devicePixelRatio, 2),
  })
  host.append(app.canvas)

  const field = new Graphics()
  for (const radius of [88, 156, 232, 320, 414]) {
    field.circle(width / 2, height / 2, radius).stroke({ width: 1, color: 0x36516a, alpha: .44 })
  }
  for (let i = 1; i <= 166; i++) {
    const x = (i * 241 + i * i * 13) % width
    const y = (i * 97 + i * i * 17) % height
    field.circle(x, y, i % 13 === 0 ? 1.7 : .8).fill({ color: 0xa7d9de, alpha: .16 + (i % 7) * .07 })
  }
  field.moveTo(48, height / 2).lineTo(width - 48, height / 2)
    .moveTo(width / 2, 36).lineTo(width / 2, height - 36)
    .stroke({ width: 1, color: 0x526c81, alpha: .28 })
  field.rect(22, 22, width - 44, height - 44).stroke({ width: 1, color: 0x405b73, alpha: .56 })
  app.stage.addChild(field)

  const gem = new GraphicsContext()
    .poly([0, -14, 11, 0, 0, 14, -11, 0]).fill(0xf8ba64)
    .poly([0, -14, 0, 3, -11, 0]).fill(0xffe4a0)
    .poly([0, 3, 11, 0, 0, 14]).fill(0xc57645)
  const crystals = Array.from({ length: 7 }, () => new Graphics(gem))
  const player = ship(0x89f1ce)
  const bot = ship(0xfb9b87)
  app.stage.addChild(...crystals, player, bot)

  let frame
  const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)')
  app.ticker.add(({ deltaMS }) => {
    if (!frame) return
    const blend = reducedMotion.matches ? 1 : 1 - Math.exp(-deltaMS / 35)
    placeShip(player, frame.player, blend)
    placeShip(bot, frame.bot, blend)
  })

  return function render(next, reset = false) {
    frame = next
    if (reset) {
      placeShip(player, next.player, 1)
      placeShip(bot, next.bot, 1)
    }
    crystals.forEach((crystal, i) => {
      const point = next.crystals[i]
      crystal.position.set(point.x, point.y)
    })
  }
}

function ship(color) {
  return new Graphics()
    .circle(0, 0, 26).stroke({ color, width: 1, alpha: .35 })
    .poly([22, 0, -15, -14, -7, 0, -15, 14]).fill(color)
    .poly([9, 0, -5, -4, -5, 4]).fill(0x102330)
    .moveTo(-12, 0).lineTo(-22, 0).stroke({ color, width: 3, alpha: .68 })
}

function placeShip(graphic, state, blend) {
  graphic.x += (state.x - graphic.x) * blend
  graphic.y += (state.y - graphic.y) * blend
  graphic.rotation = state.angle
}

export const standby = {
  player: { x: 224, y: 270, angle: 0 }, bot: { x: 736, y: 270, angle: Math.PI },
  crystals: [
    { x: 155, y: 120 }, { x: 318, y: 383 }, { x: 394, y: 140 },
    { x: 487, y: 275 }, { x: 585, y: 400 }, { x: 661, y: 137 }, { x: 819, y: 362 },
  ],
}
