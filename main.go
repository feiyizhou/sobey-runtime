package main

import (
	"fmt"
	"google.golang.org/grpc"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	kubeletconfiginternal "k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/pkg/kubelet/dockershim"
	"os"
	"path/filepath"
	"sobey-runtime/common"
	"sobey-runtime/config"
	"sobey-runtime/etcd"
	"sobey-runtime/src"
	util "sobey-runtime/utils"
)

const maxMsgSize = 1024 * 1024 * 16

// SobeyServer ...
type SobeyServer struct {
	endpoint string
	service  src.CRIService
	server   *grpc.Server
}

func init() {
	_, err := os.Stat(filepath.Dir(common.ServerLogDirPath))
	if err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(filepath.Dir(common.ServerLogDirPath), 0750)
			if err != nil {
				fmt.Printf("Create server log dir err, err: %v", err)
			}
		}
	}

	_, err = os.Stat(filepath.Dir(common.SockerImagesPath))
	if err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(filepath.Dir(common.SockerImagesPath), 0750)
			if err != nil {
				fmt.Printf("Create kubernetes pod log dir err, err: %v", err)
			}
		}
	}

	_, err = os.Stat(filepath.Dir(common.KubernetesPodLogDirPath))
	if err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(filepath.Dir(common.KubernetesPodLogDirPath), 0750)
			if err != nil {
				fmt.Printf("Create kubernetes pod log dir err, err: %v", err)
			}
		}
	}
}

func main() {

	err := config.InitConf()
	if err != nil {
		fmt.Printf("Init config err, err: %v", err)
		return
	}

	etcdConf := config.InitEtcdConf()
	if etcdConf == nil {
		fmt.Println("The config of etcd is not exist")
		return
	}
	err = etcd.InitEtcd(etcdConf)
	if err != nil {
		fmt.Printf("Init ercd err, err: %v", err)
		return
	}

	pluginSettings := dockershim.NetworkPluginSettings{
		HairpinMode:        kubeletconfiginternal.HairpinMode("promiscuous-bridge"),
		NonMasqueradeCIDR:  "10.0.0.0/8",
		PluginName:         "cni",
		PluginConfDir:      "/etc/cni/net.d",
		PluginBinDirString: "/opt/cni/bin",
		PluginCacheDir:     "/var/lib/cni/cache",
		MTU:                0,
	}

	serverConf := config.InitServerConf()
	if serverConf == nil {
		fmt.Println("The config of server is not exist")
		return
	}
	ss, _ := src.NewSobeyService(serverConf, &pluginSettings)
	err = ss.InitIpRange()
	if err != nil {
		fmt.Printf("Init ip range err, err: %v", err)
		return
	}
	s := SobeyServer{
		endpoint: "unix:///run/sobeyshim.sock",
		service:  ss,
	}
	l, err := util.CreateListener(s.endpoint)
	if err != nil {
		fmt.Printf("failed to listen on %q: %v", s.endpoint, err)
		return
	}
	s.server = grpc.NewServer(
		grpc.MaxRecvMsgSize(maxMsgSize),
		grpc.MaxSendMsgSize(maxMsgSize),
	)
	runtimeapi.RegisterRuntimeServiceServer(s.server, s.service)
	runtimeapi.RegisterImageServiceServer(s.server, s.service)
	err = s.server.Serve(l)
	if err != nil {
		fmt.Printf("Failed to serve connections, err: %v", err)
		return
	}
}
