package main

import (
	"net/http"
	"time"

	"github.com/kubev2v/migration-planner/test/e2e/config"

	"github.com/google/uuid"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e/service"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("e2e-share", func() {
	var (
		adminSvc     PlannerService
		customerSvc  PlannerService
		partnerSvc   PlannerService
		group        *v1alpha1.Group
		assessment   *v1alpha1.AssessmentV2
		assessmentID string
		err          error
		startTime    time.Time
	)

	inventory := &v1alpha1.Inventory{
		VcenterId: "test-vcenter",
		Clusters: map[string]v1alpha1.InventoryData{
			"test-cluster": {
				Vms: v1alpha1.VMs{
					Total:                1,
					TotalMigratable:      1,
					CpuCores:             v1alpha1.VMResourceBreakdown{Total: 4},
					RamGB:                v1alpha1.VMResourceBreakdown{Total: 8},
					DiskGB:               v1alpha1.VMResourceBreakdown{Total: 100},
					DiskCount:            v1alpha1.VMResourceBreakdown{Total: 1},
					PowerStates:          map[string]int{"poweredOn": 1},
					NotMigratableReasons: []v1alpha1.MigrationIssue{},
					MigrationWarnings:    []v1alpha1.MigrationIssue{},
					OsInfo: &map[string]v1alpha1.OsInfo{
						"Red Hat Enterprise Linux 9": {Count: 1, Supported: true},
					},
					DiskComplexityTier: &map[string]v1alpha1.DiskSizeTierSummary{
						"0-10TiB": {VmCount: 1, TotalSizeTB: 0.1},
					},
				},
				Infra: v1alpha1.Infra{
					TotalHosts:      1,
					HostPowerStates: map[string]int{"poweredOn": 1},
					Networks:        []v1alpha1.Network{},
					Datastores:      []v1alpha1.Datastore{},
				},
			},
		},
	}

	BeforeEach(func() {
		startTime = time.Now()
		assessmentID = ""

		// Admin service for setup
		adminSvc, err = DefaultPlannerService()
		Expect(err).To(BeNil())

		// Create a partner group
		group, err = adminSvc.CreateGroup(v1alpha1.GroupCreate{
			Name:        "Share Test Partner",
			Description: "E2E share test partner",
			Kind:        v1alpha1.GroupCreateKindPartner,
			Icon:        "icon",
			Company:     "ShareCo",
		})
		Expect(err).To(BeNil())

		// Add a partner member to the group
		_, err = adminSvc.CreateGroupMember(group.Id, v1alpha1.MemberCreate{
			Username: "sharepartner",
			Email:    "partner@shareco.com",
		})
		Expect(err).To(BeNil())

		// Partner service
		partnerSvc, err = NewPlannerService(UserAuth("sharepartner", "shareco", config.Cfg.Test.DefaultEmailDomain))
		Expect(err).To(BeNil())

		// Customer service — regular user who will become a customer
		customerSvc, err = NewPlannerService(UserAuth("sharecustomer", "custorg", config.Cfg.Test.DefaultEmailDomain))
		Expect(err).To(BeNil())

		// Make user a customer: create request + partner accepts
		request, statusCode, err := customerSvc.CreatePartnerRequest(group.Id, v1alpha1.PartnerRequestCreate{
			Name:         "Share Customer",
			ContactName:  "Jane",
			ContactPhone: "555-9999",
			Email:        "jane@custorg.com",
			Location:     "Boston",
		})
		Expect(err).To(BeNil())
		Expect(statusCode).To(Equal(http.StatusCreated))

		_, statusCode, err = partnerSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
			Status: v1alpha1.PartnerRequestStatusAccepted,
		})
		Expect(err).To(BeNil())
		Expect(statusCode).To(Equal(http.StatusOK))

		// Verify user is now a customer
		identity, err := customerSvc.GetIdentity()
		Expect(err).To(BeNil())
		Expect(identity.Kind).To(Equal(v1alpha1.IdentityKindCustomer))

		// Customer creates an assessment
		assessment, err = customerSvc.CreateAssessment("share-test-assessment", "inventory", nil, inventory)
		Expect(err).To(BeNil())
		Expect(assessment).ToNot(BeNil())

		assessments, err := customerSvc.GetAssessments()
		Expect(err).To(BeNil())
		Expect(assessments).ToNot(BeNil())
		for _, summary := range assessments {
			if summary.Name == "share-test-assessment" {
				assessmentID = summary.Id.String()
				break
			}
		}
		Expect(assessmentID).ToNot(BeEmpty(), "Created assessment must appear in the assessment list")
	})

	AfterEach(func() {
		zap.S().Info("Cleaning up after share test...")

		// Clean up assessment
		if assessmentID != "" {
			_ = customerSvc.RemoveAssessment(assessmentID)
		}

		// Clean up partner relationship
		_ = customerSvc.LeavePartner(group.Id)
		requests, listErr := customerSvc.ListPartnerRequests()
		if listErr == nil && requests != nil {
			for _, req := range *requests {
				_ = customerSvc.CancelPartnerRequest(req.Id)
			}
		}

		_ = adminSvc.DeleteGroupMember(group.Id, "sharepartner")
		_ = adminSvc.DeleteGroup(group.Id)

		testDuration := time.Since(startTime)
		zap.S().Infof("Test completed in: %s\n", testDuration.String())
		config.Cfg.Test.TestsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	Context("Share lifecycle", func() {
		It("customer shares assessment, partner sees it, then unshares", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Before sharing, customer sees owner permissions and isShared=false
			customerAssessment, err := customerSvc.GetAssessmentSummary(assessmentID)
			Expect(err).To(BeNil())
			Expect(customerAssessment.Permissions).ToNot(BeNil())
			Expect(*customerAssessment.Permissions).To(ContainElements(v1alpha1.Read, v1alpha1.Share, v1alpha1.Delete))
			Expect(customerAssessment.Sharing).ToNot(BeNil())
			Expect(customerAssessment.Sharing.IsShared).To(BeFalse())
			Expect(customerAssessment.Sharing.SharedWith).To(BeEmpty())

			// Partner cannot see the assessment before sharing
			partnerAssessments, err := partnerSvc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(findAssessment(partnerAssessments, assessmentID)).To(BeFalse())

			// Customer shares the assessment
			statusCode, err := customerSvc.ShareAssessment(assessmentID)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			// Partner can now see the assessment with read permission
			partnerAssessments, err = partnerSvc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(findAssessment(partnerAssessments, assessmentID)).To(BeTrue())

			// Owner sees sharing state after sharing
			customerAssessment, err = customerSvc.GetAssessmentSummary(assessmentID)
			Expect(err).To(BeNil())
			Expect(customerAssessment.Sharing).ToNot(BeNil())
			Expect(customerAssessment.Sharing.IsShared).To(BeTrue())
			Expect(customerAssessment.Sharing.SharedWith).To(HaveLen(1))
			Expect(customerAssessment.Sharing.SharedWith[0].Type).To(Equal("group"))

			// Customer unshares the assessment
			statusCode, err = customerSvc.UnshareAssessment(assessmentID)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			// Partner can no longer see the assessment
			partnerAssessments, err = partnerSvc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(findAssessment(partnerAssessments, assessmentID)).To(BeFalse())

			// After unsharing, customer still sees owner permissions and isShared=false
			customerAssessment, err = customerSvc.GetAssessmentSummary(assessmentID)
			Expect(err).To(BeNil())
			Expect(customerAssessment.Permissions).ToNot(BeNil())
			Expect(*customerAssessment.Permissions).To(ContainElements(v1alpha1.Read, v1alpha1.Share, v1alpha1.Delete))
			Expect(customerAssessment.Sharing).ToNot(BeNil())
			Expect(customerAssessment.Sharing.IsShared).To(BeFalse())
			Expect(customerAssessment.Sharing.SharedWith).To(BeEmpty())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("share is idempotent", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			statusCode, err := customerSvc.ShareAssessment(assessmentID)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			statusCode, err = customerSvc.ShareAssessment(assessmentID)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("unshare is idempotent", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			statusCode, err := customerSvc.UnshareAssessment(assessmentID)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Partner estimation on shared assessment", func() {
		const clusterID = "test-cluster"

		It("partner calls complexity-estimation on shared assessment", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			statusCode, err := customerSvc.ShareAssessment(assessmentID)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			code, err := partnerSvc.CalculateMigrationComplexity(assessmentID, clusterID)
			Expect(err).To(BeNil())
			Expect(code).To(Equal(http.StatusOK))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("partner calls migration-estimation on shared assessment", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			statusCode, err := customerSvc.ShareAssessment(assessmentID)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			code, err := partnerSvc.CalculateMigrationEstimation(assessmentID, clusterID)
			Expect(err).To(BeNil())
			Expect(code).To(Equal(http.StatusOK))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("partner calls migration-estimation/by-complexity on shared assessment", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			statusCode, err := customerSvc.ShareAssessment(assessmentID)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			code, err := partnerSvc.CalculateMigrationEstimationByComplexity(assessmentID, clusterID)
			Expect(err).To(BeNil())
			Expect(code).To(Equal(http.StatusOK))
			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Share revocation on leave", func() {
		It("customer leaves partner, shared assessment is revoked", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Customer shares the assessment
			statusCode, err := customerSvc.ShareAssessment(assessmentID)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			// Partner can see the assessment
			partnerAssessments, err := partnerSvc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(findAssessment(partnerAssessments, assessmentID)).To(BeTrue())

			// Customer leaves the partner
			err = customerSvc.LeavePartner(group.Id)
			Expect(err).To(BeNil())

			// Partner can no longer see the assessment
			partnerAssessments, err = partnerSvc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(findAssessment(partnerAssessments, assessmentID)).To(BeFalse())
			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Share authorization", func() {
		It("non-customer cannot share — returns 400", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create a regular user (not a customer)
			regularSvc, err := NewPlannerService(UserAuth("regularshareuser", "regorg", config.Cfg.Test.DefaultEmailDomain))
			Expect(err).To(BeNil())

			// Regular user creates their own assessment
			regularAssessment, err := regularSvc.CreateAssessment("regular-assessment", "inventory", nil, inventory)
			Expect(err).To(BeNil())
			Expect(regularAssessment).ToNot(BeNil())
			regularAssessments, err := regularSvc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(regularAssessments).ToNot(BeNil())
			Expect(regularAssessments).To(HaveLen(1))
			regularAssessmentID := (regularAssessments)[0].Id.String()
			DeferCleanup(func() {
				_ = regularSvc.RemoveAssessment(regularAssessmentID)
			})

			// Regular user tries to share — should get 400 (not a customer)
			statusCode, _ := regularSvc.ShareAssessment(regularAssessmentID)
			Expect(statusCode).To(Equal(http.StatusBadRequest))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("share non-existent assessment — returns 404", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			statusCode, _ := customerSvc.ShareAssessment(uuid.NewString())
			Expect(statusCode).To(Equal(http.StatusNotFound))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})

func findAssessment(list []v1alpha1.AssessmentV2Summary, id string) bool {
	if list == nil {
		return false
	}
	for _, a := range list {
		if a.Id.String() == id {
			return true
		}
	}
	return false
}
