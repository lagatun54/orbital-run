package api

import (
	"time"

	"orbitalrun/internal/arena"
)

const pickupWait = 400 * time.Millisecond

type pickupClaim struct {
	Index   int    `json:"index"`
	Version uint32 `json:"version"`
}

type pickupAck struct {
	pickupClaim
	Accepted bool `json:"accepted"`
}

// pickupTracker reports the outcome of the arena's collisions. Claims never
// change the simulation or the score.
type pickupTracker struct {
	versions [arena.CrystalCount]uint32
	resolved map[pickupClaim]bool
	pending  map[pickupClaim]time.Time
}

func (p *pickupTracker) update(frame arena.Frame, batches <-chan []pickupClaim, now time.Time) []pickupAck {
	for index, version := range frame.CrystalVersions {
		if previous := p.versions[index]; previous != 0 && version > previous {
			p.resolved[pickupClaim{index, previous}] = frame.PickedByPlayer[index]
		}
		p.versions[index] = version
	}

	var acks []pickupAck
	for range len(batches) {
		for _, claim := range <-batches {
			if claim.Index < 0 || claim.Index >= arena.CrystalCount || claim.Version == 0 {
				continue
			}
			if claim.Version == p.versions[claim.Index] && frame.Phase == "playing" {
				if _, exists := p.pending[claim]; !exists {
					p.pending[claim] = now.Add(pickupWait)
				}
			} else {
				acks = append(acks, pickupAck{claim, p.resolved[claim]})
			}
		}
	}
	for claim, deadline := range p.pending {
		accepted, resolved := p.resolved[claim]
		if resolved || frame.Phase != "playing" || !now.Before(deadline) {
			acks = append(acks, pickupAck{claim, accepted})
			delete(p.pending, claim)
		}
	}
	return acks
}
