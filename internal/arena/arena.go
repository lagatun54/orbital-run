package arena

import (
	"math"
	"math/rand/v2"
	"time"
)

const (
	Width        = 960
	Height       = 540
	CrystalCount = 7
	Duration     = 30 * time.Second
	Countdown    = 2 * time.Second
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Ship struct {
	Point
	Angle float64 `json:"angle"`
	Score int     `json:"score"`
}

type Frame struct {
	Phase     string              `json:"phase"`
	Remaining int64               `json:"remainingMs"`
	Player    Ship                `json:"player"`
	Bot       Ship                `json:"bot"`
	Crystals  [CrystalCount]Point `json:"crystals"`
}

type Result struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Score      int       `json:"score"`
	BotScore   int       `json:"botScore"`
	FinishedAt time.Time `json:"finishedAt"`
	Local      bool      `json:"local,omitempty"`
}

type BotFrame struct {
	Tick     uint32              `json:"tick"`
	Bot      Point               `json:"bot"`
	Crystals [CrystalCount]Point `json:"crystals"`
}

type BotInput struct {
	Tick      uint32 `json:"tick"`
	Direction Point  `json:"direction"`
}

type Match struct {
	Frame
	start time.Time
	last  time.Time
}

func New(now time.Time) *Match {
	m := &Match{
		Frame: Frame{
			Phase: "countdown", Remaining: Countdown.Milliseconds(),
			Player: Ship{Point: Point{X: 224, Y: 270}},
			Bot:    Ship{Point: Point{X: 736, Y: 270}, Angle: math.Pi},
		},
		start: now.Add(Countdown),
		last:  now.Add(Countdown),
	}
	for i := range m.Crystals {
		m.respawn(i)
	}
	return m
}

func (m *Match) Step(now time.Time, player, bot Point) {
	if now.Before(m.start) {
		m.Remaining = m.start.Sub(now).Milliseconds()
		return
	}
	end := m.start.Add(Duration)
	step := now
	if step.After(end) {
		step = end
	}
	elapsed := min(step.Sub(m.last).Seconds(), 0.1)
	m.last = step
	move(&m.Player, player, 265*elapsed)
	move(&m.Bot, bot, 143*elapsed)

	for i, p := range m.Crystals {
		playerDistance := distance(p, m.Player.Point)
		botDistance := distance(p, m.Bot.Point)
		switch {
		case playerDistance < 24 && playerDistance <= botDistance:
			m.Player.Score++
			m.respawn(i)
		case botDistance < 24:
			m.Bot.Score++
			m.respawn(i)
		}
	}
	m.Phase = "playing"
	m.Remaining = end.Sub(step).Milliseconds()
	if !now.Before(end) {
		m.Phase = "finished"
	}
}

func move(ship *Ship, d Point, distance float64) {
	length := math.Hypot(d.X, d.Y)
	if length == 0 || math.IsNaN(length) || math.IsInf(length, 0) {
		return
	}
	ship.Angle = math.Atan2(d.Y, d.X)
	step := distance / max(1, length)
	ship.X = min(Width-23, max(23, ship.X+d.X*step))
	ship.Y = min(Height-23, max(23, ship.Y+d.Y*step))
}

func (m *Match) respawn(index int) {
	for {
		p := Point{X: float64(44 + rand.IntN(Width-88)), Y: float64(44 + rand.IntN(Height-88))}
		if distance(p, m.Player.Point) < 65 || distance(p, m.Bot.Point) < 65 {
			continue
		}
		clear := true
		for i, existing := range m.Crystals {
			if i != index && distance(p, existing) < 45 {
				clear = false
				break
			}
		}
		if clear {
			m.Crystals[index] = p
			return
		}
	}
}

func distance(a, b Point) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}
