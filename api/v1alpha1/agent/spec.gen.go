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

	"H4sIAAAAAAAC/7RYTVPjOBP+Ky6979GJM7Nzym1gZ3ZSs7AUFMyByqGxOrEGW9JKbVIs5f++JSmOnVj5",
	"gGVuQVJ/Pf30h3lhuaq0kijJsukLs3mBFfifX4xRxv3QRmk0JNAfV2gtLNH95GhzIzQJJdk0vE/a65TR",
	"s0Y2ZZaMkEvWNCkz+HctDHI2vd+omTcpm8mFgaElDgSWlAl/CcLKDh8tDOI5aMgFPf9x5k7WdoUkXKJh",
	"TcpIEZRHH/mTlyNu+9uhxnTXj/kmfvXwE3PqLDAwBp7d34WydKVWaG4IKEQDnAsHJ5RXW1Huc7en3Wmz",
	"V2jOy9oSmi3I9opvfJFIK2UeDyEtoYoB1CGHsq4cRpZAcjCcpYwL9+yhJuQ9SA5j6+2cgl9IQgjXHsj8",
	"N4dM7H7Xfvd4V/kQ3mH6eiCmfe7GQpnJJ5SkzPMQZtEWw/8NLtiU/S/rKjRbl2cWKqZJ2VPI1KG3dxd2",
	"EKoTS9emYv5diKUBR8SZtfXB+gNr0doKJUW5kat666aXmxIesDxeceFZ2jfUqj2FJDeqNjkO/c4NAiH/",
	"7J1bKFMBsalLG45IVJH+lToRjpIElLemjEYr+Ja2uhY8pkj0s384ze3DJt1ff9YW3/E5fkVAte0Xp1Q0",
	"ypWUmLuaTNkKBAm5HC2UGXUBOnag7/8pWwIV6BSOhBTuctT5n7Jaj0iNHG6RAm8dmMmFivpXa/66LOyQ",
	"w+PrgdnEumUz7eW5b20DWoxCgTI3XsutF4nS5xgX3pTkLmGvg3IHl71Y9JyOhX53cY3Wh39mEB65Wslh",
	"7IWwpJYGqvjIfuXkqYS8g7LG+GtLqE9o3Rsla4nQgOMNwjX2A9PiqzKh+8FDiae++yGo+AFGCrm0h2Uu",
	"FR1WvxNZB3bretTPo07t8yDOgkijz3V93u5ih8fNkEKN3wQez9th8Eb5sLq9Qbhqx1k/R4f07M4/1377",
	"sF0jWCXfokb91z1Pv9vGaKB6M6LHquikEjq9fmKbGhuaSjuWtuFtmNOnoE/DNpR7EhzjzrBkGt/vQ2Mu",
	"RY7SYrcxs88a8gKTj+OJm0FuXLCCSNtplq1WqzH467Eyy2wta7M/Z+dfLm++jD6OJ+OCqtJDJsih2e1m",
	"yVUJUqJJPl/NklECS5SUoORaCR/jExobPs1qyXEhJHLPQI0StGBT9tt4Mv7gcAAqPOwZaJE9fchCwm32",
	"IniTdSNJ1zT87DOoS8gxCc8StUiowMRqzMVCIE+CLubtrouBsym7DmL9Wes9MVBh2Ofvdy3Nft9ob3UK",
	"d+7cb9eAaVgJOrKQqTFdf9aesJw18yCMls4U97M7V5LW6y1oXYrcB5H9tEp2X8zHyiiyUzTbrHaO+gOr",
	"leOA0/hxMnlnD4LVbVz/+u5o8ekdbYV/HURMnQFPrgO8weaHX2/zVkJNhTLin8D/T5NPv97opaKvqpbc",
	"ty4C19Xu2Zq2c3eWFQglFc7AEiNlFa6TvMD8cVA834JsnC3D5PZcWFude5ctmqe21EJfylgzb/4NAAD/",
	"/x2yKqIUEgAA",
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
