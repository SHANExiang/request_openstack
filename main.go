package main

import (
	"os"
	"request_openstack/configs"
	"request_openstack/core/manager"
)

var defaultFilePath string

func init() {
	configs.Viper()
	currentPath, _ := os.Getwd()
	defaultFilePath = currentPath + "\\configs\\templates\\"
}

func main() {
	cleaner := manager.NewCleaner([]string{"sdn_test"})
	cleaner.Run()

	//scheduler := manager.NewScheduler(defaultFilePath + "instance.yaml")
	//scheduler.Run()
    //m := manager.NewManager()
    //m.InterconnectInDifferentVpcNoFw()
    //m.CreateVpcConnectionWithCidrHelper("c17450b5-1224-4b22-9fdb-5d919fac7b26",
    //	"7365d653-4f70-4528-8ce8-0d3b1b87909d",
    //	[]string{"192.182.182.175/32"}, []string{"192.23.23.118/32"})

    //log.Printf("%+v", m.ListVpcConnections().Vcs)
}

