package main

import (
	"request_openstack/configs"
	"request_openstack/core/manager"
)

var defaultFilePath string

func init() {
	configs.Viper()
	//currentPath, _ := os.Getwd()
	//defaultFilePath = currentPath + "\\configs\\templates\\"
}

func main() {
	//cleaner := manager.NewCleaner([]string{"sdn_test"})
	//cleaner.Run()

	m := manager.NewManager()
	instanceId := m.CreateInstanceHelper("72255588-5bde-4929-97df-67a353d28735")
	fipId := m.CreateFloatingipHelper()
	instancePort, _ := m.GetInstancePort(instanceId)
	m.UpdateFloatingIpWithPort(fipId, instancePort)
}

