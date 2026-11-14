package broker

import (
	"context"
	"os"

	"orbitalrun/config"
)

const topic = "matches.finished"
const scoresGroup = "scores"

func OpenProducer(ctx context.Context) (Producer, error) {
	if url := legacyNATSURL(); url != "" {
		return openNATS(ctx, url)
	}
	return openKafka(ctx)
}

func OpenConsumer(ctx context.Context) (Consumer, error) {
	if url := legacyNATSURL(); url != "" {
		return openNATSConsumer(ctx, url)
	}
	return openKafkaConsumer(ctx)
}

func legacyNATSURL() string {
	if os.Getenv("KAFKA_BROKERS") != "" {
		return ""
	}
	return os.Getenv("NATS_URL")
}

func kafkaBrokers() string {
	return config.Env("KAFKA_BROKERS", "127.0.0.1:9092")
}
