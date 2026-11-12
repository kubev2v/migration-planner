package inventory

import "strings"

const (
	StorageProtocolBlock  = "Block"
	StorageProtocolFile   = "File"
	StorageProtocolObject = "Object"
)

// StorageProtocolFromDatastoreType maps a datastore technology to its storage
// access category. Unknown datastore types intentionally remain unset.
func StorageProtocolFromDatastoreType(datastoreType string) string {
	switch strings.ToUpper(strings.TrimSpace(datastoreType)) {
	case "VMFS":
		return StorageProtocolBlock
	case "NFS", "NFS41", "NFS4.1", "NFS 4.1":
		return StorageProtocolFile
	case "VSAN":
		return StorageProtocolObject
	default:
		return ""
	}
}
