package meta

import (
	"strings"
	"syscall"
)

func curLocationName() string {
	var i syscall.Timezoneinformation
	if _, err := syscall.GetTimeZoneInformation(&i); err != nil {
		return "UTC"
	}
	sb := strings.Builder{}
	for _, u := range i.StandardName {
		if u == 0 {
			break
		}
		sb.WriteRune(rune(u))
	}
	return sb.String()
}
