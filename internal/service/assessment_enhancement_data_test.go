package service_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AssessmentEnhancementDataService", func() {
	var (
		mockStore    *MockStore
		svc          service.AssessmentEnhancementDataServicer
		assessmentID uuid.UUID
	)

	BeforeEach(func() {
		mockStore = NewMockStore()
		svc = service.NewAssessmentEnhancementDataService(mockStore)
		assessmentID = uuid.New()
	})

	DescribeTable("round-trips every optional section through save and get",
		func(input model.AssessmentEnhancementData, verify func(model.AssessmentEnhancementData)) {
			input.AssessmentID = assessmentID
			saved, err := svc.Save(context.Background(), input)
			Expect(err).To(BeNil())
			verify(*saved)

			fetched, err := svc.Get(context.Background(), assessmentID)
			Expect(err).To(BeNil())
			verify(*fetched)
		},
		Entry("deployed environment", model.AssessmentEnhancementData{
			DeployedEnvEnvironment: util.ToStrPtr("on_premises"),
		}, func(out model.AssessmentEnhancementData) {
			Expect(*out.DeployedEnvEnvironment).To(Equal("on_premises"))
		}),
		Entry("vmware version counts", model.AssessmentEnhancementData{
			VMwareVerPerpetualLicensesCount: util.IntPtr(1),
			VMwareVerCoresLicensingCount:    util.IntPtr(2),
			VMwareVerEnvironmentCount:       util.IntPtr(3),
			VMwareVerOtherNodesCount:        util.IntPtr(4),
		}, func(out model.AssessmentEnhancementData) {
			Expect(*out.VMwareVerPerpetualLicensesCount).To(Equal(1))
			Expect(*out.VMwareVerCoresLicensingCount).To(Equal(2))
			Expect(*out.VMwareVerEnvironmentCount).To(Equal(3))
			Expect(*out.VMwareVerOtherNodesCount).To(Equal(4))
		}),
		Entry("active environments with multiple values", model.AssessmentEnhancementData{
			ActiveEnvEnvironments: model.StringArray{"production", "dev"},
		}, func(out model.AssessmentEnhancementData) {
			Expect(out.ActiveEnvEnvironments).To(Equal(model.StringArray{"production", "dev"}))
		}),
		Entry("active environments with empty slice", model.AssessmentEnhancementData{
			ActiveEnvEnvironments: model.StringArray{},
		}, func(out model.AssessmentEnhancementData) {
			Expect(out.ActiveEnvEnvironments).To(BeEmpty())
		}),
		Entry("vmware subscription level", model.AssessmentEnhancementData{
			VMwareSubLevel: model.StringArray{"vcf", "vsphere"},
		}, func(out model.AssessmentEnhancementData) {
			Expect(out.VMwareSubLevel).To(Equal(model.StringArray{"vcf", "vsphere"}))
		}),
		Entry("vsphere core", model.AssessmentEnhancementData{
			VsphereVmEncryptionEnabled: util.BoolPtr(true),
			VsphereVmEncryptionPolicy:  util.ToStrPtr("strict"),
			VsphereSrmEnabled:          util.BoolPtr(false),
			VsphereNiocEnabled:         util.BoolPtr(true),
			VsphereNiocDetails:         util.ToStrPtr("Storage traffic is prioritized."),
		}, func(out model.AssessmentEnhancementData) {
			Expect(*out.VsphereVmEncryptionEnabled).To(BeTrue())
			Expect(*out.VsphereVmEncryptionPolicy).To(Equal("strict"))
			Expect(*out.VsphereSrmEnabled).To(BeFalse())
			Expect(out.VsphereNiocEnabled).NotTo(BeNil())
			Expect(*out.VsphereNiocEnabled).To(BeTrue())
			Expect(out.VsphereNiocDetails).NotTo(BeNil())
			Expect(*out.VsphereNiocDetails).To(Equal("Storage traffic is prioritized."))
		}),
		Entry("NIOC-only disabled", model.AssessmentEnhancementData{
			VsphereNiocEnabled: util.BoolPtr(false),
		}, func(out model.AssessmentEnhancementData) {
			Expect(out.VsphereNiocEnabled).NotTo(BeNil())
			Expect(*out.VsphereNiocEnabled).To(BeFalse())
			Expect(out.VsphereNiocDetails).To(BeNil())
		}),
		Entry("nsx features", model.AssessmentEnhancementData{
			NsxFeatures: model.StringArray{"microsegmentation"},
		}, func(out model.AssessmentEnhancementData) {
			Expect(out.NsxFeatures).To(Equal(model.StringArray{"microsegmentation"}))
		}),
		Entry("aria ops features", model.AssessmentEnhancementData{
			AriaOpsFeatures: model.StringArray{"performance_analytics"},
		}, func(out model.AssessmentEnhancementData) {
			Expect(out.AriaOpsFeatures).To(Equal(model.StringArray{"performance_analytics"}))
		}),
		Entry("aria automation features", model.AssessmentEnhancementData{
			AriaAutomationFeatures: model.StringArray{"orchestrator"},
		}, func(out model.AssessmentEnhancementData) {
			Expect(out.AriaAutomationFeatures).To(Equal(model.StringArray{"orchestrator"}))
		}),
		Entry("aria secure features", model.AssessmentEnhancementData{
			AriaSecureFeatures: model.StringArray{"compliance_monitoring"},
		}, func(out model.AssessmentEnhancementData) {
			Expect(out.AriaSecureFeatures).To(Equal(model.StringArray{"compliance_monitoring"}))
		}),
		Entry("customer details", model.AssessmentEnhancementData{
			CustomerPhysicalLocationsCount: util.IntPtr(5),
			CustomerTargetHardware:         util.ToStrPtr("Dell PowerEdge R750"),
		}, func(out model.AssessmentEnhancementData) {
			Expect(*out.CustomerPhysicalLocationsCount).To(Equal(5))
			Expect(*out.CustomerTargetHardware).To(Equal("Dell PowerEdge R750"))
		}),
	)

	It("omits every section when no fields are set", func() {
		saved, err := svc.Save(context.Background(), model.AssessmentEnhancementData{AssessmentID: assessmentID})
		Expect(err).To(BeNil())
		Expect(saved.DeployedEnvEnvironment).To(BeNil())
		Expect(saved.VMwareVerPerpetualLicensesCount).To(BeNil())
		Expect(saved.ActiveEnvEnvironments).To(BeNil())
		Expect(saved.VMwareSubLevel).To(BeNil())
		Expect(saved.VsphereVmEncryptionEnabled).To(BeNil())
		Expect(saved.NsxFeatures).To(BeNil())
		Expect(saved.AriaOpsFeatures).To(BeNil())
		Expect(saved.AriaAutomationFeatures).To(BeNil())
		Expect(saved.AriaSecureFeatures).To(BeNil())
		Expect(saved.CustomerPhysicalLocationsCount).To(BeNil())
	})

	It("returns empty data for an assessment with nothing saved yet", func() {
		fetched, err := svc.Get(context.Background(), uuid.New())
		Expect(err).To(BeNil())
		Expect(*fetched).To(Equal(model.AssessmentEnhancementData{}))
	})

	It("clears a section on a subsequent save that omits it", func() {
		_, err := svc.Save(context.Background(), model.AssessmentEnhancementData{AssessmentID: assessmentID,
			DeployedEnvEnvironment: util.ToStrPtr("on_premises"),
		})
		Expect(err).To(BeNil())

		saved, err := svc.Save(context.Background(), model.AssessmentEnhancementData{AssessmentID: assessmentID,
			VsphereSrmEnabled: util.BoolPtr(true),
		})
		Expect(err).To(BeNil())
		Expect(saved.DeployedEnvEnvironment).To(BeNil())
		Expect(*saved.VsphereSrmEnabled).To(BeTrue())
	})
})
