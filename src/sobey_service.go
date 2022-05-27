package src

import (
	"fmt"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"net/http"
	"sobey-runtime/config"
	"sobey-runtime/etcd"
	util "sobey-runtime/utils"
	"strings"
	"sync"
)

const (
	sobeyNetNSFmt     = "/proc/%v/ns/net"
	namespaceModeHost = "host"
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

type sobeyService struct {
	// cri
	os               util.OSInterface
	networkReady     map[string]bool
	networkReadyLock sync.Mutex

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

func NewSobeyService(serverConf *config.Server) (SobeyService, error) {

	hostTmpArr := strings.Split(serverConf.Host, ":")
	ss := &sobeyService{
		os:           util.RealOS{},
		networkReady: make(map[string]bool),

		dbService: etcd.NewDBService(),

		ipRange: serverConf.IpRange,

		repo: serverConf.Repo,

		host:             strings.Join(hostTmpArr[:len(hostTmpArr)-1], ":"),
		runServerApiUrl:  fmt.Sprintf("%s%s", serverConf.Host, serverConf.Apis.Run),
		stopServerApiUrl: fmt.Sprintf("%s%s", serverConf.Host, serverConf.Apis.Stop),
		healthyApiUrl:    fmt.Sprintf("%s%s", serverConf.Host, serverConf.Apis.Healthy),
		listServerApiUrl: fmt.Sprintf("%s%s", serverConf.Host, serverConf.Apis.List),
	}
	return ss, nil
}

func (ss *sobeyService) Start() error {
	return nil
}

func (ss *sobeyService) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
}
