package realtime

import "testing"

func TestHubBroadcastsEventsToSubscribers(t *testing.T) {
	hub := NewHub()
	client := &client{send: make(chan Event, hub.buffer)}
	hub.subscribe(client)
	defer hub.unsubscribe(client)

	hub.Broadcast(EventMCPChanged, MCPChanged{Resource: "api-mocks", Action: "updated", Source: "admin-api", CreatedAt: Now()})

	event := <-client.send
	if event.Event != EventMCPChanged {
		t.Fatalf("expected event %q, got %q", EventMCPChanged, event.Event)
	}

	changed, ok := event.Payload.(MCPChanged)
	if !ok {
		t.Fatalf("expected MCPChanged payload, got %T", event.Payload)
	}
	if changed.Resource != "api-mocks" || changed.Action != "updated" {
		t.Fatalf("unexpected payload: %+v", changed)
	}
}
