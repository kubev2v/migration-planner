package e2e_test

import (
	"fmt"
	"os"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/service"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("e2e-rvtools", func() {
	var (
		svc        PlannerService
		assessment *v1alpha1.Assessment
		err        error
		startTime  time.Time
	)

	BeforeEach(func() {
		startTime = time.Now()
		TestOptions.DisconnectedEnvironment = false

		svc, err = DefaultPlannerService()
		Expect(err).To(BeNil(), "Failed to create PlannerService")

		if assessment != nil && svc != nil { // Ensure cleanup on failure to avoid DB leftovers
			_ = svc.RemoveAssessment(assessment.Id)
			assessment = nil
		}
	})

	AfterEach(func() {
		//zap.S().Info("Cleaning up after test...")
		testDuration := time.Since(startTime)
		zap.S().Infof("Test completed in: %s\n", testDuration.String())
		TestsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	Context("rvtools", func() {
		It("Create an assessment from a rvtools file", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Generate a valid example Excel file that meets validation requirements
			exampleContent, err := CreateExample1Excel()
			Expect(err).To(BeNil(), "Failed to generate example Excel file")
			tmpFile, err := CreateTempExcelFile(exampleContent)
			Expect(err).To(BeNil(), "Failed to create temporary Excel file")
			defer os.Remove(tmpFile)

			// Create Assessment
			assessment, err = svc.CreateAssessmentFromRvtools("assessment", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())

			// Change The Assessment name
			assessment, err = svc.UpdateAssessment(assessment.Id, "assessment1")
			Expect(err).To(BeNil())
			Expect(assessment.Name).To(Equal("assessment1"))

			// Delete the Assessment
			err = svc.RemoveAssessment(assessment.Id)
			Expect(err).To(BeNil())

			// Verify deletion
			_, err = svc.GetAssessment(assessment.Id)
			Expect(err).To(MatchError(ContainSubstring("status: 404")))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("corrupted files", func() {
		It("should reject invalid Excel file", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create a corrupted Excel file
			tmpFile, err := CreateTempExcelFile([]byte("This is not an Excel file content"))
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

			// Attempt to create assessment from corrupted file
			assessment, err = svc.CreateAssessmentFromRvtools("corrupted-test", tmpFile)
			Expect(err).NotTo(BeNil())
			Expect(assessment).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("job failed")))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should reject empty file", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create an empty file
			tmpFile, err := CreateTempExcelFile([]byte{})
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

			// Attempt to create assessment from empty file
			// Empty files are validated immediately at handler level before job creation
			assessment, err = svc.CreateAssessmentFromRvtools("empty-test", tmpFile)
			Expect(err).NotTo(BeNil())
			Expect(assessment).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("status: 400")))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should handle missing sheets", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create Excel file missing required sheets
			missingSheetsContent, err := CreateExcelOnlyWithVInfo()
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(missingSheetsContent)
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

			// Attempt to create assessment from file with missing sheets
			assessment, err = svc.CreateAssessmentFromRvtools("missing-sheets-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())

			// Clean up if successful
			err = svc.RemoveAssessment(assessment.Id)
			Expect(err).To(BeNil())

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		Context("API edge cases", func() {
			It("should reject duplicate assessment names", func() {
				zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

				// Create a valid Excel file
				validContent, err := CreateValidTestExcel()
				Expect(err).To(BeNil())
				tmpFile, err := CreateTempExcelFile(validContent)
				Expect(err).To(BeNil())
				defer os.Remove(tmpFile)

				// Create first assessment
				assessment1, err := svc.CreateAssessmentFromRvtools("duplicate-name-test", tmpFile)
				Expect(err).To(BeNil())
				defer func() {
					_ = svc.RemoveAssessment(assessment1.Id)
				}()
				Expect(assessment1).NotTo(BeNil())

				// Attempt to create second assessment with same name
				assessment2, err := svc.CreateAssessmentFromRvtools("duplicate-name-test", tmpFile)
				Expect(err).NotTo(BeNil())
				Expect(assessment2).To(BeNil())
				Expect(err).To(MatchError(ContainSubstring("job failed")))

				zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
			})

			//It("should validate assessment name format", func() { // Todo: ECOPROJECT-3433
			//	zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)
			//
			//	// Create a valid Excel file
			//	validContent, err := CreateValidTestExcel()
			//	Expect(err).To(BeNil())
			//	tmpFile, err := CreateTempExcelFile(validContent)
			//	Expect(err).To(BeNil())
			//	defer os.Remove(tmpFile)
			//
			//	// Test empty name - validated immediately at handler level
			//	assessment, err = svc.CreateAssessmentFromRvtools("", tmpFile)
			//	Expect(err).NotTo(BeNil())
			//	Expect(assessment).To(BeNil())
			//	Expect(err.Error()).To(ContainSubstring("status: 400"))
			//
			//	// Test very long name - if validation added, would be immediate (400)
			//	// Currently no length validation, so job would be created and might succeed or fail
			//	longName := strings.Repeat("a", 1000)
			//	assessment, err = svc.CreateAssessmentFromRvtools(longName, tmpFile)
			//	Expect(err).NotTo(BeNil())
			//	Expect(assessment).To(BeNil())
			//	// Update this assertion based on actual validation implementation
			//	Expect(err.Error()).To(Or(ContainSubstring("status: 400"), ContainSubstring("job failed")))
			//
			//	// Test name with special characters - if validation added, would be immediate (400)
			//	// Currently no format validation, so job would be created and might succeed or fail
			//	assessment, err = svc.CreateAssessmentFromRvtools("test@#$%^&*()", tmpFile)
			//	Expect(err).NotTo(BeNil())
			//	Expect(assessment).To(BeNil())
			//	// Update this assertion based on actual validation implementation
			//	Expect(err).To(Or(MatchError(ContainSubstring("status: 400")), MatchError(ContainSubstring("job failed"))))
			//
			//	zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
			//})

			It("should handle large file uploads", func() {
				zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

				// Create a large Excel file
				largeContent, err := CreateLargeExcel()
				Expect(err).To(BeNil())
				tmpFile, err := CreateTempExcelFile(largeContent)
				Expect(err).To(BeNil())
				defer os.Remove(tmpFile)

				// Attempt to create assessment from large file
				assessment, err = svc.CreateAssessmentFromRvtools("large-file-test", tmpFile)
				Expect(err).To(BeNil())
				Expect(assessment).NotTo(BeNil())

				// Clean up if successful
				err = svc.RemoveAssessment(assessment.Id)
				Expect(err).To(BeNil())

				zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
			})

			It("should handle concurrent uploads", func() {
				zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

				// Create a valid Excel file
				validContent, err := CreateValidTestExcel()
				Expect(err).To(BeNil())
				tmpFile, err := CreateTempExcelFile(validContent)
				Expect(err).To(BeNil())
				defer os.Remove(tmpFile)

				// Create multiple assessments concurrently (simulate concurrent uploads)
				done := make(chan bool, 3)
				var assessments []*v1alpha1.Assessment
				var errors []error

				for i := 0; i < 3; i++ {
					go func(index int) {
						assessment, err := svc.CreateAssessmentFromRvtools(fmt.Sprintf("concurrent-test-%d", index), tmpFile)
						assessments = append(assessments, assessment)
						errors = append(errors, err)
						done <- true
					}(i)
				}

				// Wait for all goroutines to complete
				for i := 0; i < 3; i++ {
					<-done
				}

				successCount := 0
				for i, err := range errors {
					if err == nil && assessments[i] != nil {
						successCount++

						_ = svc.RemoveAssessment(assessments[i].Id)
					}
				}
				Expect(successCount).To(BeNumerically("==", 3))

				zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
			})
		})
	})
})
