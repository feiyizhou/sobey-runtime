package src

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	"k8s.io/kubernetes/pkg/kubelet/dockershim"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/cni"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/kubenet"
	"net/http"
	"path/filepath"
	"sobey-runtime/config"
	"sobey-runtime/etcd"
	util "sobey-runtime/utils"
	"strings"
	"sync"
)

const (
	sobeyNetNSFmt     = "/proc/%s/ns/net"
	namespaceModeHost = "host"
	sobeyshimRootDir  = "/var/lib/sobeyshim"
)

type CRIService interface {
	runtimeapi.RuntimeServiceServer
	runtimeapi.ImageServiceServer
	SobeyCniInterface
	Start() error
}

type SobeyService interface {
	CRIService
	http.Handler
}

type namespaceGetter struct {
	ss *sobeyService
}

// portMappingGetter is a wrapper around the dockerService that implements
// the network.PortMappingGetter interface.
type portMappingGetter struct {
	ss *sobeyService
}

type dockerNetworkHost struct {
	*namespaceGetter
	*portMappingGetter
}

func (d namespaceGetter) GetNetNS(pid string) (string, error) {
	return d.ss.GetNetNS(pid)
}

func (d namespaceGetter) GetPodPortMappings(containerID string) ([]*hostport.PortMapping, error) {
	return d.ss.GetPodPortMappings(containerID)
}

type sobeyService struct {
	// cri
	os util.OSInterface

	network          *network.PluginManager
	networkReady     map[string]bool
	networkReadyLock sync.Mutex

	checkpointManager checkpointmanager.CheckpointManager

	// etcd
	dbService *etcd.DBService

	// ipRange
	ipRange string

	// repo
	repo string

	// server
	host             string
	runServerApiUrl  string
	stopServerApiUrl string
	healthyApiUrl    string
	listServerApiUrl string
}

func NewSobeyService(serverConf *config.Server, pluginSettings *dockershim.NetworkPluginSettings) (SobeyService, error) {
	checkpointManager, err := checkpointmanager.NewCheckpointManager(filepath.Join(sobeyshimRootDir, "sandbox"))
	if err != nil {
		return nil, err
	}
	hostTmpArr := strings.Split(serverConf.Host, ":")
	ss := &sobeyService{
		os:           util.RealOS{},
		networkReady: make(map[string]bool),

		dbService: etcd.NewDBService(),

		ipRange: serverConf.IpRange,

		repo: serverConf.Repo,

		checkpointManager: checkpointManager,

		host:             strings.Join(hostTmpArr[:len(hostTmpArr)-1], ":"),
		runServerApiUrl:  fmt.Sprintf("%s%s", serverConf.Host, serverConf.Apis.Run),
		stopServerApiUrl: fmt.Sprintf("%s%s", serverConf.Host, serverConf.Apis.Stop),
		healthyApiUrl:    fmt.Sprintf("%s%s", serverConf.Host, serverConf.Apis.Healthy),
		listServerApiUrl: fmt.Sprintf("%s%s", serverConf.Host, serverConf.Apis.List),
	}
	// Determine the hairpin mode.
	if err := effectiveHairpinMode(pluginSettings); err != nil {
		// This is a non-recoverable error. Returning it up the callstack will just
		// lead to retries of the same failure, so just fail hard.
		return nil, err
	}
	klog.InfoS("Hairpin mode is set", "hairpinMode", pluginSettings.HairpinMode)

	// dockershim currently only supports CNI plugins.
	pluginSettings.PluginBinDirs = cni.SplitDirs(pluginSettings.PluginBinDirString)
	cniPlugins := cni.ProbeNetworkPlugins(pluginSettings.PluginConfDir, pluginSettings.PluginCacheDir, pluginSettings.PluginBinDirs)
	cniPlugins = append(cniPlugins, kubenet.NewPlugin(pluginSettings.PluginBinDirs, pluginSettings.PluginCacheDir))
	netHost := &dockerNetworkHost{
		&namespaceGetter{ss},
		&portMappingGetter{ss},
	}
	plug, err := network.InitNetworkPlugin(cniPlugins, pluginSettings.PluginName, netHost, pluginSettings.HairpinMode, pluginSettings.NonMasqueradeCIDR, pluginSettings.MTU)
	if err != nil {
		return nil, fmt.Errorf("didn't find compatible CNI plugin with given settings %+v: %v", pluginSettings, err)
	}
	ss.network = network.NewPluginManager(plug)

	return ss, nil
}

func (ss *sobeyService) Start() error {
	return nil
}

func (ss *sobeyService) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
}

func (ss *sobeyService) GetNetNS(pid string) (string, error) {
	if len(pid) == 0 {
		return "", fmt.Errorf("cannot find network namespace with null process id")
	}
	return fmt.Sprintf(sobeyNetNSFmt, pid), nil
}

func (ss *sobeyService) GetPodPortMappings(podSandboxID string) ([]*hostport.PortMapping, error) {
	// TODO: get portmappings from docker labels for backward compatibility
	//checkpoint := dockershim.NewPodSandboxCheckpoint("", "", &dockershim.CheckpointData{})
	//err := ss.checkpointManager.GetCheckpoint(podSandboxID, checkpoint)
	//// Return empty portMappings if checkpoint is not found
	//if err != nil {
	//	if err == errors.ErrCheckpointNotFound {
	//		return nil, nil
	//	}
	//	errRem := ss.checkpointManager.RemoveCheckpoint(podSandboxID)
	//	if errRem != nil {
	//		klog.ErrorS(errRem, "Failed to delete corrupt checkpoint for sandbox", "podSandboxID", podSandboxID)
	//	}
	//	return nil, err
	//}
	//_, _, _, checkpointedPortMappings, _ := checkpoint.GetData()
	//portMappings := make([]*hostport.PortMapping, 0, len(checkpointedPortMappings))
	//for _, pm := range checkpointedPortMappings {
	//	proto := toAPIProtocol(*pm.Protocol)
	//	portMappings = append(portMappings, &hostport.PortMapping{
	//		HostPort:      *pm.HostPort,
	//		ContainerPort: *pm.ContainerPort,
	//		Protocol:      proto,
	//		HostIP:        pm.HostIP,
	//	})
	//}
	//return portMappings, nil
	return nil, nil
}

func toAPIProtocol(protocol dockershim.Protocol) v1.Protocol {
	switch protocol {
	case dockershim.ProtocolTCP:
		return v1.ProtocolTCP
	case dockershim.ProtocolUDP:
		return v1.ProtocolUDP
	case dockershim.ProtocolSCTP:
		return v1.ProtocolSCTP
	}
	klog.InfoS("Unknown protocol, defaulting to TCP", "protocol", protocol)
	return v1.ProtocolTCP
}

// effectiveHairpinMode determines the effective hairpin mode given the
// configured mode, and whether cbr0 should be configured.
func effectiveHairpinMode(s *dockershim.NetworkPluginSettings) error {
	// The hairpin mode setting doesn't matter if:
	// - We're not using a bridge network. This is hard to check because we might
	//   be using a plugin.
	// - It's set to hairpin-veth for a container runtime that doesn't know how
	//   to set the hairpin flag on the veth's of containers. Currently the
	//   docker runtime is the only one that understands this.
	// - It's set to "none".
	if s.HairpinMode == kubeletconfig.PromiscuousBridge || s.HairpinMode == kubeletconfig.HairpinVeth {
		if s.HairpinMode == kubeletconfig.PromiscuousBridge && s.PluginName != "kubenet" {
			// This is not a valid combination, since promiscuous-bridge only works on kubenet. Users might be using the
			// default values (from before the hairpin-mode flag existed) and we
			// should keep the old behavior.
			klog.InfoS("Hairpin mode is set but kubenet is not enabled, falling back to HairpinVeth", "hairpinMode", s.HairpinMode)
			s.HairpinMode = kubeletconfig.HairpinVeth
			return nil
		}
	} else if s.HairpinMode != kubeletconfig.HairpinNone {
		return fmt.Errorf("unknown value: %q", s.HairpinMode)
	}
	return nil
}
