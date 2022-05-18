package util

import (
	"fmt"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"math/rand"
	"strconv"
	"time"
)

var defaultLetters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

// RandomString returns a random string with a fixed length
func RandomString() string {
	b := make([]rune, 12)
	for i := range b {
		b[i] = defaultLetters[rand.Intn(len(defaultLetters))]
	}

	return fmt.Sprintf("%s_%s", string(b), strconv.FormatInt(time.Now().UnixNano(), 10))
}

// BuildContainerName creates a unique container name string.
func BuildContainerName(metadata *runtimeapi.ContainerMetadata, sandboxID string) string {
	// include the sandbox ID to make the container ID unique.
	return fmt.Sprintf("%s_%s_%d", sandboxID, metadata.Name, metadata.Attempt)
}

// BuildSandboxName creates a unique sandbox name string.
func BuildSandboxName(metadata *runtimeapi.PodSandboxMetadata) string {
	return fmt.Sprintf("%s_%s_%s_%d", metadata.Name, metadata.Namespace, metadata.Uid, metadata.Attempt)
}
