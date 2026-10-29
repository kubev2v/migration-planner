package v1alpha1_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"

	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/google/uuid"
	imageapi "github.com/kubev2v/migration-planner/api/v1alpha1/image"
	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
	imageServer "github.com/kubev2v/migration-planner/internal/api/server/image"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/service"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

// Fake tools let the real cache publish an image without a registry or ISO tooling.
var fakeApplianceTools = map[string]string{
	"skopeo": `for arg do if [ "$arg" = "inspect" ]; then echo v0.31.0; exit 0; fi; done
previous=""
for arg do
 if [ "$previous" = "--digestfile" ]; then printf 'sha256:test\n' > "$arg"; fi
 previous="$arg"
 target="$arg"
done
printf 'agent image' > "${target#oci-archive:}"`,
	"osirrox":   `for arg do target="$arg"; done; mkdir -p "$target/images" "$target/isolinux"; printf boot > "$target/images/efiboot.img"`,
	"rsync":     `cp -R "$2". "$3"`,
	"xorriso":   `if [ "$3" = "-toc" ]; then printf "Volume id    : 'label'\n" >&2; exit 0; fi; for arg do work="$arg"; done; cat "$work/images/migration-planner-agent.tar" > "$4"`,
	"isohybrid": `exit 0`,
}

func newApplianceCache(start bool) *service.ApplianceImageCache {
	root := GinkgoT().TempDir()
	tools := filepath.Join(root, "tools")
	Expect(os.Mkdir(tools, 0755)).To(Succeed())
	for name, script := range fakeApplianceTools {
		Expect(os.WriteFile(filepath.Join(tools, name), []byte("#!/bin/sh\nset -eu\n"+script+"\n"), 0755)).To(Succeed())
	}
	GinkgoT().Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))

	cache := service.NewApplianceImageCache(&config.ApplianceImage{Image: "test/appliance", Tag: "latest", CachePath: filepath.Join(root, "cache"), RefreshInterval: "24h", TLSVerify: true}, "base.iso", zap.NewNop())
	DeferCleanup(cache.Stop)
	if start {
		Expect(cache.Start()).To(Succeed())
		Eventually(func() error { _, err := cache.GetImage(); return err }, "10s").Should(Succeed())
	}
	return cache
}

// agentImageRequest returns a context carrying the agent token and HTTP stream, as the agent server does.
func agentImageRequest(method string, sourceID uuid.UUID, header http.Header) (context.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "/api/v1/sources/"+sourceID.String()+"/agent-image", nil)
	for key, values := range header {
		req.Header[key] = values
	}
	recorder := httptest.NewRecorder()
	ctx := auth.NewTokenContext(req.Context(), auth.AgentJWT{SourceID: sourceID.String()})
	ctx = context.WithValue(ctx, image.ResponseWriterKey, http.ResponseWriter(recorder))
	ctx = context.WithValue(ctx, image.RequestKey, req)
	return ctx, recorder
}

var _ = Describe("agent image handlers", func() {
	sourceID := uuid.New()

	It("serves the agent image as an OCI archive with its digest as ETag", func() {
		srv := handlers.NewAgentHandler(nil, newApplianceCache(true))
		ctx, recorder := agentImageRequest(http.MethodGet, sourceID, nil)

		resp, err := srv.GetAgentImage(ctx, agentServer.GetAgentImageRequestObject{Id: sourceID})
		Expect(err).To(BeNil())
		Expect(resp).To(BeNil())
		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(recorder.Header().Get("ETag")).To(Equal(`"sha256:test"`))
		Expect(recorder.Header().Get("Content-Type")).To(Equal("application/x-tar"))
		Expect(recorder.Body.String()).To(Equal("agent image"))
	})

	It("returns 304 when the agent already runs the cached image", func() {
		srv := handlers.NewAgentHandler(nil, newApplianceCache(true))
		ctx, recorder := agentImageRequest(http.MethodGet, sourceID, http.Header{"If-None-Match": {`"sha256:test"`}})

		_, err := srv.GetAgentImage(ctx, agentServer.GetAgentImageRequestObject{Id: sourceID})
		Expect(err).To(BeNil())
		Expect(recorder.Code).To(Equal(http.StatusNotModified))
		Expect(recorder.Body.Len()).To(BeZero())
	})

	It("forbids an agent from another source", func() {
		srv := handlers.NewAgentHandler(nil, newApplianceCache(false))
		ctx, _ := agentImageRequest(http.MethodGet, uuid.New(), nil)

		resp, err := srv.GetAgentImage(ctx, agentServer.GetAgentImageRequestObject{Id: sourceID})
		Expect(err).To(BeNil())
		Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(agentServer.GetAgentImage403JSONResponse{}).String()))
	})

	It("returns 503 until the agent image is cached", func() {
		srv := handlers.NewAgentHandler(nil, newApplianceCache(false))
		ctx, _ := agentImageRequest(http.MethodGet, sourceID, nil)

		resp, err := srv.GetAgentImage(ctx, agentServer.GetAgentImageRequestObject{Id: sourceID})
		Expect(err).To(BeNil())
		Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(agentServer.GetAgentImage503JSONResponse{}).String()))
	})

	It("rejects an invalid presigned token", func() {
		srv := handlers.NewImageHandler(nil, nil, newApplianceCache(false))

		resp, err := srv.GetAgentImageByToken(context.TODO(), imageServer.GetAgentImageByTokenRequestObject{Token: "invalid"})
		Expect(err).To(BeNil())
		Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(imageServer.GetAgentImageByToken401JSONResponse{}).String()))
	})

	It("routes the presigned agent image path to the agent image operation, not the OVA one", func() {
		swagger, err := imageapi.GetSwagger()
		Expect(err).To(BeNil())
		swagger.Servers = nil
		router, err := gorillamux.NewRouter(swagger)
		Expect(err).To(BeNil())

		route, _, err := router.FindRoute(httptest.NewRequest(http.MethodGet, "/api/v1/image/bytoken/token/agent-image", nil))
		Expect(err).To(BeNil())
		Expect(route.Operation.OperationID).To(Equal("GetAgentImageByToken"))
		route, _, err = router.FindRoute(httptest.NewRequest(http.MethodGet, "/api/v1/image/bytoken/token/source.ova", nil))
		Expect(err).To(BeNil())
		Expect(route.Operation.OperationID).To(Equal("GetImageByToken"))
	})
})
