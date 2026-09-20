package compose

import "gopkg.in/yaml.v3"

// DockerCompose represents a complete docker-compose.yml file.
// Services are stored as a map for fast lookup, but ServiceOrder tracks
// insertion order so that MarshalYAML emits them in dependency order
// (init containers before their dependents, postgres before hasura, nginx last).
type DockerCompose struct {
	Name         string                   `yaml:"name"`
	Networks     map[string]NetworkConfig `yaml:"networks"`
	Volumes      map[string]VolumeConfig  `yaml:"volumes"`
	Services     map[string]ServiceConfig `yaml:"-"`
	ServiceOrder []string                 `yaml:"-"`
}

// AddService inserts a service into the compose file, preserving insertion
// order via ServiceOrder. If the service name already exists, it is replaced
// in-place without changing the order.
func (dc *DockerCompose) AddService(name string, svc ServiceConfig) {
	if dc.Services == nil {
		dc.Services = make(map[string]ServiceConfig)
	}
	if _, exists := dc.Services[name]; !exists {
		dc.ServiceOrder = append(dc.ServiceOrder, name)
	}
	dc.Services[name] = svc
}

// MarshalYAML outputs the compose file with services in ServiceOrder rather
// than Go's default alphabetical map ordering. This ensures init containers
// appear before services that depend on them, which Docker Compose v5 requires.
func (dc *DockerCompose) MarshalYAML() (interface{}, error) {
	// Build an ordered services mapping node.
	servicesNode := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}
	for _, name := range dc.ServiceOrder {
		svc, ok := dc.Services[name]
		if !ok {
			continue
		}
		// Marshal the service config to a yaml.Node.
		var svcNode yaml.Node
		if err := svcNode.Encode(svc); err != nil {
			return nil, err
		}
		// Node.Encode uses textless=true internally, which discards comments.
		// Inject CapDropComment here — after encoding but before the node is
		// added to the tree. The final yaml.Marshal emits comments from nodes
		// in the tree without another round-trip, so HeadComment is preserved.
		if svc.CapDropComment != "" && svcNode.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(svcNode.Content); i += 2 {
				if svcNode.Content[i].Value == "cap_add" {
					svcNode.Content[i].HeadComment = svc.CapDropComment
					break
				}
			}
		}
		servicesNode.Content = append(servicesNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: name, Tag: "!!str"},
			&svcNode,
		)
	}

	// Build the top-level document as an ordered mapping.
	doc := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}

	// name
	doc.Content = append(doc.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "name", Tag: "!!str"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: dc.Name, Tag: "!!str"},
	)

	// networks (if any)
	if len(dc.Networks) > 0 {
		var networksNode yaml.Node
		if err := networksNode.Encode(dc.Networks); err != nil {
			return nil, err
		}
		doc.Content = append(doc.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "networks", Tag: "!!str"},
			&networksNode,
		)
	}

	// volumes (if any)
	if len(dc.Volumes) > 0 {
		var volumesNode yaml.Node
		if err := volumesNode.Encode(dc.Volumes); err != nil {
			return nil, err
		}
		doc.Content = append(doc.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "volumes", Tag: "!!str"},
			&volumesNode,
		)
	}

	// services
	doc.Content = append(doc.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "services", Tag: "!!str"},
		servicesNode,
	)

	return doc, nil
}

// ServiceConfig represents a single service in docker-compose.yml.
type ServiceConfig struct {
	Image         string            `yaml:"image,omitempty"`
	Build         *BuildConfig      `yaml:"build,omitempty"`
	ContainerName string            `yaml:"container_name,omitempty"`
	Restart       string            `yaml:"restart,omitempty"`
	ShmSize       string            `yaml:"shm_size,omitempty"`
	User          string            `yaml:"user,omitempty"`
	GroupAdd      []string          `yaml:"group_add,omitempty"`
	Networks      []string          `yaml:"networks,omitempty"`
	DependsOn     map[string]DepOn  `yaml:"depends_on,omitempty"`
	Environment   map[string]string `yaml:"environment,omitempty"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	Ports         []string          `yaml:"ports,omitempty"`
	Expose        []string          `yaml:"expose,omitempty"`
	Command       interface{}       `yaml:"command,omitempty"`
	Entrypoint    interface{}       `yaml:"entrypoint,omitempty"`
	Healthcheck   *Healthcheck      `yaml:"healthcheck,omitempty"`
	Deploy        *DeployConfig     `yaml:"deploy,omitempty"`
	Logging       *LoggingConfig    `yaml:"logging,omitempty"`
	SecurityOpt   []string          `yaml:"security_opt,omitempty"`
	CapDrop       []string          `yaml:"cap_drop,omitempty"`
	CapAdd        []string          `yaml:"cap_add,omitempty"`
	StopGrace     string            `yaml:"stop_grace_period,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
	Profiles      []string          `yaml:"profiles,omitempty"`
	ReadOnly      bool              `yaml:"read_only,omitempty"`
	Tmpfs         []string          `yaml:"tmpfs,omitempty"`
	Privileged    bool              `yaml:"privileged,omitempty"`
	WorkingDir    string            `yaml:"working_dir,omitempty"`
	StdinOpen     bool              `yaml:"stdin_open,omitempty"`
	Tty           bool              `yaml:"tty,omitempty"`

	// CapDropComment is an optional comment injected into the generated YAML
	// above cap_add when cap_drop is intentionally omitted. Not serialized as
	// a YAML field — rendered as a comment via MarshalYAML.
	CapDropComment string `yaml:"-"`
}

// DepOn represents a service dependency with a condition.
type DepOn struct {
	Condition string `yaml:"condition"`
}

// Healthcheck represents a service health check configuration.
type Healthcheck struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
}

// DeployConfig represents deployment constraints for a service.
type DeployConfig struct {
	Resources *Resources `yaml:"resources,omitempty"`
}

// Resources represents resource constraints.
type Resources struct {
	Limits       *ResourceLimits `yaml:"limits,omitempty"`
	Reservations *ResourceLimits `yaml:"reservations,omitempty"`
}

// ResourceLimits represents CPU and memory limits.
type ResourceLimits struct {
	Memory string `yaml:"memory,omitempty"`
	CPUs   string `yaml:"cpus,omitempty"`
	Pids   int64  `yaml:"pids,omitempty"`
}

// LoggingConfig represents the logging driver and options for a service.
type LoggingConfig struct {
	Driver  string            `yaml:"driver"`
	Options map[string]string `yaml:"options,omitempty"`
}

// BuildConfig represents the build context for a service.
type BuildConfig struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
}

// NetworkConfig represents a docker-compose network definition.
type NetworkConfig struct {
	Driver string `yaml:"driver,omitempty"`
	// Name pins the real Docker network name. Without it, compose prefixes the
	// project name (e.g. key "myproj_network" becomes "myproj_myproj_network"),
	// which split recreated services onto a new network on existing deployments
	// (P1 EOP prod incident 2026-06-10: nginx 502 on api.nself.org).
	Name string `yaml:"name,omitempty"`
}

// VolumeConfig represents a docker-compose volume definition.
type VolumeConfig struct {
	Driver string `yaml:"driver,omitempty"`
}
