package storage

type StorageType struct {
	ID     uint32
	Name   string
	Suffix string
}

var (
	StorageTypeBinary = StorageType{ID: 0, Name: "binary", Suffix: "bin"}
	StorageTypeRTP    = StorageType{ID: 1, Name: "rtp", Suffix: "rtp"}
	StorageTypePS     = StorageType{ID: 96, Name: "ps", Suffix: "mpg"}
	StorageTypeH264   = StorageType{ID: 98, Name: "h264", Suffix: "h264"}
)

var storageTypeMap = map[uint32]*StorageType{
	StorageTypeBinary.ID: &StorageTypeBinary,
	StorageTypeRTP.ID:    &StorageTypeRTP,
	StorageTypePS.ID:     &StorageTypePS,
	StorageTypeH264.ID:   &StorageTypeH264,
}

func GetStorageType(id uint32) *StorageType {
	return storageTypeMap[id]
}
