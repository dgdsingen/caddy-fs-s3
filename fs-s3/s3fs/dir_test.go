package s3fs

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// TestReadDirSkipsFolderMarker verifies that a zero-byte "folder marker"
// object whose key equals the directory's own prefix (e.g. "docs/") is not
// surfaced as a child entry named after the directory itself.
func TestReadDirSkipsFolderMarker(t *testing.T) {
	cl := fakeClient{
		headObject: func(_ context.Context, _ *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
			return nil, notFound()
		},
		listObjects: func(_ context.Context, _ *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
			falsy := false
			return &s3.ListObjectsOutput{
				IsTruncated: &falsy,
				Contents: []types.Object{
					{Key: new("docs/")},      // the folder marker object
					{Key: new("docs/a.txt")}, // a real file
				},
				CommonPrefixes: []types.CommonPrefix{
					{Prefix: new("docs/sub/")},
				},
			}, nil
		},
	}

	entries, err := New(cl, "bucket").ReadDir("docs")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	got := drain(entries)
	want := []string{"a.txt", "sub"}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries = %v, want %v", got, want)
		}
	}
}

// TestReadDirStopsWhenIsTruncatedNil guards against an infinite listing loop
// on S3-compatible stores that omit IsTruncated: a nil value must be treated
// as "not truncated" (no more pages), not as "keep paging".
func TestReadDirStopsWhenIsTruncatedNil(t *testing.T) {
	var pages int
	cl := fakeClient{
		headObject: func(_ context.Context, _ *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
			return nil, notFound()
		},
		listObjects: func(_ context.Context, in *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
			page := &s3.ListObjectsOutput{
				IsTruncated: nil, // store omits the field entirely
				Contents:    []types.Object{{Key: new("docs/a.txt")}},
			}
			if in.MaxKeys != nil {
				return page, nil // stat's directory probe, not a listing page
			}
			pages++
			if pages > 1 {
				// Defensive: break the loop so a regression fails fast
				// instead of hanging the test run.
				return &s3.ListObjectsOutput{}, nil
			}
			return page, nil
		},
	}

	entries, err := New(cl, "bucket").ReadDir("docs")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if pages != 1 {
		t.Fatalf("listing fetched %d pages, want 1 (nil IsTruncated must terminate)", pages)
	}

	got := drain(entries)
	if len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("entries = %v, want [a.txt]", got)
	}
}

// TestReadDirPaginatesWithoutNextMarker covers V1 listings where NextMarker is
// not populated (some stores, and pages with no common prefixes). Pagination
// must advance using the last returned key instead of resetting to nil and
// repeating the first page.
func TestReadDirPaginatesWithoutNextMarker(t *testing.T) {
	var pages int
	var page2Marker string
	cl := fakeClient{
		headObject: func(_ context.Context, _ *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
			return nil, notFound()
		},
		listObjects: func(_ context.Context, in *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
			if in.MaxKeys != nil {
				// stat's directory probe: just confirm the prefix exists.
				return &s3.ListObjectsOutput{
					Contents: []types.Object{{Key: new("docs/a.txt")}},
				}, nil
			}
			pages++
			switch pages {
			case 1:
				truthy := true
				return &s3.ListObjectsOutput{
					IsTruncated: &truthy,
					NextMarker:  nil, // not populated despite truncation
					Contents:    []types.Object{{Key: new("docs/a.txt")}},
				}, nil
			case 2:
				if in.Marker != nil {
					page2Marker = *in.Marker
				}
				falsy := false
				return &s3.ListObjectsOutput{
					IsTruncated: &falsy,
					Contents:    []types.Object{{Key: new("docs/b.txt")}},
				}, nil
			default:
				t.Errorf("listing fetched %d pages, want 2", pages)
				return &s3.ListObjectsOutput{}, nil
			}
		},
	}

	entries, err := New(cl, "bucket").ReadDir("docs")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if page2Marker != "docs/a.txt" {
		t.Fatalf("page 2 marker = %q, want last key %q", page2Marker, "docs/a.txt")
	}

	got := drain(entries)
	want := []string{"a.txt", "b.txt"}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries = %v, want %v", got, want)
		}
	}
}
