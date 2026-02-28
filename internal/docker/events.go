// Package docker provides a Docker API client for listing containers, inspecting
// health status, and restarting containers, plus logging and health-status event subscription.
package docker

import (
	"context"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// HealthEvent is emitted when a container's health status changes.
type HealthEvent struct {
	// ContainerID is the container ID.
	ContainerID string
	// ContainerName is the container name.
	ContainerName string
	// Status is the event action (e.g. "health_status: unhealthy").
	Status string
}

// SubscribeHealthStatus subscribes to Docker container events: health_status (unhealthy), die, and stop.
// When a parent container goes unhealthy or stops, the event is sent to the channel so recovery can run.
// The context cancels the subscription. The channel is closed when the subscription ends.
func (c *Client) SubscribeHealthStatus(ctx context.Context, out chan<- HealthEvent) {
	opts := events.ListOptions{Filters: newRecoveryEventFilter()}
	msgs, errs := c.cli.Events(ctx, opts)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errs:
				if err != nil {
					LogError("docker events", "error", err)
				}
				return
			case e, ok := <-msgs:
				if !ok {
					return
				}
				if e.Type != events.ContainerEventType {
					continue
				}
				action := e.Action
				if action != "health_status: unhealthy" && action != "die" && action != "stop" {
					continue
				}
				// For health_status the attribute is "health_status"; for die/stop use "name"
				name := e.Actor.Attributes["name"]
				if name == "" {
					name = e.Actor.ID
				}
				select {
				case out <- HealthEvent{
					ContainerID:   e.Actor.ID,
					ContainerName: name,
					Status:        string(action),
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
}

func newRecoveryEventFilter() filters.Args {
	f := filters.NewArgs()
	f.Add("type", "container")
	f.Add("event", "health_status")
	f.Add("event", "die")
	f.Add("event", "stop")
	return f
}
