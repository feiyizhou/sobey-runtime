package util

import (
	"fmt"
	"sobey-runtime/common"
	"strings"
	"time"
)

const (
	annotationPrefix     = "annotation."
	securityOptSeparator = '='
)

// MakeLabels converts annotations to labels and merge them with the given
// labels. This is necessary because docker does not support annotations;
// we *fake* annotations using labels. Note that docker labels are not
// updatable.
func MakeLabels(labels, annotations map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range labels {
		merged[k] = v
	}
	for k, v := range annotations {
		// Assume there won't be conflict.
		merged[fmt.Sprintf("%s%s", annotationPrefix, k)] = v
	}
	return merged
}

func GetContainerTimestamps(created, started, finished string) (time.Time, time.Time, time.Time, error) {
	var createdAt, startedAt, finishedAt time.Time
	var err error
	createdAt, err = ParseDockerTimestamp(created)
	if err != nil {
		return createdAt, startedAt, finishedAt, err
	}
	startedAt, err = ParseDockerTimestamp(started)
	if err != nil {
		return createdAt, startedAt, finishedAt, err
	}
	finishedAt, err = ParseDockerTimestamp(finished)
	if err != nil {
		return createdAt, startedAt, finishedAt, err
	}
	return createdAt, startedAt, finishedAt, nil
}

// ParseDockerTimestamp parses the timestamp returned by Interface from string to time.Time
func ParseDockerTimestamp(s string) (time.Time, error) {
	// Timestamp returned by Docker is in time.RFC3339Nano format.
	return time.Parse(time.RFC3339Nano, s)
}

func ExtractLabels(input map[string]string) (map[string]string, map[string]string) {
	labels := make(map[string]string)
	annotations := make(map[string]string)
	for k, v := range input {
		// Check if the key is used internally by the shim.
		internal := false
		for _, internalKey := range common.InternalLabelKeys {
			if k == internalKey {
				internal = true
				break
			}
		}
		if internal {
			continue
		}

		// Delete the container name label for the sandbox. It is added in the shim,
		// should not be exposed via CRI.
		if k == common.KubernetesContainerNameLabel &&
			input[common.ContainerTypeLabelKey] == common.ContainerTypeLabelSandbox {
			continue
		}

		// Check if the label should be treated as an annotation.
		if strings.HasPrefix(k, annotationPrefix) {
			annotations[strings.TrimPrefix(k, annotationPrefix)] = v
			continue
		}
		labels[k] = v
	}
	return labels, annotations
}
