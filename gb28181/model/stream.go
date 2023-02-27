package model

const StreamTableName = "stream"

type Stream struct {
	Id int64 `gorm:"column:id;primaryKey;not null;autoIncrement;comment:主键ID" json:"id"`

	Channel string `gorm:"column:channel;type:varchar(255);uniqueIndex:channelUniqueIndex;not null;comment:通道名称" json:"channel"`

	CallId  string `gorm:"column:callId;type:varchar(255);uniqueIndex:dialogIdUniqueIndex,priority:3;not null;comment:对话callId" json:"callId"`
	FromTag string `gorm:"column:fromTag;type:varchar(255);uniqueIndex:dialogIdUniqueIndex,priority:2;not null;comment:对话fromTag" json:"fromTag"`
	ToTag   string `gorm:"column:toTag;type:varchar(255);uniqueIndex:dialogIdUniqueIndex,priority:1;not null;comment:对话toTag" json:"toTag"`

	CSeq uint32 `gorm:"column:cSeq;not null;comment:对话CSeq" json:"cSeq"`

	RemoteDisplayName string `gorm:"column:remoteDisplayName;type:varchar(255);not null;comment:对话远端显示名称" json:"remoteDisplayName"`
	RemoteId          string `gorm:"column:remoteId;type:varchar(50);not null;comment:对话远端ID" json:"remoteId"`
	RemoteIp          string `gorm:"column:remoteIp;type:varchar(255);not null;comment:对话远端IP" json:"remoteIp"`
	RemotePort        int32  `gorm:"column:remotePort;not null;comment:对话远端端口" json:"remotePort"`
	RemoteDomain      string `gorm:"column:remoteDomain;not null;comment:对话远端域" json:"remoteDomain"`
	Transport         string `gorm:"column:transport;type:varchar(20);not null;comment:SIP传输协议，UDP/TCP" json:"transport"`
}

var (
	StreamModel    = new(Stream)
	StreamFieldMap = GetFieldMap(StreamModel)
)

func (s *Stream) Match(callId string, fromTag, toTag string) bool {
	return s.CallId == callId && s.FromTag == fromTag && s.ToTag == toTag
}
