// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	protoutils "github.com/DataDog/datadog-agent/pkg/util/proto"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remote"
)

const (
	collectorID = "remote-workloadmeta"
)

type client struct {
	cl pb.AgentSecureClient
}

func (c *client) StreamEntities(ctx context.Context, opts ...grpc.CallOption) (remote.Stream, error) {
	streamcl, err := c.cl.WorkloadmetaStreamEntities(
		ctx,
		&pb.WorkloadmetaStreamRequest{
			Filter: nil, // Subscribes to all events
		},
	)
	if err != nil {
		return nil, err
	}
	return &stream{cl: streamcl}, nil
}

type stream struct {
	cl pbgo.AgentSecure_WorkloadmetaStreamEntitiesClient
}

func (s *stream) Recv() (interface{}, error) {
	return s.cl.Recv()
}

type streamHandler struct {
	port int
}

func init() {
	grpclog.SetLoggerV2(grpcutil.NewLogger())

	workloadmeta.RegisterRemoteCollector(collectorID, func() workloadmeta.Collector {
		return &remote.GenericCollector{
			CollectorID:   collectorID,
			StreamHandler: &streamHandler{},
		}
	})
}

func (s *streamHandler) Port() int {
	if s.port == 0 {
		return config.Datadog.GetInt("cmd_port")
	}
	// for tests
	return s.port
}

func (s *streamHandler) NewClient(cc grpc.ClientConnInterface) remote.RemoteGrpcClient {
	return &client{cl: pb.NewAgentSecureClient(cc)}
}

// IsEnabled always return true for the remote workloadmeta because it uses the remote catalog
func (s *streamHandler) IsEnabled() bool {
	return true
}

func (s *streamHandler) HandleResponse(resp interface{}) ([]workloadmeta.CollectorEvent, error) {
	response, ok := resp.(*pb.WorkloadmetaStreamResponse)
	if !ok {
		return nil, fmt.Errorf("incorrect response type")
	}
	var collectorEvents []workloadmeta.CollectorEvent

	for _, protoEvent := range response.Events {
		workloadmetaEvent, err := protoutils.WorkloadmetaEventFromProtoEvent(protoEvent)
		if err != nil {
			return nil, err
		}

		collectorEvent := workloadmeta.CollectorEvent{
			Type:   workloadmetaEvent.Type,
			Source: workloadmeta.SourceRemoteWorkloadmeta,
			Entity: workloadmetaEvent.Entity,
		}

		collectorEvents = append(collectorEvents, collectorEvent)
	}

	return collectorEvents, nil
}

func (s *streamHandler) HandleResync(store workloadmeta.Store, events []workloadmeta.CollectorEvent) {
	var entities []workloadmeta.Entity
	for _, event := range events {
		entities = append(entities, event.Entity)
	}
	// This should be the first response that we got from workloadmeta after
	// we lost the connection and specified that a re-sync is needed. So, at
	// this point we know that "entities" contains all the existing entities
	// in the store, because when a client subscribes to workloadmeta, the
	// first response is always a bundle of events with all the existing
	// entities in the store that match the filters specified (see
	// workloadmeta.Store#Subscribe).
	store.Reset(entities, workloadmeta.SourceRemoteWorkloadmeta)
}
