package broker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type natsBroker struct {
	conn   *nats.Conn
	js     jetstream.JetStream
	stream jetstream.Stream
}

func openNATS(ctx context.Context, url string) (*natsBroker, error) {
	conn, err := nats.Connect(url, nats.Timeout(3*time.Second), nats.MaxReconnects(-1))
	if err != nil {
		return nil, err
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: "MATCH_RESULTS", Subjects: []string{topic}, Storage: jetstream.FileStorage,
		Retention: jetstream.WorkQueuePolicy, MaxAge: 7 * 24 * time.Hour, MaxBytes: 64 << 20,
		Discard: jetstream.DiscardNew,
	})
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &natsBroker{conn: conn, js: js, stream: stream}, nil
}

func (b *natsBroker) Publish(ctx context.Context, id string, payload []byte) error {
	_, err := b.js.Publish(ctx, topic, payload, jetstream.WithMsgID(id))
	return err
}

func (b *natsBroker) Ready(context.Context) error {
	if !b.conn.IsConnected() {
		return errors.New("NATS disconnected")
	}
	return nil
}

func (b *natsBroker) Close() {
	b.conn.Close()
}

type natsConsumer struct {
	*natsBroker
	consumer jetstream.Consumer
}

func openNATSConsumer(ctx context.Context, url string) (*natsConsumer, error) {
	b, err := openNATS(ctx, url)
	if err != nil {
		return nil, err
	}
	consumer, err := b.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable: scoresGroup, AckPolicy: jetstream.AckExplicitPolicy, AckWait: 15 * time.Second,
		FilterSubject: topic, MaxAckPending: 64,
	})
	if err != nil {
		b.Close()
		return nil, err
	}
	return &natsConsumer{natsBroker: b, consumer: consumer}, nil
}

func (c *natsConsumer) Run(ctx context.Context, save func(context.Context, []byte) error) error {
	messages, err := c.consumer.Consume(func(message jetstream.Msg) {
		if err := save(ctx, message.Data()); err != nil {
			log.Printf("store match result: %v", err)
			if err := message.NakWithDelay(2 * time.Second); err != nil {
				log.Printf("reschedule match result: %v", err)
			}
			return
		}
		if err := message.Ack(); err != nil {
			log.Printf("ack match result: %v", err)
		}
	}, jetstream.PullMaxMessages(16), jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		log.Printf("consume match results: %v", err)
	}))
	if err != nil {
		return err
	}
	<-ctx.Done()
	messages.Stop()
	<-messages.Closed()
	return nil
}
