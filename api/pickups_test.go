package api

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"orbitalrun/internal/arena"
)

func TestPickupTrackerResolvesBatchesFromArenaCollisions(t *testing.T) {
	player := pickupClaim{Index: 0, Version: 1}
	bot := pickupClaim{Index: 1, Version: 1}
	missed := pickupClaim{Index: 2, Version: 1}

	var message clientMessage
	err := json.Unmarshal([]byte(`{"direction":8,"pickups":[{"index":0,"version":1},{"index":1,"version":1},{"index":2,"version":1}]}`), &message)
	if err != nil || message.Direction != inputRight || !slices.Equal(message.Pickups, []pickupClaim{player, bot, missed}) {
		t.Fatalf("decode input: %+v, %v", message, err)
	}

	batches := make(chan []pickupClaim, 1)
	batches <- message.Pickups
	tracker := pickupTracker{resolved: make(map[pickupClaim]bool), pending: make(map[pickupClaim]time.Time)}
	frame := arena.Frame{Phase: "playing", CrystalVersions: [arena.CrystalCount]uint32{1, 1, 1, 1, 1, 1, 1}}
	now := time.Now()
	if acks := tracker.update(frame, batches, now); len(acks) != 0 {
		t.Fatalf("claims resolved before collisions: %v", acks)
	}

	frame.CrystalVersions[0]++
	frame.CrystalVersions[1]++
	frame.PickedByPlayer[0] = true
	acks := tracker.update(frame, batches, now.Add(50*time.Millisecond))
	if len(acks) != 2 || !slices.Contains(acks, pickupAck{player, true}) || !slices.Contains(acks, pickupAck{bot, false}) {
		t.Fatalf("wrong collision outcomes: %v", acks)
	}

	acks = tracker.update(frame, batches, now.Add(pickupWait))
	if !slices.Equal(acks, []pickupAck{{missed, false}}) {
		t.Fatalf("unresolved claim should expire: %v", acks)
	}

	batches <- []pickupClaim{player}
	acks = tracker.update(frame, batches, now.Add(pickupWait))
	if !slices.Equal(acks, []pickupAck{{player, true}}) {
		t.Fatalf("late duplicate should get the authoritative outcome: %v", acks)
	}
	payload, err := json.Marshal(acks)
	if err != nil || string(payload) != `[{"index":0,"version":1,"accepted":true}]` {
		t.Fatalf("encode acknowledgements: %s, %v", payload, err)
	}
}
