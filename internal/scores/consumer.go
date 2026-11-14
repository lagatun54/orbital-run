package scores

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"orbitalrun/internal/arena"
)

var matchID = regexp.MustCompile(`^[A-Z2-7]{26}$`)

// ValidOnlineResultID reports whether an ID can identify an online result.
func ValidOnlineResultID(id string) bool {
	return matchID.MatchString(id)
}

// NewResultConsumer validates and stores online results delivered by the broker.
func NewResultConsumer(store ResultWriter) func(context.Context, []byte) error {
	return func(ctx context.Context, payload []byte) error {
		var result arena.Result
		if err := json.Unmarshal(payload, &result); err != nil || !validOnlineResult(result) {
			log.Print("discard invalid match result")
			return nil
		}
		result.Local = false
		return store.Save(ctx, result)
	}
}

func validOnlineResult(result arena.Result) bool {
	nameLength := utf8.RuneCountInString(result.Name)
	return ValidOnlineResultID(result.ID) &&
		nameLength >= 1 && nameLength <= 20 && !strings.ContainsRune(result.Name, 0) &&
		result.Score >= 0 && result.Score <= math.MaxInt32 &&
		result.BotScore >= 0 && result.BotScore <= math.MaxInt32 &&
		!result.FinishedAt.IsZero()
}
