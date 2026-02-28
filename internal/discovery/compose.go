package discovery

import (
	"context"
	"os"

	"gopkg.in/yaml.v3"

	"watch-dog/internal/docker"
)

// ComposeFile represents the minimal structure needed to read root-level depends_on.
// Only "services" and each service's "depends_on" are used.
type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
}

// ComposeService holds a single service's depends_on (and optional fields we ignore).
type ComposeService struct {
	DependsOn interface{} `yaml:"depends_on"` // short: []string, long: map[string]DependsOnEntry
}

// DependsOnEntry is the long-form value (condition, restart, etc.).
type DependsOnEntry struct {
	Condition string `yaml:"condition"`
	Restart   *bool  `yaml:"restart"`
}

// ParseComposeFile reads and parses a compose YAML file into ComposeFile.
// Returns nil if the file cannot be read or parsed.
func ParseComposeFile(path string) (*ComposeFile, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f ComposeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Services == nil {
		f.Services = make(map[string]ComposeService)
	}
	return &f, nil
}

// ServiceParents returns the list of parent service names for a given service's depends_on value.
// Supports short form (list of strings) and long form (map of service name to optional object).
func ServiceParents(dependsOn interface{}) []string {
	if dependsOn == nil {
		return nil
	}
	switch v := dependsOn.(type) {
	case []interface{}:
		return serviceParentsShort(v)
	case map[string]interface{}:
		return serviceParentsLong(v)
	default:
		return nil
	}
}

func serviceParentsShort(list []interface{}) []string {
	out := make([]string, 0, len(list))
	seen := make(map[string]bool)
	for _, item := range list {
		s, _ := item.(string)
		s = trim(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func serviceParentsLong(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		name = trim(name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func trim(s string) string {
	const space = " \t"
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// BuildServiceParentToDependents builds a map from parent service name to list of dependent service names
// from the parsed compose file. Used together with running containers (compose labels) to build
// ParentToDependents keyed by container name (see BuildParentToDependentsFromCompose).
func BuildServiceParentToDependents(f *ComposeFile) map[string][]string {
	if f == nil || len(f.Services) == 0 {
		return nil
	}
	// Set of valid service names (parents must be defined as services)
	serviceNames := make(map[string]bool)
	for name := range f.Services {
		serviceNames[name] = true
	}
	m := make(map[string][]string)
	for depName, svc := range f.Services {
		parents := ServiceParents(svc.DependsOn)
		for _, parent := range parents {
			if !serviceNames[parent] {
				continue
			}
			m[parent] = append(m[parent], depName)
		}
	}
	return m
}

const (
	labelComposeService = "com.docker.compose.service"
	labelComposeProject = "com.docker.compose.project"
)

// BuildParentToDependentsFromCompose parses the compose file at composePath, builds the
// service-level parent→dependents map, maps service names to running container names
// using com.docker.compose.service (and project) labels, and returns ParentToDependents
// keyed by container name. Services with no running container are ignored.
func BuildParentToDependentsFromCompose(ctx context.Context, cli *docker.Client, composePath string) (ParentToDependents, error) {
	if composePath == "" {
		return make(ParentToDependents), nil
	}
	f, err := ParseComposeFile(composePath)
	if err != nil || f == nil {
		return nil, err
	}
	svcParentToDeps := BuildServiceParentToDependents(f)
	if len(svcParentToDeps) == 0 {
		return make(ParentToDependents), nil
	}
	// Include stopped containers so we still see parent services when a parent is stopped.
	containers, err := cli.ListContainers(ctx, true)
	if err != nil {
		return nil, err
	}
	// service name -> container names (one service can have multiple replicas)
	serviceToContainers := make(map[string][]string)
	for _, c := range containers {
		svc, ok := c.Labels[labelComposeService]
		if !ok || svc == "" {
			continue
		}
		serviceToContainers[svc] = append(serviceToContainers[svc], c.Name)
	}
	// parent container name -> dependent container names
	m := make(ParentToDependents)
	for parentSvc, depSvcs := range svcParentToDeps {
		parentContainers := serviceToContainers[parentSvc]
		if len(parentContainers) == 0 {
			continue
		}
		var allDepContainers []string
		for _, depSvc := range depSvcs {
			allDepContainers = append(allDepContainers, serviceToContainers[depSvc]...)
		}
		if len(allDepContainers) == 0 {
			continue
		}
		for _, parentName := range parentContainers {
			m[parentName] = append(m[parentName], allDepContainers...)
		}
	}
	return m, nil
}
