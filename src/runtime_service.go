package src

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"time"
)

const (
	// Name of the underlying container runtime
	runtimeName = "sobey"
)

var (
	// Termination grace period
	defaultSandboxGracePeriod = time.Duration(10) * time.Second
)

func (ss *sobeyService) Version(context.Context, *runtimeapi.VersionRequest) (*runtimeapi.VersionResponse, error) {
	return &runtimeapi.VersionResponse{
		Version:           "0.1.0",
		RuntimeName:       "sobey",
		RuntimeVersion:    "1.0.0",
		RuntimeApiVersion: "1.0.0",
	}, nil
}

func (ss *sobeyService) ReopenContainerLog(ctx context.Context, req *runtimeapi.ReopenContainerLogRequest) (*runtimeapi.ReopenContainerLogResponse, error) {
	return &runtimeapi.ReopenContainerLogResponse{}, nil
}

func (ss *sobeyService) ExecSync(ctx context.Context, req *runtimeapi.ExecSyncRequest) (*runtimeapi.ExecSyncResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExecSync not implemented")
}

func (ss *sobeyService) Exec(ctx context.Context, req *runtimeapi.ExecRequest) (*runtimeapi.ExecResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Exec not implemented")
}

func (ss *sobeyService) Attach(ctx context.Context, req *runtimeapi.AttachRequest) (*runtimeapi.AttachResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Attach not implemented")
}

func (ss *sobeyService) PortForward(ctx context.Context, req *runtimeapi.PortForwardRequest) (*runtimeapi.PortForwardResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PortForward not implemented")
}

func (ss *sobeyService) ListContainerStats(ctx context.Context, req *runtimeapi.ListContainerStatsRequest) (*runtimeapi.ListContainerStatsResponse, error) {
	containerStatsFilter := req.GetFilter()
	filter := &runtimeapi.ContainerFilter{}

	if containerStatsFilter != nil {
		filter.Id = containerStatsFilter.Id
		filter.PodSandboxId = containerStatsFilter.PodSandboxId
		filter.LabelSelector = containerStatsFilter.LabelSelector
	}
	listResp, err := ss.ListContainers(ctx, &runtimeapi.ListContainersRequest{Filter: filter})
	if err != nil {
		return nil, err
	}
	var stats []*runtimeapi.ContainerStats
	for _, container := range listResp.Containers {
		containerStats, err := ss.getContainerStats(container)
		if err != nil {
			return nil, err
		}
		if containerStats != nil {
			stats = append(stats, containerStats)
		}
	}
	return &runtimeapi.ListContainerStatsResponse{Stats: stats}, nil
}

func (ss *sobeyService) UpdateRuntimeConfig(ctx context.Context, req *runtimeapi.UpdateRuntimeConfigRequest) (*runtimeapi.UpdateRuntimeConfigResponse, error) {
	return &runtimeapi.UpdateRuntimeConfigResponse{}, nil
}
func (ss *sobeyService) Status(ctx context.Context, req *runtimeapi.StatusRequest) (*runtimeapi.StatusResponse, error) {
	runtimeReady := &runtimeapi.RuntimeCondition{
		Type:   runtimeapi.RuntimeReady,
		Status: true,
	}
	networkReady := &runtimeapi.RuntimeCondition{
		Type:   runtimeapi.NetworkReady,
		Status: true,
	}
	conditions := []*runtimeapi.RuntimeCondition{runtimeReady, networkReady}
	runtimeStatus := &runtimeapi.RuntimeStatus{Conditions: conditions}
	return &runtimeapi.StatusResponse{Status: runtimeStatus}, nil
}

func (ss *sobeyService) getContainerStats(c *runtimeapi.Container) (*runtimeapi.ContainerStats, error) {
	timestamp := time.Now().UnixNano()
	containerStats := &runtimeapi.ContainerStats{
		Attributes: &runtimeapi.ContainerAttributes{
			Id:          c.Id,
			Metadata:    c.Metadata,
			Labels:      c.Labels,
			Annotations: c.Annotations,
		},
		// TODO calculate every server physic usage
		Cpu: &runtimeapi.CpuUsage{
			Timestamp:            timestamp,
			UsageCoreNanoSeconds: &runtimeapi.UInt64Value{Value: 1},
		},
		Memory: &runtimeapi.MemoryUsage{
			Timestamp:       timestamp,
			WorkingSetBytes: &runtimeapi.UInt64Value{Value: 1},
		},
		WritableLayer: &runtimeapi.FilesystemUsage{
			Timestamp: timestamp,
			//FsId:      &runtimeapi.FilesystemIdentifier{Mountpoint: common.SockerImagesPath},
			UsedBytes: &runtimeapi.UInt64Value{Value: 1},
		},
	}
	return containerStats, nil
}
