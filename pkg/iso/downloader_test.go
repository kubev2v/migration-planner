package iso_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/kubev2v/migration-planner/pkg/iso"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("iso download manager", func() {
	Context("manager", func() {
		It("ok with 1 downloader", func() {
			td := &testDownloader{}
			md := iso.NewDownloaderManager().Register(td)

			ws := &writerSeeker{buffer: bytes.NewBuffer([]byte{})}
			err := md.Download(context.TODO(), ws)
			Expect(err).To(BeNil())
			Expect(td.hasBeenCalled).To(BeTrue())
		})

		It("ok with 2 downloaders -- the second is not called", func() {
			td1 := &testDownloader{}
			td2 := &testDownloader{}
			md := iso.NewDownloaderManager().Register(td1).Register(td2)

			ws := &writerSeeker{buffer: bytes.NewBuffer([]byte{})}
			err := md.Download(context.TODO(), ws)
			Expect(err).To(BeNil())
			Expect(td1.hasBeenCalled).To(BeTrue())
			Expect(td2.hasBeenCalled).To(BeFalse())
		})

		It("2 downloaders -- the first failed and the second is called", func() {
			td1 := &testDownloader{shouldReturnError: true}
			td2 := &testDownloader{}
			md := iso.NewDownloaderManager().Register(td1).Register(td2)

			ws := &writerSeeker{buffer: bytes.NewBuffer([]byte{})}
			err := md.Download(context.TODO(), ws)
			Expect(err).To(BeNil())
			Expect(td1.hasBeenCalled).To(BeTrue())
			Expect(td2.hasBeenCalled).To(BeTrue())
		})

		It("2 downloaders -- both failed", func() {
			td1 := &testDownloader{shouldReturnError: true}
			td2 := &testDownloader{shouldReturnError: true}
			md := iso.NewDownloaderManager().Register(td1).Register(td2)

			ws := &writerSeeker{buffer: bytes.NewBuffer([]byte{})}
			err := md.Download(context.TODO(), ws)
			Expect(err).ToNot(BeNil())
			Expect(td1.hasBeenCalled).To(BeTrue())
			Expect(td2.hasBeenCalled).To(BeTrue())
		})

		It("3 downloaders -- order of calling is ensured", func() {
			td1 := &testDownloader{shouldReturnError: true}
			td2 := &testDownloader{shouldReturnError: true}
			td3 := &testDownloader{}
			md := iso.NewDownloaderManager().Register(td1).Register(td2).Register(td3)

			ws := &writerSeeker{buffer: bytes.NewBuffer([]byte{})}
			err := md.Download(context.TODO(), ws)
			Expect(err).To(BeNil())
			Expect(td1.hasBeenCalled).To(BeTrue())
			Expect(td2.hasBeenCalled).To(BeTrue())
			Expect(td3.hasBeenCalled).To(BeTrue())

			Expect(td1.calledTime.Before(td2.calledTime)).To(BeTrue())
			Expect(td2.calledTime.Before(td3.calledTime)).To(BeTrue())
		})
	})

	Context("manager with http downloader", func() {
		It("one http downloader with two downloads - buffer is rewinded properly", func() {
			// Setup HTTP test server
			handler := newHttpTestServerHandler(false)
			ts := httptest.NewServer(http.HandlerFunc(handler.getHandler()))
			defer ts.Close()

			// Create downloader manager with one HTTP downloader
			httpDownloader1 := iso.NewHttpDownloader(ts.URL, "should give an error")
			httpDownloader2 := iso.NewHttpDownloader(ts.URL, handler.sha256Sum)
			md := iso.NewDownloaderManager().Register(httpDownloader1).Register(httpDownloader2)

			f, err := os.CreateTemp("", "iso-planner")
			if err != nil {
				log.Fatal(err)
			}
			defer os.Remove(f.Name())

			err = md.Download(context.TODO(), f)
			Expect(err).To(BeNil())
			f.Close()

			content, err := os.ReadFile(f.Name())
			Expect(err).To(BeNil())
			Expect(hex.EncodeToString(content)).To(Equal(hex.EncodeToString(handler.testData)))
		})
	})
})

type writerSeeker struct {
	buffer *bytes.Buffer
}

func (w *writerSeeker) Write(p []byte) (int, error) {
	w.buffer.Write(p)
	return len(p), nil
}

func (w *writerSeeker) Seek(offset int64, start int) (int64, error) {
	return 0, nil
}

type testDownloader struct {
	shouldReturnError bool
	hasBeenCalled     bool
	calledTime        time.Time
}

func (t *testDownloader) Get(ctx context.Context, dst io.Writer) error {
	t.hasBeenCalled = true
	defer func() {
		t.calledTime = time.Now()
	}()

	if t.shouldReturnError {
		return errors.New("error downloading")
	}

	testSize := 100
	data, err := t.generate(testSize)
	if err != nil {
		return err
	}

	_, _ = dst.Write(data)

	return nil
}

func (t *testDownloader) Type() string {
	return "test"
}

func (t *testDownloader) generate(size int) ([]byte, error) {
	blk := make([]byte, size)
	_, err := rand.Read(blk)
	if err != nil {
		return []byte{}, err
	}

	return blk, nil
}
