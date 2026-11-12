package arena

import (
	"math"
	"testing"
	"time"
)

func TestRoundUsesServerTimeAndAwardsCrystalOnce(t *testing.T) {
	now := time.Now()
	match := New(now)
	match.Crystals[0] = Point{X: match.Player.X + 27, Y: match.Player.Y}
	startX := match.Player.X
	match.Step(now.Add(time.Second), Point{X: 1}, Point{})
	if match.Player.X != startX || match.Player.Score != 0 {
		t.Fatal("player moved or scored during countdown")
	}

	match.Step(now.Add(Countdown+50*time.Millisecond), Point{X: 1}, Point{})
	if match.Player.Score != 1 {
		t.Fatalf("collected score = %d, want 1", match.Player.Score)
	}
	if point := match.Crystals[0]; distance(point, match.Player.Point) < 65 {
		t.Fatalf("crystal respawned within reach: %+v", point)
	}

	match.Step(now.Add(Countdown+Duration), Point{}, Point{})
	if match.Phase != "finished" || match.Remaining != 0 || match.Player.Score != 1 {
		t.Fatalf("unexpected final frame: %+v", match.Frame)
	}
}

func TestMoveCapsDiagonalSpeed(t *testing.T) {
	ship := Ship{Point: Point{X: 240, Y: 240}}
	start := ship.Point
	move(&ship, Point{X: 1, Y: 1}, 16)
	if got := distance(ship.Point, start); math.Abs(got-16) > 0.001 {
		t.Fatalf("diagonal distance = %f, want 16", got)
	}
}
