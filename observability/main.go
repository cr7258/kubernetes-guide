package main

import (
	"context"
	"go.opentelemetry.io/otel"
	"log"
	"observability/pkg/exporter"
	"os"
	"time"
)

const TraceName = "mytrace"

func main() {
	f, err := os.OpenFile("trace.txt", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		log.Fatalln(err)
	}

	defer f.Close()

	tp := exporter.NewProvider(f)
	ctx, span := otel.Tracer(TraceName).Start(context.Background(), "main")

	time.Sleep(time.Second * 3)

	span.End()
	tp.ForceFlush(ctx)
}

