package util

import (
	"fmt"
	"math/rand"
	"sobey-runtime/common"
	"strconv"
	"strings"
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

// BuildContainerID ...
func BuildContainerID(id string) string {
	if strings.HasPrefix(id, common.ContainerIDPrefix) {
		return id
	}
	return fmt.Sprintf("%s_%s", common.ContainerIDPrefix, id)
}

// RemoveContainerIDPrefix ...
func RemoveContainerIDPrefix(id string) string {
	if !strings.HasPrefix(id, common.ContainerIDPrefix) {
		return id
	}
	return strings.ReplaceAll(id, common.ContainerIDPrefix, "")
}

// BuildSandboxID ...
func BuildSandboxID(id string) string {
	if strings.HasPrefix(id, common.SandboxIDPrefix) {
		return id
	}
	return fmt.Sprintf("%s_%s", common.SandboxIDPrefix, id)
}

// RemoveSandboxIDPrefix ...
func RemoveSandboxIDPrefix(id string) string {
	if !strings.HasPrefix(id, common.SandboxIDPrefix) {
		return id
	}
	return strings.ReplaceAll(id, common.SandboxIDPrefix, "")
}
