package src

import (
	"fmt"
	"sobey-runtime/config"
	"sobey-runtime/etcd"
	"testing"
)

var (
	service, _ = NewSobeyService(&config.Server{
		Host: "http://172.16.200.112:9067",
		Apis: config.ServerApi{
			Run:     "/v1/server/run",
			Stop:    "/v1/server/stop",
			Healthy: "/v1/server/healthy",
			List:    "/v1/server/list",
		},
		IpRange: "172.244.0.0/24",
	})

	etcdConf = &config.Etcd{
		RootCertPath:      "/opt/etcd/ssl/ca.pem",
		ClientCertPath:    "/opt/etcd/ssl/server.pem",
		ClientKeyCertPath: "/opt/etcd/ssl/server-key.pem",
		EndPoints:         []string{"https://172.16.166.87:2379"},
	}
)

func TestSobeyService_InitIpRange(t *testing.T) {
	_ = etcd.InitEtcd(etcdConf)
	_ = service.InitIpRange()
}

func TestSobeyService_PutReleasedIP(t *testing.T) {
	_ = etcd.InitEtcd(etcdConf)
	_ = service.PutReleasedIP("172.16.200.2")
}

func TestSobeyService_NewSandboxIP(t *testing.T) {
	_ = etcd.InitEtcd(etcdConf)
	ip, _ := service.NewSandboxIP()
	fmt.Println(ip)
}
