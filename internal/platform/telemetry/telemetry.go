package telemetry

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "go-order-management-system/internal/platform/telemetry"

type ShutdownFunc func(context.Context) error

func Setup(ctx context.Context, service string) (ShutdownFunc, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		return nil, fmt.Errorf("telemetry service name is required")
	}
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ratio, err := sampleRatioFromEnvironment()
	if err != nil {
		return nil, err
	}
	deployment := strings.TrimSpace(os.Getenv("OTEL_DEPLOYMENT_ENVIRONMENT"))
	if deployment == "" {
		deployment = "unknown"
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
		sdktrace.WithResource(resource.NewWithAttributes(
			"",
			attribute.String("service.name", service),
			attribute.String("deployment.environment.name", deployment),
		)),
	}

	var setupErr error
	if exportConfigured() {
		exporter, exporterErr := otlptracehttp.New(ctx)
		if exporterErr != nil {
			setupErr = fmt.Errorf("create OTLP trace exporter: %w", exporterErr)
		} else {
			options = append(options, sdktrace.WithBatcher(
				exporter,
				sdktrace.WithBatchTimeout(time.Second),
				sdktrace.WithExportTimeout(5*time.Second),
			))
		}
	}

	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	return provider.Shutdown, setupErr
}

func Tracer() trace.Tracer {
	return otel.Tracer(instrumentationName)
}

func exportConfigured() bool {
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != ""
}

func sampleRatioFromEnvironment() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG"))
	if raw == "" {
		return 1, nil
	}
	ratio, err := strconv.ParseFloat(raw, 64)
	if err != nil || ratio < 0 || ratio > 1 {
		return 0, fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be a number between 0 and 1")
	}
	return ratio, nil
}
