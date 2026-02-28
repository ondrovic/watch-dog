package docker

import (
	"context"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// HealthEvent is emitted when a container's health status changes.
type HealthEvent struct {
	ContainerID   string
	ContainerName string
	Status        string // "health_status: unhealthy" etc.
}

// SubscribeHealthStatus subscribes to Docker health_status events and sends unhealthy events to the channel.
// The context cancels the subscription. The channel is closed when the subscription ends.
func (c *Client) SubscribeHealthStatus(ctx context.Context, out chan<- HealthEvent) {
	opts := events.ListOptions{Filters: newHealthStatusFilter()}
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
				if e.Action != "health_status: unhealthy" {
					continue
				}
				name := e.Actor.Attributes["name"]
				if name == "" {
					name = e.Actor.ID
				}
				select {
				case out <- HealthEvent{
					ContainerID:   e.Actor.ID,
					ContainerName: name,
					Status:        string(e.Action),
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
}

func newHealthStatusFilter() filters.Args {
	f := filters.NewArgs()
	f.Add("event", "health_status")
	return f
}
