package service_test

import (
	"context"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
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
		func(input api.EnhancementData, verify func(api.EnhancementData)) {
			saved, err := svc.Save(context.Background(), assessmentID, input)
			Expect(err).To(BeNil())
			verify(*saved)

			fetched, err := svc.Get(context.Background(), assessmentID)
			Expect(err).To(BeNil())
			verify(*fetched)
		},
		Entry("deployed environment", api.EnhancementData{
			DeployedEnvironment: &api.DeployedEnvironmentInput{
				Environment: util.Ptr(api.DeployedEnvironmentInputEnvironment("on_premises")),
			},
		}, func(out api.EnhancementData) {
			Expect(out.DeployedEnvironment).NotTo(BeNil())
			Expect(*out.DeployedEnvironment.Environment).To(Equal(api.DeployedEnvironmentInputEnvironment("on_premises")))
		}),
		Entry("vmware version counts", api.EnhancementData{
			VmwareVersionCounts: &api.VMwareVersionCountsInput{
				PerpetualLicensesCount: util.IntPtr(1),
				CoresLicensingCount:    util.IntPtr(2),
				EnvironmentCount:       util.IntPtr(3),
				OtherNodesCount:        util.IntPtr(4),
			},
		}, func(out api.EnhancementData) {
			Expect(out.VmwareVersionCounts).NotTo(BeNil())
			Expect(*out.VmwareVersionCounts.PerpetualLicensesCount).To(Equal(1))
			Expect(*out.VmwareVersionCounts.CoresLicensingCount).To(Equal(2))
			Expect(*out.VmwareVersionCounts.EnvironmentCount).To(Equal(3))
			Expect(*out.VmwareVersionCounts.OtherNodesCount).To(Equal(4))
		}),
		Entry("active environments with multiple values", api.EnhancementData{
			ActiveEnvironments: &api.ActiveEnvironmentsInput{
				Environments: util.Ptr([]api.ActiveEnvironmentsInputEnvironments{"production", "dev"}),
			},
		}, func(out api.EnhancementData) {
			Expect(out.ActiveEnvironments).NotTo(BeNil())
			Expect(*out.ActiveEnvironments.Environments).To(Equal([]api.ActiveEnvironmentsInputEnvironments{"production", "dev"}))
		}),
		Entry("active environments with empty slice", api.EnhancementData{
			ActiveEnvironments: &api.ActiveEnvironmentsInput{
				Environments: util.Ptr([]api.ActiveEnvironmentsInputEnvironments{}),
			},
		}, func(out api.EnhancementData) {
			Expect(out.ActiveEnvironments).NotTo(BeNil())
			Expect(*out.ActiveEnvironments.Environments).To(BeEmpty())
		}),
		Entry("vmware subscription level", api.EnhancementData{
			VmwareSubscription: &api.VMwareSubscriptionInput{
				Level: util.Ptr([]api.VMwareSubscriptionInputLevel{"vcf", "vsphere"}),
			},
		}, func(out api.EnhancementData) {
			Expect(out.VmwareSubscription).NotTo(BeNil())
			Expect(*out.VmwareSubscription.Level).To(Equal([]api.VMwareSubscriptionInputLevel{"vcf", "vsphere"}))
		}),
		Entry("vsphere core", api.EnhancementData{
			VsphereCore: &api.VsphereCoreInput{
				VmEncryptionEnabled: util.BoolPtr(true),
				VmEncryptionPolicy:  util.ToStrPtr("strict"),
				SrmEnabled:          util.BoolPtr(false),
			},
		}, func(out api.EnhancementData) {
			Expect(out.VsphereCore).NotTo(BeNil())
			Expect(*out.VsphereCore.VmEncryptionEnabled).To(BeTrue())
			Expect(*out.VsphereCore.VmEncryptionPolicy).To(Equal("strict"))
			Expect(*out.VsphereCore.SrmEnabled).To(BeFalse())
		}),
		Entry("nsx features", api.EnhancementData{
			Nsx: &api.NsxInput{
				Features: util.Ptr([]api.NsxInputFeatures{"microsegmentation"}),
			},
		}, func(out api.EnhancementData) {
			Expect(out.Nsx).NotTo(BeNil())
			Expect(*out.Nsx.Features).To(Equal([]api.NsxInputFeatures{"microsegmentation"}))
		}),
		Entry("aria ops features", api.EnhancementData{
			AriaOps: &api.AriaOpsInput{
				Features: util.Ptr([]api.AriaOpsInputFeatures{"performance_analytics"}),
			},
		}, func(out api.EnhancementData) {
			Expect(out.AriaOps).NotTo(BeNil())
			Expect(*out.AriaOps.Features).To(Equal([]api.AriaOpsInputFeatures{"performance_analytics"}))
		}),
		Entry("aria automation features", api.EnhancementData{
			AriaAutomation: &api.AriaAutomationInput{
				Features: util.Ptr([]api.AriaAutomationInputFeatures{"orchestrator"}),
			},
		}, func(out api.EnhancementData) {
			Expect(out.AriaAutomation).NotTo(BeNil())
			Expect(*out.AriaAutomation.Features).To(Equal([]api.AriaAutomationInputFeatures{"orchestrator"}))
		}),
		Entry("aria secure features", api.EnhancementData{
			AriaSecure: &api.AriaSecureInput{
				Features: util.Ptr([]api.AriaSecureInputFeatures{"compliance_monitoring"}),
			},
		}, func(out api.EnhancementData) {
			Expect(out.AriaSecure).NotTo(BeNil())
			Expect(*out.AriaSecure.Features).To(Equal([]api.AriaSecureInputFeatures{"compliance_monitoring"}))
		}),
		Entry("customer details", api.EnhancementData{
			CustomerDetails: &api.CustomerDetailsInput{
				PhysicalLocationsCount: util.IntPtr(5),
				TargetHardware:         util.ToStrPtr("Dell PowerEdge R750"),
			},
		}, func(out api.EnhancementData) {
			Expect(out.CustomerDetails).NotTo(BeNil())
			Expect(*out.CustomerDetails.PhysicalLocationsCount).To(Equal(5))
			Expect(*out.CustomerDetails.TargetHardware).To(Equal("Dell PowerEdge R750"))
		}),
	)

	It("omits every section when no fields are set", func() {
		saved, err := svc.Save(context.Background(), assessmentID, api.EnhancementData{})
		Expect(err).To(BeNil())
		Expect(saved.DeployedEnvironment).To(BeNil())
		Expect(saved.VmwareVersionCounts).To(BeNil())
		Expect(saved.ActiveEnvironments).To(BeNil())
		Expect(saved.VmwareSubscription).To(BeNil())
		Expect(saved.VsphereCore).To(BeNil())
		Expect(saved.Nsx).To(BeNil())
		Expect(saved.AriaOps).To(BeNil())
		Expect(saved.AriaAutomation).To(BeNil())
		Expect(saved.AriaSecure).To(BeNil())
		Expect(saved.CustomerDetails).To(BeNil())
	})

	It("returns empty data for an assessment with nothing saved yet", func() {
		fetched, err := svc.Get(context.Background(), uuid.New())
		Expect(err).To(BeNil())
		Expect(*fetched).To(Equal(api.EnhancementData{}))
	})

	It("clears a section on a subsequent save that omits it", func() {
		_, err := svc.Save(context.Background(), assessmentID, api.EnhancementData{
			DeployedEnvironment: &api.DeployedEnvironmentInput{
				Environment: util.Ptr(api.DeployedEnvironmentInputEnvironment("on_premises")),
			},
		})
		Expect(err).To(BeNil())

		saved, err := svc.Save(context.Background(), assessmentID, api.EnhancementData{
			VsphereCore: &api.VsphereCoreInput{
				SrmEnabled: util.BoolPtr(true),
			},
		})
		Expect(err).To(BeNil())
		Expect(saved.DeployedEnvironment).To(BeNil())
		Expect(saved.VsphereCore).NotTo(BeNil())
		Expect(*saved.VsphereCore.SrmEnabled).To(BeTrue())
	})
})
