package cli

import (
	"fmt"
	"strings"
)

const (
	SourceKind = "source"
)

var (
	pluralKinds = map[string]string{
		SourceKind: "sources",
	}
)

func parseAndValidateKindId(arg string) (string, string, error) {
	kind, id, _ := strings.Cut(arg, "/")
	kind = singular(kind)
	if _, ok := pluralKinds[kind]; !ok {
		return "", "", fmt.Errorf("invalid resource kind: %s", kind)
	}
	return kind, id, nil
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
