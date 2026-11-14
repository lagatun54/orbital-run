package bot

import (
	"math"

	"orbitalrun/internal/arena"
)

// Direction aims the bot at the nearest crystal.
func Direction(position arena.Point, crystals [arena.CrystalCount]arena.Point) arena.Point {
	var direction arena.Point
	nearest := math.Inf(1)
	for _, p := range crystals {
		dx, dy := p.X-position.X, p.Y-position.Y
		distance := math.Hypot(dx, dy)
		if distance < nearest {
			nearest = distance
			direction = arena.Point{X: dx, Y: dy}
		}
	}
	return direction
}
