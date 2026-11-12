package broker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type kafkaBroker struct {
	client *kgo.Client
}

func openKafka(ctx context.Context, options ...kgo.Opt) (*kafkaBroker, error) {
	brokers := strings.Split(kafkaBrokers(), ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}
	client, err := kgo.NewClient(append(options, kgo.SeedBrokers(brokers...))...)
	if err != nil {
		return nil, fmt.Errorf("configure Kafka: %w", err)
	}
	b := &kafkaBroker{client: client}
	if err := b.Ready(ctx); err != nil {
		b.Close()
		return nil, fmt.Errorf("connect to Kafka: %w", err)
	}
	return b, nil
}

func openKafkaConsumer(ctx context.Context) (*kafkaBroker, error) {
	return openKafka(ctx, kgo.ConsumerGroup(scoresGroup), kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll())
}

func (b *kafkaBroker) Publish(ctx context.Context, id string, payload []byte) error {
	record := &kgo.Record{Topic: topic, Key: []byte(id), Value: payload}
	return b.client.ProduceSync(ctx, record).FirstErr()
}

func (b *kafkaBroker) Ready(ctx context.Context) error {
	return b.client.Ping(ctx)
}

func (b *kafkaBroker) Close() {
	b.client.CloseAllowingRebalance()
}

func (b *kafkaBroker) Run(ctx context.Context, save func(context.Context, []byte) error) error {
	for ctx.Err() == nil {
		fetches := b.client.PollRecords(ctx, 1)
		fetches.EachError(func(topic string, partition int32, err error) {
			if ctx.Err() == nil {
				log.Printf("fetch Kafka topic %s partition %d: %v", topic, partition, err)
			}
		})
		var err error
		if records := fetches.Records(); len(records) != 0 {
			err = b.process(ctx, records[0], save)
		}
		b.client.AllowRebalance()
		if err != nil && ctx.Err() == nil {
			return err
		}
	}
	return nil
}

func (b *kafkaBroker) process(ctx context.Context, record *kgo.Record, save func(context.Context, []byte) error) error {
	// Hold the partition while retrying this record so later commits cannot skip it.
	write, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	for {
		err := save(write, record.Value)
		if err == nil {
			break
		}
		log.Printf("store Kafka match result: %v", err)
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-write.Done():
			timer.Stop()
			return fmt.Errorf("store Kafka record %d/%d: %w", record.Partition, record.Offset, err)
		case <-timer.C:
		}
	}
	commit, cancelCommit := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCommit()
	if err := b.client.CommitRecords(commit, record); err != nil {
		return fmt.Errorf("commit Kafka record %d/%d: %w", record.Partition, record.Offset, err)
	}
	return nil
}
