// Package oteljetstream provides OpenTelemetry tracing for NATS JetStream.
// It mirrors the API of github.com/nats-io/nats.go/jetstream: New, JetStream, Stream, Consumer, PushConsumer.
// Consumer management methods are fully wrapped for both:
//   - jetstream.StreamConsumerManager via JetStream
//   - jetstream.ConsumerManager via Stream
//
// Usage aligns with the official package:
//   - New(conn) takes a *otelnats.Conn so that trace is propagated.
//   - Publish and PublishMsg accept context.Context (same as official).
//   - Consume(handler): handler is MsgHandler func(m Msg); m implements Msg (Data, Ack, etc.) and m.Context() carries trace. Naming aligns with otelnats.MsgHandler.
//   - PushConsumer.Consume(handler): same Msg behavior for push-based consumption.
//   - Messages(): Next() returns (ctx, msg, error) with ctx carrying extracted trace.
//   - Next(): returns (ctx, msg, error) for a single message.
//   - Fetch/FetchBytes/FetchNoWait: return MessageBatch; iterate Messages() for Msg (Msg + Context()) with trace per message.
//   - Consumer management methods (e.g. UpdateConsumer, OrderedConsumer, Pause/Resume, ListConsumers) are exposed and forwarded.
//     Trace-bearing behavior remains in message publish/consume paths.
//
// Async publish APIs (PublishAsync, PublishMsgAsync) are not provided.
package oteljetstream
