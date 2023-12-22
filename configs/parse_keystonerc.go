package configs

import (
	"log"
	"os"
	"regexp"
	"request_openstack/consts"
)

// [root@con01 ~]# cat /etc/kolla/admin-openrc.sh
//# Clear any old environment that may conflict.
//for key in $( set | awk '{FS="="}  /^OS_/ {print $1}' ); do unset $key ; done
//export OS_PROJECT_DOMAIN_NAME=Default
//export OS_USER_DOMAIN_NAME=Default
//export OS_PROJECT_NAME=admin
//export OS_TENANT_NAME=admin
//export OS_USERNAME=admin
//export OS_PASSWORD=a01wXH5lN9e2CZFQASe0pBwg2Jqt8lA71ozKSjHJ
//export OS_AUTH_URL=http://10.50.31.1:35357/v3
//export OS_INTERFACE=internal
//export OS_ENDPOINT_TYPE=internalURL
//export OS_IDENTITY_API_VERSION=3
//export OS_REGION_NAME=RegionOne
//export OS_AUTH_PLUGIN=password

func Parse(pattern, content string) string {
	r := regexp.MustCompile(pattern)
	findUserNames := r.FindStringSubmatch(content)
	if len(findUserNames) == 2 {
		return findUserNames[1]
	}
	return ""
}

func ParseKeystonerc(keystonerc string) {
	data, err := os.ReadFile(keystonerc)
	if err != nil {
		log.Fatalln("Failed to read keystonerc file", err)
	}
	content := string(data)
	userName := Parse("OS_USERNAME=(.*?)\n", content)
	projectName := Parse("OS_PROJECT_NAME=(.*?)\n", content)
	password := Parse("OS_PASSWORD=(.*?)\n", content)
	host := Parse("OS_AUTH_URL=http://(.*?):", content)
	CONF.UserName = userName
	CONF.ProjectName = projectName
	if userName == consts.ADMIN {
		CONF.AdminPassword = password
	} else {
		CONF.UserPassword = password
	}
	CONF.Host = host
}
