// Package jetstreamtrace provides OpenTelemetry tracing for NATS JetStream.
// It mirrors the API of github.com/nats-io/nats.go/jetstream: New, JetStream, Stream, Consumer.
//
// Usage aligns with the official package:
//   - New(conn) takes a *natstrace.Conn so that trace is propagated.
//   - Publish and PublishMsg accept context.Context (same as official).
//   - Consume(handler): handler is MsgHandler func(m MsgWithContext); m implements Msg (Data, Ack, etc.) and m.Context() carries trace. Naming aligns with natstrace.MsgHandler.
//   - Messages(): Next() returns (ctx, msg, error) with ctx carrying extracted trace.
//   - Next(): returns (ctx, msg, error) for a single message.
//   - Fetch/FetchBytes/FetchNoWait: return MessageBatch; iterate MessagesWithContext() for MsgWithContext (Msg + Context()) with trace per message.
//
// Async publish APIs (PublishAsync, PublishMsgAsync) are not provided.
package jetstreamtrace
