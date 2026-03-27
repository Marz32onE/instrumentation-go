package oteljetstream

import (
	"context"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/Marz32onE/instrumentation-go/otel-nats/otelnats"
)

// Stream mirrors jetstream.Stream for managing consumers with tracing.
type Stream interface {
	Info(ctx context.Context, opts ...StreamInfoOpt) (*StreamInfo, error)
	CachedInfo() *StreamInfo
	Consumer(ctx context.Context, name string) (Consumer, error)
	PushConsumer(ctx context.Context, name string) (PushConsumer, error)
	CreateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error)
	CreateOrUpdateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error)
	UpdateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error)
	OrderedConsumer(ctx context.Context, cfg OrderedConsumerConfig) (Consumer, error)
	PauseConsumer(ctx context.Context, consumer string, pauseUntil time.Time) (*ConsumerPauseResponse, error)
	ResumeConsumer(ctx context.Context, consumer string) (*ConsumerPauseResponse, error)
	ListConsumers(ctx context.Context) ConsumerInfoLister
	CreatePushConsumer(ctx context.Context, cfg ConsumerConfig) (PushConsumer, error)
	CreateOrUpdatePushConsumer(ctx context.Context, cfg ConsumerConfig) (PushConsumer, error)
	UpdatePushConsumer(ctx context.Context, cfg ConsumerConfig) (PushConsumer, error)
	DeleteConsumer(ctx context.Context, name string) error
	ConsumerNames(ctx context.Context) ConsumerNameLister
	UnpinConsumer(ctx context.Context, consumer string, group string) error
}

type streamImpl struct {
	conn       *otelnats.Conn
	streamName string
	s          jetstream.Stream
}

func (s *streamImpl) Info(ctx context.Context, opts ...StreamInfoOpt) (*StreamInfo, error) {
	return s.s.Info(ctx, opts...)
}

func (s *streamImpl) CachedInfo() *StreamInfo {
	return s.s.CachedInfo()
}

func (s *streamImpl) Consumer(ctx context.Context, name string) (Consumer, error) {
	cons, err := s.s.Consumer(ctx, name)
	if err != nil {
		return nil, err
	}
	return &consumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) PushConsumer(ctx context.Context, name string) (PushConsumer, error) {
	cons, err := s.s.PushConsumer(ctx, name)
	if err != nil {
		return nil, err
	}
	return &pushConsumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) CreateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error) {
	cons, err := s.s.CreateConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := consumerNameFromConfig(cfg)
	return &consumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) CreateOrUpdateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error) {
	cons, err := s.s.CreateOrUpdateConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := consumerNameFromConfig(cfg)
	return &consumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) UpdateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error) {
	cons, err := s.s.UpdateConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := consumerNameFromConfig(cfg)
	return &consumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) OrderedConsumer(ctx context.Context, cfg OrderedConsumerConfig) (Consumer, error) {
	cons, err := s.s.OrderedConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := cfg.NamePrefix
	if name == "" {
		name = "ordered-consumer"
	}
	return &consumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) PauseConsumer(ctx context.Context, consumer string, pauseUntil time.Time) (*ConsumerPauseResponse, error) {
	return s.s.PauseConsumer(ctx, consumer, pauseUntil)
}

func (s *streamImpl) ResumeConsumer(ctx context.Context, consumer string) (*ConsumerPauseResponse, error) {
	return s.s.ResumeConsumer(ctx, consumer)
}

func (s *streamImpl) ListConsumers(ctx context.Context) ConsumerInfoLister {
	return s.s.ListConsumers(ctx)
}

func (s *streamImpl) CreatePushConsumer(ctx context.Context, cfg ConsumerConfig) (PushConsumer, error) {
	cons, err := s.s.CreatePushConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := consumerNameFromConfig(cfg)
	return &pushConsumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) CreateOrUpdatePushConsumer(ctx context.Context, cfg ConsumerConfig) (PushConsumer, error) {
	cons, err := s.s.CreateOrUpdatePushConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := consumerNameFromConfig(cfg)
	return &pushConsumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) UpdatePushConsumer(ctx context.Context, cfg ConsumerConfig) (PushConsumer, error) {
	cons, err := s.s.UpdatePushConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := consumerNameFromConfig(cfg)
	return &pushConsumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) DeleteConsumer(ctx context.Context, name string) error {
	return s.s.DeleteConsumer(ctx, name)
}

func (s *streamImpl) ConsumerNames(ctx context.Context) ConsumerNameLister {
	return s.s.ConsumerNames(ctx)
}

func (s *streamImpl) UnpinConsumer(ctx context.Context, consumer string, group string) error {
	return s.s.UnpinConsumer(ctx, consumer, group)
}

func consumerNameFromConfig(cfg ConsumerConfig) string {
	name := cfg.Durable
	if name == "" && cfg.Name != "" {
		name = cfg.Name
	}
	if name == "" {
		name = "consumer"
	}
	return name
}
