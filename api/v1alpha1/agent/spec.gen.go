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
	externalRef0 "github.com/kubev2v/migration-planner/api/v1alpha1"
)

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"H4sIAAAAAAAC/7RVzW7bPBB8FWK/7yiZTpqTbkmbAkbbNEiQU+ADK64kBhLJkisbqaF3L0jalmOrLQIk",
	"N5o/O7Mzo/UGStNZo1GTh2IDvmywE3F57ZxxYWGdsehIYdzu0HtRY1hK9KVTlpTRUKT7bHecAT1bhAI8",
	"OaVrGIYMHP7slUMJxeO+zHLI4N70rsRTqNKhIJSXFH5UxnWCoAApCHNS3QRGFp5I1KRE++Da8OzkhpLT",
	"23qFmox7njzVosPJA0+C+kgWdd+FxtZCkdJ1XhmXj2w8ZIBR0AxqQQ2GArnSKhzmI3gGvc3J5KFJWGZ/",
	"Alzoykzy6a18nWRHtigJ2273vb3APBTqWO3swLBDJmMb5scTlgR7y+9j4Yd4c9L+f3n5V9NGb16n4pEk",
	"r5XhtN0hUk1YrSpR+9htChVcWlE2yM5n86Ba6BQaIusLztfr9UzE45lxNd++9fzr4uP1zf11fj6bzxrq",
	"2tASKWpDuW+qdiJ8kuy2FVqjY5e3C5YzUaMmhlpao3QwaIXOpy+31xIrpVGGOsaiFlZBAR9m89kZZGAF",
	"NVFGLqziqzPuo3meb5Qc+Kiy7el0Kji0rSiRpWvMVIwaZN5iqSqFkqVaEHET74WEAu7Ss8OYRCZOdEjo",
	"PBSPx0iLT/vqu5oq7Af6u1AXKeCjveR6zLZTbyoKy3QZPV0ZGWNWGk2oY6fC2laVkTR/8oHE5qDU/w4r",
	"KOA/Pk5Yvh2vfCL+w8vcBWJxw1sTPA8Vz+fzN2aQUF/q+P1LiMHFG2Klf5IJqCsh2V2SN2GevT/mgxY9",
	"NcapXynvF/OL9we9MfTZ9FrG4UKi9nGwJAuW8b5Ht9rlOg0BDsNy+B0AAP//Uyt7kKAHAAA=",
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

	pathPrefix := path.Dir(pathToFile)

	for rawPath, rawFunc := range externalRef0.PathToRawSpec(path.Join(pathPrefix, "../openapi.yaml")) {
		if _, ok := res[rawPath]; ok {
			// it is not possible to compare functions in golang, so always overwrite the old value
		}
		res[rawPath] = rawFunc
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