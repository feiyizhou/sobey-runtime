package src

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
	"sobey-runtime/common"
	util "sobey-runtime/utils"
	"strings"
	"time"
)

type SobeyContainer struct {
	ID               string                       `json:"id"`
	Name             string                       `json:"name"`
	ServerName       string                       `json:"serverName"`
	Image            string                       `json:"image"`
	Pid              string                       `json:"pid"`
	Path             string                       `json:"path"`
	PortMapping      []*runtimeapi.PortMapping    `json:"port"`
	PodSandboxConfig *runtimeapi.PodSandboxConfig `json:"podSandboxConfig"`
	State            runtimeapi.ContainerState    `json:"state"`
	Uid              string                       `json:"uid"`
	ApiVersion       string                       `json:"apiVersion"`
	Labels           map[string]string            `json:"labels"`
	CreateAt         int64                        `json:"createAt"`
	StartedAt        int64                        `json:"startedAt"`
	FinishedAt       int64                        `json:"finishedAt"`
}

type ContainerStartRequest struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Port   string `json:"port"`
	LogDir string `json:"log_dir"`
}

type ContainerStartResponse struct {
	Name   string `json:"name"`
	Pid    string `json:"pid"`
	UpTime int64  `json:"up_time"`
}

type ContainerStopRequest struct {
	Name string `json:"name"`
	Pid  string `json:"pid"`
}

func (ss *sobeyService) ListContainers(ctx context.Context, req *runtimeapi.ListContainersRequest) (*runtimeapi.ListContainersResponse, error) {
	var result []*runtimeapi.Container
	containerInfos, err := ss.dbService.GetByPrefix("container")
	if err != nil {
		return nil, err
	}
	if len(containerInfos) != 0 {
		var sobeyContainers []*SobeyContainer
		for _, containerInfo := range containerInfos {
			sobeyContainer := new(SobeyContainer)
			err = json.Unmarshal([]byte(containerInfo), &sobeyContainer)
			if err != nil {
				return nil, err
			}
			sobeyContainers = append(sobeyContainers, sobeyContainer)
		}
		sobeyContainers = filterContainers(req.GetFilter(), sobeyContainers)
		for _, containerInfo := range sobeyContainers {
			metadata, err := util.ParseContainerName(containerInfo.Name)
			if err != nil {
				return nil, err
			}
			labels, annotations := util.ExtractLabels(containerInfo.Labels)
			result = append(result, &runtimeapi.Container{
				Id:           containerInfo.ID,
				PodSandboxId: containerInfo.Labels[common.SandboxIDLabelKey],
				Metadata:     metadata,
				Image:        &runtimeapi.ImageSpec{Image: containerInfo.Image},
				ImageRef:     util.ToPullableImageID(containerInfo.Image, true),
				State:        runtimeapi.ContainerState_CONTAINER_RUNNING,
				CreatedAt:    containerInfo.CreateAt,
				Labels:       labels,
				Annotations:  annotations,
			})
		}
	}
	return &runtimeapi.ListContainersResponse{Containers: result}, nil
}

func filterContainers(filter *runtimeapi.ContainerFilter, containers []*SobeyContainer) []*SobeyContainer {
	if filter == nil {
		return containers
	}
	var idFilterItems []*SobeyContainer
	if len(filter.Id) != 0 {
		for _, containerInfo := range containers {
			if strings.EqualFold(filter.Id, containerInfo.ID) {
				idFilterItems = append(idFilterItems, containerInfo)
			}
		}
	} else {
		idFilterItems = containers
	}

	var sandboxIdFilterItems []*SobeyContainer
	if len(filter.PodSandboxId) != 0 {
		for _, containerInfo := range idFilterItems {
			if strings.EqualFold(filter.PodSandboxId,
				containerInfo.Labels[common.SandboxIDLabelKey]) {
				sandboxIdFilterItems = append(sandboxIdFilterItems, containerInfo)
			}
		}
	} else {
		sandboxIdFilterItems = idFilterItems
	}

	var uidFilterItems []*SobeyContainer
	if uid, ok := filter.LabelSelector[common.KubernetesPodUIDLabel]; ok && len(uid) != 0 {
		for _, containerInfo := range sandboxIdFilterItems {
			if strings.EqualFold(filter.LabelSelector[common.KubernetesPodUIDLabel],
				containerInfo.Uid) {
				uidFilterItems = append(uidFilterItems, containerInfo)
			}
		}
	} else {
		uidFilterItems = sandboxIdFilterItems
	}

	var stateFilerItems []*SobeyContainer
	if filter.State != nil {
		for _, containerInfo := range uidFilterItems {
			if filter.State.State == containerInfo.State {
				stateFilerItems = append(stateFilerItems, containerInfo)
			}
		}
	} else {
		stateFilerItems = uidFilterItems
	}
	return stateFilerItems
}

func (ss *sobeyService) CreateContainer(ctx context.Context, req *runtimeapi.CreateContainerRequest) (*runtimeapi.CreateContainerResponse, error) {
	podSandboxID := req.PodSandboxId
	config := req.GetConfig()
	sandboxConfig := req.GetSandboxConfig()

	if config == nil {
		return nil, fmt.Errorf("container config is nil")
	}
	if sandboxConfig == nil {
		return nil, fmt.Errorf("sandbox config is nil for container %q", config.Metadata.Name)
	}

	labels := util.MakeLabels(config.GetLabels(), config.GetAnnotations())
	labels[common.ContainerTypeLabelKey] = common.ContainerTypeLabelContainer
	labels[common.ContainerLogPathLabelKey] = filepath.Join(sandboxConfig.LogDirectory, config.LogPath)
	labels[common.SandboxIDLabelKey] = podSandboxID

	dirArr := []string{sandboxConfig.LogDirectory}
	tailDirArr := strings.Split(config.LogPath, string(os.PathSeparator))
	dirArr = append(dirArr, tailDirArr[:len(tailDirArr)-1]...)
	err := ss.os.MkdirAll(filepath.Join(dirArr...), 0750)
	if err != nil {
		fmt.Printf("Create server log file err, err: %v", err)
	}

	containerLogFullPath := labels[common.ContainerLogPathLabelKey]
	_, err = ss.os.Create(containerLogFullPath)
	if err != nil {
		fmt.Printf("Create container log err, err: %v", err)
	}

	apiVersion := common.SobeyRuntimeApiVersion
	image := config.Image.Image
	containerName := util.MakeContainerName(sandboxConfig, config)

	containerID := util.RandomString()
	logFilePath := fmt.Sprintf("%s%s_%v.log", common.ServerLogDirPath,
		containerName, time.Now().UnixNano())
	_, err = ss.os.Create(logFilePath)
	if err != nil {
		return nil, err
	}
	containerInfo := SobeyContainer{
		ID:               containerID,
		Name:             containerName,
		ServerName:       config.Metadata.Name,
		Image:            image,
		PortMapping:      sandboxConfig.PortMappings,
		PodSandboxConfig: sandboxConfig,
		State:            runtimeapi.ContainerState_CONTAINER_CREATED,
		Uid:              sandboxConfig.Metadata.Uid,
		ApiVersion:       apiVersion,
		Labels:           labels,
		Path:             logFilePath,
		CreateAt:         time.Now().UnixNano(),
	}
	bytes, err := json.Marshal(containerInfo)
	if err != nil {
		return nil, err
	}
	err = ss.dbService.PutWithPrefix("container", containerID, string(bytes))
	if err != nil {
		return nil, err
	}
	return &runtimeapi.CreateContainerResponse{ContainerId: containerID}, nil
}
func (ss *sobeyService) StartContainer(ctx context.Context, req *runtimeapi.StartContainerRequest) (*runtimeapi.StartContainerResponse, error) {
	res, err := ss.dbService.Get(fmt.Sprintf("container_%s", req.ContainerId))
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("Container is not exist, please create the container first ")
	}
	containerInfo := SobeyContainer{}
	err = json.Unmarshal([]byte(res), &containerInfo)
	if err != nil {
		return nil, err
	}

	// create the log file
	logFilePath := containerInfo.Labels[common.ContainerLogPathLabelKey]
	_, err = ss.os.Create(logFilePath)
	if err != nil {
		return nil, err
	}
	// send http request to start a server
	startRes, err := startServer(containerInfo, ss.runServerApiUrl)
	if err != nil {
		return nil, err
	}
	containerInfo.Pid = startRes.Pid
	containerInfo.StartedAt = startRes.UpTime * 1000
	containerInfo.FinishedAt = time.Now().UnixNano()
	containerInfo.State = runtimeapi.ContainerState_CONTAINER_RUNNING
	bytes, err := json.Marshal(containerInfo)
	if err != nil {
		return nil, err
	}
	err = ss.dbService.PutWithPrefix("container", containerInfo.ID, string(bytes))
	if err != nil {
		return nil, err
	}
	realPath := containerInfo.Path
	path := containerInfo.Labels[common.ContainerLogPathLabelKey]
	if len(realPath) != 0 {
		// Delete possibly existing file first
		if err = ss.os.Remove(path); err == nil {
			klog.InfoS("Deleted previously existing symlink file", "path", path)
		}
		if err = ss.os.Symlink(realPath, path); err != nil {
			return nil, fmt.Errorf("failed to create symbolic link %q to the container log file %q for container %q: %v",
				path, realPath, containerInfo.ID, err)
		}
	}
	return &runtimeapi.StartContainerResponse{}, nil
}

func startServer(info SobeyContainer, url string) (*ContainerStartResponse, error) {

	containerPort := "22"
	req := ContainerStartRequest{
		Name:   info.ServerName,
		Image:  info.Image,
		Port:   containerPort,
		LogDir: info.Path,
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resStr, err := util.HttpPost(url, string(reqBytes))
	if err != nil {
		return nil, err
	}
	res := new(ContainerStartResponse)
	err = json.Unmarshal([]byte(resStr), &res)
	if err != nil {
		return nil, err
	}
	return res, err
}

func (ss *sobeyService) StopContainer(ctx context.Context, req *runtimeapi.StopContainerRequest) (*runtimeapi.StopContainerResponse, error) {
	var containerID string
	if strings.Contains(req.ContainerId, "container") {
		containerID = req.ContainerId
	} else {
		containerID = fmt.Sprintf("container_%s", req.ContainerId)
	}
	res, err := ss.dbService.Get(containerID)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("Container is not existed, containerId : %s ", containerID)
	}
	containerInfo := SobeyContainer{}
	err = json.Unmarshal([]byte(res), &containerInfo)
	if err != nil {
		return nil, err
	}
	err = stopServer(containerInfo, ss.stopServerApiUrl)
	if err != nil {
		return nil, err
	}
	containerInfo.State = runtimeapi.ContainerState_CONTAINER_EXITED
	bytes, err := json.Marshal(containerInfo)
	if err != nil {
		return nil, err
	}
	err = ss.dbService.PutWithPrefix("container", req.ContainerId, string(bytes))
	if err != nil {
		return nil, err
	}
	return &runtimeapi.StopContainerResponse{}, nil
}

func stopServer(info SobeyContainer, url string) error {
	req := ContainerStopRequest{
		Name: info.ServerName,
		Pid:  info.Pid,
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = util.HttpPost(url, string(reqBytes))
	return err
}

func (ss *sobeyService) RemoveContainer(ctx context.Context, req *runtimeapi.RemoveContainerRequest) (*runtimeapi.RemoveContainerResponse, error) {
	containerID := fmt.Sprintf("container_%s", req.ContainerId)
	res, err := ss.dbService.Get(containerID)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, fmt.Errorf(fmt.Sprintf("Container is not existed, containerID : %s ", containerID))
	}
	containerInfo := SobeyContainer{}
	err = json.Unmarshal([]byte(res), &containerInfo)
	if err != nil {
		return nil, err
	}
	if containerInfo.State != runtimeapi.ContainerState_CONTAINER_EXITED {
		return nil, fmt.Errorf(fmt.Sprintf("Container is not stoped, please stop the container before remove, containerID : %s ", containerID))
	}
	err = ss.os.Remove(containerInfo.Path)
	if err != nil {
		fmt.Printf("remove path file err, path: %s, err: %v", containerInfo.Path, err)
	}
	err = ss.dbService.Delete(containerID)
	if err != nil {
		return nil, err
	}
	return &runtimeapi.RemoveContainerResponse{}, nil
}

func (ss *sobeyService) ContainerStatus(ctx context.Context, req *runtimeapi.ContainerStatusRequest) (*runtimeapi.ContainerStatusResponse, error) {
	res, err := ss.dbService.Get(fmt.Sprintf("container_%s", req.ContainerId))
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("Container is not exists ")
	}
	containerInfo := SobeyContainer{}
	err = json.Unmarshal([]byte(res), &containerInfo)
	if err != nil {
		return nil, err
	}
	// Parse the timestamps.
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp for container %q: %v", containerInfo.ID, err)
	}
	imageID := util.ToPullableImageID(containerInfo.Image, true)

	metadata, err := util.ParseContainerName(containerInfo.Name)
	if err != nil {
		return nil, err
	}

	labels, annotations := util.ExtractLabels(containerInfo.Labels)

	imageName := containerInfo.Image

	mounts := make([]*runtimeapi.Mount, 0, 1)
	mounts = append(mounts, &runtimeapi.Mount{
		ContainerPath: fmt.Sprintf("/tmp/path/%s", imageName),
		HostPath:      "sobey",
		Readonly:      false,
	})
	containerStatus := &runtimeapi.ContainerStatus{
		Id:          containerInfo.ID,
		Metadata:    metadata,
		Image:       &runtimeapi.ImageSpec{Image: imageName},
		ImageRef:    imageID,
		Mounts:      mounts,
		ExitCode:    0,
		State:       containerInfo.State,
		CreatedAt:   containerInfo.CreateAt,
		StartedAt:   containerInfo.StartedAt,
		FinishedAt:  containerInfo.FinishedAt,
		Reason:      "",
		Message:     "",
		Labels:      labels,
		Annotations: annotations,
		LogPath:     containerInfo.Labels[common.ContainerLogPathLabelKey],
	}
	return &runtimeapi.ContainerStatusResponse{Status: containerStatus}, nil
}
func (ss *sobeyService) UpdateContainerResources(ctx context.Context, req *runtimeapi.UpdateContainerResourcesRequest) (*runtimeapi.UpdateContainerResourcesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateContainerResources not implemented")
}

func (ss *sobeyService) ContainerStats(ctx context.Context, req *runtimeapi.ContainerStatsRequest) (*runtimeapi.ContainerStatsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ContainerStats not implemented")
}
