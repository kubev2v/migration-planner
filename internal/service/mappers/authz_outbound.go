package mappers

import (
	svcmodel "github.com/kubev2v/migration-planner/internal/service/model"
	storemodel "github.com/kubev2v/migration-planner/internal/store/model"
)

// PermissionsFromStore converts stored permissions to independent service values.
func PermissionsFromStore(permissions []storemodel.Permission) []svcmodel.Permission {
	if permissions == nil {
		return nil
	}
	result := make([]svcmodel.Permission, len(permissions))
	for i, permission := range permissions {
		result[i] = svcmodel.Permission(permission)
	}
	return result
}

// SharingFromStore converts stored sharing metadata to independent service values.
func SharingFromStore(sharing *storemodel.Sharing) *svcmodel.Sharing {
	if sharing == nil {
		return nil
	}
	result := &svcmodel.Sharing{IsShared: sharing.IsShared}
	if sharing.SharedWith != nil {
		result.SharedWith = make([]svcmodel.SharingSubject, len(sharing.SharedWith))
		for i, subject := range sharing.SharedWith {
			result.SharedWith[i] = svcmodel.SharingSubject{
				Type: subject.Type,
				ID:   subject.ID,
				Name: subject.Name,
			}
		}
	}
	if sharing.SharedBy != nil {
		result.SharedBy = &svcmodel.SharingSubject{
			Type: sharing.SharedBy.Type,
			ID:   sharing.SharedBy.ID,
			Name: sharing.SharedBy.Name,
		}
	}
	return result
}
