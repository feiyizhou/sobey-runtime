package src

import (
	"context"
	"encoding/json"
	"fmt"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"path/filepath"
	"sobey-runtime/common"
	util "sobey-runtime/utils"
	"strings"
	"time"
)

type SobeySandbox struct {
	Config     *runtimeapi.PodSandboxConfig
	ID         string                     `json:"id"`
	IP         string                     `json:"ip"`
	State      runtimeapi.PodSandboxState `json:"state"`
	Hostname   string                     `json:"hostname"`
	CreateTime int64                      `json:"createTime"`
}

// Returns whether the sandbox network is ready, and whether the sandbox is known
func (ss *sobeyService) getNetworkReady(podSandboxID string) (bool, bool) {
	ss.networkReadyLock.Lock()
	defer ss.networkReadyLock.Unlock()
	ready, ok := ss.networkReady[podSandboxID]
	return ready, ok
}

func (ss *sobeyService) setNetworkReady(podSandboxID string, ready bool) {
	ss.networkReadyLock.Lock()
	defer ss.networkReadyLock.Unlock()
	ss.networkReady[podSandboxID] = ready
}

func (ss *sobeyService) clearNetworkReady(podSandboxID string) {
	ss.networkReadyLock.Lock()
	defer ss.networkReadyLock.Unlock()
	delete(ss.networkReady, podSandboxID)
}

func (ss *sobeyService) RunPodSandbox(ctx context.Context, req *runtimeapi.RunPodSandboxRequest) (*runtimeapi.RunPodSandboxResponse, error) {
	config := req.GetConfig()
	sandboxID := util.RandomString()
	sandboxInfo := SobeySandbox{}
	sandboxInfo.Config = config
	sandboxInfo.CreateTime = time.Now().UnixNano()
	sandboxInfo.ID = sandboxID
	sandboxInfo.State = runtimeapi.PodSandboxState_SANDBOX_READY
	sandboxInfo.Hostname, _ = ss.os.Hostname()
	ip, err := ss.NewSandboxIP()
	if err != nil {
		return nil, err
	}
	sandboxInfo.IP = ip
	configBytes, err := json.Marshal(sandboxInfo)
	if err != nil {
		return nil, err
	}
	err = ss.os.MkdirAll(filepath.Dir(config.LogDirectory), 0750)
	if err != nil {
		fmt.Printf("Create pod log directory err, err: %v", err)
	}
	err = ss.dbService.PutWithPrefix(common.SandboxIDPrefix, sandboxID, string(configBytes))
	if err != nil {
		return nil, err
	}
	ss.setNetworkReady(sandboxID, true)
	resp := &runtimeapi.RunPodSandboxResponse{PodSandboxId: sandboxID}
	return resp, nil
}

func (ss *sobeyService) StopPodSandbox(ctx context.Context, req *runtimeapi.StopPodSandboxRequest) (*runtimeapi.StopPodSandboxResponse, error) {
	// 1.Get the sandbox info from etcd
	sandboxInfoStr, err := ss.dbService.Get(util.BuildSandboxID(req.PodSandboxId))
	if err != nil {
		return nil, err
	}
	sandboxInfo := new(SobeySandbox)
	err = json.Unmarshal([]byte(sandboxInfoStr), &sandboxInfo)
	if err != nil {
		return nil, err
	}

	// 2.Set network to notReady and release the IP of sandbox
	ready, ok := ss.getNetworkReady(sandboxInfo.ID)
	if ready || !ok {
		ss.setNetworkReady(sandboxInfo.ID, false)
	}
	err = ss.PutReleasedIP(sandboxInfo.IP)
	if err != nil {
		return nil, err
	}

	// 3.Update the state of sandbox to notReady
	sandboxInfo.State = runtimeapi.PodSandboxState_SANDBOX_NOTREADY
	sandboxBytes, err := json.Marshal(sandboxInfo)
	if err != nil {
		return nil, err
	}
	err = ss.dbService.PutWithPrefix(common.SandboxIDPrefix, sandboxInfo.ID, string(sandboxBytes))
	if err != nil {
		return nil, err
	}
	return &runtimeapi.StopPodSandboxResponse{}, nil
}
func (ss *sobeyService) RemovePodSandbox(ctx context.Context, req *runtimeapi.RemovePodSandboxRequest) (*runtimeapi.RemovePodSandboxResponse, error) {
	// 1.Remove all container in the sandbox
	containerReq := &runtimeapi.ListContainersRequest{
		Filter: &runtimeapi.ContainerFilter{
			PodSandboxId: req.PodSandboxId,
		},
	}
	containers, err := ss.ListContainers(ctx, containerReq)
	if err != nil {
		return nil, err
	}
	if containers != nil {
		for _, container := range containers.Containers {
			_, err = ss.RemoveContainer(ctx, &runtimeapi.RemoveContainerRequest{
				ContainerId: util.RemoveContainerIDPrefix(container.Id),
			})
			if err != nil {
				return nil, err
			}
		}
	}

	// 2.Remove the sandbox
	sandboxID := util.BuildSandboxID(req.PodSandboxId)
	err = ss.dbService.Delete(sandboxID)
	if err != nil {
		return nil, err
	}

	return &runtimeapi.RemovePodSandboxResponse{}, nil
}
func (ss *sobeyService) PodSandboxStatus(ctx context.Context, req *runtimeapi.PodSandboxStatusRequest) (*runtimeapi.PodSandboxStatusResponse, error) {
	// 1. Get sandbox info by sandbox ID from etcd
	sandboxInfoStr, err := ss.dbService.Get(util.BuildSandboxID(req.PodSandboxId))
	if err != nil {
		return nil, err
	}
	if len(sandboxInfoStr) == 0 {
		return nil, fmt.Errorf("Sandbox is not exist, sandboxID: %s ", req.PodSandboxId)
	}
	sandbox := new(SobeySandbox)
	err = json.Unmarshal([]byte(sandboxInfoStr), &sandbox)
	if err != nil {
		return nil, err
	}
	if sandbox == nil {
		return nil, err
	}

	// 2. Get sandbox IP with ID from plugin
	//ips := ss.getIPs(sandbox)
	ips := []string{sandbox.IP}
	ip := ""
	if len(ips) != 0 {
		ip = ips[0]
		ips = ips[1:]
	}

	sandboxStatus := &runtimeapi.PodSandboxStatus{
		Id:        sandbox.ID,
		State:     sandbox.State,
		CreatedAt: sandbox.CreateTime,
		Metadata: &runtimeapi.PodSandboxMetadata{
			Name:      sandbox.Config.Metadata.Name,
			Uid:       sandbox.Config.Metadata.Uid,
			Namespace: sandbox.Config.Metadata.Namespace,
			Attempt:   sandbox.Config.Metadata.Attempt,
		},
		Labels:      sandbox.Config.Labels,
		Annotations: sandbox.Config.Annotations,
		Network: &runtimeapi.PodSandboxNetworkStatus{
			Ip: ip,
		},
		Linux: &runtimeapi.LinuxPodSandboxStatus{
			Namespaces: &runtimeapi.Namespace{
				Options: &runtimeapi.NamespaceOption{
					Network: networkNamespaceMode(sandbox),
					Pid:     pidNamespaceMode(sandbox),
					Ipc:     ipcNamespaceMode(sandbox),
				},
			},
		},
	}
	// add additional IPs
	additionalPodIPs := make([]*runtimeapi.PodIP, 0, len(ips))
	for _, ipItem := range ips {
		additionalPodIPs = append(additionalPodIPs, &runtimeapi.PodIP{
			Ip: ipItem,
		})
	}
	sandboxStatus.Network.AdditionalIps = additionalPodIPs
	return &runtimeapi.PodSandboxStatusResponse{Status: sandboxStatus}, nil
}
func (ss *sobeyService) ListPodSandbox(ctx context.Context, req *runtimeapi.ListPodSandboxRequest) (*runtimeapi.ListPodSandboxResponse, error) {
	filter := req.GetFilter()
	results, err := ss.dbService.GetByPrefix(common.SandboxIDPrefix)
	if err != nil {
		return &runtimeapi.ListPodSandboxResponse{}, err
	}
	if len(results) == 0 {
		return &runtimeapi.ListPodSandboxResponse{}, err
	}
	var items []*runtimeapi.PodSandbox
	hostName, _ := ss.os.Hostname()
	for _, result := range results {
		config := new(SobeySandbox)
		err = json.Unmarshal([]byte(result), &config)
		if err != nil {
			return &runtimeapi.ListPodSandboxResponse{}, err
		}
		if strings.EqualFold(hostName, config.Hostname) {
			items = append(items, &runtimeapi.PodSandbox{
				Id:          config.ID,
				Metadata:    config.Config.Metadata,
				State:       config.State,
				CreatedAt:   config.CreateTime,
				Labels:      config.Config.Labels,
				Annotations: config.Config.Annotations,
			})
		}
	}
	if len(items) == 0 {
		return &runtimeapi.ListPodSandboxResponse{Items: items}, nil
	}
	return &runtimeapi.ListPodSandboxResponse{Items: filterPodSandbox(filter, items)}, nil
}

func filterPodSandbox(filter *runtimeapi.PodSandboxFilter, podSandboxes []*runtimeapi.PodSandbox) []*runtimeapi.PodSandbox {
	if filter == nil {
		return podSandboxes
	}
	var idFilterItems []*runtimeapi.PodSandbox
	if len(filter.Id) != 0 {
		for _, item := range podSandboxes {
			if strings.EqualFold(filter.Id, item.Id) {
				idFilterItems = append(idFilterItems, item)
			}
		}
	} else {
		idFilterItems = podSandboxes
	}
	var stateFilterItems []*runtimeapi.PodSandbox
	if filter.State != nil {
		for _, item := range idFilterItems {
			if item.State == filter.State.State {
				stateFilterItems = append(stateFilterItems, item)
			}
		}
	} else {
		stateFilterItems = idFilterItems
	}
	var uidFilterItems []*runtimeapi.PodSandbox
	if len(filter.LabelSelector[common.KubernetesPodUIDLabel]) != 0 {
		for _, item := range stateFilterItems {
			if strings.EqualFold(filter.LabelSelector[common.KubernetesPodUIDLabel], item.Labels[common.KubernetesPodUIDLabel]) {
				uidFilterItems = append(uidFilterItems, item)
			}
		}
	} else {
		uidFilterItems = stateFilterItems
	}
	return uidFilterItems
}

// networkNamespaceMode returns the network runtimeapi.NamespaceMode for this container.
// Supports: POD, NODE
func networkNamespaceMode(container *SobeySandbox) runtimeapi.NamespaceMode {
	if container != nil && container.Config != nil && container.Config.Linux != nil &&
		container.Config.Linux.SecurityContext != nil &&
		container.Config.Linux.SecurityContext.NamespaceOptions != nil &&
		string(container.Config.Linux.SecurityContext.NamespaceOptions.Network) == namespaceModeHost {
		return runtimeapi.NamespaceMode_NODE
	}
	return runtimeapi.NamespaceMode_POD
}

func pidNamespaceMode(container *SobeySandbox) runtimeapi.NamespaceMode {
	if container != nil && container.Config != nil && container.Config.Linux != nil &&
		container.Config.Linux.SecurityContext != nil &&
		container.Config.Linux.SecurityContext.NamespaceOptions != nil &&
		string(container.Config.Linux.SecurityContext.NamespaceOptions.Pid) == namespaceModeHost {
		return runtimeapi.NamespaceMode_NODE
	}
	return runtimeapi.NamespaceMode_CONTAINER
}

func ipcNamespaceMode(container *SobeySandbox) runtimeapi.NamespaceMode {
	if container != nil && container.Config != nil && container.Config.Linux != nil &&
		container.Config.Linux.SecurityContext != nil &&
		container.Config.Linux.SecurityContext.NamespaceOptions != nil &&
		string(container.Config.Linux.SecurityContext.NamespaceOptions.Ipc) == namespaceModeHost {
		return runtimeapi.NamespaceMode_NODE
	}
	return runtimeapi.NamespaceMode_POD
}
