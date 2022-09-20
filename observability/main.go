package main

import (
	"context"
	"go.opentelemetry.io/otel"
	"observability/pkg/exporter"
	"time"
)

const TraceName = "mytrace"

func main() {
	tp := exporter.NewJaegerProvider()
	ctx, span := otel.Tracer(TraceName).Start(context.Background(), "main")

	time.Sleep(time.Second * 3)

	span.End()
	tp.ForceFlush(ctx)
}
