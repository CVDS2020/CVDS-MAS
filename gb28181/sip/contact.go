package sip

type Contact struct {
	Address  Address    `json:"address,omitempty"`
	Expires  *int       `json:"expires,omitempty"`
	QValue   *float64   `json:"qValue,omitempty"`
	WildCard bool       `json:"wildCard,omitempty"`
	Params   Parameters `json:"params,omitempty"`
}
