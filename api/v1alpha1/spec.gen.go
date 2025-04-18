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
