package metrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"strings"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

type ProtocolDescriptor interface {
	ServiceDescriptor() ([]byte, int)
}

type metricsContextKey string

const (
	contextStartTime = metricsContextKey("start-time")
)

func RegisterService(d ProtocolDescriptor) error {
	p, _ := d.ServiceDescriptor()
	reader, err := gzip.NewReader(bytes.NewReader(p))
	if err != nil {
		return err
	}

	de, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	pb := descriptorpb.FileDescriptorProto{}
	if err := proto.Unmarshal(de, &pb); err != nil {
		return err
	}

	labelBuilder := strings.Builder{}
	keyBuilder := strings.Builder{}
	for _, svc := range pb.Service {
		for _, m := range svc.Method {
			keyBuilder.Reset()
			labelBuilder.Reset()

			labelBuilder.WriteString("rpc_durations_seconds")
			labelBuilder.WriteString(`{service="`)
			labelBuilder.WriteString(svc.GetName())
			labelBuilder.WriteString(`",`)
			labelBuilder.WriteString(`method="`)
			labelBuilder.WriteString(m.GetName())
			labelBuilder.WriteString(`"}`)

			keyBuilder.WriteString(svc.GetName())
			keyBuilder.WriteString(".")
			keyBuilder.WriteString(m.GetName())

			histo := metricSet.GetOrCreateHistogram(labelBuilder.String())
			histoMap.LoadOrStore(keyBuilder.String(), histo)
		}
	}

	return nil
}

func BeginRPC(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextStartTime, time.Now())
}

func FinishRPC(ctx context.Context) {
	start, ok := ctx.Value(contextStartTime).(time.Time)
	if !ok {
		return
	}
	service, sok := twirp.ServiceName(ctx)
	method, mok := twirp.MethodName(ctx)
	if !sok || !mok {
		return
	}
	keyBuilder := strings.Builder{}
	keyBuilder.WriteString(service)
	keyBuilder.WriteString(".")
	keyBuilder.WriteString(method)
	h, ok := histoMap.Load(keyBuilder.String())
	if !ok {
		return
	}
	h.(*metrics.Histogram).UpdateDuration(start)
}
