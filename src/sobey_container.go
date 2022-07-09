package src

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mitchellh/go-ps"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sobey-runtime/common"
	"sobey-runtime/module"
	util "sobey-runtime/utils"
	"strconv"
	"strings"
	"time"
)

type SobeyContainer struct {
	ID               string                       `json:"id"`
	Name             string                       `json:"name"`
	Hostname         string                       `json:"hostname"`
	Image            string                       `json:"image"`
	Pid              string                       `json:"pid"`
	Path             string                       `json:"path"`
	PortMapping      []*runtimeapi.PortMapping    `json:"port"`
	PodSandboxConfig *runtimeapi.PodSandboxConfig `json:"podSandboxConfig"`
	ContainerConfig  *runtimeapi.ContainerConfig  `json:"containerConfig"`
	State            runtimeapi.ContainerState    `json:"state"`
	Uid              string                       `json:"uid"`
	ApiVersion       string                       `json:"apiVersion"`
	Labels           map[string]string            `json:"labels"`
	CreateAt         int64                        `json:"createAt"`
	StartedAt        int64                        `json:"startedAt"`
	FinishedAt       int64                        `json:"finishedAt"`
}

type ContainerStartResult struct {
	Name         string `json:"name"`
	Pid          string `json:"pid"`
	Port         int    `json:"port"`
	UpTime       int64  `json:"up_time"`
	FinishedTime int64  `json:"finished_time"`
}

func (ss *sobeyService) ListContainers(ctx context.Context, req *runtimeapi.ListContainersRequest) (*runtimeapi.ListContainersResponse, error) {
	var result []*runtimeapi.Container
	containerInfos, err := ss.dbService.GetByPrefix(common.ContainerIDPrefix)
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
			hostname, _ := ss.os.Hostname()
			if strings.EqualFold(hostname, sobeyContainer.Hostname) {
				sobeyContainers = append(sobeyContainers, sobeyContainer)
			}
		}
		if len(sobeyContainers) == 0 {
			return &runtimeapi.ListContainersResponse{Containers: result}, nil
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
				State:        containerInfo.State,
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
			if strings.EqualFold(util.RemoveSandboxIDPrefix(filter.PodSandboxId),
				util.RemoveSandboxIDPrefix(containerInfo.Labels[common.SandboxIDLabelKey])) {
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
	config := req.GetConfig()
	if config == nil {
		return nil, fmt.Errorf("container config is nil")
	}

	sandboxConfig := req.GetSandboxConfig()
	if sandboxConfig == nil {
		return nil, fmt.Errorf("sandbox config is nil for container %q", config.Metadata.Name)
	}

	labels := util.MakeLabels(config.GetLabels(), config.GetAnnotations())
	labels[common.ContainerTypeLabelKey] = common.ContainerTypeLabelContainer
	labels[common.ContainerLogPathLabelKey] = filepath.Join(sandboxConfig.LogDirectory, config.LogPath)
	labels[common.SandboxIDLabelKey] = util.RemoveSandboxIDPrefix(req.PodSandboxId)

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
	if strings.HasSuffix(image, ":latest") {
		imageArr := strings.Split(image, ":")
		image = strings.Join(imageArr[:len(imageArr)-1], ":")
	}
	containerName := util.MakeContainerName(sandboxConfig, config)

	containerID := util.RandomString()
	logFilePath := fmt.Sprintf("%s%s_%v.log", common.ServerLogDirPath,
		containerName, time.Now().UnixNano())
	_, err = ss.os.Create(logFilePath)
	if err != nil {
		return nil, err
	}
	hostname, _ := ss.os.Hostname()
	containerInfo := SobeyContainer{
		ID:               containerID,
		Name:             containerName,
		Hostname:         hostname,
		Image:            image,
		PortMapping:      sandboxConfig.PortMappings,
		PodSandboxConfig: sandboxConfig,
		ContainerConfig:  config,
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
	res, err := ss.dbService.Get(util.BuildContainerID(req.ContainerId))
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

	if containerInfo.State == runtimeapi.ContainerState_CONTAINER_RUNNING {
		return &runtimeapi.StartContainerResponse{}, nil
	}
	// Start a server
	startRes, err := ss.startServer(containerInfo)
	if err != nil {
		return nil, err
	}
	containerInfo.Pid = startRes.Pid
	containerInfo.StartedAt = startRes.UpTime
	containerInfo.FinishedAt = startRes.UpTime + 1000
	containerInfo.State = runtimeapi.ContainerState_CONTAINER_RUNNING
	bytes, err := json.Marshal(containerInfo)
	if err != nil {
		return nil, err
	}
	err = ss.dbService.PutWithPrefix(common.ContainerIDPrefix, containerInfo.ID, string(bytes))
	if err != nil {
		return nil, err
	}
	realPath := containerInfo.Path
	path := containerInfo.Labels[common.ContainerLogPathLabelKey]
	if realPath != "" {
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

func (ss *sobeyService) startServer(info SobeyContainer) (*ContainerStartResult, error) {
	// 1.Get the sandbox info from etcd
	sandboxInfoStr, err := ss.dbService.Get(util.BuildSandboxID(info.Labels[common.SandboxIDLabelKey]))
	if err != nil {
		return nil, err
	}
	sandboxInfo := new(SobeySandbox)
	err = json.Unmarshal([]byte(sandboxInfoStr), &sandboxInfo)
	if err != nil {
		return nil, err
	}
	criParam := make(map[string]string)
	err = json.Unmarshal([]byte(info.PodSandboxConfig.Annotations["sobey.com/cri-param"]), &criParam)
	if err != nil {
		return nil, err
	}
	var ppid string
	switch criParam["appType"] {
	case "jar":
		err = writeConfFile(info, criParam["imageName"], criParam["imageTag"],
			sandboxInfo.Pid)
		if err != nil {
			return nil, err
		}
		sockerArgs := []string{
			"run",
			info.ID,
		}
		command := exec.Command("socker", sockerArgs...)
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		err = command.Start()
		if err != nil {
			return nil, err
		}
		ppid, err = getPPid(info.ID, ss.polling)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid application type : %s ", criParam["appType"])
	}
	return &ContainerStartResult{
		Name:   info.Name,
		Pid:    ppid,
		Port:   0,
		UpTime: time.Now().UnixNano(),
	}, err
}

func getPPid(id string, polling []int) (string, error) {
	pidFileName := fmt.Sprintf(common.SockerContainerPidHome, id)
	for _, space := range polling {
		time.Sleep(time.Duration(space) * time.Second)
		_, err := os.Stat(pidFileName)
		if err != nil {
			if err == os.ErrNotExist {
				continue
			}
		} else {
			bytes, err := ioutil.ReadFile(pidFileName)
			if err != nil {
				return "", err
			}
			return string(bytes), err
		}
	}
	return "", fmt.Errorf("Get container ppid over time ")
}

func writeConfFile(info SobeyContainer, imageName, imageTag,
	sandboxPid string) error {
	conf := new(module.ContainerConf)
	conf.ID = info.ID
	conf.SandboxPid = sandboxPid
	conf.Mem = info.ContainerConfig.Linux.Resources.MemoryLimitInBytes
	conf.Swap = info.ContainerConfig.Linux.Resources.MemorySwapLimitInBytes
	conf.PIDs = 100
	conf.CPUs = 20
	conf.Image = module.Image{
		Name: imageName,
		Tag:  imageTag,
	}
	conf.Args = info.ContainerConfig.Args
	var envArr []module.KeyValue
	for _, envInfo := range info.ContainerConfig.Envs {
		envArr = append(envArr, module.KeyValue{
			Key:   envInfo.Key,
			Value: envInfo.Value,
		})
	}
	conf.Env = envArr
	var mountArr []module.Mount
	for _, mountInfo := range info.ContainerConfig.Mounts {
		mountArr = append(mountArr, module.Mount{
			ContainerPath: mountInfo.ContainerPath,
			HostPath:      mountInfo.HostPath,
		})
	}
	conf.Mount = mountArr
	confPath := fmt.Sprintf(common.SockerContainerConfHome, info.ID)
	err := util.CreateDirsIfDontExist([]string{confPath})
	if err != nil {
		return err
	}
	confFilePath := fmt.Sprintf("%s/config.json", confPath)
	bytes, err := json.Marshal(conf)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(confFilePath, bytes, 0777)
}

func (ss *sobeyService) StopContainer(ctx context.Context, req *runtimeapi.StopContainerRequest) (*runtimeapi.StopContainerResponse, error) {
	res, err := ss.dbService.Get(util.BuildContainerID(req.ContainerId))
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		klog.InfoS("When stop container, container is not existed", "containerID", req.ContainerId)
		return &runtimeapi.StopContainerResponse{}, nil
	}
	containerInfo := SobeyContainer{}
	err = json.Unmarshal([]byte(res), &containerInfo)
	if err != nil {
		return nil, err
	}
	err = stopServer(containerInfo.Pid)
	if err != nil {
		return nil, err
	}
	containerInfo.State = runtimeapi.ContainerState_CONTAINER_EXITED
	bytes, err := json.Marshal(containerInfo)
	if err != nil {
		return nil, err
	}
	err = ss.dbService.PutWithPrefix(common.ContainerIDPrefix, containerInfo.ID, string(bytes))
	if err != nil {
		return nil, err
	}
	return &runtimeapi.StopContainerResponse{}, nil
}

func stopServer(pidStr string) error {
	list, err := ps.Processes()
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	err = process.Kill()
	if err != nil {
		return err
	}
	_, _ = process.Wait()
	for _, p := range list {
		if p.Pid() == pid {
			parentProcPID := p.PPid()
			ppProcess, err := os.FindProcess(parentProcPID)
			if err != nil {
				return err
			}
			err = ppProcess.Kill()
			if err != nil {
				return err
			}
			_, _ = ppProcess.Wait()
		}
	}
	return err
}

func (ss *sobeyService) RemoveContainer(ctx context.Context, req *runtimeapi.RemoveContainerRequest) (*runtimeapi.RemoveContainerResponse, error) {
	res, err := ss.dbService.Get(util.BuildContainerID(req.ContainerId))
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		klog.InfoS("When remove container, container is not existed", "containerID", req.ContainerId)
		return &runtimeapi.RemoveContainerResponse{}, nil
	}
	containerInfo := SobeyContainer{}
	err = json.Unmarshal([]byte(res), &containerInfo)
	if err != nil {
		return nil, err
	}
	if containerInfo.State != runtimeapi.ContainerState_CONTAINER_EXITED {
		return nil, fmt.Errorf(fmt.Sprintf("Container is not stoped, please stop the container before remove, containerID : %s ", req.ContainerId))
	}
	err = ss.os.Remove(containerInfo.Path)
	if err != nil {
		fmt.Printf("remove path file err, path: %s, err: %v", containerInfo.Path, err)
	}
	err = ss.dbService.Delete(util.BuildContainerID(req.ContainerId))
	if err != nil {
		return nil, err
	}
	removeFS(containerInfo)
	return &runtimeapi.RemoveContainerResponse{}, nil
}

func removeFS(info SobeyContainer) {
	mntPath := fmt.Sprintf(common.SockerContainerFSHome, info.ID) + "/mnt"
	for _, mountInfo := range info.ContainerConfig.Mounts {
		if !strings.Contains(mountInfo.ContainerPath, "sobey") {
			continue
		}
		target := fmt.Sprintf("%s%s", mntPath, mountInfo.ContainerPath)
		err := unix.Unmount(target, 0)
		if err != nil {
			fmt.Printf("Unmount config file err, err : %v", err)
		}
	}
	err := mountOverlayFS(info.ID)
	if err != nil {
		fmt.Printf("Unmount overlay file err, err : %v", err)
	}
	err = os.RemoveAll(fmt.Sprintf(common.SockerContainerHome, info.ID))
	if err != nil {
		fmt.Printf("remove all file err, err : %v", err)
	}
}

func mountOverlayFS(id string) error {
	mntPath := fmt.Sprintf(common.SockerContainerFSHome, id) + "/mnt"
	err := unix.Unmount(fmt.Sprintf("%s/dev/pts",
		mntPath), 0)
	if err != nil {
		return err
	}
	err = unix.Unmount(fmt.Sprintf("%s/dev", mntPath), 0)
	if err != nil {
		return err
	}
	err = unix.Unmount(fmt.Sprintf("%s/sys", mntPath), 0)
	if err != nil {
		return err
	}
	err = unix.Unmount(fmt.Sprintf("%s/proc", mntPath), 0)
	if err != nil {
		return err
	}
	err = unix.Unmount(fmt.Sprintf("%s/tmp", mntPath), 0)
	if err != nil {
		return err
	}
	err = unix.Unmount(mntPath, 0)
	if err != nil {
		return err
	}
	return err
}

func (ss *sobeyService) ContainerStatus(ctx context.Context, req *runtimeapi.ContainerStatusRequest) (*runtimeapi.ContainerStatusResponse, error) {
	res, err := ss.dbService.Get(util.BuildContainerID(req.ContainerId))
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
