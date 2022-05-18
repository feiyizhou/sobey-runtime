package util

import (
	"fmt"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"strconv"
	"strings"
)

const (
	// kubePrefix is used to identify the containers/sandboxes on the node managed by kubelet
	kubePrefix = "k8s"
	// sandboxContainerName is a string to include in the docker container so
	// that users can easily identify the sandboxes.
	sandboxContainerName = "POD"
	// Delimiter used to construct docker container names.
	nameDelimiter = "_"
	// SobeyImageIDPrefix is the prefix of image id in container status.
	SobeyImageIDPrefix = "sobey://"
	// SobeyPullableImageIDPrefix is the prefix of pullable image id in container status.
	SobeyPullableImageIDPrefix = "sobey-pullable://"
)

func MakeContainerName(s *runtimeapi.PodSandboxConfig, c *runtimeapi.ContainerConfig) string {
	return strings.Join([]string{
		kubePrefix,                            // 0
		c.Metadata.Name,                       // 1:
		s.Metadata.Name,                       // 2: sandbox name
		s.Metadata.Namespace,                  // 3: sandbox namesapce
		s.Metadata.Uid,                        // 4  sandbox uid
		fmt.Sprintf("%d", c.Metadata.Attempt), // 5
	}, nameDelimiter)
}

func ToPullableImageID(id string, pullable bool) string {
	// Default to the image ID, but if RepoDigests is not empty, use
	// the first digest instead.
	imageID := SobeyImageIDPrefix + id
	if pullable {
		imageID = SobeyPullableImageIDPrefix + id
	}
	return imageID
}

func ParseContainerName(name string) (*runtimeapi.ContainerMetadata, error) {
	// Docker adds a "/" prefix to names. so trim it.
	name = strings.TrimPrefix(name, "/")

	parts := strings.Split(name, nameDelimiter)
	// Tolerate the random suffix.
	if len(parts) != 6 && len(parts) != 7 {
		return nil, fmt.Errorf("failed to parse the container name: %q", name)
	}
	if parts[0] != kubePrefix {
		return nil, fmt.Errorf("container is not managed by kubernetes: %q", name)
	}

	attempt, err := parseUint32(parts[5])
	if err != nil {
		return nil, fmt.Errorf("failed to parse the container name %q: %v", name, err)
	}

	return &runtimeapi.ContainerMetadata{
		Name:    parts[1],
		Attempt: attempt,
	}, nil
}

func ParseSandboxName(name string) (*runtimeapi.PodSandboxMetadata, error) {
	// Docker adds a "/" prefix to names. so trim it.
	name = strings.TrimPrefix(name, "/")

	parts := strings.Split(name, nameDelimiter)
	// Tolerate the random suffix.
	if len(parts) != 6 && len(parts) != 7 {
		return nil, fmt.Errorf("failed to parse the sandbox name: %q", name)
	}
	if parts[0] != kubePrefix {
		return nil, fmt.Errorf("container is not managed by kubernetes: %q", name)
	}

	attempt, err := parseUint32(parts[5])
	if err != nil {
		return nil, fmt.Errorf("failed to parse the sandbox name %q: %v", name, err)
	}

	return &runtimeapi.PodSandboxMetadata{
		Name:      parts[2],
		Namespace: parts[3],
		Uid:       parts[4],
		Attempt:   attempt,
	}, nil
}

func parseUint32(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(n), nil
}
