// Package v1alpha1 provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/oapi-codegen/oapi-codegen/v2 version v2.3.0 DO NOT EDIT.
package v1alpha1

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"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",
	"vbwJbA9vZBO+ud78LwAA//+CrTRXXCcAAA==",
}

// GetSwagger returns the content of the embedded swagger specification file
// or error if failed to decode
func decodeSpec() ([]byte, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %w", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}

	return buf.Bytes(), nil
}

var rawSpec = decodeSpecCached()

// a naive cached of a decoded swagger spec
func decodeSpecCached() func() ([]byte, error) {
	data, err := decodeSpec()
	return func() ([]byte, error) {
		return data, err
	}
}

// Constructs a synthetic filesystem for resolving external references when loading openapi specifications.
func PathToRawSpec(pathToFile string) map[string]func() ([]byte, error) {
	res := make(map[string]func() ([]byte, error))
	if len(pathToFile) > 0 {
		res[pathToFile] = rawSpec
	}

	return res
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file. The external references of Swagger specification are resolved.
// The logic of resolving external references is tightly connected to "import-mapping" feature.
// Externally referenced files must be embedded in the corresponding golang packages.
// Urls can be supported but this task was out of the scope.
func GetSwagger() (swagger *openapi3.T, err error) {
	resolvePath := PathToRawSpec("")

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
		pathToFile := url.String()
		pathToFile = path.Clean(pathToFile)
		getSpec, ok := resolvePath[pathToFile]
		if !ok {
			err1 := fmt.Errorf("path not found: %s", pathToFile)
			return nil, err1
		}
		return getSpec()
	}
	var specData []byte
	specData, err = rawSpec()
	if err != nil {
		return
	}
	swagger, err = loader.LoadFromData(specData)
	if err != nil {
		return
	}
	return
}
