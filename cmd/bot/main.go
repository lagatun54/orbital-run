package main

import (
	"encoding/json"
	"log"
	"math"
	"net"

	"orbitalrun/internal/arena"
	"orbitalrun/internal/env"
)

func main() {
	address, err := net.ResolveUDPAddr("udp", env.Get("BOT_LISTEN", ":9001"))
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", address)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Printf("bot listening on %s/udp", conn.LocalAddr())

	var buf [1500]byte
	for {
		size, peer, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			log.Fatal(err)
		}
		var frame arena.BotFrame
		if err := json.Unmarshal(buf[:size], &frame); err != nil || frame.Tick == 0 {
			continue
		}
		var direction arena.Point
		nearest := math.Inf(1)
		for _, p := range frame.Crystals {
			dx, dy := p.X-frame.Bot.X, p.Y-frame.Bot.Y
			distance := math.Hypot(dx, dy)
			if distance < nearest {
				nearest = distance
				direction = arena.Point{X: dx, Y: dy}
			}
		}
		data, err := json.Marshal(arena.BotInput{Tick: frame.Tick, Direction: direction})
		if err != nil {
			continue
		}
		if _, err := conn.WriteToUDP(data, peer); err != nil {
			log.Printf("send bot input: %v", err)
		}
	}
}
