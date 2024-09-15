package cli

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	SourceKind = "source"
)

var (
	pluralKinds = map[string]string{
		SourceKind: "sources",
	}
)

func parseAndValidateKindId(arg string) (string, *uuid.UUID, error) {
	kind, idStr, _ := strings.Cut(arg, "/")
	kind = singular(kind)
	if _, ok := pluralKinds[kind]; !ok {
		return "", nil, fmt.Errorf("invalid resource kind: %s", kind)
	}

	if len(idStr) == 0 {
		return kind, nil, nil
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return "", nil, fmt.Errorf("invalid ID: %w", err)
	}
	return kind, &id, nil
}

func singular(kind string) string {
	for singular, plural := range pluralKinds {
		if kind == plural {
			return singular
		}
	}
	return kind
}

func plural(kind string) string {
	return pluralKinds[kind]
}
