package scores

import (
	"context"

	"orbitalrun/internal/arena"
)

// ResultWriter saves results received from either the game server or the browser.
type ResultWriter interface {
	Save(ctx context.Context, result arena.Result) error
}
