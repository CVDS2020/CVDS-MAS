package storage

import (
	"fmt"
	"time"
)

type FileNameFormatter func(c FileChannel, seq uint64, createTime time.Time, storageType uint32) string

func DefaultFileNameFormatter(c FileChannel, seq uint64, createTime time.Time, storageType uint32) string {
	var suffix string
	if typ := GetStorageType(storageType); typ != nil {
		suffix = "." + typ.Suffix
	}
	return fmt.Sprintf("%s-%s-%d%s", c.Name(), createTime.Format("20060102150405"), seq, suffix)
}

type FileChannel interface {
	Channel

	FileNameFormatter() FileNameFormatter

	SetFileNameFormatter(formatter FileNameFormatter) FileChannel

	Directory() string

	SetDirectory(directory string) FileChannel
}
