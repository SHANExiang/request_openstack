package manager

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"request_openstack/configs"
	"request_openstack/consts"
	"request_openstack/internal/entity"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Worker struct {
    AdminManager     *Manager
    UserName         string
    projectId        string
    concurrency      int
}

func NewWorker(userName string) *Worker {
	manager := NewAdminManager()
	projectId := manager.GetProjectId(userName)
	return &Worker{
		UserName: userName,
		AdminManager: manager,
		projectId: projectId,
		concurrency: 1,
	}
}

type fipAssoc struct {
	FipId                      string       `json:"fip_id"`
	FipAssocPort               string       `json:"fipAssocPort"`
	FixedIpAddress             string       `json:"fixed_ip_address"`
	QosPolicyId                string       `json:"qos_policy_id"`
	FipPort                    string       `json:"fip_port"`
	FloatingNetworkID          string       `json:"floating_network_id"`
	FloatingIpAddr             string       `json:"floating_ip_address"`
	RouterId                   string       `json:"router_id"`
	entity.PortForwardings                  `json:"port_forwardings"`
}

type routerAssoc struct {
	RouterId                  string      `json:"router_id"`
	entity.GatewayInfo                    `json:"external_gateway_info"`
	Fips                      []string    `json:"fips"`
}

type L3RelatedResource struct {
	Routers              map[string]routerAssoc      `json:"routers"`
	Fips                 map[string]fipAssoc         `json:"fips"`
}

func (w *Worker) listRouters(projectId string) entity.Routers {
	var urlSuffix string
	if w.UserName == consts.ADMIN {
		urlSuffix = consts.ROUTERS
	} else {
		urlSuffix = fmt.Sprintf("routers?project_id=%s", projectId)
	}
	resp := w.AdminManager.Neutron.List(w.AdminManager.Neutron.Headers, urlSuffix)
	var routers entity.Routers
	_ = json.Unmarshal(resp, &routers)
	log.Println("==============List routers success, there had", routers.Count)
	return routers
}

func (w *Worker) listFIPs(projectId string) entity.Fips {
	var urlSuffix string
	if w.UserName == consts.ADMIN {
		urlSuffix = consts.FLOATINGIPS
	} else {
		urlSuffix = fmt.Sprintf("floatingips?project_id=%s", projectId)
	}

	resp := w.AdminManager.Neutron.List(w.AdminManager.Neutron.Headers, urlSuffix)
	var fs entity.Fips
	_ = json.Unmarshal(resp, &fs)
	log.Println("==============List fip success, there had", fs.Count)
	return fs
}


func (w *Worker) listRouterFIPs(routerId string) entity.Fips {
	var urlSuffix string
	if w.UserName == consts.ADMIN {
		urlSuffix = consts.FLOATINGIPS
	} else {
		urlSuffix = fmt.Sprintf("floatingips?router_id=%s", routerId)
	}

	resp := w.AdminManager.Neutron.List(w.AdminManager.Neutron.Headers, urlSuffix)
	var fs entity.Fips
	_ = json.Unmarshal(resp, &fs)
	log.Println("==============List fip success, there had", fs.Count)
	return fs
}

func (w *Worker) generateL3RelatedResObjs() L3RelatedResource {
	var lrr = L3RelatedResource{}

	lrr.Routers = make(map[string]routerAssoc)
	routers := w.listRouters(w.projectId)
	for _, router := range routers.Rs {
		fips := w.listRouterFIPs(router.Id)
		fipIds := make([]string, 0)
		for _, fip := range fips.Fs {
			fipIds = append(fipIds, fip.Id)
		}
		routerAssoc := routerAssoc{
			RouterId: router.Id,
			GatewayInfo: router.GatewayInfo,
			Fips: fipIds,
		}
		lrr.Routers[router.Id] = routerAssoc
	}

	lrr.Fips = make(map[string]fipAssoc)
	fips := w.listFIPs(w.projectId)
	for _, fip := range fips.Fs {
		pfs := w.AdminManager.ListPortForwarding(fip.Id)
		fipPort := w.AdminManager.GetFloatingipPort(fip.Id)
		var qosPolicyId string
		if fipPort.QosPolicyId != nil {
			qosPolicyId = fipPort.QosPolicyId.(string)
		} else {
			qosPolicyId = ""
		}

		fipAssoc := fipAssoc{
			FipId: fip.Id,
			FipAssocPort: fip.PortId,
			PortForwardings: pfs,
			QosPolicyId: qosPolicyId,
			FloatingNetworkID: fip.FloatingNetworkId,
			FipPort: fipPort.Id,
			FloatingIpAddr: fip.FloatingIpAddress,
			FixedIpAddress: fip.FixedIpAddress,
			RouterId: fip.RouterId,
		}
		lrr.Fips[fip.Id] = fipAssoc
	}
	log.Printf("==============Generate l3 related resource %+v", lrr)
	return lrr
}

func (w *Worker) exportToJsonFile(lrr L3RelatedResource) {
	currentPath, _ := os.Getwd()
	if runtime.GOOS == "windows" {
		currentPath += "\\"
	} else if runtime.GOOS == "linux" {
		currentPath += "/"
	}
	fileName := currentPath + time.Now().Format("2006-01-02_15-04-05-1") + fmt.Sprintf("_record_%s.json", w.UserName)
	var file, err = os.Create(fileName)
	defer file.Close()
	if err != nil {
		log.Fatalf("Failed to create file %s, %v", fileName, err)
	}
	data, _ := json.Marshal(lrr)
	file.Write(data)
	log.Println("==============Export to json file success", fileName)
}

func (w *Worker) deleteFipResources(fip fipAssoc, wg *sync.WaitGroup, msg *string) {
	defer func() {
		if err := recover(); err != nil {
			errInfo := fmt.Sprintf("fip disassociated port %s error %v", fip.FipId, err)
			log.Println("************************catch error：", errInfo)
			*msg += "\n" + errInfo
		}
		wg.Done()
	}()
	for _, pf := range fip.PortForwardings.Pfs {
		w.AdminManager.DeletePortForwarding(fip.FipId, pf.Id)
	}

	if len(fip.FipAssocPort) != 0 {
		w.AdminManager.FloatingIpDisassociatePort(fip.FipId)
	}

	w.AdminManager.UpdatePortWithNoQos(fip.FipPort)
}

func (w *Worker) deleteRouterResources(router routerAssoc, msg *string) {
	defer func() {
		if err := recover(); err != nil {
			errInfo := fmt.Sprintf("router %s clear gateway error %v", router.RouterId, err)
			log.Println("************************catch error：", errInfo)
			*msg += "\n" + errInfo
		}
	}()
	if reflect.DeepEqual(router.GatewayInfo, entity.GatewayInfo{}) {
		log.Println("router no gateway", router.RouterId)
		return
	}
	output := w.AdminManager.ClearRouterGateway(router.RouterId, router.NetworkID)
	if !output.Success {
		panic(output.Response)
	}
}

func (w *Worker) deleteRouterFip(assoc routerAssoc, lrr L3RelatedResource, msg *string) {
	//defer func() {
	//	if err := recover(); err != nil {
	//		log.Println("************************catch error：", err)
	//	}
	//}()
	var wg sync.WaitGroup
	for _, fipId := range assoc.Fips {
		wg.Add(1)
		if fip, ok := lrr.Fips[fipId]; ok {
			go w.deleteFipResources(fip, &wg, msg)
		}
	}
	wg.Wait()
	if len(*msg) != 0 {
		log.Printf("##########有错误THERE OCCUR ERROR FOR %s\n %s", assoc.RouterId, *msg)
		return
	}
	w.deleteRouterResources(assoc, msg)
	if len(*msg) != 0 {
		log.Printf("##########有错误THERE OCCUR ERROR FOR %s\n %s", assoc.RouterId, *msg)
		return
	}
}

func (w *Worker) deleteFipNoAssoc(lrr L3RelatedResource) string {
	msg := ""
	var wg sync.WaitGroup
	for _, fip := range lrr.Fips {
		if len(fip.RouterId) == 0 {
			wg.Add(1)
			go w.deleteFipResources(fip, &wg, &msg)
		}
	}
	wg.Wait()
	return msg
}

func (w *Worker) recoverFipNoAssoc(lrr L3RelatedResource) string {
	msg := ""
	var wg sync.WaitGroup
	for _, fip := range lrr.Fips {
		if len(fip.RouterId) == 0 {
			wg.Add(1)
			go w.processFip(fip, &wg, &msg)
		}
	}
	wg.Wait()
	return msg
}

func (w *Worker) DeleteAndRecoverL3RelatedRes(lrr L3RelatedResource) {
	msgDelete := w.deleteFipNoAssoc(lrr)
	if len(msgDelete) != 0 {
		log.Println("##########有错误THERE OCCUR ERROR", msgDelete)
	}
	msgRecover := w.recoverFipNoAssoc(lrr)
	if len(msgRecover) != 0 {
		log.Println("##########有错误THERE OCCUR ERROR", msgDelete)
	}

	for _, assoc := range lrr.Routers {
		log.Printf("*******************The VPC is deleting and recovering... %s\n", assoc.RouterId)
		msg := ""
        w.deleteRouterFip(assoc, lrr, &msg)
		if len(msg) != 0 {
			continue
		}
        w.recoverRouterFip(assoc, lrr, &msg)
	}

	log.Println("*******************ALL L3 related resources were deleted and recovered success!!!")
}

func (w *Worker) getRecordFromJsonFile(jsonFile string) L3RelatedResource {
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		log.Fatalln("Failed to read json file", err)
	}

	var lrr L3RelatedResource
	err = json.Unmarshal(data, &lrr)
	if err != nil {
		log.Fatalln("Failed to unmarshal json file", err)
	}
	return lrr
}

func (w *Worker) setRouterGateway(router routerAssoc, msg *string) {
	defer func() {
		if err := recover(); err != nil {
			errInfo := fmt.Sprintf("router %s set gateway error %v", router.RouterId, err)
			log.Println("************************catch error: ", errInfo)
			*msg += "\n" + errInfo
		}
	}()
	opts := entity.UpdateRouterOpts{
		GatewayInfo: &router.GatewayInfo,
	}
	if reflect.DeepEqual(router.GatewayInfo, entity.GatewayInfo{}) {
		log.Println("router no gateway", router.RouterId)
		return
	}
	w.AdminManager.UpdateRouter(router.RouterId, &opts)
}

func (w *Worker) handleRouterError(routerId string) bool {
	timeout := 5 * 60 * time.Second
	done := make(chan bool, 1)
	go func() {
		router := w.AdminManager.GetRouter(routerId)
		state := router.Router.Status
		for state != "ACTIVE" {
			time.Sleep(10 * time.Second)
			router = w.AdminManager.GetRouter(routerId)
			state = router.Router.Status
		}
		done <- true
	}()
	select {
	case <-done:
		log.Printf("*******************Router %s is ACTIVE\n", routerId)
		return true
	case <-time.After(timeout):
		log.Printf("*******************Router %s is not ACTIVE\n", routerId)
		return false
	}
}

func (w *Worker) processFip(fip fipAssoc, wg *sync.WaitGroup, msg *string) {
	defer func() {
		if err := recover(); err != nil {
			errInfo := fmt.Sprintf("fip associated port %s error %v", fip.FipId, err)
			log.Println("************************catch error：", errInfo)
			*msg += "\n" + errInfo
		}
		wg.Done()
	}()
	if len(fip.FipAssocPort) != 0 {
		w.AdminManager.UpdateFloatingIpWithPortIpAddress(fip.FipId, fip.FipAssocPort, fip.FixedIpAddress)
	}

	if len(fip.QosPolicyId) != 0 {
		w.AdminManager.UpdatePortWithQos(fip.FipPort, fip.QosPolicyId)
	}

	for _, pf := range fip.PortForwardings.Pfs {
		pfOpts := entity.CreatePortForwardingOpts{
			Protocol: pf.Protocol,
			InternalIPAddress: pf.InternalIpAddress,
			InternalPort: pf.InternalPort,
			InternalPortID: pf.InternalPortId,
			ExternalPort: pf.ExternalPort,
		}
		w.AdminManager.CreatePortForwarding(fip.FipId, &pfOpts)
	}
}

func (w *Worker) recoverRouterFip(assoc routerAssoc, lrr L3RelatedResource, msg *string) {
	if w.handleRouterError(assoc.RouterId) {
		w.setRouterGateway(assoc, msg)
		if len(*msg) != 0 {
			log.Printf("##########有错误THERE OCCUR ERROR FOR %s\n %s", assoc.RouterId, *msg)
			return
		}
		fips := make([]fipAssoc, 0)
		for _, fipId := range assoc.Fips {
			if fip, ok := lrr.Fips[fipId]; ok {
				fips = append(fips, fip)
			}
		}
		if w.handleRouterError(assoc.RouterId) {
			var wg sync.WaitGroup
			for _, fip := range fips {
				wg.Add(1)
				go w.processFip(fip, &wg, msg)
			}
			wg.Wait()
			if len(*msg) != 0 {
				log.Printf("##########有错误THERE OCCUR ERROR FOR %s\n %s", assoc.RouterId, *msg)
			} else {
				log.Printf("*******************The VPC was deleted and recovered success %s\n", assoc.RouterId)
			}
		} else {
			log.Printf("*******************There has fips will not be processed %+v\n", fips)
		}
	} else {
		log.Printf("*******************The router is NOT ACTIVE in 5min%s\n", assoc.RouterId)
	}
}

func (w *Worker) RunRecoverL3Res(jsonFile string, routerId string) {
	lrr := w.getRecordFromJsonFile(jsonFile)
	//lrr := m.generateL3RelatedResObjs()
	msg := ""
	if assoc, ok := lrr.Routers[routerId]; ok {
		w.recoverRouterFip(assoc, lrr, &msg)
	}
}

func (w *Worker) RunGenerateRecord() {
	lrr := w.generateL3RelatedResObjs()
	w.exportToJsonFile(lrr)
}

func (w *Worker) RunDeleteAndRecoverRes() {
	lrr := w.generateL3RelatedResObjs()
	w.exportToJsonFile(lrr)
	w.DeleteAndRecoverL3RelatedRes(lrr)
}

func (w *Worker) RunDeleteRecoverVpcResource(jsonFile string, routerId string) {
	lrr := w.getRecordFromJsonFile(jsonFile)
	msg := ""
	if assoc, ok := lrr.Routers[routerId]; ok {
		log.Printf("*******************The VPC was deleting and recovering... %s\n", assoc.RouterId)
		w.deleteRouterFip(assoc, lrr, &msg)
		if len(msg) != 0 {
			log.Printf("##########有错误THERE OCCUR ERROR FOR %s\n %s", assoc.RouterId, msg)
			return
		}
		w.recoverRouterFip(assoc, lrr, &msg)
	}
}

func L3RelatedCLI() {
	flag.Usage = func() {
		usage := fmt.Sprintf("Usage: %s ", os.Args[0])
		fmt.Fprintf(os.Stderr, "%s[-h]\n", usage)
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--keystonerc </etc/kolla/admin-openrc.sh>]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--delete_recover]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--recover]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--vpc]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--username <username>]")
		fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat(" ", len(usage)) + "[--backupFile <backupFile>]")
		fmt.Fprintf(os.Stderr, "optional arguments:\n  -h, --help            show this help message and exit\n")
		flag.PrintDefaults()
	}
	keystonercFile := flag.String("keystonerc", "/etc/kolla/admin-openrc.sh", "Openstack keystone auth file")
	toDeleteRecover := flag.Bool("delete_recover", false, "Clear router gateway/Disassociate fip/Update fip no qos policy/Delete fip port forwarding--->Set router gateway/Associate fip/Update fip with qos policy/Add fip port forwarding")
	username := flag.String("username", "", "User name")
	vpc := flag.String("vpc", "", "vpc id")
	toRecover := flag.Bool("recover", false, "recover vpc")
	backupFile := flag.String("backupFile", "", "Backup json file")
	flag.Parse()
	configs.ParseKeystonerc(*keystonercFile)

	if len(*username) == 0 {
		log.Fatalf("==============The parameter usernmae must be specified!!!\n\n")
	}
	worker := NewWorker(*username)
	log.Printf("CONF=%+v", configs.CONF)
	if *toDeleteRecover {

		if len(*vpc) == 0 {
			worker.RunDeleteAndRecoverRes()
		} else {
			if len(*backupFile) == 0 {
				log.Fatalf("==============The backupFile is necessary when to recover vpc resources\n\n")
			}

		}

	} else if *toRecover {
		if len(*backupFile) == 0 {
			log.Fatalf("==============The backupFile is necessary when to recover resources\n\n")
		}
		if len(*vpc) == 0 {
			log.Fatalf("==============The vpc is necessary when to recover resources\n\n")
		}
		worker.RunRecoverL3Res(*backupFile, *vpc)
	} else {
		worker.RunGenerateRecord()
	}
}

