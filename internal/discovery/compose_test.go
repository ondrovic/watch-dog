package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContainerNameToServiceName_emptyPath(t *testing.T) {
	got, err := ContainerNameToServiceName("")
	if err != nil {
		t.Fatalf("ContainerNameToServiceName(\"\"): err = %v", err)
	}
	if got == nil {
		t.Fatal("ContainerNameToServiceName(\"\"): got nil map, want initialized empty map")
	}
	if len(got) != 0 {
		t.Errorf("ContainerNameToServiceName(\"\"): len(got) = %d, want 0", len(got))
	}
}

func TestContainerNameToServiceName_missingFile(t *testing.T) {
	_, err := ContainerNameToServiceName("/nonexistent/compose.yml")
	if err == nil {
		t.Error("ContainerNameToServiceName(missing file): want error, got nil")
	}
}

func TestContainerNameToServiceName_resolvesContainerNameToService(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	content := `
services:
  gluetun:
    container_name: vpn
    image: ghcr.io/qdm12/gluetun:latest
  qbittorrent:
    container_name: dler
    image: linuxserver/qbittorrent:latest
  sonarr:
    # no container_name; service name is sonarr
    image: linuxserver/sonarr:latest
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	got, err := ContainerNameToServiceName(path)
	if err != nil {
		t.Fatalf("ContainerNameToServiceName: err = %v", err)
	}

	// Only services with container_name appear in the map.
	if got["vpn"] != "gluetun" {
		t.Errorf("got[%q] = %q, want gluetun", "vpn", got["vpn"])
	}
	if got["dler"] != "qbittorrent" {
		t.Errorf("got[%q] = %q, want qbittorrent", "dler", got["dler"])
	}
	if _, ok := got["sonarr"]; ok {
		t.Error("service without container_name should not appear in map")
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
}

func TestContainerNameToServiceName_duplicateContainerName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	content := `
services:
  a:
    container_name: same
  b:
    container_name: same
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	_, err := ContainerNameToServiceName(path)
	if err == nil {
		t.Fatal("ContainerNameToServiceName(duplicate container_name): want error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "duplicate") || !strings.Contains(msg, "same") {
		t.Errorf("error should mention duplicate and container_name; got %q", msg)
	}
	// Conflicting service names (order may vary)
	if !strings.Contains(msg, "a") || !strings.Contains(msg, "b") {
		t.Errorf("error should mention both service names a and b; got %q", msg)
	}
}
