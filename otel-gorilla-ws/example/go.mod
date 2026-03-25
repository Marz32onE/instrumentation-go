module github.com/Marz32onE/instrumentation-go/otel-gorilla-ws/example

go 1.25.0

replace github.com/Marz32onE/instrumentation-go/otel-gorilla-ws => ../

require (
	go.opentelemetry.io/otel v1.42.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.42.0
	go.opentelemetry.io/otel/sdk v1.42.0
)
