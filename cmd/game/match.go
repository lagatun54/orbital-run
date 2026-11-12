package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"orbitalrun/internal/arena"
)

type matchServer struct {
	frontendOrigin string
	bot            *net.UDPAddr
	slots          chan struct{}
	publish        func(context.Context, arena.Result) error
}

type clientMessage struct {
	Name      string `json:"name"`
	Direction uint32 `json:"direction"`
}

type roundEnd struct {
	arena.Frame
	Result arena.Result `json:"result"`
	Queued bool         `json:"queued"`
}

func (s matchServer) play(w http.ResponseWriter, r *http.Request) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		http.Error(w, "all arenas are busy", http.StatusServiceUnavailable)
		return
	}
	var opts *websocket.AcceptOptions
	if s.frontendOrigin != "" && r.Header.Get("Origin") == s.frontendOrigin {
		opts = &websocket.AcceptOptions{OriginPatterns: []string{s.frontendOrigin}}
	}
	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(512)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var first clientMessage
	hello, cancelHello := context.WithTimeout(ctx, 4*time.Second)
	err = wsjson.Read(hello, conn, &first)
	cancelHello()
	if err != nil {
		return
	}
	name := pilotName(first.Name)

	bot, err := net.DialUDP("udp", nil, s.bot)
	if err != nil {
		log.Printf("dial bot: %v", err)
		_ = conn.Close(websocket.StatusInternalError, "bot unavailable")
		return
	}
	defer bot.Close()

	var playerInput atomic.Uint32
	go func() {
		defer cancel()
		for {
			var input clientMessage
			if err := wsjson.Read(ctx, conn, &input); err != nil {
				return
			}
			playerInput.Store(input.Direction & 15)
		}
	}()

	var botInput atomic.Pointer[arena.BotInput]
	go readBot(bot, &botInput)

	match := arena.New(time.Now())
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var tick uint32
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			tick++
			var direction arena.Point
			if input := botInput.Load(); input != nil && input.Tick <= tick && tick-input.Tick <= 10 {
				direction = input.Direction
			}
			match.Step(now, keys(playerInput.Load()), direction)
			if match.Phase == "finished" {
				result := arena.Result{
					ID: rand.Text(), Name: name, Score: match.Player.Score,
					BotScore: match.Bot.Score, FinishedAt: now.UTC(),
				}
				publish, cancelPublish := context.WithTimeout(context.Background(), 2*time.Second)
				err := s.publish(publish, result)
				cancelPublish()
				if err != nil {
					log.Printf("publish match %s: %v", result.ID, err)
				}
				_ = writeFrame(ctx, conn, roundEnd{Frame: match.Frame, Result: result, Queued: err == nil})
				_ = conn.Close(websocket.StatusNormalClosure, "round complete")
				return
			}
			if tick%3 == 1 {
				sendBot(bot, arena.BotFrame{Tick: tick, Bot: match.Bot.Point, Crystals: match.Crystals})
			}
			if err := writeFrame(ctx, conn, match.Frame); err != nil {
				return
			}
		}
	}
}

func writeFrame(ctx context.Context, conn *websocket.Conn, frame any) error {
	write, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	return wsjson.Write(write, conn, frame)
}

func readBot(bot *net.UDPConn, latest *atomic.Pointer[arena.BotInput]) {
	var buffer [512]byte
	var lastTick uint32
	for {
		size, err := bot.Read(buffer[:])
		if err != nil {
			// A connected UDP socket can receive a transient ICMP port-unreachable.
			if errors.Is(err, syscall.ECONNREFUSED) {
				continue
			}
			return
		}
		var input arena.BotInput
		if err := json.Unmarshal(buffer[:size], &input); err == nil && input.Tick > lastTick {
			lastTick = input.Tick
			latest.Store(&input)
		}
	}
}

func sendBot(bot *net.UDPConn, frame arena.BotFrame) {
	payload, err := json.Marshal(frame)
	if err == nil {
		_, _ = bot.Write(payload)
	}
}

func keys(mask uint32) arena.Point {
	return arena.Point{
		X: float64(int((mask>>3)&1) - int((mask>>2)&1)),
		Y: float64(int((mask>>1)&1) - int(mask&1)),
	}
}

func pilotName(raw string) string {
	var name strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '-' || r == '_' {
			name.WriteRune(r)
			if utf8.RuneCountInString(name.String()) == 20 {
				break
			}
		}
	}
	if clean := strings.TrimSpace(name.String()); clean != "" {
		return clean
	}
	return "Пилот"
}
