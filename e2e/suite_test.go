package e2e

import (
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestDemoMultipleE2E(t *testing.T) {
	nats := features.New("e2e demo with nats").
		WithLabel("feature", "e2e-nats").
		Setup(setupNats()).
		Assess("nats server running", natsRunning()).
		Assess("publisher running", publisherRunning()).
		Assess("subscriber received message", subscriberRunning()).
		Feature()

	eb := features.New("e2e demo with eventbridge").
		WithLabel("feature", "e2e-eventbridge").
		Setup(setupEventBridge()).
		Teardown(teardownEventBridge()).
		Assess("event bus created", eventbusCreated()).
		Assess("event bus deleted", eventbusDeleted()).
		Feature()

	testEnv.Test(t, nats, eb)
}
