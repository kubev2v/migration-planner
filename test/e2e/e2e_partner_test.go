package e2e_test

import (
	"net/http"
	"time"

	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/service"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("e2e-partner", func() {
	var (
		adminSvc   PlannerService
		userSvc    PlannerService
		partnerSvc PlannerService
		group      *v1alpha1.Group
		err        error
		startTime  time.Time
	)

	partnerRequestCreate := v1alpha1.PartnerRequestCreate{
		Name:         "Test Company",
		ContactName:  "John Doe",
		ContactPhone: "555-1234",
		Email:        "john@example.com",
		Location:     "New York",
	}

	BeforeEach(func() {
		startTime = time.Now()

		// Admin service for setup
		adminSvc, err = DefaultPlannerService()
		Expect(err).To(BeNil())

		// Create a partner group
		group, err = adminSvc.CreateGroup(v1alpha1.GroupCreate{
			Name:        "Partner Org",
			Description: "E2E test partner",
			Kind:        v1alpha1.GroupCreateKindPartner,
			Icon:        "icon",
			Company:     "Acme",
		})
		Expect(err).To(BeNil())

		// Add a partner member to the group
		_, err = adminSvc.CreateGroupMember(group.Id, v1alpha1.MemberCreate{
			Username: "partneruser",
			Email:    "partner@acme.com",
		})
		Expect(err).To(BeNil())

		// Partner service
		partnerSvc, err = NewPlannerService(UserAuth("partneruser", "acme", DefaultEmailDomain))
		Expect(err).To(BeNil())

		// Regular user service
		userSvc, err = NewPlannerService(UserAuth("regularuser", "userorg", DefaultEmailDomain))
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		zap.S().Info("Cleaning up after partner test...")
		_ = adminSvc.DeleteGroupMember(group.Id, "partneruser")
		_ = adminSvc.DeleteGroup(group.Id)
		testDuration := time.Since(startTime)
		zap.S().Infof("Test completed in: %s\n", testDuration.String())
		TestsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	Context("Partner Request Lifecycle", func() {
		It("full lifecycle: regular -> request -> accept -> customer -> leave -> regular", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// User starts as regular
			identity, err := userSvc.GetIdentity()
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(v1alpha1.IdentityKindRegular))

			// List partners
			partners, err := userSvc.ListPartners()
			Expect(err).To(BeNil())
			Expect(*partners).ToNot(BeEmpty())

			// Create partner request
			request, statusCode, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusCreated))
			Expect(request.RequestStatus).To(Equal(v1alpha1.PartnerRequestRequestStatusAwaiting))

			// User sees the pending request
			requests, err := userSvc.ListPartnerRequests()
			Expect(err).To(BeNil())
			Expect(*requests).To(HaveLen(1))

			// Partner sees the pending request
			customers, err := partnerSvc.ListCustomers()
			Expect(err).To(BeNil())
			Expect(*customers).To(HaveLen(1))
			Expect((*customers)[0].RequestStatus).To(Equal(v1alpha1.PartnerRequestRequestStatusAwaiting))

			// Partner accepts the request
			updated, statusCode, err := partnerSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestUpdateStatusAccepted,
			})
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))
			Expect(updated.RequestStatus).To(Equal(v1alpha1.PartnerRequestRequestStatusAccepted))

			// User is now a customer
			identity, err = userSvc.GetIdentity()
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(v1alpha1.IdentityKindCustomer))
			Expect(identity.PartnerId).ToNot(BeNil())

			// Customer can get partner details
			partnerGroup, err := userSvc.GetPartner(group.Id)
			Expect(err).To(BeNil())
			Expect(partnerGroup.Name).To(Equal("Partner Org"))

			// Customer leaves partner
			err = userSvc.LeavePartner(group.Id)
			Expect(err).To(BeNil())

			// User is regular again
			identity, err = userSvc.GetIdentity()
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(v1alpha1.IdentityKindRegular))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("regular user creates a request and cancels it", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			err = userSvc.CancelPartnerRequest(request.Id)
			Expect(err).To(BeNil())

			requests, err := userSvc.ListPartnerRequests()
			Expect(err).To(BeNil())
			Expect(*requests).To(HaveLen(0))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("partner rejects a request with reason", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			reason := "Not a good fit"
			updated, statusCode, err := partnerSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestUpdateStatusRejected,
				Reason: &reason,
			})
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))
			Expect(updated.RequestStatus).To(Equal(v1alpha1.PartnerRequestRequestStatusRejected))
			Expect(updated.Reason).ToNot(BeNil())
			Expect(*updated.Reason).To(Equal("Not a good fit"))

			// User is still regular after rejection
			identity, err := userSvc.GetIdentity()
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(v1alpha1.IdentityKindRegular))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("partner rejects without reason fails", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			_, statusCode, _ := partnerSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestUpdateStatusRejected,
			})
			Expect(statusCode).To(Equal(http.StatusBadRequest))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("partner removes customer", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			_, _, err = partnerSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestUpdateStatusAccepted,
			})
			Expect(err).To(BeNil())

			// User is customer
			identity, err := userSvc.GetIdentity()
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(v1alpha1.IdentityKindCustomer))

			// Partner removes customer
			err = partnerSvc.RemoveCustomer("regularuser")
			Expect(err).To(BeNil())

			// Partner sees no customers
			customers, err := partnerSvc.ListCustomers()
			Expect(err).To(BeNil())
			Expect(*customers).To(HaveLen(0))

			// User is regular again
			identity, err = userSvc.GetIdentity()
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(v1alpha1.IdentityKindRegular))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Authorization", func() {
		It("non-partner user trying to update request gets 400", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			// Regular user tries to accept — not a partner, so bad request
			_, statusCode, _ := userSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestUpdateStatusAccepted,
			})
			Expect(statusCode).To(Equal(http.StatusBadRequest))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("partner from wrong group gets 403", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create a second partner group + member
			group2, err := adminSvc.CreateGroup(v1alpha1.GroupCreate{
				Name:        "Other Partner",
				Description: "Another partner org",
				Kind:        v1alpha1.GroupCreateKindPartner,
				Icon:        "icon2",
				Company:     "OtherCo",
			})
			Expect(err).To(BeNil())
			DeferCleanup(func() {
				_ = adminSvc.DeleteGroupMember(group2.Id, "otherpartner")
				_ = adminSvc.DeleteGroup(group2.Id)
			})

			_, err = adminSvc.CreateGroupMember(group2.Id, v1alpha1.MemberCreate{
				Username: "otherpartner",
				Email:    "other@otherco.com",
			})
			Expect(err).To(BeNil())

			partnerSvc2, err := NewPlannerService(UserAuth("otherpartner", "otherco", DefaultEmailDomain))
			Expect(err).To(BeNil())

			// User creates request for group1
			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			// Partner from group2 tries to accept — should get 403
			_, statusCode, _ := partnerSvc2.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestUpdateStatusAccepted,
			})
			Expect(statusCode).To(Equal(http.StatusForbidden))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("customer cannot create another partner request", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create and accept request — user becomes customer
			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			_, _, err = partnerSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestUpdateStatusAccepted,
			})
			Expect(err).To(BeNil())

			// Customer tries to create another request — should fail
			_, statusCode, _ := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(statusCode).To(Equal(http.StatusBadRequest))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Duplicate prevention", func() {
		It("cannot create request when one is pending", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			_, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			// Second request should fail
			_, statusCode, _ := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(statusCode).To(Equal(http.StatusBadRequest))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})
