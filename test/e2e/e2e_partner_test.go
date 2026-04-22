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

		// Clean up any partner requests left by regularuser
		_ = userSvc.LeavePartner(group.Id)
		requests, listErr := userSvc.ListPartnerRequests()
		if listErr == nil && requests != nil {
			for _, req := range *requests {
				_ = userSvc.CancelPartnerRequest(req.Id)
			}
		}

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
			Expect(request.RequestStatus).To(Equal(v1alpha1.PartnerRequestStatusPending))

			// User sees the pending request
			requests, err := userSvc.ListPartnerRequests()
			Expect(err).To(BeNil())
			found := false
			for _, r := range *requests {
				if r.Id == request.Id {
					Expect(r.RequestStatus).To(Equal(v1alpha1.PartnerRequestStatusPending))
					found = true
				}
			}
			Expect(found).To(BeTrue())

			// Partner sees no customers yet (pending requests are not customers)
			customers, err := partnerSvc.ListCustomers()
			Expect(err).To(BeNil())
			Expect(*customers).To(HaveLen(0))

			// Partner accepts the request
			updated, statusCode, err := partnerSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestStatusAccepted,
			})
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))
			Expect(updated.RequestStatus).To(Equal(v1alpha1.PartnerRequestStatusAccepted))

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
			// Cancelled request is preserved
			found := false
			for _, r := range *requests {
				if r.Id == request.Id {
					Expect(r.RequestStatus).To(Equal(v1alpha1.PartnerRequestStatusCancelled))
					found = true
				}
			}
			Expect(found).To(BeTrue())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("partner rejects a request with reason", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			reason := "Not a good fit"
			updated, statusCode, err := partnerSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestStatusRejected,
				Reason: &reason,
			})
			Expect(err).To(BeNil())
			Expect(statusCode).To(Equal(http.StatusOK))
			Expect(updated.RequestStatus).To(Equal(v1alpha1.PartnerRequestStatusRejected))
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
				Status: v1alpha1.PartnerRequestStatusRejected,
			})
			Expect(statusCode).To(Equal(http.StatusBadRequest))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("partner removes customer", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			_, _, err = partnerSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestStatusAccepted,
			})
			Expect(err).To(BeNil())

			// User is customer
			identity, err := userSvc.GetIdentity()
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(v1alpha1.IdentityKindCustomer))

			// Partner removes customer
			err = partnerSvc.RemoveCustomer("regularuser")
			Expect(err).To(BeNil())

			// Partner sees no customers (removed customer is no longer accepted)
			customers, err := partnerSvc.ListCustomers()
			Expect(err).To(BeNil())
			Expect(*customers).To(HaveLen(0))

			// User is regular again
			identity, err = userSvc.GetIdentity()
			Expect(err).To(BeNil())
			Expect(identity.Kind).To(Equal(v1alpha1.IdentityKindRegular))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("partner cannot remove a pending request via RemoveCustomer", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			_, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			// Partner tries to remove user who is still pending — should fail
			err = partnerSvc.RemoveCustomer("regularuser")
			Expect(err).ToNot(BeNil())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("Authorization", func() {
		It("non-partner user trying to update request gets 403", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			request, _, err := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(err).To(BeNil())

			// Regular user tries to accept — not a partner, so forbidden
			_, statusCode, _ := userSvc.UpdatePartnerRequest(request.Id, v1alpha1.PartnerRequestUpdate{
				Status: v1alpha1.PartnerRequestStatusAccepted,
			})
			Expect(statusCode).To(Equal(http.StatusForbidden))

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
				Status: v1alpha1.PartnerRequestStatusAccepted,
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
				Status: v1alpha1.PartnerRequestStatusAccepted,
			})
			Expect(err).To(BeNil())

			// Customer tries to create another request — forbidden (not a regular user)
			_, statusCode, _ := userSvc.CreatePartnerRequest(group.Id, partnerRequestCreate)
			Expect(statusCode).To(Equal(http.StatusForbidden))

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
