package main

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"
)

func initTracer(ctx context.Context) (func(context.Context) error, error) {
	exp, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithInsecure(), // dev only
		otlptracegrpc.WithEndpoint("127.0.0.1:4317"),
		otlptracegrpc.WithHeaders(map[string]string{
			"authorization": "Basic YWRtaW5AZXhhbXBsZS5jb206YWRtaW4=",
			"organization":  "default",
		}),
	)
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("example-http-server"),
		)),
	)

	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}
