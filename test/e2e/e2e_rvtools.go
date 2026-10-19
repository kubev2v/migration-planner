package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/test/e2e/config"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
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
		config.Cfg.Test.TestsExecutionTime[CurrentSpecReport().LeafNodeText] = testDuration
	})

	Context("rvtools", func() {
		It("Create an assessment from a rvtools file", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Generate a valid example Excel file that meets validation requirements
			exampleContent, err := CreateExample1Excel(uuid.New())
			Expect(err).To(BeNil(), "Failed to generate example Excel file")
			tmpFile, err := CreateTempExcelFile(exampleContent)
			Expect(err).To(BeNil(), "Failed to create temporary Excel file")
			defer func() { _ = os.Remove(tmpFile) }()

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

	Context("assessment v2 RVTools aggregation", func() {
		It("POSTs the initial inventory and PUT aggregates a second vCenter inventory", func() {
			firstVCenterID := uuid.New()
			secondVCenterID := uuid.New()

			firstContent, err := CreateValidTestExcel(firstVCenterID)
			Expect(err).NotTo(HaveOccurred())
			firstFile, err := CreateTempExcelFile(firstContent)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(firstFile) }()

			secondContent, err := CreateMultiClusterTestExcel(secondVCenterID)
			Expect(err).NotTo(HaveOccurred())
			secondFile, err := CreateTempExcelFile(secondContent)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(secondFile) }()

			createJob, err := svc.CreateRVToolsV2Job("v2-rvtools-aggregation-test", firstFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(createJob).NotTo(BeNil())
			Expect(createJob.Status).NotTo(Equal(v1alpha1.JobStatusFailed))

			createJob, err = waitForRVToolsJob(svc, createJob.Id)
			if createJob != nil && createJob.AssessmentId != nil {
				assessmentID := *createJob.AssessmentId
				defer func() { _ = svc.RemoveAssessment(assessmentID) }()
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(createJob.AssessmentId).NotTo(BeNil())
			assessmentID := *createJob.AssessmentId

			initial, err := svc.GetAssessmentV2(assessmentID)
			Expect(err).NotTo(HaveOccurred())
			Expect(initial.Data.Id).To(Equal(firstVCenterID.String()))
			Expect(initial.Data.Data.Vms.Total).To(Equal(2))
			Expect(initial.Data.Data.Vms.CpuCores.Total).To(Equal(6))
			Expect(initial.Data.Data.Vms.RamGB.Total).To(Equal(12))
			Expect(initial.Data.Data.Vms.PowerStates).To(Equal(map[string]int{
				"poweredOn":  1,
				"poweredOff": 1,
			}))
			Expect(initial.Vcenters).To(ConsistOf(v1alpha1.SubResource{Id: firstVCenterID.String()}))

			updateJob, err := svc.UploadRVToolsToAssessment(assessmentID, secondFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(updateJob).NotTo(BeNil())
			Expect(updateJob.Status).NotTo(Equal(v1alpha1.JobStatusFailed))

			updateJob, err = waitForRVToolsJob(svc, updateJob.Id)
			Expect(err).NotTo(HaveOccurred())
			Expect(updateJob.AssessmentId).NotTo(BeNil())
			Expect(*updateJob.AssessmentId).To(Equal(assessmentID))

			aggregated, err := svc.GetAssessmentV2(assessmentID)
			Expect(err).NotTo(HaveOccurred())
			Expect(aggregated.Data.Data.Vms.Total).To(Equal(6))
			Expect(aggregated.Data.Data.Vms.CpuCores.Total).To(Equal(24))
			Expect(aggregated.Data.Data.Vms.RamGB.Total).To(Equal(48))
			Expect(aggregated.Data.Data.Vms.PowerStates).To(Equal(map[string]int{
				"poweredOn":  4,
				"poweredOff": 2,
			}))
			Expect(aggregated.Vcenters).To(ConsistOf(
				v1alpha1.SubResource{Id: firstVCenterID.String()},
				v1alpha1.SubResource{Id: secondVCenterID.String()},
			))
		})

		It("upserts the same vCenter so re-uploading the identical file leaves the aggregate unchanged", func() {
			vCenterID := uuid.New()

			content, err := CreateValidTestExcel(vCenterID)
			Expect(err).NotTo(HaveOccurred())
			file, err := CreateTempExcelFile(content)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(file) }()

			createJob, err := svc.CreateRVToolsV2Job("v2-rvtools-upsert-test", file)
			Expect(err).NotTo(HaveOccurred())
			Expect(createJob).NotTo(BeNil())
			Expect(createJob.Status).NotTo(Equal(v1alpha1.JobStatusFailed))

			createJob, err = waitForRVToolsJob(svc, createJob.Id)
			if createJob != nil && createJob.AssessmentId != nil {
				assessmentID := *createJob.AssessmentId
				defer func() { _ = svc.RemoveAssessment(assessmentID) }()
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(createJob.AssessmentId).NotTo(BeNil())
			assessmentID := *createJob.AssessmentId

			initial, err := svc.GetAssessmentV2(assessmentID)
			Expect(err).NotTo(HaveOccurred())
			Expect(initial.Data.Data.Vms.Total).To(Equal(2))
			Expect(initial.Data.Data.Vms.CpuCores.Total).To(Equal(6))
			Expect(initial.Data.Data.Vms.RamGB.Total).To(Equal(12))
			Expect(initial.Data.Data.Vms.PowerStates).To(Equal(map[string]int{
				"poweredOn":  1,
				"poweredOff": 1,
			}))
			Expect(initial.Vcenters).To(ConsistOf(v1alpha1.SubResource{Id: vCenterID.String()}))

			// Re-upload the identical file: the same vCenter ID must be upserted, not aggregated.
			updateJob, err := svc.UploadRVToolsToAssessment(assessmentID, file)
			Expect(err).NotTo(HaveOccurred())
			Expect(updateJob).NotTo(BeNil())
			Expect(updateJob.Status).NotTo(Equal(v1alpha1.JobStatusFailed))

			updateJob, err = waitForRVToolsJob(svc, updateJob.Id)
			Expect(err).NotTo(HaveOccurred())
			Expect(updateJob.AssessmentId).NotTo(BeNil())
			Expect(*updateJob.AssessmentId).To(Equal(assessmentID))

			// Totals and vCenter list must be identical to the initial upload.
			afterUpsert, err := svc.GetAssessmentV2(assessmentID)
			Expect(err).NotTo(HaveOccurred())
			Expect(afterUpsert.Data.Data.Vms.Total).To(Equal(initial.Data.Data.Vms.Total))
			Expect(afterUpsert.Data.Data.Vms.CpuCores.Total).To(Equal(initial.Data.Data.Vms.CpuCores.Total))
			Expect(afterUpsert.Data.Data.Vms.RamGB.Total).To(Equal(initial.Data.Data.Vms.RamGB.Total))
			Expect(afterUpsert.Data.Data.Vms.PowerStates).To(Equal(initial.Data.Data.Vms.PowerStates))
			Expect(afterUpsert.Vcenters).To(ConsistOf(v1alpha1.SubResource{Id: vCenterID.String()}))
		})

		It("rejects uploading RVTools to an assessment owned by another organization", func() {
			firstVCenterID := uuid.New()

			firstContent, err := CreateValidTestExcel(firstVCenterID)
			Expect(err).NotTo(HaveOccurred())
			firstFile, err := CreateTempExcelFile(firstContent)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(firstFile) }()

			secondContent, err := CreateMultiClusterTestExcel(uuid.New())
			Expect(err).NotTo(HaveOccurred())
			secondFile, err := CreateTempExcelFile(secondContent)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(secondFile) }()

			// Create the assessment as the default user.
			createJob, err := svc.CreateRVToolsV2Job("v2-rvtools-authz-test", firstFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(createJob).NotTo(BeNil())

			createJob, err = waitForRVToolsJob(svc, createJob.Id)
			if createJob != nil && createJob.AssessmentId != nil {
				assessmentID := *createJob.AssessmentId
				defer func() { _ = svc.RemoveAssessment(assessmentID) }()
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(createJob.AssessmentId).NotTo(BeNil())
			assessmentID := *createJob.AssessmentId

			// A user from a different organization must not be able to upload to it.
			otherSvc, err := NewPlannerService(UserAuth("rvtools-intruder", "otherorg", config.Cfg.Test.DefaultEmailDomain))
			Expect(err).NotTo(HaveOccurred())

			updateJob, err := otherSvc.UploadRVToolsToAssessment(assessmentID, secondFile)
			Expect(err).To(HaveOccurred())
			Expect(updateJob).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("403")))

			// The assessment must remain untouched (single vCenter inventory).
			unchanged, err := svc.GetAssessmentV2(assessmentID)
			Expect(err).NotTo(HaveOccurred())
			Expect(unchanged.Vcenters).To(ConsistOf(v1alpha1.SubResource{Id: firstVCenterID.String()}))
		})

		It("rejects uploading RVTools to a nonexistent assessment", func() {
			content, err := CreateMultiClusterTestExcel(uuid.New())
			Expect(err).NotTo(HaveOccurred())
			file, err := CreateTempExcelFile(content)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(file) }()

			updateJob, err := svc.UploadRVToolsToAssessment(uuid.New(), file)
			Expect(err).To(HaveOccurred())
			Expect(updateJob).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("404")))
		})
	})

	Context("corrupted files", func() {
		It("should reject invalid Excel file", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create a corrupted Excel file
			tmpFile, err := CreateTempExcelFile([]byte("This is not an Excel file content"))
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

			// Attempt to create assessment from corrupted file
			assessment, err = svc.CreateAssessmentFromRvtools("corrupted-test", tmpFile)
			Expect(err).NotTo(BeNil())
			Expect(assessment).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("status code: 400")))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should reject empty file", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create an empty file
			tmpFile, err := CreateTempExcelFile([]byte{})
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

			// Attempt to create assessment from empty file
			// Empty files are validated immediately at handler level before job creation
			assessment, err = svc.CreateAssessmentFromRvtools("empty-test", tmpFile)
			Expect(err).NotTo(BeNil())
			Expect(assessment).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("status code: 400")))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should handle missing sheets", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// Create Excel file missing required sheets
			missingSheetsContent, err := CreateExcelOnlyWithVInfo(uuid.New())
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(missingSheetsContent)
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

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
				validContent, err := CreateValidTestExcel(uuid.New())
				Expect(err).To(BeNil())
				tmpFile, err := CreateTempExcelFile(validContent)
				Expect(err).To(BeNil())
				defer func() { _ = os.Remove(tmpFile) }()

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

			It("should validate assessment name format", func() { // Todo: ECOPROJECT-3433
				zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

				// Create a valid Excel file
				validContent, err := CreateValidTestExcel(uuid.New())
				Expect(err).To(BeNil())
				tmpFile, err := CreateTempExcelFile(validContent)
				Expect(err).To(BeNil())
				defer func() { _ = os.Remove(tmpFile) }()

				// Test empty name - validated immediately at handler level
				assessment, err = svc.CreateAssessmentFromRvtools("", tmpFile)
				Expect(err).NotTo(BeNil())
				Expect(assessment).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("status code: 400"))

				// Test very long name - if validation added, would be immediate (400)
				// Currently no length validation, so job would be created and might succeed or fail
				longName := strings.Repeat("a", 1000)
				assessment, err = svc.CreateAssessmentFromRvtools(longName, tmpFile)
				Expect(err).NotTo(BeNil())
				Expect(assessment).To(BeNil())
				// Update this assertion based on actual validation implementation
				Expect(err.Error()).To(Or(ContainSubstring("status code: 400"), ContainSubstring("job failed")))

				// Test name with special characters - if validation added, would be immediate (400)
				// Currently no format validation, so job would be created and might succeed or fail
				assessment, err = svc.CreateAssessmentFromRvtools("test@#$%^&*()", tmpFile)
				Expect(err).NotTo(BeNil())
				Expect(assessment).To(BeNil())
				// Update this assertion based on actual validation implementation
				Expect(err).To(Or(MatchError(ContainSubstring("status code: 400")), MatchError(ContainSubstring("job failed"))))

				zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
			})

			It("should handle large file uploads", func() {
				zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

				// Create a large Excel file
				largeContent, err := CreateLargeExcel(uuid.New())
				Expect(err).To(BeNil())
				tmpFile, err := CreateTempExcelFile(largeContent)
				Expect(err).To(BeNil())
				defer func() { _ = os.Remove(tmpFile) }()

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
				validContent, err := CreateValidTestExcel(uuid.New())
				Expect(err).To(BeNil())
				tmpFile, err := CreateTempExcelFile(validContent)
				Expect(err).To(BeNil())
				defer func() { _ = os.Remove(tmpFile) }()

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

				for _, err := range errors {
					Expect(err).To(BeNil())
				}

				for _, a := range assessments {
					err = svc.RemoveAssessment(a.Id)
					Expect(err).To(BeNil())
				}

				zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
			})
		})
	})

	Context("inventory verification", func() {
		It("should build inventory with correct VM counts", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateValidTestExcel(uuid.New())
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

			assessment, err = svc.CreateAssessmentFromRvtools("inventory-vm-counts-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())
			defer func() { _ = svc.RemoveAssessment(assessment.Id) }()

			// Verify inventory (accessed via Snapshots)
			Expect(assessment.Snapshots).NotTo(BeEmpty())
			inventory := assessment.Snapshots[0].Inventory
			Expect(inventory.Vcenter).NotTo(BeNil())
			Expect(inventory.Vcenter.Vms.Total).To(Equal(2))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should have correct power state distribution", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateValidTestExcel(uuid.New())
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

			assessment, err = svc.CreateAssessmentFromRvtools("inventory-power-states-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())
			defer func() { _ = svc.RemoveAssessment(assessment.Id) }()

			// Verify power states (1 poweredOn, 1 poweredOff from CreateValidTestExcel)
			Expect(assessment.Snapshots).NotTo(BeEmpty())
			inventory := assessment.Snapshots[0].Inventory
			powerStates := inventory.Vcenter.Vms.PowerStates
			Expect(powerStates).NotTo(BeNil())
			Expect(powerStates["poweredOn"]).To(Equal(1))
			Expect(powerStates["poweredOff"]).To(Equal(1))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should populate infrastructure data", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateValidTestExcel(uuid.New())
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

			assessment, err = svc.CreateAssessmentFromRvtools("inventory-infra-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())
			defer func() { _ = svc.RemoveAssessment(assessment.Id) }()

			// Verify infrastructure data
			Expect(assessment.Snapshots).NotTo(BeEmpty())
			inventory := assessment.Snapshots[0].Inventory
			infra := inventory.Vcenter.Infra
			Expect(infra.TotalHosts).To(Equal(2))
			Expect(infra.Hosts).NotTo(BeNil())
			Expect(*infra.Hosts).To(HaveLen(2))
			Expect(infra.Datastores).NotTo(BeEmpty())
			Expect(infra.VmsPerHostAverage).NotTo(BeNil())
			Expect(*infra.VmsPerHostAverage).To(Equal(1.0))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should have resource breakdowns that sum correctly", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateValidTestExcel(uuid.New())
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

			assessment, err = svc.CreateAssessmentFromRvtools("inventory-resources-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())
			defer func() { _ = svc.RemoveAssessment(assessment.Id) }()

			// Verify resource breakdowns (4 + 2 = 6 CPUs from CreateValidTestExcel)
			Expect(assessment.Snapshots).NotTo(BeEmpty())
			inventory := assessment.Snapshots[0].Inventory
			vms := inventory.Vcenter.Vms
			Expect(vms.CpuCores.Total).To(Equal(6))
			// RAM: 8192 + 4096 = 12288 MB = 12 GB
			Expect(vms.RamGB.Total).To(Equal(12))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("multi-cluster inventory", func() {
		It("should handle multiple clusters correctly", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateMultiClusterTestExcel(uuid.New())
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

			assessment, err = svc.CreateAssessmentFromRvtools("multi-cluster-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())
			defer func() { _ = svc.RemoveAssessment(assessment.Id) }()

			// Verify vCenter total matches sum of cluster VMs
			Expect(assessment.Snapshots).NotTo(BeEmpty())
			inventory := assessment.Snapshots[0].Inventory
			Expect(inventory.Vcenter.Vms.Total).To(Equal(4))

			// Verify clusters are populated
			Expect(inventory.Clusters).To(HaveLen(2))

			// Sum of cluster VMs should equal vCenter total
			var clusterVMTotal int
			for _, cluster := range inventory.Clusters {
				clusterVMTotal += cluster.Vms.Total
			}
			Expect(clusterVMTotal).To(Equal(4))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should have consistent cluster totals", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateMultiClusterTestExcel(uuid.New())
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

			assessment, err = svc.CreateAssessmentFromRvtools("cluster-totals-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())
			defer func() { _ = svc.RemoveAssessment(assessment.Id) }()

			// Each cluster should have 2 VMs
			Expect(assessment.Snapshots).NotTo(BeEmpty())
			inventory := assessment.Snapshots[0].Inventory
			for clusterID, cluster := range inventory.Clusters {
				Expect(cluster.Vms.Total).To(Equal(2), fmt.Sprintf("Cluster %s should have 2 VMs", clusterID))
			}

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("migration concerns", func() {
		It("should populate migration concerns in inventory", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateExcelWithConcerns(uuid.New())
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tmpFile) }()

			assessment, err = svc.CreateAssessmentFromRvtools("concerns-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())
			defer func() { _ = svc.RemoveAssessment(assessment.Id) }()

			// Verify total VMs (template VM is excluded at ingestion)
			Expect(assessment.Snapshots).NotTo(BeEmpty())
			inventory := assessment.Snapshots[0].Inventory
			Expect(inventory.Vcenter.Vms.Total).To(Equal(2))

			// Verify that migratable count and total are not equal when concerns exist
			// vm-normal has RDM disk (Critical), vm-cbt-disabled has CBT warning
			vms := inventory.Vcenter.Vms
			Expect(vms.TotalMigratable).To(BeNumerically("<", vms.Total))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})

func waitForRVToolsJob(svc PlannerService, jobID int64) (*v1alpha1.Job, error) {
	deadline := time.Now().Add(time.Minute)
	for {
		job, err := svc.GetJob(jobID)
		if err != nil {
			return nil, fmt.Errorf("get RVTools job %d: %w", jobID, err)
		}

		switch job.Status {
		case v1alpha1.JobStatusCompleted:
			return job, nil
		case v1alpha1.JobStatusFailed:
			if job.Error != nil {
				return job, fmt.Errorf("RVTools job %d failed: %s", jobID, *job.Error)
			}
			return job, fmt.Errorf("RVTools job %d failed", jobID)
		case v1alpha1.JobStatusCancelled:
			return job, fmt.Errorf("RVTools job %d was cancelled", jobID)
		}

		if time.Now().After(deadline) {
			return job, fmt.Errorf("timed out waiting for RVTools job %d", jobID)
		}
		time.Sleep(time.Second)
	}
}
