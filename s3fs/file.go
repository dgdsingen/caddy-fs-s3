package s3fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"time"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var (
	_ fs.File     = (*file)(nil)
	_ fs.FileInfo = (*fileInfo)(nil)
	_ io.Seeker   = (*file)(nil)
)

type file struct {
	cl     Client
	bucket string
	name   string

	rc io.ReadCloser

	stat   func() (fs.FileInfo, error)
	size   int64
	offset int64
	eTag   string
}

func openFile(cl Client, bucket string, name string) (fs.File, error) {
	head, err := cl.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: &bucket,
		Key:    &name,
	})
	if err != nil {
		return nil, err
	}

	var etag string
	if head.ETag != nil {
		etag = *head.ETag
	}
	size := derefInt64(head.ContentLength)
	modTime := derefTime(head.LastModified)

	return &file{
		cl:     cl,
		bucket: bucket,
		name:   name,
		size:   size,
		offset: 0,
		eTag:   etag,
		stat: func() (fs.FileInfo, error) {
			return &fileInfo{
				name:    path.Base(name),
				size:    size,
				modTime: modTime,
			}, nil
		},
	}, nil
}

func (f *file) open() error {
	if f.rc != nil {
		return nil
	}
	if f.offset >= f.size {
		f.rc = io.NopCloser(eofReader{})
		return nil
	}

	in := &s3.GetObjectInput{
		Bucket: &f.bucket,
		Key:    &f.name,
	}
	if f.offset > 0 {
		in.Range = new(fmt.Sprintf("bytes=%d-", f.offset))
		if f.eTag != "" {
			in.IfMatch = &f.eTag
		}
	}

	out, err := f.cl.GetObject(context.Background(), in)
	if err != nil {
		if e := new(awshttp.ResponseError); errors.As(err, &e) {
			if e.HTTPStatusCode() == http.StatusPreconditionFailed {
				return fmt.Errorf("s3fs.file.Seek: file has changed while seeking: %w", fs.ErrNotExist)
			}
		}
		return err
	}

	f.rc = out.Body
	return nil
}

func (f *file) Read(p []byte) (int, error) {
	if err := f.open(); err != nil {
		return 0, err
	}
	n, err := f.rc.Read(p)
	f.offset += int64(n)
	return n, err
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	newOffset := f.offset

	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset += offset
	case io.SeekEnd:
		newOffset = f.size + offset
	default:
		return 0, errors.New("s3fs.file.Seek: invalid whence")
	}

	if newOffset < 0 {
		return 0, errors.New("s3fs.file.Seek: seeked to a negative position")
	}

	if newOffset == f.offset {
		return newOffset, nil
	}

	if f.rc != nil {
		if err := f.rc.Close(); err != nil {
			return f.offset, err
		}
		f.rc = nil
	}

	f.offset = newOffset
	return f.offset, nil
}

func (f *file) Stat() (fs.FileInfo, error) { return f.stat() }

func (f *file) Close() error {
	if f.rc != nil {
		return f.rc.Close()
	}
	return nil
}

type fileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (fi fileInfo) Name() string       { return path.Base(fi.name) }
func (fi fileInfo) Size() int64        { return fi.size }
func (fi fileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi fileInfo) ModTime() time.Time { return fi.modTime }
func (fi fileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi fileInfo) Sys() any           { return nil }

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }
