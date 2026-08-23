package v1alpha1_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"reflect"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	agentAPI "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("disconnected inventory upload", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
		srv    *handlers.ServiceHandler
		ctx    context.Context
	)

	strPtr := func(v string) *string { return &v }
	intPtr := func(v int) *int { return &v }

	buildUpload := func(fileContent []byte) *multipart.Reader {
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		filePart, _ := w.CreateFormFile("file", "upload")
		_, _ = filePart.Write(fileContent)
		_ = w.Close()
		return multipart.NewReader(&b, w.Boundary())
	}

	buildBundle := func(mainInv v1alpha1.Inventory, subsets map[string]agentAPI.SourceSubsetUpdate) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		mainBytes, _ := json.Marshal(mainInv)
		mw, _ := zw.Create("inventory.json")
		_, _ = mw.Write(mainBytes)
		for id, subset := range subsets {
			sb, _ := json.Marshal(subset)
			sw, _ := zw.Create("subsets/" + id + ".json")
			_, _ = sw.Write(sb)
		}
		_ = zw.Close()
		return buf.Bytes()
	}

	mainInventory := func() v1alpha1.Inventory {
		return v1alpha1.Inventory{
			VcenterId: "test-vcenter",
			Vcenter: &v1alpha1.InventoryData{
				Vms: v1alpha1.VMs{Total: 5},
			},
		}
	}

	insertSource := func() uuid.UUID {
		id := uuid.New()
		tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, id, "admin", "admin"))
		Expect(tx.Error).To(BeNil())
		return id
	}

	subsetCount := func(sourceID uuid.UUID) int {
		count := 0
		gormdb.Raw("SELECT COUNT(*) FROM source_subset_inventories WHERE source_id = ?;", sourceID).Scan(&count)
		return count
	}

	agentCount := func(sourceID uuid.UUID) int {
		count := 0
		gormdb.Raw("SELECT COUNT(*) FROM agents WHERE source_id = ?;", sourceID).Scan(&count)
		return count
	}

	getAgentIDs := func(sourceID uuid.UUID) []uuid.UUID {
		var ids []uuid.UUID
		gormdb.Raw("SELECT id FROM agents WHERE source_id = ? ORDER BY created_at;", sourceID).Scan(&ids)
		return ids
	}

	buildInventoryWithAgent := func(agentID uuid.UUID) []byte {
		inv := v1alpha1.Inventory{
			VcenterId: "test-vcenter",
			Vcenter: &v1alpha1.InventoryData{
				Vms: v1alpha1.VMs{Total: 5},
			},
		}
		// Wrap in agent envelope like the agent does
		wrapped := map[string]interface{}{
			"agentId":   agentID.String(),
			"inventory": inv,
		}
		data, _ := json.Marshal(wrapped)
		return data
	}

	uploadZIP := func(sourceID uuid.UUID, content []byte) server.UpdateInventoryResponseObject {
		resp, err := srv.UpdateInventory(ctx, server.UpdateInventoryRequestObject{
			Id:            sourceID,
			MultipartBody: buildUpload(content),
		})
		Expect(err).To(BeNil())
		return resp
	}

	uploadJSON := func(sourceID uuid.UUID, agentID uuid.UUID, inv v1alpha1.Inventory) server.UpdateInventoryResponseObject {
		resp, err := srv.UpdateInventory(ctx, server.UpdateInventoryRequestObject{
			Id: sourceID,
			JSONBody: &v1alpha1.UpdateInventory{
				AgentId:   agentID,
				Inventory: inv,
			},
		})
		Expect(err).To(BeNil())
		return resp
	}

	// upload is the old helper - defaults to ZIP upload for backward compatibility
	upload := uploadZIP

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

	BeforeEach(func() {
		user := auth.User{
			Username:     "admin",
			Organization: "admin",
			EmailDomain:  "admin.example.com",
		}
		ctx = auth.NewTokenContext(context.TODO(), user)
		srv = handlers.NewServiceHandler(service.NewSourceService(s, nil), service.NewAssessmentService(s, nil, nil), nil, service.NewSizerService(nil, s), nil, nil, nil, nil)
	})

	AfterEach(func() {
		gormdb.Exec("DELETE FROM source_subset_inventories;")
		gormdb.Exec("DELETE FROM agents;")
		gormdb.Exec("DELETE FROM image_infras;")
		gormdb.Exec("DELETE FROM sources;")
	})

	Context("Multipart JSON file upload", func() {
		It("updates the source inventory without creating subsets", func() {
			sourceID := insertSource()
			invBytes, _ := json.Marshal(mainInventory())

			resp := upload(sourceID, invBytes)
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			updated := resp.(server.UpdateInventory200JSONResponse)
			Expect(updated.OnPremises).To(BeTrue())
			Expect(updated.Inventory).ToNot(BeNil())
			Expect(updated.Inventory.VcenterId).To(Equal("test-vcenter"))
			Expect(subsetCount(sourceID)).To(Equal(0))
		})

		It("clears existing subset inventories", func() {
			sourceID := insertSource()
			groupA := uuid.New()
			resp := upload(sourceID, buildBundle(mainInventory(), map[string]agentAPI.SourceSubsetUpdate{
				groupA.String(): {
					Name:      "group-a",
					VcenterId: strPtr("vc-a"),
					VmsCount:  intPtr(3),
					Inventory: v1alpha1.Inventory{VcenterId: "vc-a"},
				},
			}))
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
			Expect(subsetCount(sourceID)).To(Equal(1))

			invBytes, _ := json.Marshal(mainInventory())
			resp = upload(sourceID, invBytes)
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
			Expect(subsetCount(sourceID)).To(Equal(0))
		})
	})

	Context("Direct JSON body upload", func() {
		It("updates the source inventory without creating subsets", func() {
			sourceID := insertSource()
			agentID := uuid.New()

			resp := uploadJSON(sourceID, agentID, mainInventory())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			updated := resp.(server.UpdateInventory200JSONResponse)
			Expect(updated.OnPremises).To(BeTrue())
			Expect(updated.Inventory).ToNot(BeNil())
			Expect(updated.Inventory.VcenterId).To(Equal("test-vcenter"))
			Expect(subsetCount(sourceID)).To(Equal(0))
		})

		It("clears existing subset inventories", func() {
			sourceID := insertSource()
			agentID := uuid.New()
			groupA := uuid.New()

			// First create a source with subsets using ZIP upload
			resp := upload(sourceID, buildBundle(mainInventory(), map[string]agentAPI.SourceSubsetUpdate{
				groupA.String(): {
					Name:      "group-a",
					VcenterId: strPtr("vc-a"),
					VmsCount:  intPtr(3),
					Inventory: v1alpha1.Inventory{VcenterId: "vc-a"},
				},
			}))
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
			Expect(subsetCount(sourceID)).To(Equal(1))

			// Now update with JSON body - should clear subsets
			resp = uploadJSON(sourceID, agentID, mainInventory())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
			Expect(subsetCount(sourceID)).To(Equal(0))
		})
	})

	Context("ZIP bundle upload", func() {
		It("updates the source and replaces subset inventories", func() {
			sourceID := insertSource()
			groupA := uuid.New()
			groupB := uuid.New()
			subsets := map[string]agentAPI.SourceSubsetUpdate{
				groupA.String(): {
					Name:      "group-a",
					VcenterId: strPtr("vc-a"),
					VmsCount:  intPtr(3),
					Inventory: v1alpha1.Inventory{VcenterId: "vc-a"},
				},
				groupB.String(): {
					Name:      "group-b",
					VcenterId: strPtr("vc-b"),
					VmsCount:  intPtr(7),
					Inventory: v1alpha1.Inventory{VcenterId: "vc-b"},
				},
			}

			resp := upload(sourceID, buildBundle(mainInventory(), subsets))
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
			Expect(subsetCount(sourceID)).To(Equal(2))

			var vmsCount int
			gormdb.Raw("SELECT vms_count FROM source_subset_inventories WHERE source_id = ? AND name = 'group-a';", sourceID).Scan(&vmsCount)
			Expect(vmsCount).To(Equal(3))

			var updateType string
			gormdb.Raw("SELECT update_type FROM source_subset_inventories WHERE source_id = ? AND name = 'group-b';", sourceID).Scan(&updateType)
			Expect(updateType).To(Equal("manual"))
		})

		It("clears existing subset inventories when the zip has none", func() {
			sourceID := insertSource()
			groupA := uuid.New()
			resp := upload(sourceID, buildBundle(mainInventory(), map[string]agentAPI.SourceSubsetUpdate{
				groupA.String(): {
					Name:      "group-a",
					Inventory: v1alpha1.Inventory{VcenterId: "vc-a"},
				},
			}))
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
			Expect(subsetCount(sourceID)).To(Equal(1))

			resp = upload(sourceID, buildBundle(mainInventory(), nil))
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
			Expect(subsetCount(sourceID)).To(Equal(0))
		})

		It("returns 400 when the zip is missing inventory.json", func() {
			sourceID := insertSource()
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			sw, _ := zw.Create("subsets/" + uuid.New().String() + ".json")
			sb, _ := json.Marshal(agentAPI.SourceSubsetUpdate{Name: "orphan"})
			_, _ = sw.Write(sb)
			_ = zw.Close()

			resp := upload(sourceID, buf.Bytes())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory400JSONResponse{}).String()))
			Expect(resp.(server.UpdateInventory400JSONResponse).Message).To(ContainSubstring("inventory.json"))
		})
	})

	Context("validation", func() {
		It("returns 404 for a missing source", func() {
			invBytes, _ := json.Marshal(mainInventory())
			resp := upload(uuid.New(), invBytes)
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory404JSONResponse{}).String()))
		})

		It("returns 403 when the source belongs to another user", func() {
			otherID := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceWithUsernameStm, otherID, "batman", "batman"))
			Expect(tx.Error).To(BeNil())

			invBytes, _ := json.Marshal(mainInventory())
			resp := upload(otherID, invBytes)
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory403JSONResponse{}).String()))
			Expect(subsetCount(otherID)).To(Equal(0))
		})

		It("returns 400 for an empty body", func() {
			resp, err := srv.UpdateInventory(ctx, server.UpdateInventoryRequestObject{
				Id: uuid.New(),
			})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory400JSONResponse{}).String()))
		})
	})

	Context("agent ID preservation", func() {
		It("preserves the uploaded agent ID when creating agent via JSON", func() {
			sourceID := insertSource()
			agentID := uuid.New()

			// Upload JSON with specific agent ID
			resp := uploadJSON(sourceID, agentID, mainInventory())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			// Verify agent was created with the correct ID
			Expect(agentCount(sourceID)).To(Equal(1))
			agentIDs := getAgentIDs(sourceID)
			Expect(agentIDs).To(HaveLen(1))
			Expect(agentIDs[0]).To(Equal(agentID))
		})

		It("does not create duplicate agents on repeated JSON uploads with same agent ID", func() {
			sourceID := insertSource()
			agentID := uuid.New()

			// First upload
			resp := uploadJSON(sourceID, agentID, mainInventory())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
			Expect(agentCount(sourceID)).To(Equal(1))

			// Second upload with same agent ID
			resp = uploadJSON(sourceID, agentID, mainInventory())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			// Still only 1 agent
			Expect(agentCount(sourceID)).To(Equal(1))
			agentIDs := getAgentIDs(sourceID)
			Expect(agentIDs).To(HaveLen(1))
			Expect(agentIDs[0]).To(Equal(agentID))
		})

		It("preserves the uploaded agent ID when creating agent via ZIP", func() {
			sourceID := insertSource()
			agentID := uuid.New()

			// First upload with specific agent ID
			resp := upload(sourceID, buildInventoryWithAgent(agentID))
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			// Verify agent was created with the correct ID
			Expect(agentCount(sourceID)).To(Equal(1))
			agentIDs := getAgentIDs(sourceID)
			Expect(agentIDs).To(HaveLen(1))
			Expect(agentIDs[0]).To(Equal(agentID))
		})

		It("does not create duplicate agents on repeated uploads with same agent ID", func() {
			sourceID := insertSource()
			agentID := uuid.New()

			// First upload
			resp := upload(sourceID, buildInventoryWithAgent(agentID))
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))
			Expect(agentCount(sourceID)).To(Equal(1))

			// Second upload with same agent ID
			resp = upload(sourceID, buildInventoryWithAgent(agentID))
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			// Still only 1 agent
			Expect(agentCount(sourceID)).To(Equal(1))
			agentIDs := getAgentIDs(sourceID)
			Expect(agentIDs).To(HaveLen(1))
			Expect(agentIDs[0]).To(Equal(agentID))
		})

		It("preserves agent ID from ZIP bundle", func() {
			sourceID := insertSource()
			agentID := uuid.New()

			// Build ZIP with agent ID in inventory.json
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)

			inv := map[string]interface{}{
				"agentId": agentID.String(),
				"inventory": v1alpha1.Inventory{
					VcenterId: "test-vcenter",
					Vcenter: &v1alpha1.InventoryData{
						Vms: v1alpha1.VMs{Total: 5},
					},
				},
			}
			invBytes, _ := json.Marshal(inv)
			mw, _ := zw.Create("inventory.json")
			_, _ = mw.Write(invBytes)
			_ = zw.Close()

			resp := upload(sourceID, buf.Bytes())
			Expect(reflect.TypeOf(resp).String()).To(Equal(reflect.TypeOf(server.UpdateInventory200JSONResponse{}).String()))

			// Verify agent was created with the correct ID from ZIP
			Expect(agentCount(sourceID)).To(Equal(1))
			agentIDs := getAgentIDs(sourceID)
			Expect(agentIDs).To(HaveLen(1))
			Expect(agentIDs[0]).To(Equal(agentID))
		})
	})
})
