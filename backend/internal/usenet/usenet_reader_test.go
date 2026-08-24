package usenet

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/acomagu/bufpipe"
	"github.com/javi11/nntppool"
	"go.uber.org/mock/gomock"
)

func TestFetchSegmentBodyKeepsWriterSnapshotAcrossRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	cp := nntppool.NewMockUsenetConnectionPool(ctrl)
	cp.EXPECT().GetMetricsSnapshot().Return(nntppool.PoolMetricsSnapshot{}).AnyTimes()

	pipeReader, pipeWriter := bufpipe.New(nil)
	seg := &segment{
		Id:     "<retry@usenet>",
		groups: []string{"alt.test"},
		reader: pipeReader,
		writer: pipeWriter,
	}

	writerSnapshot := seg.Writer()
	if writerSnapshot == nil {
		t.Fatal("expected an open segment writer")
	}

	firstAttempt := cp.EXPECT().Body(
		gomock.Any(),
		seg.Id,
		writerSnapshot,
		seg.groups,
	).DoAndReturn(func(context.Context, string, io.Writer, []string) (int64, error) {
		if err := seg.Close(); err != nil {
			t.Fatalf("segment.Close() error = %v", err)
		}
		return 0, io.ErrClosedPipe
	})
	cp.EXPECT().Body(
		gomock.Any(),
		seg.Id,
		writerSnapshot,
		seg.groups,
	).After(firstAttempt).DoAndReturn(func(_ context.Context, _ string, w io.Writer, _ []string) (int64, error) {
		actual, ok := w.(*bufpipe.PipeWriter)
		if !ok || actual == nil {
			t.Fatalf("retry received a nil pipe writer: %#v", w)
		}
		if actual != writerSnapshot {
			t.Fatal("retry did not reuse the original writer snapshot")
		}
		return 17, nil
	})

	reader := &usenetReader{
		id:  1,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	bytesFetched, err := reader.fetchSegmentBody(
		context.Background(),
		cp,
		seg.Id,
		writerSnapshot,
		seg.groups,
	)
	if err != nil {
		t.Fatalf("fetchSegmentBody() error = %v", err)
	}
	if bytesFetched != 17 {
		t.Fatalf("fetchSegmentBody() bytes = %d, want 17", bytesFetched)
	}
	if seg.Writer() != nil {
		t.Fatal("expected segment.Close() to clear the stored writer")
	}
}
