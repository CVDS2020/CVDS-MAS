package sip

type Address struct {
	DisplayName string `json:"displayName,omitempty"`
	URI         string `json:"uri"`
}
