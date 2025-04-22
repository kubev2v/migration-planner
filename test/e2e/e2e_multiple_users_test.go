package e2e_test

import (
	"fmt"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_service"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"strings"
	"time"
)

var _ = Describe("e2e-multiple-users", func() {
	var (
		users         = []string{"user", "admin", "koko"}
		organizations = []string{"redhat", "intel", "apple", "microsoft", "nvidia"}
		svc           PlannerService
		err           error
		startTime     time.Time
	)

	BeforeEach(func() {
		startTime = time.Now()
		TestOptions.DownloadImageByUrl = false
		TestOptions.DisconnectedEnvironment = false

		svc, err = DefaultPlannerService()
		Expect(err).To(BeNil())

		// Iterate over each organization and user to authenticate and create a unique source per org-user pair
		for _, org := range organizations {
			for _, user := range users {
				err = svc.ChangeCredentials(UserAuth(user, org))
				Expect(err).To(BeNil())
				_, err = svc.CreateSource(fmt.Sprintf("%s-%s", org, user))
				Expect(err).To(BeNil())
			}
		}

	})

	AfterEach(func() {
		zap.S().Info("Cleaning up after test...")
		for _, org := range organizations {
			for _, user := range users {
				err = svc.ChangeCredentials(UserAuth(user, org))
				Expect(err).To(BeNil())
				err := svc.RemoveSources()
				Expect(err).To(BeNil(), "Failed to remove sources from DB")
			}
		}
		testDuration := time.Since(startTime)
		zap.S().Infof("Test completed in: %s\n", testDuration.String())
		TestsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	Context("Multiple Users", func() {
		It("Users should see only organizational sources", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Verify that each user sees only the sources created by their own organization
			for _, org := range organizations {
				for _, user := range users {
					err = svc.ChangeCredentials(UserAuth(user, org))
					Expect(err).To(BeNil())
					visibleSources, err := svc.GetSources()
					Expect(err).To(BeNil())
					Expect(*visibleSources).To(HaveLen(len(users)))
					for _, source := range *visibleSources {
						Expect(strings.Split(source.Name, "-")[0]).To(Equal(org))
					}
				}
			}

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})
