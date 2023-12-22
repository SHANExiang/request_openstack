package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"request_openstack/consts"
	"request_openstack/internal/cache"
	"request_openstack/internal/entity"
	"request_openstack/utils"
	"strconv"
	"strings"
	"sync"
	"time"
)

var maxEfficientQOS string
var maxSSDEfficientQOS string
var supportedCinderResourceTypes = [...]string{
	consts.VOLUME, consts.SNAPSHOT,
}


type Cinder struct {
	Request
	adminProjectId       string
	projectId            string
	headers              map[string]string
	snowflake            *utils.Snowflake
	wg                   sync.WaitGroup
	ctx                  context.Context
	tag                  string
	DeleteChannels       map[string]chan Output
	mu                   sync.Mutex
}

func initCinderOutputChannels() map[string]chan Output {
	outputChannel := make(map[string]chan Output)
	for _, resourceType := range supportedCinderResourceTypes {
		outputChannel[resourceType] = make(chan Output, 0)
	}
	return outputChannel
}

func NewCinder(options ...Option) *Cinder {
	opts := new(Options)
	for _, option := range options {
		option(opts)
	}
	var cinder *Cinder
	cinder = &Cinder{
		wg: sync.WaitGroup{},
		DeleteChannels: initCinderOutputChannels(),
	}
	cinder.projectId = opts.ProjectId
	cinder.adminProjectId = opts.AdminProject
	headers := make(map[string]string)
	headers["Openstack-Api-Version"] = "volume 3.59"
	headers[consts.AuthToken] = opts.Token
	cinder.headers = headers
	cinder.Request = opts.Request
	return cinder
}

func (c *Cinder) getDeleteChannel(resourceType string) chan Output {
	defer c.mu.Unlock()
	c.mu.Lock()
	return c.DeleteChannels[resourceType]
}

func (c *Cinder) makeDeleteChannel(resourceType string, length int) chan Output {
	defer c.mu.Unlock()
	c.mu.Lock()

	c.DeleteChannels[resourceType] = make(chan Output, length)
	return c.DeleteChannels[resourceType]
}


// Volume type
func (c *Cinder) createVolumeType(projectId string, reqBody string) map[string]interface{} {
	PostUrl := fmt.Sprintf("%s/types", projectId)
	resp := c.DecorateResp(c.Post)(c.headers, PostUrl, reqBody)
	return resp
}

func (c *Cinder) UpdateVolumeType(volumeTypeId, name, desc string) entity.VolumeType {
	reqSuffix := fmt.Sprintf("%s/types/%s", c.adminProjectId, volumeTypeId)
	formatter := `{
                       "volume_type": {
                            "name": "%+v",
                            "description": "%+v"
                       }
                  }`
	reqBody := fmt.Sprintf(formatter, name, desc)
	resp := c.Put(c.headers, reqSuffix, reqBody)
	var volumeType entity.VolumeType
	_ = json.Unmarshal(resp, &volumeType)
	return volumeType
}

func (c *Cinder) constructVolumeTypeSSD(size int) string {
	name := fmt.Sprintf("SSD-%d", size)
	description := fmt.Sprintf("SEBS-ssd-%dG", size)
	reqBody := fmt.Sprintf("{\"volume_type\": {\"name\": \"%+v\", \"description\": \"%+v\"}}", name, description)
	return reqBody
}

func (c *Cinder) constructVolumeTypeEfficient(size int) string {
	name := fmt.Sprintf("高效硬盘-%d", size)
	description := fmt.Sprintf("SEBS-efficient-%dG", size)
	reqBody := fmt.Sprintf("{\"volume_type\": {\"name\": \"%+v\", \"description\": \"%+v\"}}", name, description)
	return reqBody
}

func (c *Cinder) constructVolumeTypeCommon() string {
	name := "普通硬盘"
	description := "SEBS-common"
	reqBody := fmt.Sprintf("{\"volume_type\": {\"name\": \"%+v\", \"description\": \"%+v\"}}", name, description)
	return reqBody
}

func (c *Cinder) VolumeTypeAssociateQos(projectId, qosId, volTypeId string) map[string]interface{} {
	URL := fmt.Sprintf("/%s/qos-specs/%s/associate?vol_type_id=%s", projectId, qosId, volTypeId)
	return c.DecorateGetResp(c.Get)(c.headers, URL)
}

func (c *Cinder) GetVolumeType(volumeTypeId string) entity.VolumeType {
	resp := c.Get(c.headers, fmt.Sprintf("%s/types/%s", c.adminProjectId, volumeTypeId))
	var volumeType entity.VolumeType

	log.Println(string(resp))
	_ = json.Unmarshal(resp, &volumeType)

	return volumeType
}

func (c *Cinder) ListVolumeTypes() entity.VolumeTypes {
	resp := c.List(c.headers, fmt.Sprintf("%s/types", c.adminProjectId))
	var volumeTypes entity.VolumeTypes
	_ = json.Unmarshal(resp, &volumeTypes)

	return volumeTypes
}

func (c *Cinder) AddExtraSpecsForVolumeType(volumeTypeId string, key, val string) {
     urlSuffix := fmt.Sprintf("%s/types/%s/extra_specs", c.adminProjectId, volumeTypeId)
	 formatter := `{
                   "extra_specs": {
                       "%+v": "%+v"
                   }
                }`
	 reqBody := fmt.Sprintf(formatter, key, val)
     c.Post(c.headers, urlSuffix, reqBody)

	log.Println("==============Add extra specs for volume type success", volumeTypeId)
}

func (c *Cinder) RequestQosLimitCommons() {
	readIopsSec := "600"
	writeIopsSec := readIopsSec
	readBytesSec := fmt.Sprintf("%d", 30 * 1024 * 1024)
	writeBytesSec := readBytesSec

	reqBody := c.constructQosBody(readIopsSec, writeIopsSec, readBytesSec, writeBytesSec)
	log.Println("create volume qos")
	var qos map[string]interface{}
	qos = c.createVolumeQOS(c.adminProjectId, reqBody)
	qosId := c.parseVolumeQosId(qos)

	reqVolType := c.constructVolumeTypeCommon()
	volType := c.createVolumeType(c.adminProjectId, reqVolType)
	volTypeId := c.parseVolumeTypeId(volType)
	log.Println(volType)

	resp := c.VolumeTypeAssociateQos(c.adminProjectId, qosId, volTypeId)
	log.Println("completed", resp)
}

func (c *Cinder) RequestQosLimitEfficients()  {
	for i := 50; i <= 1000; i=i+50 {
		readIopsSec := fmt.Sprintf("%g", math.Min(float64(1800 + 8 * i), 5000))
		writeIopsSec := readIopsSec
		readBytesSec := fmt.Sprintf("%.f", math.Min(100 + 0.15 * float64(i), 130) * 1024 * 1024)
		writeBytesSec := readBytesSec
		reqBody := c.constructQosBody(readIopsSec, writeIopsSec, readBytesSec, writeBytesSec)
		var qos map[string]interface{}
		log.Println("create volume qos", i)
		if i <= 400 {
			qos = c.createVolumeQOS(c.adminProjectId, reqBody)
			qosId := c.parseVolumeQosId(qos)
			maxEfficientQOS = qosId
		}
		log.Println(maxEfficientQOS)
		reqVolType := c.constructVolumeTypeEfficient(i)
		volType := c.createVolumeType(c.adminProjectId, reqVolType)
		volTypeId := c.parseVolumeTypeId(volType)
		log.Println(volType)
		resp := c.VolumeTypeAssociateQos(c.adminProjectId, maxEfficientQOS, volTypeId)
		log.Println("completed", i, resp)
	}
}

func (c *Cinder) RequestQosLimitSSDs()  {
	for i := 50; i <= 1000; i=i+50 {
		readIopsSec := fmt.Sprintf("%g", math.Min(float64(1800 + 30 * i), 25000))
		writeIopsSec := readIopsSec
		readBytesSec := fmt.Sprintf("%.f", math.Min(120 + 0.5 * float64(i), 256) * 1024 * 1024)
		writeBytesSec := readBytesSec
		reqBody := c.constructQosBody(readIopsSec, writeIopsSec, readBytesSec, writeBytesSec)
		var qos map[string]interface{}
		log.Println("create volume qos", i)
		if i <= 800 {
			qos = c.createVolumeQOS(c.adminProjectId, reqBody)
			maxSSDEfficientQOS = c.parseVolumeQosId(qos)
		}
		log.Println(maxSSDEfficientQOS)
		reqVolType := c.constructVolumeTypeSSD(i)
		volType := c.createVolumeType(c.adminProjectId, reqVolType)
		volTypeId := c.parseVolumeTypeId(volType)
		log.Println(volType)
		resp := c.VolumeTypeAssociateQos(c.adminProjectId, maxSSDEfficientQOS, volTypeId)
		log.Println("completed", i, resp)
	}
}

func (c *Cinder) ListVolumeQos() entity.QosSpecss {
	urlSuffix := fmt.Sprintf("/%s/qos-specs", c.projectId)
	resp := c.Get(c.headers, urlSuffix)

	var qss entity.QosSpecss
	_ = json.Unmarshal(resp, &qss)
	log.Println("==============Get qos specs success")
    return qss
}

func (c *Cinder) UpdateQosSpec(qosId, updateBody string)  {
	urlSuffix := fmt.Sprintf("/%s/qos-specs/%s", c.projectId, qosId)
	c.Put(c.headers, urlSuffix, updateBody)
}

func (c *Cinder) QosSpecDeleteKey(qosId string, keyName string)  {
	urlSuffix := fmt.Sprintf("/%s/qos-specs/%s/delete_keys", c.projectId, qosId)
	formatter := `{
                      "keys": ["%+v"]
                  }`
	updateBody := fmt.Sprintf(formatter, keyName)
	c.Put(c.headers, urlSuffix, updateBody)
}

func (c *Cinder) CorrectQosSpecs() {
    qss := c.ListVolumeQos()
    for _, qs := range qss.Qss {
    	qosId := qs.Id
    	byteVal := qs.Specs.ReadBytesSecMax
    	iopsVal := qs.Specs.ReadIopsSec
    	byteConvertVal, _ := strconv.ParseFloat(byteVal, 64)
    	byteVal = strconv.FormatFloat(byteConvertVal, 'f', -1, 64)
    	body := c.constructQosBody(iopsVal, iopsVal, byteVal, byteVal)
		c.UpdateQosSpec(qosId, body)
	}
}

func (c *Cinder) DeleteQosSpecKeyName()  {
	qss := c.ListVolumeQos()
	for _, qs := range qss.Qss {
		fmt.Printf("%+v", qs.Specs)
		//if len(qs.Name) != 0 {
		//	c.QosSpecDeleteKey(qosId, "name")
		//}
	}
}

func (c *Cinder) ListQosAndDisassociateAll()  {
	resp := c.DecorateGetResp(c.Get)(c.headers, fmt.Sprintf("/%s/qos-specs", c.projectId))
	qosSpecs := resp["qos_specs"]
	for _, v := range qosSpecs.([]interface{}) {
		qos := v.(map[string]interface{})
		name := qos["name"].(string)
		qosId := qos["id"].(string)
		if strings.Contains(name, "read_iops_sec") {
            c.QosDisAssociateAll(qosId)
		}
		c.DeleteVolumeQos(qosId)
	}
}

func (c *Cinder) QosDisAssociateAll(qosId string)  {
	c.DecorateGetResp(c.Get)(c.headers, fmt.Sprintf("/%s/qos-specs/%s/disassociate_all", c.projectId, qosId))
}

func (c *Cinder) DeleteVolumeQos(qosId string)  {
	c.Delete(c.headers, fmt.Sprintf("/%s/qos-specs/%s", c.projectId, qosId))
}

func (c *Cinder) ListVolumeTypeAndDelete() {
	resp := c.DecorateGetResp(c.Get)(c.headers, fmt.Sprintf("/%s/types", c.projectId))
	volumeTypes := resp["volume_types"]
	for _, v := range volumeTypes.([]interface{}) {
		typ := v.(map[string]interface{})
		name := typ["name"].(string)
		id := typ["id"].(string)
		if strings.Contains(name, "SEBS") {
			c.deleteVolumeType(id)
		}
	}
}

func (c *Cinder) deleteVolumeType(typeId string)  {
	c.Delete(c.headers, fmt.Sprintf("/%s/types/%s", c.projectId, typeId))
	log.Println("success delete", typeId)
}


func (c *Cinder) parseVolumeQosId(resp map[string]interface{}) string {
    if v, ok := resp["qos_specs"]; ok {
    	if id, ok := v.(map[string]interface{})["id"]; ok {
    		return id.(string)
		}
	}
	return ""
}

func (c *Cinder) parseVolumeTypeId(resp map[string]interface{}) string {
	if v, ok := resp["volume_type"]; ok {
		if id, ok := v.(map[string]interface{})["id"]; ok {
			return id.(string)
		}
	}
	return ""
}

func (c *Cinder) RequestQosLimitScenario()  {
    c.RequestQosLimitCommons()
    c.RequestQosLimitSSDs()
    c.RequestQosLimitEfficients()
}

func (c *Cinder) RequestClearQosLimit()  {
	c.ListQosAndDisassociateAll()
	c.ListVolumeTypeAndDelete()
}

// volume

// CreateVolume create volume
func (c *Cinder) CreateVolume() string {
	urlSuffix := fmt.Sprintf("/%s/volumes", c.projectId)
	name := c.snowflake.NextVal()
	formatter := `{
                     "volume": {
                         "size": 1, 
                         "name": "dx_vol_%+v"
                     }
                  }`
	reqBody := fmt.Sprintf(formatter, name)
	resp := c.Post(c.headers, urlSuffix, reqBody)
    var volume entity.VolumeMap
    _ = json.Unmarshal(resp, &volume)

	//cache.RedisClient.AddSliceAndJson(volumeId, c.tag + consts.VOLUMES, volume)
	c.MakeSureVolumeAvailable(volume.Id)
	log.Println("==============Create volume success", volume.Id)
	return volume.Id
}

func (c *Cinder) CreateVolumeBySnapshot(snapshotId string) string {
	urlSuffix := fmt.Sprintf("/%s/volumes", c.projectId)
	name := fmt.Sprintf("snapshot-%s_to_volume", snapshotId)
	formatter := `{
                     "volume": {
                         "name": "%+v",
                         "snapshot_id": "%+v" 
                     }
                  }`
	reqBody := fmt.Sprintf(formatter, name, snapshotId)
	resp := c.Post(c.headers, urlSuffix, reqBody)
	var volume entity.VolumeMap
	_ = json.Unmarshal(resp, &volume)

	//cache.RedisClient.AddSliceAndJson(volumeId, c.tag + consts.VOLUMES, volume)
	c.MakeSureVolumeAvailable(volume.Id)
	log.Println("==============Create volume from snapshot success", volume.Id)
	return volume.Id
}

func (c *Cinder) CreateVolumeByVolume(volumeId string) string {
	urlSuffix := fmt.Sprintf("/%s/volumes", c.projectId)
	name := fmt.Sprintf("volume-%s_to_volume", volumeId)
	formatter := `{
                     "volume": {
                         "name": "%+v",
                         "source_volid": "%+v"
                     }
                  }`
	reqBody := fmt.Sprintf(formatter, name, volumeId)
	resp := c.Post(c.headers, urlSuffix, reqBody)
	var volume entity.VolumeMap
	_ = json.Unmarshal(resp, &volume)

	//cache.RedisClient.AddSliceAndJson(volumeId, c.tag + consts.VOLUMES, volume)
	c.MakeSureVolumeAvailable(volume.Id)
	log.Println("==============Create volume from volume success", volume.Id)
	return volume.Id
}


func (c *Cinder) SetVolumeBootable(volumeId string) string {
	urlSuffix := fmt.Sprintf("%s/volumes/%s/action", c.projectId, volumeId)
	formatter := `{"os-set_bootable": {"bootable": true}}`
	resp := c.Post(c.headers, urlSuffix, formatter)
	var volume entity.VolumeMap
	_ = json.Unmarshal(resp, &volume)

	//cache.RedisClient.AddSliceAndJson(volumeId, c.tag + consts.VOLUMES, volume)
	c.MakeSureVolumeAvailable(volumeId)
	log.Println("==============Set volume bootable success", volume.Id)
	return volume.Id
}

func (c *Cinder) MakeSureVolumeAvailable(volumeId string) {
	volume := c.GetVolume(volumeId)
	done := make(chan bool, 1)
	go func() {
		state := volume.Status
		for state != consts.Available && state != consts.Error {
			time.Sleep(consts.IntervalTime)
			volume = c.GetVolume(volumeId)
			state = volume.Status
		}
		done <- true
	}()
	select {
	case <-done:
		log.Println("*******************Create Volume success")
	case <-time.After(consts.Timeout):
		log.Fatalln("*******************Create volume timeout")
	}
}

func (c *Cinder) DeleteProjectVolume(volumeId string) {
	defer c.wg.Done()
	urlSuffix := fmt.Sprintf("/%s/volumes/%s", c.projectId, volumeId)
	if ok, _ := c.Delete(c.headers, urlSuffix); ok {
		log.Println("==============Delete volume success", volumeId)
	} else {
		log.Fatalln("==============Delete volume failed", volumeId)
	}
}

func (c *Cinder) UpdateVolumeNoAttachments(volumeId string) {
	urlSuffix := fmt.Sprintf("/%s/volumes/%s", c.adminProjectId, volumeId)
	formatter := `{
                     "volume": {
                         "attachments": []
                     }
                  }`
	resp := c.Put(c.headers, urlSuffix, formatter)
	var volume entity.VolumeMap
	_ = json.Unmarshal(resp, &volume)

	c.MakeSureVolumeAvailable(volume.Id)
	log.Println("==============Update volume no attachments success", volume.Id)
}

func (c *Cinder) ListProjectVolumes() entity.Volumes {
	res := c.List(c.headers, fmt.Sprintf("/%s/volumes", c.projectId))
	var volumes entity.Volumes
	_ = json.Unmarshal(res, &volumes)
	log.Println("==============List volume success")
	return volumes
}

func (c *Cinder) ListVolumes() entity.Volumes {
	res := c.List(c.headers, fmt.Sprintf("/%s/volumes/detail?all_tenants=True&project_id=%s", c.adminProjectId, c.projectId))
	var volumes entity.Volumes
	_ = json.Unmarshal(res, &volumes)
	log.Println("==============List volume success, there had", len(volumes.Vs))
	return volumes
}

func (c *Cinder) GetVolume(volumeId string) entity.VolumeMap {
	res := c.Get(c.headers, fmt.Sprintf("%s/volumes/%s", c.projectId, volumeId))
	var volume entity.VolumeMap
	_ = json.Unmarshal(res, &volume)
	log.Println("==============Get volume success", volumeId)
	return volume
}

func (c *Cinder) CreateVolumes() {
	for i := 0;i < 2;i++ {
		c.wg.Add(1)
		go c.CreateVolume()
		log.Println("create volume", i)
	}
	c.wg.Wait()
}

func (c *Cinder) CinderDetachVolume(volumeId string) {
	urlSuffix := fmt.Sprintf("%s/volumes/%s/action", c.projectId, volumeId)
	formatter := `{
		"os-detach": {
		}
	}`
	c.Post(c.headers, urlSuffix, formatter)
	log.Println("==============Detach volume success", volumeId)
}

func (c *Cinder) DetachVolume(volumeId string) {
	urlSuffix := fmt.Sprintf("%s/volumes/%s/action", c.adminProjectId, volumeId)
	formatter := `{
		"os-detach": {
		}
	}`
	c.Post(c.headers, urlSuffix, formatter)
	log.Println("==============Detach volume success", volumeId)
}


func (c *Cinder) DeleteAttachment(attachmentId string) {
	urlSuffix := fmt.Sprintf("%s/attachments/%s", c.adminProjectId, attachmentId)
	if ok, _ := c.Delete(c.headers, urlSuffix); ok {
		log.Println("==============Delete attachment success", attachmentId)
		return
	}
	log.Println("==============Delete attachment failed", attachmentId)
}

func (c *Cinder) UploadToImage(volumeId string) string {
	imageName := fmt.Sprintf("%s_to_image", volumeId)
	urlSuffix := fmt.Sprintf("/%s/volumes/%s/action", c.projectId, volumeId)
	formatter := `{
		"os-volume_upload_image": {
			"image_name": "%+v",
            "force": true,
			"disk_format": "raw",
			"container_format": "bare",
			"visibility": "private",
			"protected": false
		}
	}`
	reqBody := fmt.Sprintf(formatter, imageName)
	resp := c.Post(c.headers, urlSuffix, reqBody)
	var volumeToImage entity.VolumeToImage
	_ = json.Unmarshal(resp, &volumeToImage)
	log.Println("==============Volume uploads to image request success", volumeId)
	return volumeToImage.OsVolumeUploadImage.ImageId
}

func (c *Cinder) DeleteVolume(volumeId string, ch chan Output) {
	outputObj := Output{ParametersMap: map[string]string{"volume_id": volumeId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
		ch <- outputObj
	}()

	urlSuffix := fmt.Sprintf("/%s/volumes/%s", c.adminProjectId, volumeId)
	outputObj.Success, outputObj.Response = c.Delete(c.headers, urlSuffix)
}

func (c *Cinder) DeleteVolumes() {
	volumes := c.ListVolumes()
	ch := c.makeDeleteChannel(consts.VOLUME, len(volumes.Vs))
	for _, volume := range volumes.Vs {
		if len(volume.Attachments) != 0 {
			for _, attachment := range volume.Attachments {
				c.DeleteAttachment(attachment.AttachmentId)
			}
		}
		go c.DeleteVolume(volume.Id, ch)
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Volumes were deleted completely")
}

func (c *Cinder) DeleteVolumesFromRedis() {
	volumeIds := cache.RedisClient.GetMembers(c.tag + consts.VOLUMES)
	for _, volumeId := range volumeIds {
		c.DeleteProjectVolume(volumeId)
	}
}

func (c *Cinder) CheckSysOrDataDisk(volumeId string) bool {
	volume := c.GetVolume(volumeId)
	if volume.Bootable == "true" {
		return true
	}
	return false
}

// Volume Qos

//AddQosSpecs add qos specs
func (c *Cinder) AddQosSpecs(qosId, reqBody string) map[string]interface{} {
	urlSuffix := fmt.Sprintf("/%s/qos-specs/%s", c.projectId, qosId)
	resp := c.DecorateResp(c.Put)(c.headers, urlSuffix, reqBody)
	return resp
}

func (c *Cinder) ListQos() map[string]interface{} {
	urlSuffix := fmt.Sprintf("/%s/qos-specs", c.projectId)
	return c.DecorateGetResp(c.List)(c.headers, urlSuffix)
}

func (c *Cinder) createVolumeQOS(projectId string, reqBody string) map[string]interface{} {
	PostUrl := fmt.Sprintf("/%s/qos-specs", projectId)
	resp := c.DecorateResp(c.Post)(c.headers, PostUrl, reqBody)
	log.Println("create volume qos resp", resp)
	return resp
}

func (c *Cinder) constructQosBody(readIopsSec, writeIopsSec, readBytesSec, writeBytesSec string) string {
	consumer := "front-end"
	name := fmt.Sprintf("read_iops_sec-%s-read_iops_sec_max-%s-write_iops_sec-%s-write_iops_sec_max-%s" +
		"-read_bytes_sec-%s-read_bytes_sec_max-%s-write_bytes_sec-%s-write_bytes_sec_max-%s",
		readIopsSec, readIopsSec, writeIopsSec, writeIopsSec, readBytesSec, readBytesSec, writeBytesSec, writeBytesSec)
	body := fmt.Sprintf("{\"qos_specs\": {\"name\": \"%+v\", \"consumer\": \"%+v\", \"read_iops_sec\": \"%+v\", \"write_iops_sec\": \"%+v\", \"read_iops_sec_max\": \"%+v\", \"write_iops_sec_max\": \"%+v\",  \"read_bytes_sec\": \"%+v\", \"write_bytes_sec\": \"%+v\", \"read_bytes_sec_max\": \"%+v\", \"write_bytes_sec_max\": \"%+v\"}}",
		name, consumer, readIopsSec, readIopsSec, writeIopsSec, writeIopsSec,
		readBytesSec, readBytesSec, writeBytesSec, writeBytesSec)
	log.Println(body)
	return body
}

func (c *Cinder) RequestAddVolumeQosSpecs() {
	resp := c.ListQos()
	qosSpecs := resp["qos_specs"]
	for _, v := range qosSpecs.([]interface{}) {
		qos := v.(map[string]interface{})
		log.Println("qos==", qos)
		qosId := qos["id"]
		if specs, exist := qos["specs"]; exist {
			qosMap := make(map[string]string)
			for _, key := range []string{"read_bytes_sec", "read_iops_sec", "write_bytes_sec", "write_iops_sec"} {
				newSpec := fmt.Sprintf("%s_max", key)
				if _, exist = specs.(map[string]interface{})[newSpec]; !exist {
					qosMap[newSpec] = specs.(map[string]interface{})[key].(string)
				}
			}
			if len(qosMap) == 0 {
				continue
			}
			var putBody strings.Builder
			putBody.WriteString("{\"qos_specs\": {")
			tempStr := make([]string, 0)
			for k, v := range qosMap {
				tempStr = append(tempStr, fmt.Sprintf("\"%+v\": \"%+v\"", k, v))
			}
			putBody.WriteString(strings.Join(tempStr, ", "))
			putBody.WriteString("}}")
			log.Println("putBody", putBody.String())
			resp = c.AddQosSpecs(qosId.(string), putBody.String())
			log.Println("增加_max参数，resp==", resp)
		}
	}
}

func (c *Cinder) updateQos(qosId, reqBody string) {
	urlSuffix := fmt.Sprintf("/%s/qos-specs/%s", c.projectId, qosId)
    c.Put(c.headers, urlSuffix, reqBody)
	log.Println("==============Update qos success")
}

func (c *Cinder) ModifyQosSpecs() {
	resp := c.ListQos()
	qosSpecs := resp["qos_specs"]
	for _, v := range qosSpecs.([]interface{}) {
		qos := v.(map[string]interface{})
		log.Println("qos==", qos)
		qosId := qos["id"].(string)
		if specs, exist := qos["specs"]; exist {
			formatter := `{
                             "qos_specs": {
                                 "read_bytes_sec": "%+v",
                                 "read_bytes_sec_max": "%+v",
                                 "write_bytes_sec": "%+v",
                                 "write_bytes_sec_max": "%+v"
                             }
                         }`
			var readBytesSec string
			var readBytesSecMax string
			var writeBytesSec string
			var writeBytesSecMax string
			if val, exist := specs.(map[string]interface{})["read_bytes_sec"]; exist {
				oldVal, _ := strconv.Atoi(val.(string))
				newVal := oldVal * 1024
				readBytesSec = strconv.Itoa(newVal)
			}
			if val, exist := specs.(map[string]interface{})["read_bytes_sec_max"]; exist {
				oldVal, _ := strconv.Atoi(val.(string))
				newVal := oldVal * 1024
				readBytesSecMax = strconv.Itoa(newVal)
			}
			if val, exist := specs.(map[string]interface{})["write_bytes_sec"]; exist {
				oldVal, _ := strconv.Atoi(val.(string))
				newVal := oldVal * 1024
				writeBytesSec = strconv.Itoa(newVal)
			}
			if val, exist := specs.(map[string]interface{})["write_bytes_sec_max"]; exist {
				oldVal, _ := strconv.Atoi(val.(string))
				newVal := oldVal * 1024
				writeBytesSecMax = strconv.Itoa(newVal)
			}
			putBody := fmt.Sprintf(formatter, readBytesSec, readBytesSecMax, writeBytesSec, writeBytesSecMax)
			log.Println("putBody", putBody)
			c.updateQos(qosId, putBody)
			log.Println("修改吞吐参数 success")
		}
	}
}

// snapshot

// CreateSnapshot create snapshot from volume
func (c *Cinder) CreateSnapshot(volumeId string) string {
    urlSuffix := fmt.Sprintf("/%s/snapshots", c.projectId)
    name := "dx_vol_" + strconv.FormatUint(c.snowflake.NextVal(), 10)
    description := "dx volume"
    reqBody := fmt.Sprintf("{\"snapshot\": {\"name\": \"%+v\", \"description\": \"%+v\", \"volume_id\": \"%+v\", \"force\": true}}", name, description, volumeId)
    resp := c.Post(c.headers, urlSuffix, reqBody)
    var snapshot entity.SnapshotMap
    _ = json.Unmarshal(resp, &snapshot)
    //cache.RedisClient.AddSliceAndJson(snapshotId, c.tag + consts.SNAPSHOTS, snapshot)
    c.makeSureSnapshotAvailable(snapshot.Id)
    return snapshot.Id
}

func (c *Cinder) getSnapshot(snapshotId string) entity.SnapshotMap {
	urlSuffix := fmt.Sprintf("/%s/snapshots/%s", c.projectId, snapshotId)
	resp := c.Get(c.headers, urlSuffix)
	var snapshot entity.SnapshotMap
	_ = json.Unmarshal(resp, &snapshot)
	log.Println("==============Get snapshot success", snapshotId)
	return snapshot
}

func (c *Cinder) makeSureSnapshotAvailable(snapshotId string) {
	snapshot := c.getSnapshot(snapshotId)
	done := make(chan bool, 1)
	go func() {
		state := snapshot.Status
		for state != consts.Available && state != consts.Error {
			time.Sleep(consts.IntervalTime)
			snapshot = c.getSnapshot(snapshotId)
			state = snapshot.Status
		}
		done <- true
	}()
	select {
	case <-done:
		log.Println("*******************Create snapshot success")
	case <-time.After(consts.Timeout):
		log.Fatalln("*******************Create snapshot timeout")
	}
}

func (c *Cinder) listProjectSnapshots() entity.Snapshots {
	urlSuffix := fmt.Sprintf("/%s/snapshots", c.projectId)
	resp := c.List(c.headers, urlSuffix)
	var ss entity.Snapshots
	_ = json.Unmarshal(resp, &ss)
	log.Println("==============List snapshot success")
	return ss
}

func (c *Cinder) listSnapshots() entity.Snapshots {
	urlSuffix := fmt.Sprintf("/%s/snapshots/detail?all_tenants=True&project_id=%s", c.adminProjectId, c.projectId)
	resp := c.List(c.headers, urlSuffix)
	var ss entity.Snapshots
	_ = json.Unmarshal(resp, &ss)
	log.Println("==============List snapshot success, there had", len(ss.Ss))
	return ss
}

func (c *Cinder) DeleteProjectSnapshot(snapshotId string) {
	urlSuffix := fmt.Sprintf("/%s/snapshots/%s", c.projectId, snapshotId)
	if ok, _ := c.Delete(c.headers, urlSuffix); ok {
		log.Println("==============Delete snapshot success", snapshotId)
	} else {
		log.Fatalln("==============Delete snapshot failed", snapshotId)
	}
}

//func (c *Cinder) DeleteProjectSnapshots() {
//	snapshots := c.listSnapshots()
//    for _, snapshot := range snapshots.Ss {
//    	c.DeleteSnapshot(snapshot.Id)
//	}
//}

func (c *Cinder) DeleteSnapshot(snapshotId string, ch chan Output) {
	outputObj := Output{ParametersMap: map[string]string{"snapshot_id": snapshotId}}
	defer func() {
		if err := recover(); err != nil {
			log.Println("catch error：", err)
			outputObj.Success = false
			outputObj.Response = err
		}
		ch <- outputObj
	}()

	urlSuffix := fmt.Sprintf("/%s/snapshots/%s", c.adminProjectId, snapshotId)
	outputObj.Success, outputObj.Response = c.Delete(c.headers, urlSuffix)
}

func (c *Cinder) DeleteSnapshots() {
	snapshots := c.listSnapshots()
	ch := c.makeDeleteChannel(consts.SNAPSHOT, len(snapshots.Ss))
	for _, snapshot := range snapshots.Ss {
		go c.DeleteSnapshot(snapshot.Id, ch)
	}
	if len(ch) != cap(ch) {
		for len(ch) != cap(ch) {}
	}
	log.Println("Snapshots were deleted completely")
}

func (c *Cinder) revertToAnySnapshot(volumeId string, snapshotId string)  {
	reqBody := fmt.Sprintf("{\"revert_any\": {\"snapshot_id\": \"%+v\"}}", snapshotId)
	reqUrl := fmt.Sprintf("/%s/volumes/%s/action", c.projectId, volumeId)
	resp := c.DecorateResp(c.Post)(c.headers, reqUrl, reqBody)
	volume := resp["volume"].(map[string]interface{})

	c.SyncResource(volumeId, nil, volume)
	log.Println("==============revert to any snapshot success", volumeId)
}

// backup
func (c *Cinder) createBackup(volumeId string) string {
	urlSuffix := fmt.Sprintf("/%s/snapshots", c.projectId)
	name := "dx_vol_" + strconv.FormatUint(c.snowflake.NextVal(), 10)
	description := "dx volume"
	reqBody := fmt.Sprintf("{\"snapshot\": {\"name\": \"%+v\", \"description\": \"%+v\", \"volume_id\": \"%+v\"}}", name, description, volumeId)
	resp := c.DecorateResp(c.Post)(c.headers, urlSuffix, reqBody)
	snapshot := resp["snapshot"].(map[string]interface{})
	snapshotId := snapshot["id"].(string)

	cache.RedisClient.AddSliceAndJson(snapshotId, c.tag + consts.SNAPSHOTS, snapshot)
	log.Println("==============create snapshot success", snapshotId)
	return snapshotId
}


