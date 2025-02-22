// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mocksender

import (
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/log"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// NewMockSender initiates the aggregator and returns a
// functional mocked Sender for testing
func NewMockSender(id checkid.ID) *MockSender {
	mockSender := new(MockSender)

	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = 1 * time.Hour
	opts.DontStartForwarders = true
	log := log.NewTemporaryLoggerWithoutInit()
	sharedForwarder := forwarder.NewDefaultForwarder(config.Datadog, log, forwarder.NewOptions(config.Datadog, log, nil))
	aggregator.InitAndStartAgentDemultiplexer(log, sharedForwarder, opts, "")

	SetSender(mockSender, id)

	return mockSender
}

// SetSender sets passed sender with the passed ID.
func SetSender(sender *MockSender, id checkid.ID) {
	aggregator.SetSender(sender, id) //nolint:errcheck
}

// MockSender allows mocking of the checks sender for unit testing
type MockSender struct {
	mock.Mock
}

// SetupAcceptAll sets mock expectations to accept any call in the Sender interface
func (m *MockSender) SetupAcceptAll() {
	metricCalls := []string{"Rate", "Count", "MonotonicCount", "Counter", "Histogram", "Historate", "Gauge"}
	for _, call := range metricCalls {
		m.On(call,
			mock.AnythingOfType("string"),   // Metric
			mock.AnythingOfType("float64"),  // Value
			mock.AnythingOfType("string"),   // Hostname
			mock.AnythingOfType("[]string"), // Tags
		).Return()
	}
	m.On("MonotonicCountWithFlushFirstValue",
		mock.AnythingOfType("string"),   // Metric
		mock.AnythingOfType("float64"),  // Value
		mock.AnythingOfType("string"),   // Hostname
		mock.AnythingOfType("[]string"), // Tags
		mock.AnythingOfType("bool"),     // FlushFirstValue
	).Return()
	m.On("ServiceCheck",
		mock.AnythingOfType("string"),                          // checkName (e.g: docker.exit)
		mock.AnythingOfType("servicecheck.ServiceCheckStatus"), // (e.g: servicecheck.ServiceCheckOK)
		mock.AnythingOfType("string"),                          // Hostname
		mock.AnythingOfType("[]string"),                        // Tags
		mock.AnythingOfType("string"),                          // message
	).Return()
	m.On("Event", mock.AnythingOfType("event.Event")).Return()
	// The second argument should have been `mock.AnythingOfType("[]byte")` instead of `mock.AnythingOfType("[]uint8")`
	// See https://github.com/stretchr/testify/issues/387
	m.On("EventPlatformEvent", mock.AnythingOfType("[]uint8"), mock.AnythingOfType("string")).Return()
	m.On("HistogramBucket",
		mock.AnythingOfType("string"),   // metric name
		mock.AnythingOfType("int64"),    // value
		mock.AnythingOfType("float64"),  // lower bound
		mock.AnythingOfType("float64"),  // upper bound
		mock.AnythingOfType("bool"),     // monotonic
		mock.AnythingOfType("string"),   // hostname
		mock.AnythingOfType("[]string"), // tags
		mock.AnythingOfType("bool"),     // FlushFirstValue
	).Return()
	m.On("GetSenderStats", mock.AnythingOfType("stats.SenderStats")).Return()
	m.On("DisableDefaultHostname", mock.AnythingOfType("bool")).Return()
	m.On("SetCheckCustomTags", mock.AnythingOfType("[]string")).Return()
	m.On("SetCheckService", mock.AnythingOfType("string")).Return()
	m.On("FinalizeCheckServiceTag").Return()
	m.On("SetNoIndex").Return()
	m.On("Commit").Return()
}

// ResetCalls makes the mock forget previous calls
func (m *MockSender) ResetCalls() {
	m.Mock.Calls = m.Mock.Calls[0:0]
}
