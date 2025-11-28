package jobs_test

import (
	"encoding/base64"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubev2v/migration-planner/internal/rvtools/jobs"
)

func TestJobs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Jobs Suite")
}

var _ = Describe("RVToolsArgs", func() {
	Describe("Kind", func() {
		It("returns the correct job kind", func() {
			args := jobs.RVToolsArgs{}
			Expect(args.Kind()).To(Equal("rvtools_parse"))
		})
	})

	Describe("InsertOpts", func() {
		It("returns default insert options", func() {
			args := jobs.RVToolsArgs{}
			opts := args.InsertOpts()
			Expect(opts.Queue).To(Equal(jobs.DefaultQueue))
			Expect(opts.MaxAttempts).To(Equal(jobs.MaxJobRetries))
		})
	})

	Describe("FileData", func() {
		It("stores base64 encoded string", func() {
			testData := []byte("test file content")
			args := jobs.RVToolsArgs{
				OrgID:    "org1",
				Username: "user1",
				FileData: base64.StdEncoding.EncodeToString(testData),
			}
			decoded, err := base64.StdEncoding.DecodeString(args.FileData)
			Expect(err).To(BeNil())
			Expect(decoded).To(Equal(testData))
		})
	})
})

var _ = Describe("RVToolsWorker", func() {
	Describe("Timeout", func() {
		It("returns 5 minute timeout", func() {
			worker := jobs.NewRVToolsWorker(nil)
			Expect(worker.Timeout(nil)).To(Equal(jobs.JobTimeout))
		})
	})
})
