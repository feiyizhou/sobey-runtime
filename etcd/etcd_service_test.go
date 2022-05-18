package etcd

import (
	"encoding/json"
	"fmt"
	"sobey-runtime/config"
	"testing"
)

type Demo struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

var etcdConf = &config.Etcd{
	RootCertPath:      "/opt/etcd/ssl/ca.pem",
	ClientCertPath:    "/opt/etcd/ssl/server.pem",
	ClientKeyCertPath: "/opt/etcd/ssl/server-key.pem",
	EndPoints:         []string{"https://172.16.166.87:2379"},
}

func TestDBService_Put(t *testing.T) {
	demo := Demo{
		Name: "test",
		Age:  123,
	}
	bytes, _ := json.Marshal(demo)
	_ = InitEtcd(etcdConf)
	_ = NewDBService().Put("test", string(bytes))
	defer func() { _ = db.Close() }()
}

func TestDBService_Get(t *testing.T) {
	_ = InitEtcd(etcdConf)
	res, _ := NewDBService().Get("releasedIp")
	fmt.Println(res)
	defer func() { _ = db.Close() }()
}

func TestNewDBService(t *testing.T) {
	_ = InitEtcd(etcdConf)
	_ = NewDBService().Delete("sandbox_BpLnfgDsc2WD_1652431723372739586")
	defer func() { _ = db.Close() }()
}

func TestDBService_PutWithPrefix(t *testing.T) {
	_ = InitEtcd(etcdConf)
	_ = NewDBService().PutWithPrefix("test", "1", "test1")
	_ = NewDBService().PutWithPrefix("test", "2", "test2")
	_ = NewDBService().PutWithPrefix("test", "3", "test3")
	defer func() { _ = db.Close() }()
}

func TestDBService_GetByPrefix(t *testing.T) {
	_ = InitEtcd(etcdConf)
	responses, _ := NewDBService().GetByPrefix("container")
	for _, response := range responses {
		fmt.Println(response)
	}
	defer func() { _ = db.Close() }()
}

func TestDBService_DeleteByPrefix(t *testing.T) {
	_ = InitEtcd(etcdConf)
	_ = NewDBService().DeleteByPrefix("sandbox")
	_ = NewDBService().DeleteByPrefix("container")
	defer func() { _ = db.Close() }()
}
