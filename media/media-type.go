package media

import "strings"

type MediaType struct {
	ID        uint32
	LowerName string
	UpperName string
	Level     int
}

var (
	MediaTypePS   = MediaType{ID: 96, LowerName: "ps", UpperName: "PS", Level: 0}
	MediaTypeH264 = MediaType{ID: 98, LowerName: "h264", UpperName: "H264", Level: 1}
	MediaTypeMPEG = MediaType{ID: 97, LowerName: "mpeg", UpperName: "MPEG", Level: 3}
	MediaTypeH265 = MediaType{ID: 99, LowerName: "h265", UpperName: "H265", Level: 2}
)

var mediaTypeMap = map[uint32]*MediaType{
	MediaTypePS.ID:   &MediaTypePS,
	MediaTypeH264.ID: &MediaTypeH264,
	MediaTypeMPEG.ID: &MediaTypeMPEG,
	MediaTypeH265.ID: &MediaTypeH265,
}

var mediaTypeUpperNameMap = map[string]*MediaType{
	MediaTypePS.UpperName:   &MediaTypePS,
	MediaTypeH264.UpperName: &MediaTypeH264,
	MediaTypeMPEG.UpperName: &MediaTypeMPEG,
	MediaTypeH265.UpperName: &MediaTypeH265,
}

func GetMediaType(id uint32) *MediaType {
	return mediaTypeMap[id]
}

func ParseMediaType(name string) *MediaType {
	return mediaTypeUpperNameMap[strings.ToUpper(name)]
}
