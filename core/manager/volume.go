package manager

import (
	"strings"
)

func (m *Manager) CreateCinderQosType() {
	m.RequestQosLimitCommons()
	m.RequestQosLimitEfficients()
	m.RequestQosLimitSSDs()
}

func (m *Manager) SetVolumeTypeBackendProperty() {
	volumeTypes := m.ListVolumeTypes()

	for _, volumeType := range volumeTypes.VTs {
		if strings.Contains(volumeType.Name, "SEBS-ssd") {
            m.AddExtraSpecsForVolumeType(volumeType.Id, "volume_backend_name", "rbd-2")
		}
	}
}
//
//func (m *Manager) DetachAndDeleteVolume() {
//	volumeIds := []string{"78ae5419-634c-4c43-8aa1-766d0650997d",
//		"8acbad21-e013-46ae-8e38-1030f6a6bf73",
//		"3e30f251-b57e-47d4-96a3-9d3b9567d850",
//		"8700f43b-57fe-4dc8-98dc-e957bdda09a7",
//	}
//	var wg sync.WaitGroup
//	for _, volumeId := range volumeIds {
//		wg.Add(1)
//		volume := m.GetVolume(volumeId)
//		if len(volume.Attachments) != 0 {
//			for _, attachment := range volume.Attachments {
//				m.DeleteAttachment(attachment.AttachmentId)
//			}
//		}
//		go m.BeforeDeleteVolumeEnsureVolumeAvailable(&wg, volumeId)
//	}
//	wg.Wait()
//
//}



// #####################################################################################

//func (m *Manager) BeforeDeleteVolumeEnsureVolumeAvailable(wg *sync.WaitGroup, volumeId string) {
//	defer wg.Done()
//	volume := m.GetVolume(volumeId)
//	done := make(chan bool, 1)
//	go func() {
//		state := volume.Status
//		for state != consts.Available && state != consts.Error {
//			time.Sleep(consts.IntervalTime)
//			volume = m.GetVolume(volumeId)
//			state = volume.Status
//		}
//		done <- true
//	}()
//	select {
//	case <-done:
//		m.DeleteVolume(volumeId)
//		log.Println("*******************Delete Volume success")
//	case <-time.After(consts.Timeout):
//		log.Fatalln("*******************Delete volume timeout")
//	}
//}

func (m *Manager) SetVolumeState(destState string) {

}