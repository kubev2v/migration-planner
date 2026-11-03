package image_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kubev2v/migration-planner/internal/image"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAgentImage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agent image suite")
}

var _ = Describe("ServeAgentImageTar", func() {
	var (
		dir     string
		tarPath string
		body    = []byte("fake-oci-archive")
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		tarPath = filepath.Join(dir, image.AgentImageTarName)
		Expect(os.WriteFile(tarPath, body, 0o644)).To(Succeed())
	})

	serve := func(r *http.Request) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		Expect(image.ServeAgentImageTar(w, r, filepath.Join(dir, "agent.iso"))).To(Succeed())
		return w
	}

	It("serves the tar next to the ISO with an ETag", func() {
		w := serve(httptest.NewRequest(http.MethodGet, "/agent-image", nil))
		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Body.Bytes()).To(Equal(body))
		Expect(w.Header().Get("ETag")).NotTo(BeEmpty())
		Expect(w.Header().Get("Content-Type")).To(Equal("application/x-tar"))
	})

	It("supports byte ranges and conditional requests", func() {
		w := serve(httptest.NewRequest(http.MethodGet, "/agent-image", nil))
		etag := w.Header().Get("ETag")

		r := httptest.NewRequest(http.MethodGet, "/agent-image", nil)
		r.Header.Set("Range", "bytes=5-8")
		w = serve(r)
		Expect(w.Code).To(Equal(http.StatusPartialContent))
		Expect(w.Body.Bytes()).To(Equal(body[5:9]))

		r = httptest.NewRequest(http.MethodGet, "/agent-image", nil)
		r.Header.Set("If-None-Match", etag)
		w = serve(r)
		Expect(w.Code).To(Equal(http.StatusNotModified))
	})

	It("errors when the tar is missing", func() {
		Expect(os.Remove(tarPath)).To(Succeed())
		w := httptest.NewRecorder()
		err := image.ServeAgentImageTar(w, httptest.NewRequest(http.MethodGet, "/agent-image", nil), filepath.Join(dir, "agent.iso"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("AgentImageTarPath", func() {
	It("resolves next to the served ISO", func() {
		Expect(image.AgentImageTarPath("/iso/agent.iso")).To(Equal("/iso/migration-planner-agent.tar"))
	})
})
