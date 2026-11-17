package db

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"orbitalrun/internal/arena"
)

const (
	leaderboardKey = "orbital:scores:leaderboard:v2"
	leaderboardTTL = 30 * time.Second
	fillTTL        = 3 * time.Second
	cacheTimeout   = 100 * time.Millisecond
)

// Only the reader that reserved the key may fill it. A save can remove that
// reservation while the reader is still querying PostgreSQL.
const fillLeaderboard = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("SET", KEYS[1], ARGV[2], "PX", ARGV[3])
end
return 0
`

// Apply the committed result in place. If a PostgreSQL read is filling the key,
// cancel its reservation so it cannot publish a snapshot taken before the insert.
const updateLeaderboard = `
local cached = redis.call("GET", KEYS[1])
if not cached then
    return 0
end
if string.sub(cached, 1, 1) ~= "[" then
    return redis.call("DEL", KEYS[1])
end
local ok, entries = pcall(cjson.decode, cached)
if not ok or ARGV[1] == "" then
    return redis.call("DEL", KEYS[1])
end

local result = cjson.decode(ARGV[1])
local position = #entries + 1
for i, entry in ipairs(entries) do
    if result.id == entry.id then
        return 0
    end
    if result.score > entry.score then
        position = i
        break
    end
    if result.score == entry.score then
        -- Both timestamps are UTC. Drop Z so a whole second sorts before its fractions.
        local resultTime = string.sub(result.finishedAt, 1, -2)
        local entryTime = string.sub(entry.finishedAt, 1, -2)
        if resultTime == entryTime then
            -- PostgreSQL decides the order of IDs using its database collation.
            return redis.call("DEL", KEYS[1])
        end
        if resultTime < entryTime then
            position = i
            break
        end
    end
end
if position > 10 then
    return 0
end
table.insert(entries, position, result)
if #entries > 10 then
    table.remove(entries, 11)
end
return redis.call("SET", KEYS[1], cjson.encode(entries), "KEEPTTL")
`

// RedisLeaderboardCache stores the leaderboard in a single Redis key.
type RedisLeaderboardCache struct {
	client *redis.Client
}

// NewRedisLeaderboardCache configures the cache without connecting at startup.
func NewRedisLeaderboardCache(url string) (*RedisLeaderboardCache, error) {
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, errors.New("invalid REDIS_URL")
	}
	options.ContextTimeoutEnabled = true
	options.DialTimeout = cacheTimeout
	options.DialerRetries = 1
	options.ReadTimeout = cacheTimeout
	options.WriteTimeout = cacheTimeout
	options.MaxRetries = -1
	return &RedisLeaderboardCache{client: redis.NewClient(options)}, nil
}

func (c *RedisLeaderboardCache) Close() error {
	return c.client.Close()
}

func (c *RedisLeaderboardCache) Update(ctx context.Context, result arena.Result) error {
	result.FinishedAt = result.FinishedAt.UTC()
	// A timestamp outside JSON's range yields an empty payload and evicts the cache.
	value, _ := json.Marshal(result)

	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	return c.client.Eval(ctx, updateLeaderboard, []string{leaderboardKey}, string(value)).Err()
}

func (c *RedisLeaderboardCache) Lookup(ctx context.Context) ([]arena.Result, string, error) {
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	value, err := c.client.Get(ctx, leaderboardKey).Bytes()
	if errors.Is(err, redis.Nil) {
		token := ":" + rand.Text()
		ok, err := c.client.SetNX(ctx, leaderboardKey, token, fillTTL).Result()
		if err != nil || !ok {
			return nil, "", err
		}
		return nil, token, nil
	}
	if err != nil {
		return nil, "", err
	}
	if len(value) == 0 || value[0] != '[' {
		return nil, "", nil
	}
	var entries []arena.Result
	if err := json.Unmarshal(value, &entries); err != nil {
		return nil, "", err
	}
	return entries, "", nil
}

func (c *RedisLeaderboardCache) Fill(ctx context.Context, token string, entries []arena.Result) error {
	normalized := make([]arena.Result, len(entries))
	copy(normalized, entries)
	for i := range normalized {
		normalized[i].FinishedAt = normalized[i].FinishedAt.UTC()
	}
	value, err := json.Marshal(normalized)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	return c.client.Eval(ctx, fillLeaderboard, []string{leaderboardKey}, token, value, leaderboardTTL.Milliseconds()).Err()
}
