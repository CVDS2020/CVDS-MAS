package model

const GatewayTableName = "gateway"

type Gateway struct {
	Id   int64  `gorm:"column:id;primaryKey;not null;autoIncrement;comment:主键ID" json:"id"`
	Name string `gorm:"column:name;type:varchar(255);uniqueIndex:nameUniqueIndex;not null;comment:名称" json:"name"`

	GatewayId     string `gorm:"column:gatewayId;type:varchar(50);uniqueIndex:gatewayIdUniqueIndex;not null;comment:SIP网关ID" json:"gatewayId"`
	GatewayDomain string `gorm:"column:gatewayDomain;type:varchar(50);not null;comment:SIP网关域" json:"gatewayDomain"`
	GatewayIp     string `gorm:"column:gatewayIp;type:varchar(50);uniqueIndex:gatewayIpPortUniqueIndex,priority:2;not null;comment:SIP网关IP" json:"gatewayIp"`
	GatewayPort   int32  `gorm:"column:gatewayPort;uniqueIndex:gatewayIpPortUniqueIndex,priority:1;not null;comment:SIP网关端口" json:"gatewayPort"`

	EnableRegister bool   `gorm:"column:enableRegister;not null;comment:开启向网关注册" json:"enableRegister"`
	Password       string `gorm:"column:password;type:varchar(255);not null;comment:注册密码" json:"password"`
	Expires        int32  `gorm:"column:expires;not null;comment:注册过期时间" json:"expires"`

	Transport string `gorm:"column:transport;type:varchar(20);not null;comment:SIP传输协议，UDP/TCP" json:"transport"`
	Charset   string `gorm:"column:charset;type:varchar(20);not null;comment:字符集，UTF-8/GB2312" json:"charset"`

	Status int32 `gorm:"column:status;not null;comment:网关状态，在线: 1，离线：0" json:"status"`

	CreateTime int64 `gorm:"column:createTime;not null;comment:创建时间" json:"createTime"`
	UpdateTime int64 `gorm:"column:updateTime;not null;comment:更新时间" json:"updateTime"`
}

var (
	GatewayModel    = new(Gateway)
	GatewayFieldMap = GetFieldMap(GatewayModel)
)

func (g *Gateway) Clone() *Gateway {
	n := new(Gateway)
	*n = *g
	return n
}
