package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"request_openstack/configs"
	"request_openstack/consts"
	"request_openstack/core/manager"
	"strings"
)

var defaultFilePath string

func init() {
	//configs.Viper()
	//currentPath, _ := os.Getwd()
	//defaultFilePath = currentPath + "\\configs\\templates\\"


}

func CLI() {
	flag.Usage = func() {
		usage := fmt.Sprintf("Usage: %s ", os.Args[0])
		fmt.Fprintf(os.Stderr, "%s[-h]\n", usage)
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--keystonerc </etc/kolla/admin-openrc.sh>]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--delete]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--recover]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--username <username>]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--userpassword <userpassword>]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--backupFile <backupFile>]")
		fmt.Fprintf(os.Stderr, "optional arguments:\n  -h, --help            show this help message and exit\n")
		flag.PrintDefaults()
	}
	keystonercFile := flag.String("keystonerc", "/etc/kolla/admin-openrc.sh", "Openstack keystone auth file")
	toDelete := flag.Bool("delete", false, "Clear router gateway/disassociate fip/update fip no qos policy/delete fip port forwarding")
	toRecover := flag.Bool("recover", false, "Set router gateway/associate fip/update fip with qos policy/add fip port forwarding")
	username := flag.String("username", "admin", "User name")
	userpassword := flag.String("userpassword", "", "User password")
	backupFile := flag.String("backupFile", "", "Backup json file")
	flag.Parse()
    configs.ParseKeystonerc(*keystonercFile)
	var m *manager.Manager
	if *username == consts.ADMIN {
		m = manager.NewAdminManager()
	} else {
		if len(*userpassword) == 0 {
			log.Fatalln("==============The parameter userpassword is necessary while username is specified")
		}
		configs.CONF.UserName = *username
		configs.CONF.UserPassword = *userpassword
		configs.CONF.ProjectName = *username
		m = manager.NewManager()
	}
	log.Printf("CONF=%+v", configs.CONF)
	if *toDelete {
		m.RunDeleteRes()
	} else if *toRecover {
		if len(*backupFile) == 0 {
			log.Fatalln("==============The backupFile is necessary when to recover resources")
		} else {
			m.RunRecoverL3Res(*backupFile)
		}
	} else {
		m.RunGenerateRecord()
	}
}

func main() {
	//cleaner := manager.NewCleaner([]string{"sdn_test"})
	//cleaner.Run()

	//m := manager.NewManager()
	//m.RunGenerateRecord()
	CLI()
}

