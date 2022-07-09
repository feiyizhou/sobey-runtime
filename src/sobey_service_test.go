package src

import (
	"fmt"
	"sobey-runtime/config"
	"sobey-runtime/etcd"
	"testing"
)

func TestSobeyService_InitIpRange(t *testing.T) {
	etcdConf := &config.Etcd{
		RootCertPath:      "/root/k8s/tmp/ca.pem",
		ClientCertPath:    "/root/k8s/tmp/server.pem",
		ClientKeyCertPath: "/root/k8s/tmp/server-key.pem",
		EndPoints: []string{"https://172.16.200.167:2379",
			"https://172.16.200.168:2379",
			"https://172.16.200.169:2379"},
	}
	_ = etcd.InitEtcd(etcdConf)
	service, _ := NewSobeyService(&config.Server{
		Host: "http://172.16.200.112:9067",
		Apis: config.ServerApi{
			Run:     "/v1/server/run",
			Stop:    "/v1/server/stop",
			Healthy: "/v1/server/healthy",
			List:    "/v1/server/list",
		},
		IpRange: "172.244.0.0/24",
	}, nil)
	_ = service.InitIpRange()
}

func TestSobeyService_PutReleasedIP(t *testing.T) {
	etcdConf := &config.Etcd{
		RootCertPath:      "/root/k8s/tmp/ca.pem",
		ClientCertPath:    "/root/k8s/tmp/server.pem",
		ClientKeyCertPath: "/root/k8s/tmp/server-key.pem",
		EndPoints: []string{"https://172.16.200.167:2379",
			"https://172.16.200.168:2379",
			"https://172.16.200.169:2379"},
	}
	_ = etcd.InitEtcd(etcdConf)
	service, _ := NewSobeyService(&config.Server{
		Host: "http://172.16.200.112:9067",
		Apis: config.ServerApi{
			Run:     "/v1/server/run",
			Stop:    "/v1/server/stop",
			Healthy: "/v1/server/healthy",
			List:    "/v1/server/list",
		},
		IpRange: "172.244.0.0/24",
	}, nil)
	_ = service.PutReleasedIP("172.16.200.2")
}

func TestSobeyService_NewSandboxIP(t *testing.T) {
	etcdConf := &config.Etcd{
		RootCertPath:      "/root/k8s/tmp/ca.pem",
		ClientCertPath:    "/root/k8s/tmp/server.pem",
		ClientKeyCertPath: "/root/k8s/tmp/server-key.pem",
		EndPoints: []string{"https://172.16.200.167:2379",
			"https://172.16.200.168:2379",
			"https://172.16.200.169:2379"},
	}
	_ = etcd.InitEtcd(etcdConf)
	service, _ := NewSobeyService(&config.Server{
		Host: "http://172.16.200.112:9067",
		Apis: config.ServerApi{
			Run:     "/v1/server/run",
			Stop:    "/v1/server/stop",
			Healthy: "/v1/server/healthy",
			List:    "/v1/server/list",
		},
		IpRange: "172.244.0.0/24",
	}, nil)
	ip, _ := service.NewSandboxIP()
	fmt.Println(ip)
}
