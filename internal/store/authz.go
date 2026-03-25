package store

import (
	"context"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

type Authz interface {
	WriteRelationships(ctx context.Context, updates []model.RelationshipUpdate) error
	DeleteRelationships(ctx context.Context, resource model.Resource) error
	ListResources(ctx context.Context, userID string, resourceType model.ResourceType) ([]model.Resource, error)
	GetPermissions(ctx context.Context, userID string, resource model.Resource) (model.Resource, error)
	ListRelationships(ctx context.Context, resource model.Resource) ([]model.Relationship, error)
	Close() error
}

type AuthzStore struct {
	db *gorm.DB
}

func NewAuthzStore(db *gorm.DB) Authz {
	return &AuthzStore{db: db}
}

func (a *AuthzStore) getDB(ctx context.Context) *gorm.DB {
	if tx := FromContext(ctx); tx != nil {
		return tx
	}
	return a.db
}

func (a *AuthzStore) WriteRelationships(ctx context.Context, updates []model.RelationshipUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	db := a.getDB(ctx)

	var touchRows []model.RelationSqlModel
	var deleteRows []model.RelationSqlModel
	for _, u := range updates {
		row := u.Relationship.ToSql()
		switch u.Operation {
		case model.OperationTouch:
			touchRows = append(touchRows, row)
		case model.OperationDelete:
			deleteRows = append(deleteRows, row)
		}
	}

	if len(touchRows) > 0 {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&touchRows).Error; err != nil {
			return err
		}
	}

	if len(deleteRows) > 0 {
		placeholders := make([]string, len(deleteRows))
		args := make([]interface{}, 0, len(deleteRows)*5)
		for i, row := range deleteRows {
			placeholders[i] = "(?, ?, ?, ?, ?)"
			args = append(args, row.Resource, row.ResourceID, row.Relation, row.SubjectNamespace, row.SubjectID)
		}
		query := "(resource, resource_id, relation, subject_namespace, subject_id) IN (" + strings.Join(placeholders, ", ") + ")"
		if err := db.Where(query, args...).Delete(&model.RelationSqlModel{}).Error; err != nil {
			return err
		}
	}

	return nil
}

func (a *AuthzStore) DeleteRelationships(ctx context.Context, resource model.Resource) error {
	db := a.getDB(ctx)
	q := db.Where("resource = ?", string(resource.Type))
	if resource.ID != "" {
		q = q.Where("resource_id = ?", resource.ID)
	}
	return q.Delete(&model.RelationSqlModel{}).Error
}

func (a *AuthzStore) ListRelationships(ctx context.Context, resource model.Resource) ([]model.Relationship, error) {
	db := a.getDB(ctx)
	q := db.Where("resource = ?", string(resource.Type))
	if resource.ID != "" {
		q = q.Where("resource_id = ?", resource.ID)
	}

	var rows []model.RelationSqlModel
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]model.Relationship, len(rows))
	for i, r := range rows {
		result[i] = r.ToRelationship()
	}
	return result, nil
}

func (a *AuthzStore) ListResources(ctx context.Context, userID string, resourceType model.ResourceType) ([]model.Resource, error) {
	db := a.getDB(ctx)

	// Direct relations: user is directly related to resources
	direct := db.Model(&model.RelationSqlModel{}).
		Select("resource_id, relation").
		Where("resource = ?", string(resourceType)).
		Where("subject_namespace = ?", string(model.UserSubject)).
		Where("subject_id = ?", userID)

	// Indirect relations: resources shared with an org the user is a member of
	indirect := db.Model(&model.RelationSqlModel{}).
		Select("r.resource_id, r.relation").
		Table("relations r").
		Joins("JOIN relations m ON m.resource = ? AND m.resource_id = r.subject_id AND m.relation = ? AND m.subject_namespace = ? AND m.subject_id = ?",
			string(model.OrgResource), string(model.MemberRelation), string(model.UserSubject), userID).
		Where("r.resource = ?", string(resourceType)).
		Where("r.subject_namespace = ?", string(model.OrgSubject))

	var rows []struct {
		ResourceID string
		Relation   string
	}
	if err := db.Raw("? UNION ?", direct, indirect).Scan(&rows).Error; err != nil {
		return nil, err
	}

	// Group permissions by resource ID
	permsMap := make(map[string][]model.Permission)
	for _, row := range rows {
		for _, p := range model.Relation(row.Relation).Permissions() {
			if !p.In(permsMap[row.ResourceID]) {
				permsMap[row.ResourceID] = append(permsMap[row.ResourceID], p)
			}
		}
	}

	result := make([]model.Resource, 0, len(permsMap))
	for id, perms := range permsMap {
		result = append(result, model.Resource{
			Type:        resourceType,
			ID:          id,
			Permissions: perms,
		})
	}
	return result, nil
}

func (a *AuthzStore) GetPermissions(ctx context.Context, userID string, resource model.Resource) (model.Resource, error) {
	db := a.getDB(ctx)

	// Direct relations: user is directly related to the resource
	direct := db.Model(&model.RelationSqlModel{}).
		Select("relation").
		Where("resource = ?", string(resource.Type)).
		Where("resource_id = ?", resource.ID).
		Where("subject_namespace = ?", string(model.UserSubject)).
		Where("subject_id = ?", userID)

	// Indirect relations: resource shared with an org the user is a member of
	indirect := db.Model(&model.RelationSqlModel{}).
		Select("r.relation").
		Table("relations r").
		Joins("JOIN relations m ON m.resource = ? AND m.resource_id = r.subject_id AND m.relation = ? AND m.subject_namespace = ? AND m.subject_id = ?",
			string(model.OrgResource), string(model.MemberRelation), string(model.UserSubject), userID).
		Where("r.resource = ?", string(resource.Type)).
		Where("r.resource_id = ?", resource.ID).
		Where("r.subject_namespace = ?", string(model.OrgSubject))

	var relations []string
	if err := db.Raw("? UNION ?", direct, indirect).Scan(&relations).Error; err != nil {
		return resource, err
	}

	for _, rel := range relations {
		for _, p := range model.Relation(rel).Permissions() {
			if !p.In(resource.Permissions) {
				resource.Permissions = append(resource.Permissions, p)
			}
		}
	}

	return resource, nil
}

func (a *AuthzStore) Close() error {
	return nil
}
