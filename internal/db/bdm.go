package db

import "time"

type BDM struct {
    CreateAt                      time.Time        `gorm:"created_at"`
    UpdateAt                      time.Time         `gorm:"update_at"`
    DeleteAt                      time.Time         `gorm:"delete_at"`
    Id                            int               `gorm:"id"`
    DeviceName                    string            `gorm:"device_name"`
    DeleteOnTermination           bool              `gorm:"delete_on_termination"`
    SnapshotId                    string            `gorm:"snapshot_id"`
    VolumeId                      string            `gorm:"volume_id"`
    VolumeSize                    int               `gorm:"volume_size"`
    NoDevice                      int               `gorm:"no_device"`
    ConnectionInfo                string            `gorm:"connection_info"`
    InstanceUUID                  string            `gorm:"instance_uuid"`
    Deleted                       int               `gorm:"deleted"`
    SourceType                    string               `gorm:"source_type"`
    DestinationType               string               `gorm:"destination_type"`
    GuestFormat                   string               `gorm:"guest_format"`
    DeviceType                    string               `gorm:"device_type"`
    DiskBus                       string               `gorm:"disk_bus"`
    BootIndex                     int                  `gorm:"boot_index"`
    ImageId                       string               `gorm:"image_id"`
    Tag                           string               `gorm:"tag"`
    AttachmentId                  string               `gorm:"attachment_id"`
    UUID                          string               `gorm:"uuid"`
    VolumeType                    string               `gorm:"volume_type"`
}


