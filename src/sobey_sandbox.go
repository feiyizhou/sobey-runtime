package src

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/dockershim"
	"os"
	"path/filepath"
	"sobey-runtime/common"
	util "sobey-runtime/utils"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type SobeySandbox struct {
	Config     *runtimeapi.PodSandboxConfig
	ID         string                     `json:"id"`
	Pid        string                     `json:"pid"`
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

func toCheckpointProtocol(protocol runtimeapi.Protocol) dockershim.Protocol {
	switch protocol {
	case runtimeapi.Protocol_TCP:
		return dockershim.ProtocolTCP
	case runtimeapi.Protocol_UDP:
		return dockershim.ProtocolUDP
	case runtimeapi.Protocol_SCTP:
		return dockershim.ProtocolSCTP
	}
	klog.InfoS("Unknown protocol, defaulting to TCP", "protocol", protocol)
	return dockershim.ProtocolTCP
}

func constructPodSandboxCheckpoint(config *runtimeapi.PodSandboxConfig) checkpointmanager.Checkpoint {
	data := dockershim.CheckpointData{}
	for _, pm := range config.GetPortMappings() {
		proto := toCheckpointProtocol(pm.Protocol)
		data.PortMappings = append(data.PortMappings, &dockershim.PortMapping{
			HostPort:      &pm.HostPort,
			ContainerPort: &pm.ContainerPort,
			Protocol:      &proto,
			HostIP:        pm.HostIp,
		})
	}
	if config.GetLinux().GetSecurityContext().GetNamespaceOptions().GetNetwork() == runtimeapi.NamespaceMode_NODE {
		data.HostNetwork = true
	}
	return dockershim.NewPodSandboxCheckpoint(config.Metadata.Namespace, config.Metadata.Name, &data)
}

func (ss *sobeyService) RunPodSandbox(ctx context.Context, req *runtimeapi.RunPodSandboxRequest) (*runtimeapi.RunPodSandboxResponse, error) {
	config := req.GetConfig()

	err := ValidateAnnotations(config.Annotations)
	if err != nil {
		return nil, err
	}

	sandboxID := util.RandomString()
	// 1. Create Sandbox Checkpoint.
	if err = ss.checkpointManager.CreateCheckpoint(sandboxID, constructPodSandboxCheckpoint(config)); err != nil {
		return nil, err
	}

	// 2. Start the sandbonx server
	pid, err := runSandboxServer()
	if err != nil {
		return nil, err
	}

	// 3. Setup the net config for sandbox
	ip, err := ss.setupNet(pid, config)
	if err != nil {
		return nil, err
	}

	// 4.Store the sandbox info with prefix
	sandboxInfo := SobeySandbox{}
	sandboxInfo.Pid = pid
	sandboxInfo.Config = config
	sandboxInfo.CreateTime = time.Now().UnixNano()
	sandboxInfo.ID = sandboxID
	sandboxInfo.State = runtimeapi.PodSandboxState_SANDBOX_READY
	sandboxInfo.Hostname, _ = ss.os.Hostname()
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

func ValidateAnnotations(annotations map[string]string) error {
	if annotations == nil || len(annotations["sobey.com/cri-param"]) == 0 {
		return fmt.Errorf("Please identify the cri param info ")
	}
	var appParams map[string]string
	err := json.Unmarshal([]byte(annotations["sobey.com/cri-param"]), &appParams)
	if err != nil {
		return fmt.Errorf("Parse application param error ")
	}
	if len(appParams["appType"]) == 0 {
		return fmt.Errorf("Please identify the application type in annotations ")
	}
	if len(appParams["imageName"]) == 0 {
		return fmt.Errorf("Please identify the application imageName in annotations ")
	}
	if len(appParams["imageTag"]) == 0 {
		return fmt.Errorf("Please identify the application imageTag in annotations ")
	}
	return nil
}

func (ss *sobeyService) setupNet(id string, config *runtimeapi.PodSandboxConfig) (string, error) {
	cID := kubecontainer.BuildContainerID(runtimeName, id)
	networkOptions := make(map[string]string)
	if dnsConfig := config.GetDnsConfig(); dnsConfig != nil {
		// Build DNS options.
		dnsOption, err := json.Marshal(dnsConfig)
		if err != nil {
			return "", fmt.Errorf("failed to marshal dns config for pod %q: %v",
				config.Metadata.Name, err)
		}
		networkOptions["dns"] = string(dnsOption)
	}
	err := ss.network.SetUpPod(config.GetMetadata().Namespace, config.GetMetadata().Name,
		cID, config.Annotations, networkOptions)
	if err != nil {
		errList := []error{fmt.Errorf("failed to set up sandbox container %q network "+
			"for pod %q: %v", id, config.Metadata.Name, err)}
		err = ss.network.TearDownPod(config.GetMetadata().Namespace, config.GetMetadata().Name, cID)
		if err != nil {
			errList = append(errList, fmt.Errorf("failed to clean up sandbox container "+
				"%q network for pod %q: %v", id, config.Metadata.Name, err))
		}
		// TODO if set up network failed, then stop the sandbox server
		return "", errList[0]
	}
	res := new(CNICache)
	path := fmt.Sprintf("/var/lib/cni/cache/results/cbr0-%s-eth0", id)
	buffer, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(buffer, &res)
	if err != nil {
		return "", err
	}
	address := res.Result.IPS[0].Address
	if strings.Contains(address, "/") {
		address = strings.Split(address, "/")[0]
	}
	return address, err
}

type CNICache struct {
	Kind        string        `json:"kind"`
	ContainerId string        `json:"containerId"`
	Config      string        `json:"config"`
	IfName      string        `json:"ifName"`
	NetworkName string        `json:"networkName"`
	Result      CNIExecResult `json:"result"`
}

type CNIExecResult struct {
	CNIVersion string                 `json:"cniVersion"`
	DNS        map[string]interface{} `json:"dns"`
	Interfaces []NetInterface         `json:"interfaces"`
	IPS        []IP                   `json:"ips"`
	Routes     []Route                `json:"routes"`
}

type NetInterface struct {
	Mac  string `json:"mac"`
	Name string `json:"name"`
}

type IP struct {
	Address   string `json:"address"`
	Gateway   string `json:"gateway"`
	Interface int    `json:"interface"`
	Version   string `json:"version"`
}

type Route struct {
	DST string `json:"dst"`
	GW  string `json:"gw"`
}

func runSandboxServer() (string, error) {
	args := []string{
		"-c",
		"pause",
	}

	return util.Exec("/bin/sh", args, nil, &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWUSER,
	}, "", "", "")

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
		cID := kubecontainer.BuildContainerID(runtimeName, sandboxInfo.ID)
		err = ss.network.TearDownPod(sandboxInfo.Config.Metadata.Namespace,
			sandboxInfo.Config.Metadata.Name, cID)
		if err == nil {
			ss.setNetworkReady(sandboxInfo.ID, false)
		} else {
			return nil, err
		}
	}
	err = ss.PutReleasedIP(sandboxInfo.IP)
	if err != nil {
		return nil, err
	}

	// 3.Stop the process
	pid, err := strconv.Atoi(sandboxInfo.Pid)
	if err != nil {
		return nil, err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	err = process.Kill()
	if err != nil {
		return nil, err
	}
	_, _ = process.Wait()

	// 4.Update the state of sandbox to notReady
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
