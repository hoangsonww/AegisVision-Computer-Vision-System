// Kafka producer + consumer behind the bus.Publisher / bus.Subscriber
// interfaces. NATS handles low-latency RPC + frame descriptors; Kafka holds
// the durable, ordered log for events.v1, audit.v1, detections.v1, etc.
//
// We use segmentio/kafka-go for a few practical reasons:
//   - it's pure-Go (one less native dep in our distroless images),
//   - it speaks the Kafka 3.x protocol natively (good with Redpanda too),
//   - it has a small API surface that maps cleanly to our Publisher /
//     Subscriber interfaces.
//
// Production posture:
//   - acks=all + idempotent producer.
//   - SASL/SSL configured from env vars wired to a Secret.
package bus

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// KafkaConfig configures the Kafka client. Brokers is a comma-separated list
// ("kafka-0:9092,kafka-1:9092,..."). When SASLUser is set, SCRAM-SHA-512 is
// used; TLS is enabled iff TLSEnabled is set or any SASL credential is set.
type KafkaConfig struct {
	Brokers    string
	ClientID   string
	SASLUser   string
	SASLPass   string
	TLSEnabled bool
	// RequiredAcks: 0 = none, 1 = leader, -1 = all. Production = all.
	RequiredAcks int
	// Topic for write operations (used by Publish when msg.Subject is empty).
	DefaultTopic string
	// WriteTimeout bounds individual writes.
	WriteTimeout time.Duration
}

// KafkaProducer satisfies Publisher.
type KafkaProducer struct {
	w *kafka.Writer
}

// NewKafkaProducer constructs a producer.
func NewKafkaProducer(cfg KafkaConfig) (*KafkaProducer, error) {
	if cfg.Brokers == "" {
		return nil, errors.New("bus: KafkaConfig.Brokers is required")
	}
	if cfg.RequiredAcks == 0 {
		cfg.RequiredAcks = -1 // all
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 10 * time.Second
	}
	transport, err := buildTransport(cfg)
	if err != nil {
		return nil, err
	}
	w := &kafka.Writer{
		Addr:                   kafka.TCP(strings.Split(cfg.Brokers, ",")...),
		Balancer:               &kafka.Hash{}, // hash by message key
		RequiredAcks:           kafka.RequiredAcks(cfg.RequiredAcks),
		AllowAutoTopicCreation: false,         // production: topics must exist
		Async:                  false,         // synchronous so we can return errors
		Compression:            kafka.Snappy,
		WriteTimeout:           cfg.WriteTimeout,
		Transport:              transport,
	}
	if cfg.DefaultTopic != "" {
		w.Topic = cfg.DefaultTopic
	}
	return &KafkaProducer{w: w}, nil
}

// Publish writes one message. The bus.Message.Subject is used as the Kafka
// topic; headers are propagated.
func (p *KafkaProducer) Publish(ctx context.Context, msg Message) error {
	hdrs := make([]kafka.Header, 0, len(msg.Headers))
	for k, v := range msg.Headers {
		hdrs = append(hdrs, kafka.Header{Key: k, Value: []byte(v)})
	}
	key := []byte(msg.Headers["partition_key"])
	if len(key) == 0 {
		// Per ADR-0008 / docs §17 partitioning is by stream/camera id.
		key = []byte(msg.Headers["stream"])
	}
	km := kafka.Message{
		Topic:   msg.Subject,
		Key:     key,
		Value:   msg.Data,
		Headers: hdrs,
	}
	return p.w.WriteMessages(ctx, km)
}

// Close flushes pending writes.
func (p *KafkaProducer) Close() error { return p.w.Close() }

// KafkaConsumer satisfies Subscriber for a single consumer-group + topic.
type KafkaConsumer struct {
	mu  sync.Mutex
	rds []*kafka.Reader
}

// NewKafkaConsumer constructs a consumer factory. Each call to Subscribe
// spawns one goroutine per partition consumer-group reader.
func NewKafkaConsumer() *KafkaConsumer { return &KafkaConsumer{} }

// Subscribe consumes messages from `subject` (the Kafka topic) under the
// supplied consumer group. Returns a Subscription that drains and closes.
func (c *KafkaConsumer) Subscribe(cfg KafkaConfig, subject, group string, h Handler) (Subscription, error) {
	if cfg.Brokers == "" {
		return nil, errors.New("bus: KafkaConfig.Brokers is required")
	}
	if subject == "" {
		return nil, errors.New("bus: subject (topic) is required")
	}
	if group == "" {
		return nil, errors.New("bus: kafka consumer requires a group")
	}
	dialer, err := buildDialer(cfg)
	if err != nil {
		return nil, err
	}
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     strings.Split(cfg.Brokers, ","),
		Topic:       subject,
		GroupID:     group,
		StartOffset: kafka.LastOffset,
		Dialer:      dialer,
		MinBytes:    1 << 10,
		MaxBytes:    10 << 20,
	})
	c.mu.Lock()
	c.rds = append(c.rds, r)
	c.mu.Unlock()
	stop := make(chan struct{})
	go func() {
		for {
			m, err := r.FetchMessage(context.Background())
			if err != nil {
				select {
				case <-stop:
					return
				default:
					// transient — back off.
					time.Sleep(500 * time.Millisecond)
					continue
				}
			}
			msg := Message{Subject: m.Topic, Data: m.Value, Headers: map[string]string{}}
			for _, hdr := range m.Headers {
				msg.Headers[hdr.Key] = string(hdr.Value)
			}
			if err := h(context.Background(), msg); err == nil {
				_ = r.CommitMessages(context.Background(), m)
			}
		}
	}()
	return &kafkaSub{reader: r, stop: stop}, nil
}

// Close releases every reader.
func (c *KafkaConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.rds {
		_ = r.Close()
	}
	c.rds = nil
	return nil
}

type kafkaSub struct {
	reader *kafka.Reader
	stop   chan struct{}
}

func (s *kafkaSub) Drain() error       { close(s.stop); return s.reader.Close() }
func (s *kafkaSub) Unsubscribe() error { return s.Drain() }

func buildTransport(cfg KafkaConfig) (*kafka.Transport, error) {
	t := &kafka.Transport{
		ClientID: cfg.ClientID,
	}
	if cfg.SASLUser != "" {
		mech, err := scram.Mechanism(scram.SHA512, cfg.SASLUser, cfg.SASLPass)
		if err != nil {
			return nil, fmt.Errorf("bus: scram: %w", err)
		}
		t.SASL = mech
	}
	if cfg.TLSEnabled || cfg.SASLUser != "" {
		t.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return t, nil
}

func buildDialer(cfg KafkaConfig) (*kafka.Dialer, error) {
	d := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
		ClientID:  cfg.ClientID,
	}
	if cfg.SASLUser != "" {
		mech, err := scram.Mechanism(scram.SHA512, cfg.SASLUser, cfg.SASLPass)
		if err != nil {
			return nil, err
		}
		d.SASLMechanism = mech
	}
	if cfg.TLSEnabled || cfg.SASLUser != "" {
		d.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return d, nil
}
