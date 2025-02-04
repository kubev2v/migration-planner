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
	"DTHWV/wwOqLr6/V/AgAA//+hbW+5CDIAAA==",
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
