package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"request_openstack/configs"
	"request_openstack/core/manager"
	"runtime"
	"time"
)

//var defaultFilePath string

func init() {
	configs.Viper()
	//currentPath, _ := os.Getwd()
	//defaultFilePath = currentPath + "\\configs\\templates\\"
}

func main() {
	//cleaner := manager.NewCleaner([]string{"sdn_test"})
	//cleaner.Run()
    //L3RelatedCLI()

	m := manager.NewManager()
	instance1 := m.CreateInstanceHelper("ed74d5fc-e644-4400-ac3f-3b946717c2f5")
	port1, _ := m.GetInstancePort(instance1)
	fipId := m.CreateFloatingipWithPortHelper(port1)
    fip := m.GetFIP(fipId)

	currentPath, _ := os.Getwd()
	if runtime.GOOS == "windows" {
		currentPath += "\\"
	} else if runtime.GOOS == "linux" {
		currentPath += "/"
	}
	fileName := currentPath + time.Now().Format("2006-01-02_15-04-05-1") + fmt.Sprintf("_record.json")
	var file, err = os.Create(fileName)
	defer file.Close()
	if err != nil {
		log.Fatalf("Failed to create file %s, %v", fileName, err)
	}
	data, _ := json.Marshal(fip)
	file.Write(data)
	log.Println("==============Export to json file success", fileName)
}

