package meta

import "time"

func curLocationName() string {
	return time.Local.String()
}
