package file

import (
	"errors"
	"gitee.com/sy_183/common/lifecycle"
	"gitee.com/sy_183/common/lock"
	"gitee.com/sy_183/common/log"
	"gitee.com/sy_183/cvds-mas/storage"
	"gitee.com/sy_183/cvds-mas/storage/file/context"
	"gitee.com/sy_183/cvds-mas/storage/meta"
	"os"
)

type ReadSession struct {
	lifecycle.Lifecycle
	runner *lifecycle.DefaultLifecycle

	channel *Channel
	ctx     *context.Context
}

func newReadSession(channel *Channel) *ReadSession {
	s := &ReadSession{channel: channel}
	s.runner = lifecycle.NewWithInterruptedRun(nil, s.run)
	s.Lifecycle = s.runner
	return s
}

func (s *ReadSession) run(_ lifecycle.Lifecycle, interrupter chan struct{}) error {
	<-interrupter
	if s.ctx != nil {
		s.closeContext(s.ctx)
	}
	return nil
}

func (s *ReadSession) closeContext(ctx *context.Context) {
	res := ctx.Close(&context.CloseRequest{})
	if res.Err != nil {
		s.channel.Logger().ErrorWith("关闭文件失败", res.Err, log.String("文件路径", res.File()), log.Duration("花费时间", res.CloseElapse))
	}
	s.channel.Logger().Info("关闭文件成功", log.String("文件路径", res.File()), log.Duration("花费时间", res.CloseElapse))
}

func (s *ReadSession) Read(buf []byte, index storage.Index) (n int, err error) {
	return lock.LockGetDouble(s.runner, func() (n int, err error) {
		if !s.runner.Running() {
			return 0, storage.ReadSessionClosedError
		}
		fileIndex := index.(*meta.Index)
		metaManager := s.channel.metaManager
		var file *meta.File
		ctx := s.ctx
		if ctx != nil {
			file = s.ctx.FileContext().(*meta.File)
		}
		if ctx == nil || file.Seq != fileIndex.FileSeq() {
			newFile := metaManager.GetFile(fileIndex.FileSeq())
			if newFile == nil {
				return 0, errors.New("索引指向的文件已被删除")
			}
			res := context.OpenFile(&context.OpenRequest{
				File:        newFile.Path,
				Flag:        os.O_RDONLY,
				FileContext: newFile,
			})
			if res.Err != nil {
				s.channel.Logger().ErrorWith("打开文件失败", res.Err, log.String("文件路径", res.File()), log.Duration("花费时间", res.Elapse()))
				return 0, res.Err
			}
			s.channel.Logger().Info("打开文件成功", log.String("文件路径", res.File()), log.Duration("花费时间", res.Elapse()))
			if ctx != nil {
				s.closeContext(ctx)
			}
			s.ctx = res.Context
		}
		var seek *context.Seek
		if s.ctx.Offset() != int64(fileIndex.FileOffset()) {
			seek = &context.Seek{Offset: int64(fileIndex.FileOffset())}
		}
		res := s.ctx.Read(&context.ReadRequest{
			Buf:  buf,
			Seek: seek,
			Full: false,
		})
		if res.Err != nil {
			s.channel.Logger().ErrorWith("读取文件失败", res.Err,
				log.String("文件路径", res.File()),
				log.Int("读取大小", len(res.Data)),
				log.Uint64("读取位置", fileIndex.FileOffset()),
				log.Duration("花费时间", res.Elapse()),
			)
			return len(res.Data), res.Err
		}
		s.channel.Logger().Info("读取文件成功",
			log.String("文件路径", res.File()),
			log.Int("读取大小", len(res.Data)),
			log.Uint64("读取位置", fileIndex.FileOffset()),
			log.Duration("花费时间", res.Elapse()),
		)
		return len(res.Data), nil
	})
}
