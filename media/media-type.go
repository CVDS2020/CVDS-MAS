package media

import "strings"

type MediaType struct {
	ID           uint32
	PT           uint8
	Name         string
	EncodingName string
	ClockRate    int
	Type         string
	Level        int
}

var (
	MediaTypePS    = MediaType{ID: 96, PT: 96, Name: "PS", EncodingName: "PS", ClockRate: 90000, Type: "video", Level: 0}
	MediaTypeH264  = MediaType{ID: 98, PT: 98, Name: "H.264", EncodingName: "H264", ClockRate: 90000, Type: "video", Level: 1}
	MediaTypeH265  = MediaType{ID: 100, PT: 100, Name: "H.265", EncodingName: "H265", ClockRate: 90000, Type: "video", Level: 2}
	MediaTypeMPEG4 = MediaType{ID: 97, PT: 97, Name: "MPEG-4", EncodingName: "MPEG4", ClockRate: 90000, Type: "video", Level: 3}
	MediaTypeSVAC  = MediaType{ID: 99, PT: 99, Name: "SVAC", EncodingName: "SVAC", ClockRate: 90000, Type: "video", Level: 4}

	MediaTypeG711A = MediaType{ID: 8, PT: 8, Name: "G.711A", EncodingName: "PCMA", ClockRate: 8000, Type: "audio", Level: 0}
	MediaTypeG7231 = MediaType{ID: 4, PT: 4, Name: "G.723.1", EncodingName: "G723", ClockRate: 8000, Type: "audio", Level: 1}
	MediaTypeG7221 = MediaType{ID: 9, PT: 9, Name: "G.722.1", EncodingName: "G722", ClockRate: 8000, Type: "audio", Level: 2}
	MediaTypeG729  = MediaType{ID: 18, PT: 18, Name: "G.729", EncodingName: "G729", ClockRate: 8000, Type: "audio", Level: 3}
	MediaTypeSVACA = MediaType{ID: 20, PT: 20, Name: "SVACA", EncodingName: "SVACA", ClockRate: 8000, Type: "audio", Level: 4}
	MediaTypeAAC   = MediaType{ID: 101, PT: 101, Name: "AAC", EncodingName: "mpeg4-generic", Type: "audio", Level: 5}
)

var mediaTypeMap = map[uint32]*MediaType{
	MediaTypePS.ID:    &MediaTypePS,
	MediaTypeH264.ID:  &MediaTypeH264,
	MediaTypeMPEG4.ID: &MediaTypeMPEG4,
	MediaTypeH265.ID:  &MediaTypeH265,
	MediaTypeSVAC.ID:  &MediaTypeSVAC,

	MediaTypeG711A.ID: &MediaTypeG711A,
	MediaTypeG7231.ID: &MediaTypeG7231,
	MediaTypeG7221.ID: &MediaTypeG7221,
	MediaTypeG729.ID:  &MediaTypeG729,
	MediaTypeSVACA.ID: &MediaTypeSVACA,
}

var (
	mediaTypePTMap           = make(map[uint8]*MediaType)
	mediaTypeEncodingNameMap = make(map[string]*MediaType)
)

func init() {
	for _, mediaType := range mediaTypeMap {
		mediaTypeEncodingNameMap[mediaType.EncodingName] = mediaType
		mediaTypePTMap[mediaType.PT] = mediaType
	}
}

func GetMediaType(id uint32) *MediaType {
	return mediaTypeMap[id]
}

func GetMediaTypeByPT(pt uint8) *MediaType {
	return mediaTypePTMap[pt]
}

func ParseMediaType(encodingName string) *MediaType {
	return mediaTypeEncodingNameMap[strings.ToUpper(encodingName)]
}
