package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"orbitalrun/internal/game"
)

type sample struct {
	gaps   []time.Duration
	status int
	err    error
}

func main() {
	url := flag.String("url", "http://127.0.0.1:8080", "base URL of the game server")
	origin := flag.String("origin", "", "browser Origin, if required by the server")
	matches := flag.Int("matches", 64, "number of concurrent WebSocket matches")
	maxP99 := flag.Duration("max-p99", 75*time.Millisecond, "maximum p99 interval between playing frames")
	flag.Parse()
	if *matches < 1 || *maxP99 <= 0 {
		log.Fatal("-matches and -max-p99 must be positive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(*url, "/") + "/ws"
	options := &websocket.DialOptions{HTTPHeader: http.Header{}}
	if *origin != "" {
		options.HTTPHeader.Set("Origin", *origin)
	}
	reports := make(chan sample, *matches)
	start := make(chan struct{})
	for range *matches {
		go func() {
			<-start
			reports <- play(ctx, endpoint, options)
		}()
	}
	close(start)

	var gaps []time.Duration
	var finished, busy, failed int
	var firstError error
	for range *matches {
		report := <-reports
		gaps = append(gaps, report.gaps...)
		switch {
		case report.err == nil:
			finished++
		case report.status == http.StatusServiceUnavailable:
			busy++
		default:
			failed++
			if firstError == nil {
				firstError = report.err
			}
		}
	}
	fmt.Printf("Matches: %d attempted, %d finished and queued, %d rejected (503), %d failed\n", *matches, finished, busy, failed)
	slices.Sort(gaps)
	if len(gaps) == 0 {
		log.Fatalf("no playing frame intervals collected (first error: %v)", firstError)
	}
	p99 := gaps[len(gaps)-len(gaps)/100-1]
	fmt.Printf("Playing frame intervals: p50 %s, p99 %s, max %s (%d samples)\n",
		gaps[(len(gaps)-1)/2].Round(time.Millisecond), p99.Round(time.Millisecond), gaps[len(gaps)-1].Round(time.Millisecond), len(gaps))
	if failed > 0 {
		log.Fatalf("first match error: %v", firstError)
	}
	if busy > 0 || p99 > *maxP99 {
		log.Fatalf("load target missed (required zero rejections and p99 <= %s)", *maxP99)
	}
}

func play(ctx context.Context, url string, options *websocket.DialOptions) (report sample) {
	conn, response, err := websocket.Dial(ctx, url, options)
	if err != nil {
		if response != nil {
			report.status = response.StatusCode
		}
		report.err = fmt.Errorf("upgrade WebSocket: %w", err)
		return report
	}
	defer conn.CloseNow()
	if err := wsjson.Write(ctx, conn, map[string]string{"name": "Load Pilot"}); err != nil {
		report.err = fmt.Errorf("send match start: %w", err)
		return report
	}

	var previous time.Time
	var lastInput time.Time
	direction := uint32(8)
	for {
		var frame game.RoundEnd
		if err := wsjson.Read(ctx, conn, &frame); err != nil {
			report.err = fmt.Errorf("read frame: %w", err)
			return report
		}
		switch frame.Phase {
		case "playing":
			now := time.Now()
			if !previous.IsZero() {
				report.gaps = append(report.gaps, now.Sub(previous))
			}
			previous = now
			if now.Sub(lastInput) >= time.Second {
				if err := wsjson.Write(ctx, conn, map[string]uint32{"direction": direction}); err != nil {
					report.err = fmt.Errorf("send move: %w", err)
					return report
				}
				direction ^= 12 // Alternate between right (8) and left (4).
				lastInput = now
			}
		case "finished":
			if frame.Result.ID == "" || !frame.Queued {
				report.err = fmt.Errorf("server did not queue match result %q", frame.Result.ID)
			}
			return report
		}
	}
}
