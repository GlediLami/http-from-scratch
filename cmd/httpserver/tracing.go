package main

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func setupTracing(ctx context.Context) (shutdown func(context.Context) error, err error) {
	// Endpoint comes from OTEL_EXPORTER_OTLP_ENDPOINT, e.g. http://localhost:4318
	exp, err := otlptracehttp.New(ctx)

	if err != nil {
		return nil, err
	}

	// Service name comes from OTEL_SERVICE_NAME.
	// OTEL_RESOURCE_ATTRIBUTES=service.instance.id=app1 tells instances apart.
	res, err := resource.New(ctx, resource.WithFromEnv())

	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}
