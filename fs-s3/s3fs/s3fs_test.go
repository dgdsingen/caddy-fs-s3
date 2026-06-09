package s3fs

import (
	"context"
	"io/fs"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// fakeClient is a test double for the S3 Client interface. Each method
// delegates to a function field so individual tests can program exactly
// the responses they need.
type fakeClient struct {
	headObject  func(ctx context.Context, in *s3.HeadObjectInput) (*s3.HeadObjectOutput, error)
	listObjects func(ctx context.Context, in *s3.ListObjectsInput) (*s3.ListObjectsOutput, error)
	getObject   func(ctx context.Context, in *s3.GetObjectInput) (*s3.GetObjectOutput, error)
}

func (f fakeClient) HeadObject(ctx context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return f.headObject(ctx, in)
}

func (f fakeClient) ListObjects(ctx context.Context, in *s3.ListObjectsInput, _ ...func(*s3.Options)) (*s3.ListObjectsOutput, error) {
	return f.listObjects(ctx, in)
}

func (f fakeClient) GetObject(ctx context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return f.getObject(ctx, in)
}

// notFound returns an error that isNotFoundErr recognizes, simulating a
// missing object so that stat falls through to its directory probe.
func notFound() error {
	return &types.NoSuchKey{}
}

// drain collects the Name of every entry, preserving order.
func drain(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
