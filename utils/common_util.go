package util

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"sobey-runtime/common"
	"strings"
)

// RandomString returns a random string with a fixed length
func RandomString() string {
	randBytes := make([]byte, 6)
	rand.Read(randBytes)
	return fmt.Sprintf("%02x%02x%02x%02x%02x%02x",
		randBytes[0], randBytes[1], randBytes[2],
		randBytes[3], randBytes[4], randBytes[5])
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

func CreateDirsIfDontExist(dirs []string) error {
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err = os.MkdirAll(dir, 0755); err != nil {
				log.Printf("Error creating directory: %v\n", err)
				return err
			}
		}
	}
	return nil
}
