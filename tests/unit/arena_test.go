package unit

import (
	"math"
	"strings"
	"testing"
	"time"

	"orbitalrun/internal/arena"
)

func TestPilotName(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{"filters punctuation and trims spaces", "  Лётчик-7_🙂!  ", "Лётчик-7_"},
		{"uses the default for empty names", " ! 🙂 ", "Пилот"},
		{"limits Unicode characters", strings.Repeat("я", 21), strings.Repeat("я", 20)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := arena.PilotName(tc.input); got != tc.want {
				t.Errorf("PilotName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRoundUsesServerTimeAndAwardsCrystalOnce(t *testing.T) {
	now := time.Now()
	match := arena.New(now)
	match.Crystals[0] = arena.Point{X: match.Player.X + 27, Y: match.Player.Y}
	startX := match.Player.X
	match.Step(now.Add(time.Second), arena.Point{X: 1}, arena.Point{})
	if match.Player.X != startX || match.Player.Score != 0 {
		t.Fatal("player moved or scored during countdown")
	}

	match.Step(now.Add(arena.Countdown+50*time.Millisecond), arena.Point{X: 1}, arena.Point{})
	if match.Player.Score != 1 {
		t.Fatalf("collected score = %d, want 1", match.Player.Score)
	}
	if point := match.Crystals[0]; math.Hypot(point.X-match.Player.X, point.Y-match.Player.Y) < 65 {
		t.Fatalf("crystal respawned within reach: %+v", point)
	}

	match.Step(now.Add(arena.Countdown+arena.Duration), arena.Point{}, arena.Point{})
	if match.Phase != "finished" || match.Remaining != 0 || match.Player.Score != 1 {
		t.Fatalf("unexpected final frame: %+v", match.Frame)
	}
}

func TestMoveCapsDiagonalSpeed(t *testing.T) {
	now := time.Now()
	match := arena.New(now)
	start := match.Player.Point
	match.Step(now.Add(arena.Countdown+50*time.Millisecond), arena.Point{X: 1, Y: 1}, arena.Point{})
	const want = 265 * 0.05
	if got := math.Hypot(match.Player.X-start.X, match.Player.Y-start.Y); math.Abs(got-want) > 0.001 {
		t.Fatalf("diagonal distance = %f, want %f", got, want)
	}
}
