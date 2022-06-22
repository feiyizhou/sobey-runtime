package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"reflect"
	"sort"
)

type Etcd struct {
	RootCertPath      string   `json:"root-cert-path" mapstructure:"root-cert-path"`
	ClientCertPath    string   `json:"client-cert-path" mapstructure:"client-cert-path"`
	ClientKeyCertPath string   `json:"client-key-cert-path" mapstructure:"client-key-cert-path"`
	EndPoints         []string `json:"endpoints" mapstructure:"endpoints"`
}

type Server struct {
	Host    string    `json:"host" mapstructure:"host"`
	Apis    ServerApi `json:"apis" mapstructure:"apis"`
	IpRange string    `json:"ipRange" mapstructure:"ipRange"`
	Repo    string    `json:"repo" mapstructure:"repo"`
}

type ServerApi struct {
	Run     string `json:"run" mapstructure:"run"`
	Stop    string `json:"stop" mapstructure:"stop"`
	Healthy string `json:"healthy" mapstructure:"healthy"`
	List    string `json:"list" mapstructure:"list"`
}

func InitConf() error {
	cond := flag.String("config", ".", "config dir, config file type is yaml")
	flag.Parse()
	viper.AddConfigPath(*cond)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		return fmt.Errorf("Fatal error config file: %s \n", err)
	}
	keys := viper.AllKeys()
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%s = %v\n", key, viper.Get(key))
	}
	return err
}

func InitServerConf() *Server {
	server := new(Server)
	dbConfMap := viper.GetStringMap("server")
	if dbConfMap == nil {
		fmt.Println("The config of etcd is not exist ")
		return nil
	}
	err := ParseInterface2Struct(dbConfMap, &server)
	if err != nil {
		fmt.Printf("Parse confStr : %+v to struct err , err : %+v", dbConfMap, err)
		return nil
	}
	return server
}

func InitEtcdConf() *Etcd {
	etcd := new(Etcd)
	dbConfMap := viper.GetStringMap("etcd")
	if dbConfMap == nil {
		fmt.Println("The config of etcd is not exist ")
		return nil
	}
	err := ParseInterface2Struct(dbConfMap, &etcd)
	if err != nil {
		fmt.Printf("Parse confStr : %+v to struct err , err : %+v", dbConfMap, err)
		return nil
	}
	return etcd
}

// ParseInterface2Struct ...
func ParseInterface2Struct(in interface{}, out interface{}) error {
	kind := reflect.TypeOf(in).Kind()
	if reflect.Map == kind {
		err := mapstructure.Decode(in, out)
		if err != nil {
			return err
		}
	} else if reflect.String == kind {
		err := json.Unmarshal([]byte(in.(string)), out)
		if err != nil {
			return err
		}
	} else {
		return errors.New(fmt.Sprintf("Can not parse this type : %s ! ", kind.String()))
	}
	return nil
}
