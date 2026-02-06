package e2e_test

import (
	"bytes"
	"encoding/json"
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

	Context("inventory verification", func() {
		It("should build inventory with correct VM counts", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateValidTestExcel()
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

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

			excelContent, err := CreateValidTestExcel()
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

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

			excelContent, err := CreateValidTestExcel()
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

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

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})

		It("should have resource breakdowns that sum correctly", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateValidTestExcel()
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

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

			excelContent, err := CreateMultiClusterTestExcel()
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

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

			excelContent, err := CreateMultiClusterTestExcel()
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

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

		It("should order clusters by VM count (biggest to smallest)", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			// skipJSONValue skips a JSON value (object, array, string, number, bool, null)
			// by reading tokens recursively to handle nested structures
			var skipJSONValue func(*json.Decoder) error
			skipJSONValue = func(decoder *json.Decoder) error {
				token, err := decoder.Token()
				if err != nil {
					return err
				}

				switch t := token.(type) {
				case json.Delim:
					if t == '{' {
						// Skip object: read all key-value pairs
						for decoder.More() {
							// Skip key
							_, err = decoder.Token()
							if err != nil {
								return err
							}
							// Skip value (recursive)
							if err = skipJSONValue(decoder); err != nil {
								return err
							}
						}
						// Read closing brace
						_, err = decoder.Token()
						return err
					} else if t == '[' {
						// Skip array: read all elements
						for decoder.More() {
							if err = skipJSONValue(decoder); err != nil {
								return err
							}
						}
						// Read closing bracket
						_, err = decoder.Token()
						return err
					}
				}
				// For primitive values (string, number, bool, null), we've already read the token
				return nil
			}

			excelContent, err := CreateMultiClusterTestExcelWithDifferentVMCounts()
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

			assessment, err = svc.CreateAssessmentFromRvtools("cluster-ordering-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())
			defer func() { _ = svc.RemoveAssessment(assessment.Id) }()

			// Verify inventory
			Expect(assessment.Snapshots).NotTo(BeEmpty())
			inventory := assessment.Snapshots[0].Inventory
			Expect(inventory.Vcenter.Vms.Total).To(Equal(9), "vCenter should have 9 VMs total (5+3+1)")

			// Verify clusters are populated
			Expect(inventory.Clusters).To(HaveLen(3), "Should have 3 clusters")

			// Verify VM counts per cluster
			// cluster-large: 5 VMs, cluster-medium: 3 VMs, cluster-small: 1 VM
			clusterVMCounts := make(map[string]int)
			for clusterID, cluster := range inventory.Clusters {
				clusterVMCounts[clusterID] = cluster.Vms.Total
			}

			// Find clusters by VM count (since cluster IDs are Object IDs from vCluster sheet)
			var foundLarge, foundMedium, foundSmall bool
			for clusterID, vmCount := range clusterVMCounts {
				switch vmCount {
				case 5:
					// cluster-large maps to domain-c100 Object ID
					Expect(clusterID).To(ContainSubstring("c100"), fmt.Sprintf("Cluster with 5 VMs should be domain-c100 (cluster-large), got: %s", clusterID))
					foundLarge = true
				case 3:
					// cluster-medium maps to domain-c200 Object ID
					Expect(clusterID).To(ContainSubstring("c200"), fmt.Sprintf("Cluster with 3 VMs should be domain-c200 (cluster-medium), got: %s", clusterID))
					foundMedium = true
				case 1:
					// cluster-small maps to domain-c300 Object ID
					Expect(clusterID).To(ContainSubstring("c300"), fmt.Sprintf("Cluster with 1 VM should be domain-c300 (cluster-small), got: %s", clusterID))
					foundSmall = true
				}
			}

			Expect(foundLarge).To(BeTrue(), "Should find cluster-large (domain-c100) with 5 VMs")
			Expect(foundMedium).To(BeTrue(), "Should find cluster-medium (domain-c200) with 3 VMs")
			Expect(foundSmall).To(BeTrue(), "Should find cluster-small (domain-c300) with 1 VM")

			// Verify sum of cluster VMs equals vCenter total
			var clusterVMTotal int
			for _, vmCount := range clusterVMCounts {
				clusterVMTotal += vmCount
			}
			Expect(clusterVMTotal).To(Equal(9), "Sum of cluster VMs should equal vCenter total")

			// Validate cluster order by fetching the raw JSON response from the API
			// This ensures we test the actual API response which uses OrderedInventory marshaler
			// Get the raw JSON response from the API
			assessmentJSON, err := svc.GetAssessmentJSON(assessment.Id)
			Expect(err).To(BeNil(), "Should be able to get assessment JSON from API")

			// Use json.Decoder to parse JSON and preserve the order of map keys
			// Navigate through the JSON structure to find clusters
			decoder := json.NewDecoder(bytes.NewReader(assessmentJSON))

			// Read the root object
			token, err := decoder.Token()
			Expect(err).To(BeNil(), "Should be able to read JSON token")
			Expect(token).To(Equal(json.Delim('{')), "Root should be a JSON object")

			// Skip to "snapshots" field
			var foundSnapshots bool
			for decoder.More() {
				keyToken, err := decoder.Token()
				Expect(err).To(BeNil(), "Should be able to read key token")
				key, ok := keyToken.(string)
				Expect(ok).To(BeTrue(), "Key should be a string")

				if key == "snapshots" {
					foundSnapshots = true
					// Read the array start
					arrayToken, err := decoder.Token()
					Expect(err).To(BeNil(), "Should be able to read array token")
					Expect(arrayToken).To(Equal(json.Delim('[')), "Snapshots should be an array")
					break
				} else {
					// Skip the value
					err = skipJSONValue(decoder)
					Expect(err).To(BeNil(), "Should be able to skip value")
				}
			}
			Expect(foundSnapshots).To(BeTrue(), "Should find snapshots field")

			// Read the first snapshot object
			snapshotToken, err := decoder.Token()
			Expect(err).To(BeNil(), "Should be able to read snapshot token")
			Expect(snapshotToken).To(Equal(json.Delim('{')), "Snapshot should be an object")

			// Skip to "inventory" field in the snapshot
			var foundInventory bool
			for decoder.More() {
				keyToken, err := decoder.Token()
				Expect(err).To(BeNil(), "Should be able to read key token")
				key, ok := keyToken.(string)
				Expect(ok).To(BeTrue(), "Key should be a string")

				if key == "inventory" {
					foundInventory = true
					// Read the inventory object start
					invToken, err := decoder.Token()
					Expect(err).To(BeNil(), "Should be able to read inventory token")
					Expect(invToken).To(Equal(json.Delim('{')), "Inventory should be an object")
					break
				} else {
					// Skip the value
					err = skipJSONValue(decoder)
					Expect(err).To(BeNil(), "Should be able to skip value")
				}
			}
			Expect(foundInventory).To(BeTrue(), "Should find inventory field")

			// Skip to "clusters" field in the inventory
			var foundClusters bool
			for decoder.More() {
				keyToken, err := decoder.Token()
				Expect(err).To(BeNil(), "Should be able to read key token")
				key, ok := keyToken.(string)
				Expect(ok).To(BeTrue(), "Key should be a string")

				if key == "clusters" {
					foundClusters = true
					// Read the clusters object start
					clustersToken, err := decoder.Token()
					Expect(err).To(BeNil(), "Should be able to read clusters token")
					Expect(clustersToken).To(Equal(json.Delim('{')), "Clusters should be an object")
					break
				} else {
					// Skip the value
					err = skipJSONValue(decoder)
					Expect(err).To(BeNil(), "Should be able to skip value")
				}
			}
			Expect(foundClusters).To(BeTrue(), "Should find clusters field")

			// Extract cluster IDs and their VM counts in the order they appear in JSON
			type clusterOrder struct {
				ID      string
				VMCount int
			}
			var clusterOrderList []clusterOrder

			// Read clusters in order
			for decoder.More() {
				// Read cluster ID (key)
				clusterIDToken, err := decoder.Token()
				Expect(err).To(BeNil(), "Should be able to read cluster ID")
				clusterID, ok := clusterIDToken.(string)
				Expect(ok).To(BeTrue(), "Cluster ID should be a string")

				// Read cluster data (value) - it's an object, so read it
				clusterObjToken, err := decoder.Token()
				Expect(err).To(BeNil(), "Should be able to read cluster object token")
				Expect(clusterObjToken).To(Equal(json.Delim('{')), "Cluster data should be an object")

				// Navigate to "vms" field
				var foundVms bool
				var vmCount int
				for decoder.More() {
					keyToken, err := decoder.Token()
					Expect(err).To(BeNil(), "Should be able to read key token")
					key, ok := keyToken.(string)
					Expect(ok).To(BeTrue(), "Key should be a string")

					if key == "vms" {
						foundVms = true
						// Read the vms object start
						vmsToken, err := decoder.Token()
						Expect(err).To(BeNil(), "Should be able to read vms token")
						Expect(vmsToken).To(Equal(json.Delim('{')), "VMs should be an object")

						// Navigate to "total" field
						for decoder.More() {
							keyToken, err := decoder.Token()
							Expect(err).To(BeNil(), "Should be able to read key token")
							key, ok := keyToken.(string)
							Expect(ok).To(BeTrue(), "Key should be a string")

							if key == "total" {
								// Read the total value
								totalToken, err := decoder.Token()
								Expect(err).To(BeNil(), "Should be able to read total token")
								total, ok := totalToken.(float64)
								Expect(ok).To(BeTrue(), "Total should be a number")
								vmCount = int(total)
								// Skip the rest of the vms object by reading remaining key-value pairs
								for decoder.More() {
									// Read the key token
									_, err = decoder.Token()
									Expect(err).To(BeNil(), "Should be able to read key token")
									// Skip the value - read token and handle nested structures
									err = skipJSONValue(decoder)
									Expect(err).To(BeNil(), "Should be able to skip value")
								}
								// Read the closing brace
								_, err = decoder.Token()
								Expect(err).To(BeNil(), "Should be able to read closing brace")
								break
							} else {
								// Skip the value
								err = skipJSONValue(decoder)
								Expect(err).To(BeNil(), "Should be able to skip value")
							}
						}
						break
					} else {
						// Skip the value
						err = skipJSONValue(decoder)
						Expect(err).To(BeNil(), "Should be able to skip value")
					}
				}
				Expect(foundVms).To(BeTrue(), "Should find vms field")
				// Skip the rest of the cluster object
				for decoder.More() {
					// Read the key
					_, err = decoder.Token()
					Expect(err).To(BeNil(), "Should be able to read key token")
					// Skip the value
					err = skipJSONValue(decoder)
					Expect(err).To(BeNil(), "Should be able to skip value")
				}
				// Read the closing brace
				_, err = decoder.Token()
				Expect(err).To(BeNil(), "Should be able to read closing brace")

				clusterOrderList = append(clusterOrderList, clusterOrder{
					ID:      clusterID,
					VMCount: vmCount,
				})
			}

			// Validate that clusters are ordered by VM count (biggest to smallest)
			// Expected order: 5 VMs -> 3 VMs -> 1 VM
			Expect(len(clusterOrderList)).To(Equal(3), "Should have 3 clusters in order list")

			// Check that VM counts are in descending order
			Expect(clusterOrderList[0].VMCount).To(BeNumerically(">=", clusterOrderList[1].VMCount),
				fmt.Sprintf("First cluster should have >= VMs than second. Got: %d, %d", clusterOrderList[0].VMCount, clusterOrderList[1].VMCount))
			Expect(clusterOrderList[1].VMCount).To(BeNumerically(">=", clusterOrderList[2].VMCount),
				fmt.Sprintf("Second cluster should have >= VMs than third. Got: %d, %d", clusterOrderList[1].VMCount, clusterOrderList[2].VMCount))

			// Verify the exact order: 5 -> 3 -> 1
			Expect(clusterOrderList[0].VMCount).To(Equal(5), "First cluster should have 5 VMs (cluster-large)")
			Expect(clusterOrderList[1].VMCount).To(Equal(3), "Second cluster should have 3 VMs (cluster-medium)")
			Expect(clusterOrderList[2].VMCount).To(Equal(1), "Third cluster should have 1 VM (cluster-small)")

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})

	Context("migration concerns", func() {
		It("should populate migration concerns in inventory", func() {
			zap.S().Infof("============Running test: %s============", CurrentSpecReport().LeafNodeText)

			excelContent, err := CreateExcelWithConcerns()
			Expect(err).To(BeNil())
			tmpFile, err := CreateTempExcelFile(excelContent)
			Expect(err).To(BeNil())
			defer os.Remove(tmpFile)

			assessment, err = svc.CreateAssessmentFromRvtools("concerns-test", tmpFile)
			Expect(err).To(BeNil())
			Expect(assessment).NotTo(BeNil())
			defer func() { _ = svc.RemoveAssessment(assessment.Id) }()

			// Verify total VMs
			Expect(assessment.Snapshots).NotTo(BeEmpty())
			inventory := assessment.Snapshots[0].Inventory
			Expect(inventory.Vcenter.Vms.Total).To(Equal(3))

			// Verify that migratable count and total are not equal when concerns exist
			// At least one VM should not be fully migratable due to template/CBT concerns
			vms := inventory.Vcenter.Vms
			// Template VM should be not migratable (Critical)
			// CBT disabled VM should have warning
			Expect(vms.TotalMigratable).To(BeNumerically("<", vms.Total))

			zap.S().Infof("============Successfully Passed: %s=====", CurrentSpecReport().LeafNodeText)
		})
	})
})
