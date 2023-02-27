package file

import "time"

func curLocationName() string {
	return time.Local.String()
}
