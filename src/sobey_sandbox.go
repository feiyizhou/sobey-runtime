package src

import (
	"context"
	"encoding/json"
	"fmt"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"path/filepath"
	util "sobey-runtime/utils"
	"strings"
	"time"
)

type SobeySandbox struct {
	Config     *runtimeapi.PodSandboxConfig
	ID         string                     `json:"id"`
	IP         string                     `json:"ip"`
	State      runtimeapi.PodSandboxState `json:"state"`
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
	sobeyConf := SobeySandbox{}
	sobeyConf.Config = config
	sobeyConf.CreateTime = time.Now().UnixNano()
	sobeyConf.ID = sandboxID
	sobeyConf.State = runtimeapi.PodSandboxState_SANDBOX_READY
	ip, err := ss.NewSandboxIP()
	if err != nil {
		return nil, err
	}
	sobeyConf.IP = ip
	configBytes, err := json.Marshal(sobeyConf)
	if err != nil {
		return nil, err
	}
	err = ss.os.MkdirAll(filepath.Dir(config.LogDirectory), 0750)
	if err != nil {
		fmt.Printf("Create pod log directory err, err: %v", err)
	}
	err = ss.dbService.PutWithPrefix("sandbox", sandboxID, string(configBytes))
	if err != nil {
		return nil, err
	}
	ss.setNetworkReady(sandboxID, true)
	resp := &runtimeapi.RunPodSandboxResponse{PodSandboxId: sandboxID}
	return resp, nil
}
func (ss *sobeyService) StopPodSandbox(ctx context.Context, req *runtimeapi.StopPodSandboxRequest) (*runtimeapi.StopPodSandboxResponse, error) {
	sandboxID := fmt.Sprintf("sandbox_%s", req.PodSandboxId)

	// 1.Get the sandbox info from etcd
	sandboxInfoStr, err := ss.dbService.Get(sandboxID)
	if err != nil {
		return nil, err
	}
	sandboxInfo := new(SobeySandbox)
	err = json.Unmarshal([]byte(sandboxInfoStr), &sandboxInfo)
	if err != nil {
		return nil, err
	}

	// 2.Set network to notReady and release the IP of sandbox
	ready, ok := ss.getNetworkReady(sandboxID)
	if ready || !ok {
		ss.setNetworkReady(sandboxID, false)
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

	err = ss.dbService.PutWithPrefix("sandbox", req.PodSandboxId, string(sandboxBytes))
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
				ContainerId: container.Id,
			})
			if err != nil {
				return nil, err
			}
		}
	}

	sandboxID := fmt.Sprintf("sandbox_%s", req.PodSandboxId)

	// 2.Remove the sandbox
	err = ss.dbService.Delete(sandboxID)
	if err != nil {
		return nil, err
	}

	return &runtimeapi.RemovePodSandboxResponse{}, nil
}
func (ss *sobeyService) PodSandboxStatus(ctx context.Context, req *runtimeapi.PodSandboxStatusRequest) (*runtimeapi.PodSandboxStatusResponse, error) {
	// 1. Get sandbox info by sandbox ID from etcd
	sandboxInfoStr, err := ss.dbService.Get(fmt.Sprintf("sandbox_%s", req.PodSandboxId))
	if err != nil {
		return nil, err
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
		Id:        req.PodSandboxId,
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
	for _, ip := range ips {
		additionalPodIPs = append(additionalPodIPs, &runtimeapi.PodIP{
			Ip: ip,
		})
	}
	sandboxStatus.Network.AdditionalIps = additionalPodIPs
	return &runtimeapi.PodSandboxStatusResponse{Status: sandboxStatus}, nil
}
func (ss *sobeyService) ListPodSandbox(ctx context.Context, req *runtimeapi.ListPodSandboxRequest) (*runtimeapi.ListPodSandboxResponse, error) {
	filter := req.GetFilter()
	results, err := ss.dbService.GetByPrefix("sandbox")
	if err != nil {
		return &runtimeapi.ListPodSandboxResponse{}, err
	}
	if len(results) == 0 {
		return &runtimeapi.ListPodSandboxResponse{}, err
	}
	var items []*runtimeapi.PodSandbox
	for _, result := range results {
		config := new(SobeySandbox)
		err = json.Unmarshal([]byte(result), &config)
		if err != nil {
			return &runtimeapi.ListPodSandboxResponse{}, err
		}
		items = append(items, &runtimeapi.PodSandbox{
			Id:          config.ID,
			Metadata:    config.Config.Metadata,
			State:       config.State,
			CreatedAt:   config.CreateTime,
			Labels:      config.Config.Labels,
			Annotations: config.Config.Annotations,
		})
	}
	if len(items) == 0 {
		return &runtimeapi.ListPodSandboxResponse{Items: items}, nil
	}
	var result []*runtimeapi.PodSandbox
	if filter != nil {
		for _, item := range items {
			if filter.LabelSelector != nil {
				if len(filter.LabelSelector["io.kubernetes.pod.uid"]) != 0 {
					if strings.EqualFold(filter.LabelSelector["io.kubernetes.pod.uid"], item.Metadata.Uid) {
						result = append(result, item)
					}
				}
			}
		}
	} else {
		result = items
	}

	return &runtimeapi.ListPodSandboxResponse{Items: result}, nil
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