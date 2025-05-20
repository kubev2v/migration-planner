package iso_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/kubev2v/migration-planner/pkg/iso"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("http downloader", func() {
	Context("http", func() {
		It("it downloads ok", func() {
			handler := &httpTestServerHandler{}
			ts := httptest.NewServer(http.HandlerFunc(handler.getHandler()))
			defer ts.Close()

			downloader := iso.NewHttpDownloader(ts.URL)
			buff := bytes.NewBuffer([]byte{})

			err := downloader.Get(context.TODO(), buff)
			Expect(err).To(BeNil())
			Expect(buff.Len()).To(Equal(100))
		})

		It("failed to download", func() {
			handler := &httpTestServerHandler{shouldReturnError: true}
			ts := httptest.NewServer(http.HandlerFunc(handler.getHandler()))
			defer ts.Close()

			downloader := iso.NewHttpDownloader(ts.URL)
			buff := bytes.NewBuffer([]byte{})

			err := downloader.Get(context.TODO(), buff)
			Expect(err).ToNot(BeNil())
		})

		It("failed to download -- incomplete download", func() {
			handler := &httpTestServerHandler{shouldReturnIncompleteDownload: true}
			ts := httptest.NewServer(http.HandlerFunc(handler.getHandler()))
			defer ts.Close()

			downloader := iso.NewHttpDownloader(ts.URL)
			buff := bytes.NewBuffer([]byte{})

			err := downloader.Get(context.TODO(), buff)
			Expect(err).ToNot(BeNil())
			Expect(buff.Len()).To(Equal(100))
		})
	})
})

type httpTestServerHandler struct {
	shouldReturnError              bool
	shouldReturnIncompleteDownload bool
}

func (h *httpTestServerHandler) getHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.shouldReturnError {
			http.Error(w, "error", http.StatusBadRequest)
		}
		blk := make([]byte, 100)
		_, err := rand.Read(blk)
		if err != nil {
			http.Error(w, "error", http.StatusBadRequest)
		}

		if h.shouldReturnIncompleteDownload {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", 200))
			_, _ = w.Write(blk)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", 100))
		_, _ = w.Write(blk)
	}
}
