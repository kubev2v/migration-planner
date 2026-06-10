package service_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("agent service", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Context("Update agent status", func() {
		It("successfully creates the agent", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			agentID := uuid.New()

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewAgentService(s)
			agent, created, err := srv.UpdateAgentStatus(ctx, mappers.AgentUpdateForm{
				ID:         agentID,
				Status:     "waiting-for-credentials",
				StatusInfo: "waiting-for-credentials",
				CredUrl:    "creds-url",
				Version:    "version-1",
				SourceID:   sourceID,
			})
			Expect(err).To(BeNil())
			Expect(created).To(BeTrue())
			Expect(agent).ToNot(BeNil())

			count := -1
			tx = gormdb.Raw("SELECT COUNT(*) FROM agents;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			status := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status from agents WHERE id = '%s';", agentID)).Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))

			status_info := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status_info from agents WHERE id = '%s';", agentID)).Scan(&status_info)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))

			credsUrl := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT cred_url from agents WHERE id = '%s';", agentID)).Scan(&credsUrl)
			Expect(tx.Error).To(BeNil())
			Expect(credsUrl).To(Equal("creds-url"))
		})

		It("successfully updates the agent", func() {
			sourceID := uuid.NewString()
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			srv := service.NewAgentService(s)
			agent, created, err := srv.UpdateAgentStatus(context.TODO(), mappers.AgentUpdateForm{
				ID:         agentID,
				Status:     "waiting-for-credentials",
				StatusInfo: "waiting-for-credentials",
				CredUrl:    "creds-url",
				Version:    "version-1",
			})
			Expect(err).To(BeNil())
			Expect(created).To(BeFalse())
			Expect(agent).NotTo(BeNil())

			count := -1
			tx = gormdb.Raw("SELECT COUNT(*) FROM agents;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			status := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status from agents WHERE id = '%s';", agentID)).Scan(&status)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))

			status_info := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT status_info from agents WHERE id = '%s';", agentID)).Scan(&status_info)
			Expect(tx.Error).To(BeNil())
			Expect(status).To(Equal("waiting-for-credentials"))

			credsUrl := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT cred_url from agents WHERE id = '%s';", agentID)).Scan(&credsUrl)
			Expect(tx.Error).To(BeNil())
			Expect(credsUrl).To(Equal("creds-url"))
		})

		It("failed to update agent -- source is missing", func() {
			sourceID := uuid.NewString()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "batman",
				Organization: "wayne_enterprises",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			srv := service.NewAgentService(s)
			_, _, err := srv.UpdateAgentStatus(ctx, mappers.AgentUpdateForm{
				ID:         uuid.New(),
				Status:     string(v1alpha1.AgentStatusWaitingForCredentials),
				StatusInfo: "waiting-for-credentials",
				CredUrl:    "creds-url",
				Version:    "version-1",
				SourceID:   uuid.New(),
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("Update source", func() {
		It("successfully updates the source", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			user := auth.User{
				Username:     "admin",
				Organization: "admin",
			}
			ctx := auth.NewTokenContext(context.TODO(), user)

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "vcenter",
			})

			srv := service.NewAgentService(s)
			_, err := srv.UpdateSourceInventory(ctx, mappers.InventoryUpdateForm{
				SourceID:  sourceID,
				VCenterID: "vcenter",
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())

			// the source should have the agent associated
			source, err := s.Source().Get(ctx, sourceID)
			Expect(err).To(BeNil())

			var inventory v1alpha1.Inventory
			err = json.Unmarshal(source.Inventory, &inventory)
			Expect(err).To(BeNil())
			Expect(inventory.VcenterId).To(Equal("vcenter"))
		})

		It("successfully updates the source - two agents", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			secondAgentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, secondAgentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "vcenter",
			})

			// first agent request
			srv := service.NewAgentService(s)
			_, err := srv.UpdateSourceInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  sourceID,
				VCenterID: "vcenter",
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())

			// the source should have the agent associated
			source, err := s.Source().Get(context.TODO(), sourceID)
			Expect(err).To(BeNil())

			var inventory v1alpha1.Inventory
			err = json.Unmarshal(source.Inventory, &inventory)
			Expect(err).To(BeNil())
			Expect(inventory.VcenterId).To(Equal("vcenter"))

			// second agent request
			_, err = srv.UpdateSourceInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  sourceID,
				VCenterID: "vcenter",
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())
		})

		It("agents not associated with the source are not allowed to update inventory", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			secondSourceID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, secondSourceID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())
			secondAgentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, secondAgentID, "not-connected", "status-info-1", "cred_url-1", secondSourceID))
			Expect(tx.Error).To(BeNil())

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{})

			srv := service.NewAgentService(s)
			// Try to update secondSource with firstAgent (not associated)
			_, err := srv.UpdateSourceInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  secondSourceID,
				AgentID:   firstAgentID, // firstAgent is not associated with secondSource
				Inventory: inventoryJSON,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrAgentUpdateForbidden)
			Expect(ok).To(BeTrue())
		})

		It("updates with a different vCenter are not allowed", func() {
			firstSourceID := uuid.New()
			firstAgentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, firstSourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, firstAgentID, "not-connected", "status-info-1", "cred_url-1", firstSourceID))
			Expect(tx.Error).To(BeNil())

			inventory1JSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "vcenter",
			})

			srv := service.NewAgentService(s)
			_, err := srv.UpdateSourceInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  firstSourceID,
				VCenterID: "vcenter",
				Inventory: inventory1JSON,
			})
			Expect(err).To(BeNil())

			inventory2JSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "anotherVCenterID",
			})

			_, err = srv.UpdateSourceInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  firstSourceID,
				VCenterID: "anotherVCenterID",
				Inventory: inventory2JSON,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidVCenterID)
			Expect(ok).To(BeTrue())
		})

		// Covers the agent lookup branch in UpdateSourceInventory: when the
		// source exists but the agent id does not, we should get back an
		// ErrResourceNotFound (agent flavor), not a generic wrapped error.
		It("returns agent not found when the source exists but the agent id is unknown", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "vcenter",
			})

			srv := service.NewAgentService(s)
			_, err := srv.UpdateSourceInventory(context.TODO(), mappers.InventoryUpdateForm{
				SourceID:  sourceID,
				VCenterID: "vcenter",
				Inventory: inventoryJSON,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("UpdateSource (new endpoint)", func() {
		It("successfully updates the source with update_type=auto", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{
				VcenterId: "vcenter",
			})

			srv := service.NewAgentService(s)
			source, err := srv.UpdateSource(context.TODO(), mappers.SourceInventoryUpdateForm{
				SourceID:  sourceID,
				VCenterID: "vcenter",
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())
			Expect(source).NotTo(BeNil())
			Expect(source.UpdateType).To(Equal("auto"))

			// Verify in database
			updateType := ""
			tx = gormdb.Raw(fmt.Sprintf("SELECT update_type FROM sources WHERE id = '%s';", sourceID)).Scan(&updateType)
			Expect(tx.Error).To(BeNil())
			Expect(updateType).To(Equal("auto"))
		})

		It("returns error when source doesn't exist", func() {
			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{})

			srv := service.NewAgentService(s)
			_, err := srv.UpdateSource(context.TODO(), mappers.SourceInventoryUpdateForm{
				SourceID:  uuid.New(), // Non-existent
				Inventory: inventoryJSON,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		It("returns error for vCenterID mismatch", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			// First update with vcenter-1
			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vcenter-1"})
			srv := service.NewAgentService(s)
			_, err := srv.UpdateSource(context.TODO(), mappers.SourceInventoryUpdateForm{
				SourceID:  sourceID,
				VCenterID: "vcenter-1",
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())

			// Second update with different vCenter
			_, err = srv.UpdateSource(context.TODO(), mappers.SourceInventoryUpdateForm{
				SourceID:  sourceID,
				VCenterID: "vcenter-2",
				Inventory: inventoryJSON,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrInvalidVCenterID)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("UpdateSourceSubset", func() {
		It("successfully creates a new subset with update_type=auto", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			subsetID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Add main inventory to source (required before creating subsets)
			mainInventoryJSON, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vcenter"})
			tx = gormdb.Exec(fmt.Sprintf("UPDATE sources SET inventory = '%s' WHERE id = '%s'", mainInventoryJSON, sourceID))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vcenter"})

			srv := service.NewAgentService(s)
			subset, wasCreated, err := srv.UpdateSourceSubset(context.TODO(), mappers.SourceSubsetUpdateForm{
				ID:        subsetID,
				Name:      "Test Subset",
				SourceID:  sourceID,
				VCenterID: "vcenter",
				VMsCount:  10,
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())
			Expect(subset).NotTo(BeNil())
			Expect(wasCreated).To(BeTrue())
			Expect(subset.ID).To(Equal(subsetID))
			Expect(subset.Name).To(Equal("Test Subset"))
			Expect(subset.VMsCount).To(Equal(10))
			Expect(subset.UpdateType).To(Equal("auto"))
		})

		It("successfully updates an existing subset", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			subsetID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Add main inventory to source (required before creating subsets)
			mainInventoryJSON, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vcenter"})
			tx = gormdb.Exec(fmt.Sprintf("UPDATE sources SET inventory = '%s' WHERE id = '%s'", mainInventoryJSON, sourceID))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vcenter"})

			srv := service.NewAgentService(s)
			// Create first
			_, wasCreated, err := srv.UpdateSourceSubset(context.TODO(), mappers.SourceSubsetUpdateForm{
				ID:        subsetID,
				Name:      "Original Name",
				SourceID:  sourceID,
				VCenterID: "vcenter",
				VMsCount:  5,
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())
			Expect(wasCreated).To(BeTrue())

			// Update
			subset, wasCreated, err := srv.UpdateSourceSubset(context.TODO(), mappers.SourceSubsetUpdateForm{
				ID:        subsetID,
				Name:      "Updated Name",
				SourceID:  sourceID,
				VCenterID: "vcenter",
				VMsCount:  15,
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())
			Expect(wasCreated).To(BeFalse())
			Expect(subset.Name).To(Equal("Updated Name"))
			Expect(subset.VMsCount).To(Equal(15))
		})

		It("returns error when source doesn't exist", func() {
			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{})

			srv := service.NewAgentService(s)
			_, _, err := srv.UpdateSourceSubset(context.TODO(), mappers.SourceSubsetUpdateForm{
				ID:        uuid.New(),
				Name:      "Test",
				SourceID:  uuid.New(), // Non-existent
				Inventory: inventoryJSON,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		It("returns error when source has no main inventory", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			// Create source WITHOUT inventory
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vcenter"})

			srv := service.NewAgentService(s)
			_, _, err := srv.UpdateSourceSubset(context.TODO(), mappers.SourceSubsetUpdateForm{
				ID:        uuid.New(),
				Name:      "Test Subset",
				SourceID:  sourceID,
				VCenterID: "vcenter",
				Inventory: inventoryJSON,
			})
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrSourceInventoryRequired)
			Expect(ok).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("must have main inventory before subsets"))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM source_subset_inventories;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})

	Context("DeleteSourceSubset", func() {
		It("successfully deletes a subset", func() {
			sourceID := uuid.New()
			agentID := uuid.New()
			subsetID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			// Add main inventory to source (required before creating subsets)
			mainInventoryJSON, _ := json.Marshal(v1alpha1.Inventory{})
			tx = gormdb.Exec(fmt.Sprintf("UPDATE sources SET inventory = '%s' WHERE id = '%s'", mainInventoryJSON, sourceID))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			inventoryJSON, _ := json.Marshal(v1alpha1.Inventory{})
			srv := service.NewAgentService(s)
			_, _, err := srv.UpdateSourceSubset(context.TODO(), mappers.SourceSubsetUpdateForm{
				ID:        subsetID,
				Name:      "Test Subset",
				SourceID:  sourceID,
				Inventory: inventoryJSON,
			})
			Expect(err).To(BeNil())

			// Delete
			err = srv.DeleteSourceSubset(context.TODO(), sourceID, subsetID)
			Expect(err).To(BeNil())

			// Verify deleted
			count := 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM source_subset_inventories WHERE id = ?", subsetID).Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("returns error when subset doesn't exist", func() {
			sourceID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, sourceID, "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			srv := service.NewAgentService(s)
			err := srv.DeleteSourceSubset(context.TODO(), sourceID, uuid.New())
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		It("returns error when source doesn't exist", func() {
			srv := service.NewAgentService(s)
			err := srv.DeleteSourceSubset(context.TODO(), uuid.New(), uuid.New())
			Expect(err).ToNot(BeNil())
			_, ok := err.(*service.ErrResourceNotFound)
			Expect(ok).To(BeTrue())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM source_subset_inventories;")
			gormdb.Exec("DELETE FROM agents;")
			gormdb.Exec("DELETE FROM sources;")
		})
	})
})
