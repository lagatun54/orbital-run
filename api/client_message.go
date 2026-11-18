package api

import "orbitalrun/internal/arena"

type clientMessage struct {
	Name      string        `json:"name"`
	Direction uint32        `json:"direction"`
	Pickups   []pickupClaim `json:"pickups"`
}

// The browser sets one bit per pressed direction. The values are part of the WebSocket protocol.
const (
	inputUp    uint32 = 1
	inputDown  uint32 = 2
	inputLeft  uint32 = 4
	inputRight uint32 = 8
)

func decodeDirection(mask uint32) arena.Point {
	var direction arena.Point
	if mask&inputLeft != 0 {
		direction.X--
	}
	if mask&inputRight != 0 {
		direction.X++
	}
	if mask&inputUp != 0 {
		direction.Y--
	}
	if mask&inputDown != 0 {
		direction.Y++
	}
	return direction
}
