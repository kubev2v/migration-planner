package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/service"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

const (
	assessmentProcessingTimeout = 120 * time.Second
	assessmentPollInterval      = 500 * time.Millisecond
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

			pwd, err := os.Getwd()
			Expect(err).To(BeNil(), "Failed to get current directory")

			// Create Assessment
			assessment, err = svc.CreateAssessmentFromRvtools("assessment",
				filepath.Join(pwd, "data/example_rvtools_files/example1.xlsx"))
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())

			// Wait for async processing to complete
			assessment, err = svc.WaitForAssessmentProcessing(assessment.Id, assessmentProcessingTimeout, assessmentPollInterval)
			Expect(err).To(BeNil(), "Assessment processing should complete successfully")
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
			Expect(err).To(MatchError(ContainSubstring("status: 400")))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should reject empty file", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create an empty file
			tmpFile, err := CreateTempExcelFile([]byte{})
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

			// Attempt to create assessment from empty file
			assessment, err = svc.CreateAssessmentFromRvtools("empty-test", tmpFile)
			Expect(err).NotTo(BeNil())
			Expect(assessment).To(BeNil())

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

			// Wait for async processing (may fail for missing sheets, which is acceptable)
			_, _ = svc.WaitForAssessmentProcessing(assessment.Id, assessmentProcessingTimeout, assessmentPollInterval)

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
				Expect(err).To(MatchError(ContainSubstring("status: 409")))

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
			//	// Test empty name
			//	assessment, err = svc.CreateAssessmentFromRvtools("", tmpFile)
			//	Expect(err).NotTo(BeNil())
			//	Expect(assessment).To(BeNil())
			//	Expect(err.Error()).To(ContainSubstring("status: 400"))
			//
			//	// Test very long name
			//	longName := strings.Repeat("a", 1000)
			//	assessment, err = svc.CreateAssessmentFromRvtools(longName, tmpFile)
			//	Expect(err).NotTo(BeNil())
			//	Expect(assessment).To(BeNil())
			//	Expect(err.Error()).To(ContainSubstring("status: 400"))
			//
			//	// Test name with special characters
			//	assessment, err = svc.CreateAssessmentFromRvtools("test@#$%^&*()", tmpFile)
			//	Expect(err).NotTo(BeNil())
			//	Expect(assessment).To(BeNil())
			//	Expect(err).To(MatchError(ContainSubstring("status: 400")))
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

				// Wait for async processing to complete
				assessment, err = svc.WaitForAssessmentProcessing(assessment.Id, assessmentProcessingTimeout, assessmentPollInterval)
				Expect(err).To(BeNil(), "Large file processing should complete successfully")
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

				// Create and process assessments concurrently
				type result struct {
					assessment *v1alpha1.Assessment
					err        error
				}
				results := make(chan result, 3)

				for i := 0; i < 3; i++ {
					go func(index int) {
						assessment, err := svc.CreateAssessmentFromRvtools(fmt.Sprintf("concurrent-test-%d", index), tmpFile)
						if err == nil && assessment != nil {
							// Wait for processing to complete
							_, err = svc.WaitForAssessmentProcessing(assessment.Id, assessmentProcessingTimeout, assessmentPollInterval)
						}
						results <- result{assessment: assessment, err: err}
					}(i)
				}

				// Collect results and verify
				successCount := 0
				var assessments []*v1alpha1.Assessment
				for i := 0; i < 3; i++ {
					r := <-results
					if r.err == nil && r.assessment != nil {
						successCount++
						assessments = append(assessments, r.assessment)
					}
				}

				Expect(successCount).To(BeNumerically("==", 3), "All assessments should be created and processed successfully")

				// Clean up
				for _, a := range assessments {
					_ = svc.RemoveAssessment(a.Id)
				}

				zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
			})
		})
	})
})
