package configs

import (
	"fmt"
	"github.com/spf13/viper"
	"log"
	"path"
	"runtime"
)

func Viper() *viper.Viper {
	v := viper.New()
	currentPath := getCurrentAbPathByCaller()
	//exePath, err := os.Executable()
	//if err != nil {
	//	fmt.Println(err)
	//}

	//currentPath := filepath.Dir(exePath)

	//currentPath, _ := os.Getwd()
	//config = fmt.Sprintf("%s\\configs\\openstack.yaml", currentPath)
	//config = "C:\\workspace\\codes\\goprojects\\internal\\request_openstack\\configs\\openstack.yaml"
	//log.Println("config is", config)
	v.SetConfigFile(currentPath + "\\openstack.yaml")
	v.SetConfigType("yaml")
	err := v.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fail to read yaml err:%v \n", err))
	}
	if err = v.Unmarshal(&CONF); err != nil {
		fmt.Println(err)
	}
	log.Println("CONF", CONF)
	return v
}

func getCurrentAbPathByCaller() string {
	var abPath string
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		abPath = path.Dir(filename)
	}
	return abPath
}