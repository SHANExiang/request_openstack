package openstack_config

import (
	"github.com/Unknwon/goconfig"
	"log"
)

var (
	ServiceConfigs = map[string]string{
		"nova": "/etc/kolla/nova-api/nova.conf",
		"cinder": "/etc/kolla/cinder-api/cinder.conf",
	}
)


func LoadConf(confName string) *goconfig.ConfigFile {
	conf, err := goconfig.LoadConfigFile(confName)
	if err != nil {
		log.Println("Failed to load config file", confName)
        return nil
	}
    return conf
}



