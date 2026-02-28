// Package docker provides a Docker API client for listing containers, inspecting
// health status, and restarting containers, plus logging and health-status event subscription.
package docker

import (
	"context"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// Client wraps the Docker API for listing containers, inspecting health, and restarting.
type Client struct {
	cli *client.Client
}

// ContainerInfo holds minimal container data for discovery.
type ContainerInfo struct {
	// ID is the container ID.
	ID string
	// Name is the container name (without leading slash).
	Name string
	// Labels holds container labels (e.g. com.docker.compose.service).
	Labels map[string]string
	// State is "running", "exited", etc.; only set when ListContainers is called with all=true.
	State string
}

// NewClient creates a Docker client using DOCKER_HOST (default unix socket).
func NewClient(ctx context.Context) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli}, nil
}

// ListContainers returns containers with their labels (name without leading /).
// If all is false, only running containers are returned and State is not set.
// If all is true, all containers are returned and State is set ("running", "exited", etc.).
func (c *Client) ListContainers(ctx context.Context, all bool) ([]ContainerInfo, error) {
	opts := container.ListOptions{All: all}
	list, err := c.cli.ContainerList(ctx, opts)
	if err != nil {
		return nil, err
	}
	out := make([]ContainerInfo, 0, len(list))
	for _, cnt := range list {
		if len(cnt.Names) == 0 {
			continue
		}
		name := cnt.Names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}
		info := ContainerInfo{
			ID:     cnt.ID,
			Name:   name,
			Labels: cnt.Labels,
		}
		if all {
			info.State = cnt.State
		}
		out = append(out, info)
	}
	return out, nil
}

// Inspect returns health status and labels for a container by ID or name.
// Health is "healthy", "unhealthy", "starting", or "" if no healthcheck.
func (c *Client) Inspect(ctx context.Context, containerID string) (health string, labels map[string]string, err error) {
	inspect, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", nil, err
	}
	labels = inspect.Config.Labels
	if inspect.State.Health != nil {
		health = inspect.State.Health.Status
	}
	return health, labels, nil
}

// Restart restarts the container (idempotent).
func (c *Client) Restart(ctx context.Context, containerID string) error {
	timeout := 10
	return c.cli.ContainerRestart(ctx, containerID, container.StopOptions{Signal: "", Timeout: &timeout})
}

// Close closes the underlying client.
func (c *Client) Close() error {
	return c.cli.Close()
}

// DOCKER_HOST is set from env for documentation; client.FromEnv uses it.
var _ = os.Getenv("DOCKER_HOST")
