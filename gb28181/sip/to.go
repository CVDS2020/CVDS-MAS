package sip

type To struct {
	Address Address    `json:"address"`
	Tag     string     `json:"tag,omitempty"`
	Params  Parameters `json:"params,omitempty"`
}
