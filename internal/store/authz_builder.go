package store

import "github.com/kubev2v/migration-planner/internal/store/model"

// RelationshipBuilder constructs a batch of relationship updates.
// Use With to add (touch) relationships and Without to remove (delete) them.
type RelationshipBuilder struct {
	updates []model.RelationshipUpdate
}

func NewRelationshipBuilder() *RelationshipBuilder {
	return &RelationshipBuilder{}
}

// With appends a TOUCH operation (create or idempotent update).
func (b *RelationshipBuilder) With(resource model.Resource, relation model.Relation, subject model.Subject) *RelationshipBuilder {
	b.updates = append(b.updates, model.RelationshipUpdate{
		Operation:    model.OperationTouch,
		Relationship: model.NewRelationship(resource.Type, resource.ID, relation, subject),
	})
	return b
}

// Without appends a DELETE operation.
func (b *RelationshipBuilder) Without(resource model.Resource, relation model.Relation, subject model.Subject) *RelationshipBuilder {
	b.updates = append(b.updates, model.RelationshipUpdate{
		Operation:    model.OperationDelete,
		Relationship: model.NewRelationship(resource.Type, resource.ID, relation, subject),
	})
	return b
}

// Build returns the accumulated relationship updates.
func (b *RelationshipBuilder) Build() []model.RelationshipUpdate {
	return b.updates
}
