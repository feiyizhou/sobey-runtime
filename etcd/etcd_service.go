package etcd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"io/ioutil"
	"k8s.io/klog/v2"
	"sobey-runtime/config"
	"time"
)

var db *clientv3.Client

// DBService ...
type DBService struct {
}

func NewDBService() *DBService {
	return &DBService{}
}

func InitEtcd(conf *config.Etcd) error {
	// load root ca cert
	etcdCA, err := ioutil.ReadFile(conf.RootCertPath)
	if err != nil {
		klog.ErrorS(err, "load root ca cert failed", "root cert path", conf.RootCertPath)
		return err
	}

	// load client cert
	etcdClientCert, err := tls.LoadX509KeyPair(conf.ClientCertPath, conf.ClientKeyCertPath)
	if err != nil {
		klog.ErrorS(err, "load client cert failed", "cert path", conf.ClientCertPath, "cert key path", conf.ClientKeyCertPath)
		return err
	}

	// create a root cert pool
	rootCertPool := x509.NewCertPool()
	rootCertPool.AppendCertsFromPEM(etcdCA)

	// create api v3 client
	db, err = clientv3.New(clientv3.Config{
		// etcd https api 端点
		Endpoints:   conf.EndPoints,
		DialTimeout: 5 * time.Second,
		TLS: &tls.Config{
			RootCAs:      rootCertPool,
			Certificates: []tls.Certificate{etcdClientCert},
		},
	})
	if err != nil {
		klog.ErrorS(err, "failed to init an etcd client", "endpoints", conf.EndPoints)
		return err
	}
	return err
}

func (ds *DBService) Put(key, val string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(3)*time.Second)
	defer cancel()
	putResp, err := db.Put(ctx, key, val)
	if err != nil {
		klog.ErrorS(err, "failed to put record to etcd", "key", key, "value", val)
		return err
	}
	klog.V(9).InfoS("success insert record to etcd", "res", putResp)
	return err
}

func (ds *DBService) PutWithPrefix(prefix, key, val string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(3)*time.Second)
	defer cancel()
	putResp, err := db.Put(ctx, fmt.Sprintf("%s_%s", prefix, key), val, clientv3.WithPrevKV())
	if err != nil {
		klog.ErrorS(err, "failed to put record to etcd", "key", fmt.Sprintf("%s_%s", prefix, key), "value", val)
		return err
	}
	klog.V(9).InfoS("success insert record to etcd", "res", putResp)
	return err
}

func (ds *DBService) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(3)*time.Second)
	defer cancel()
	delResp, err := db.Delete(ctx, key)
	if err != nil {
		klog.ErrorS(err, "failed to delete record", "key", key)
		return err
	}
	klog.V(9).InfoS("delete record successfully", "res", delResp)
	return err
}

func (ds *DBService) DeleteByPrefix(prefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	defer cancel()
	getResp, err := db.Delete(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		klog.ErrorS(err, "failed to delete record", "prefix", prefix)
		return err
	}
	if getResp.Deleted == 0 {
		klog.V(9).InfoS("record does not exist", "prefix", prefix)
		return err
	}
	return err
}

func (ds *DBService) Get(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(3)*time.Second)
	defer cancel()
	getResp, err := db.Get(ctx, key)
	if err != nil {
		klog.ErrorS(err, "failed to get record", "key", key)
		return "", err
	}
	if getResp.Count == 0 {
		klog.V(9).InfoS("record does not exist", "key", key)
		return "", err
	}
	return string(getResp.Kvs[0].Value), err
}

func (ds *DBService) GetByPrefix(ctx context.Context, prefix string) ([]string, error) {
	getResp, err := db.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		klog.ErrorS(err, "failed to get record", "prefix", prefix)
		return nil, err
	}
	if getResp.Count == 0 {
		klog.V(9).InfoS("record does not exist", "prefix", prefix)
		return nil, err
	}
	var results []string
	for _, kv := range getResp.Kvs {
		results = append(results, string(kv.Value))
	}
	return results, err
}
