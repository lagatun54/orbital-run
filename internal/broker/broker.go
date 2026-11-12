package broker

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const Subject = "matches.finished"

func Open(ctx context.Context, url string) (*nats.Conn, jetstream.JetStream, jetstream.Stream, error) {
	conn, err := nats.Connect(url, nats.Timeout(3*time.Second), nats.MaxReconnects(-1))
	if err != nil {
		return nil, nil, nil, err
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, nil, nil, err
	}
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: "MATCH_RESULTS", Subjects: []string{Subject}, Storage: jetstream.FileStorage,
		Retention: jetstream.WorkQueuePolicy, MaxAge: 7 * 24 * time.Hour, MaxBytes: 64 << 20,
		Discard: jetstream.DiscardNew,
	})
	if err != nil {
		conn.Close()
		return nil, nil, nil, err
	}
	return conn, js, stream, nil
}
