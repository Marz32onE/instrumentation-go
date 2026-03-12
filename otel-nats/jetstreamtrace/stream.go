package jetstreamtrace

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/Marz32onE/natstrace/natstrace"
)

// Stream mirrors jetstream.Stream for managing consumers with tracing.
type Stream interface {
	Info(ctx context.Context, opts ...StreamInfoOpt) (*StreamInfo, error)
	CachedInfo() *StreamInfo
	Consumer(ctx context.Context, name string) (Consumer, error)
	CreateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error)
	CreateOrUpdateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error)
	DeleteConsumer(ctx context.Context, name string) error
	ConsumerNames(ctx context.Context) ConsumerNameLister
}

type streamImpl struct {
	conn       *natstrace.Conn
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

func (s *streamImpl) CreateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error) {
	cons, err := s.s.CreateConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := cfg.Durable
	if name == "" && cfg.Name != "" {
		name = cfg.Name
	}
	if name == "" {
		name = "consumer"
	}
	return &consumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) CreateOrUpdateConsumer(ctx context.Context, cfg ConsumerConfig) (Consumer, error) {
	cons, err := s.s.CreateOrUpdateConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := cfg.Durable
	if name == "" && cfg.Name != "" {
		name = cfg.Name
	}
	if name == "" {
		name = "consumer"
	}
	return &consumerImpl{conn: s.conn, streamName: s.streamName, consumerName: name, c: cons}, nil
}

func (s *streamImpl) DeleteConsumer(ctx context.Context, name string) error {
	return s.s.DeleteConsumer(ctx, name)
}

func (s *streamImpl) ConsumerNames(ctx context.Context) ConsumerNameLister {
	return s.s.ConsumerNames(ctx)
}
