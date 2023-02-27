package file

import "time"

const DefaultTimeLayout = "2006-01-02 15:04:05.999999999"

func curLocationOffset() int64 {
	return time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC).Sub(
		time.Date(0, 0, 0, 0, 0, 0, 0, time.Local),
	).Milliseconds()
}

func formatUnixMilli(msec int64, locationName string, locationOffset int64) string {
	return time.UnixMilli(msec).In(time.FixedZone(locationName, int(locationOffset/1000))).Format(DefaultTimeLayout)
}
