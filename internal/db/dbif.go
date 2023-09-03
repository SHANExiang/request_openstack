package db

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"request_openstack/consts"
	"request_openstack/internal/openstack_config"
	"strings"
)

func getConnection(service string) string {
	configFile, ok := openstack_config.ServiceConfigs[service]
	if !ok {
		log.Println(fmt.Sprintf("the config file for %s not exist", service))
		panic("")
	}
	configObj := openstack_config.LoadConf(configFile)
	connection, err := configObj.GetValue("database", "connection")
	if err != nil {
		log.Println("the config connection not exist")
		panic("")
	}
	return connection
}

func parseConnection(connection string) (userPassword, host string) {
	// mysql+pymysql://nova:a0X4sANSpfKw6QGYVUNkrn0Gyf1tfi0qQ3IeBrdN@10.50.31.1:3306/nova_cell0
	connection = strings.Split(connection, "//")[1]
	connection = strings.Split(connection, "/")[0]
	temp := strings.Split(connection, "@")
	host = temp[1]
	userPassword = temp[0]
	return
}

func constructDSN(service string) string {
	connection := getConnection(service)
    userPassword, host := parseConnection(connection)
    dsn := fmt.Sprintf("%s@(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", userPassword, host, service)
   return dsn
}

func InitNovaDBIf() *gorm.DB {
	dsn := constructDSN(consts.NOVA)
	DBIf, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Failed to connect DB")
	}
    return DBIf
}
