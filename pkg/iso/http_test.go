package iso_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
			handler := newHttpTestServerHandler(false)
			ts := httptest.NewServer(http.HandlerFunc(handler.getHandler()))
			defer ts.Close()

			downloader := iso.NewHttpDownloader(ts.URL, handler.sha256Sum)
			buff := bytes.NewBuffer([]byte{})

			err := downloader.Get(context.TODO(), buff)
			Expect(err).To(BeNil())
			Expect(fmt.Sprintf("%x", buff.Bytes())).To(Equal(fmt.Sprintf("%x", handler.testData)))
			Expect(buff.Len()).To(Equal(100))
		})

		It("failed to download", func() {
			handler := newHttpTestServerHandler(true)
			ts := httptest.NewServer(http.HandlerFunc(handler.getHandler()))
			defer ts.Close()

			downloader := iso.NewHttpDownloader(ts.URL, handler.sha256Sum)
			buff := bytes.NewBuffer([]byte{})

			err := downloader.Get(context.TODO(), buff)
			Expect(err).ToNot(BeNil())
		})

		It("failed to download -- sha256 is different", func() {
			handler := newHttpTestServerHandler(false)
			ts := httptest.NewServer(http.HandlerFunc(handler.getHandler()))
			defer ts.Close()

			downloader := iso.NewHttpDownloader(ts.URL, "some another sha")
			buff := bytes.NewBuffer([]byte{})

			err := downloader.Get(context.TODO(), buff)
			Expect(err).ToNot(BeNil())
			Expect(buff.Len()).To(Equal(100))
		})
	})
})

type httpTestServerHandler struct {
	shouldReturnError bool
	sha256Sum         string
	testData          []byte
}

func newHttpTestServerHandler(shouldReturnError bool) *httpTestServerHandler {
	blk := make([]byte, 100)
	_, _ = rand.Read(blk)
	hasher := sha256.New()
	hasher.Write(blk)

	return &httpTestServerHandler{
		shouldReturnError: shouldReturnError,
		sha256Sum:         hex.EncodeToString(hasher.Sum(nil)),
		testData:          blk,
	}
}

func (h *httpTestServerHandler) getHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.shouldReturnError {
			http.Error(w, "error", http.StatusBadRequest)
		}

		w.Header().Set("Content-Length", fmt.Sprintf("%d", 100))
		_, _ = w.Write(h.testData)
	}
}
