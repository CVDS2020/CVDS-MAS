package storage

type StorageType struct {
	ID     uint32
	Name   string
	Suffix string
}

var (
	StorageTypeRTP  = StorageType{ID: 1, Name: "rtp", Suffix: "rtp"}
	StorageTypePS   = StorageType{ID: 96, Name: "ps", Suffix: "mpg"}
	StorageTypeH264 = StorageType{ID: 98, Name: "h264", Suffix: "h264"}
)

var storageTypeMap = map[uint32]*StorageType{
	StorageTypeRTP.ID: &StorageTypeRTP,
	StorageTypePS.ID:  &StorageTypePS,
}

func GetStorageType(id uint32) *StorageType {
	return storageTypeMap[id]
}
