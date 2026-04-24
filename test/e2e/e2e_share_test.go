package e2e_test

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/service"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("e2e-share", func() {
	var (
		adminSvc    PlannerService
		customerSvc PlannerService
		partnerSvc  PlannerService
		group       *v1alpha1.Group
		assessment  *v1alpha1.Assessment
		err         error
		startTime   time.Time
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
		partnerSvc, err = NewPlannerService(UserAuth("sharepartner", "shareco", DefaultEmailDomain))
		Expect(err).To(BeNil())

		// Customer service — regular user who will become a customer
		customerSvc, err = NewPlannerService(UserAuth("sharecustomer", "custorg", DefaultEmailDomain))
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
	})

	AfterEach(func() {
		zap.S().Info("Cleaning up after share test...")

		// Clean up assessment
		if assessment != nil {
			_ = customerSvc.RemoveAssessment(assessment.Id)
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
		TestsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	Context("Share lifecycle", func() {
		It("customer shares assessment, partner sees it, then unshares", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Before sharing, customer sees only owner permissions
			customerAssessment, err := customerSvc.GetAssessment(assessment.Id)
			Expect(err).To(BeNil())
			Expect(customerAssessment.Permissions).ToNot(BeNil())
			Expect(findUserPermission(customerAssessment.Permissions, "sharecustomer")).To(BeTrue())

			// Partner cannot see the assessment before sharing
			partnerAssessments, err := partnerSvc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(findAssessment(partnerAssessments, assessment.Id)).To(BeFalse())

			// Customer shares the assessment
			statusCode, err := customerSvc.ShareAssessment(assessment.Id)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			// Partner can now see the assessment
			partnerAssessments, err = partnerSvc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(findAssessment(partnerAssessments, assessment.Id)).To(BeTrue())

			// Customer sees viewer permission for the partner group after sharing
			customerAssessment, err = customerSvc.GetAssessment(assessment.Id)
			Expect(err).To(BeNil())
			Expect(customerAssessment.Permissions).ToNot(BeNil())
			Expect(len(*customerAssessment.Permissions)).To(BeNumerically(">=", 2))

			// Customer unshares the assessment
			statusCode, err = customerSvc.UnshareAssessment(assessment.Id)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			// Partner can no longer see the assessment
			partnerAssessments, err = partnerSvc.GetAssessments()
			Expect(err).To(BeNil())
			Expect(findAssessment(partnerAssessments, assessment.Id)).To(BeFalse())

			// After unsharing, only owner permission remains
			customerAssessment, err = customerSvc.GetAssessment(assessment.Id)
			Expect(err).To(BeNil())
			Expect(customerAssessment.Permissions).ToNot(BeNil())
			Expect(*customerAssessment.Permissions).To(HaveLen(1))
			Expect(findUserPermission(customerAssessment.Permissions, "sharecustomer")).To(BeTrue())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("share is idempotent", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			statusCode, err := customerSvc.ShareAssessment(assessment.Id)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			statusCode, err = customerSvc.ShareAssessment(assessment.Id)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("unshare is idempotent", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			statusCode, err := customerSvc.UnshareAssessment(assessment.Id)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Share authorization", func() {
		It("non-customer cannot share — returns 400", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create a regular user (not a customer)
			regularSvc, err := NewPlannerService(UserAuth("regularshareuser", "regorg", DefaultEmailDomain))
			Expect(err).To(BeNil())

			// Regular user creates their own assessment
			regularAssessment, err := regularSvc.CreateAssessment("regular-assessment", "inventory", nil, inventory)
			Expect(err).To(BeNil())
			DeferCleanup(func() {
				_ = regularSvc.RemoveAssessment(regularAssessment.Id)
			})

			// Regular user tries to share — should get 400 (not a customer)
			statusCode, _ := regularSvc.ShareAssessment(regularAssessment.Id)
			Expect(statusCode).To(Equal(http.StatusBadRequest))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("share non-existent assessment — returns 404", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			statusCode, _ := customerSvc.ShareAssessment(uuid.New())
			Expect(statusCode).To(Equal(http.StatusNotFound))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})

func findAssessment(list *v1alpha1.AssessmentList, id uuid.UUID) bool {
	if list == nil {
		return false
	}
	for _, a := range *list {
		if a.Id == id {
			return true
		}
	}
	return false
}

func findUserPermission(perms *[]v1alpha1.UserPermission, username string) bool {
	if perms == nil {
		return false
	}
	for _, p := range *perms {
		if p.Username == username {
			return true
		}
	}
	return false
}
