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

	"H4sIAAAAAAAC/+xWS2/cNhD+KwRToBc91mkOhW7OC100L9jJqfZhTM6uJpVIlRytsTX03wuS2od3tWkC",
	"2EWA5ibxMY9vvvmGd1LZtrMGDXtZ3Umvamwhfr5yzrrw0TnboWPCuNyi97DE8KnRK0cdkzWySufFZjuT",
	"vO5QVtKzI7OUw5BJh3/15FDL6o+tmeshk5e2dwqPXSmHwKjPOfwsrGuBZSU1MOZM7YSPLFzRaJig+eSa",
	"cO3oBOnpZbNCw9atJ3cNtDi54Rm4j8Gi6duQ2C0Qk1nmC+vyXTReZhIjoJlcAtcYDORkKGzmO+eZ7Luc",
	"bR6SlNfZKYdzs7CT8fSd/jbIDspCWo7ZbnO753MfqEO0s72C7UeyS8PefEbFclvyF/H8ceFPwH0Qazx1",
	"2vgb8hEFYmyj1Z8cLmQln5Q7ypcj38uRgsPWGjgH62hsW+H7dE/rgrwA4ZB7Z8QKmh7FwjqhoGm84BpY",
	"aGt+5s0JGyovUqS+kNnX9ta5qPsWTO4QNNw0KPa2hV0IrlGkKqU/8iLYhbBfTPWJQ/DB8rGjFlRNBk+6",
	"uq3XBw4CBmRiDFfyNVDTO7ySYzyFmI8BJXTIC2w7DjbQxV9jBZlE1WAMVkBNcFyIc3ERwxSqAUcLQi/A",
	"iN8+fvywSVZZjeKmDyhjsMTCrtA50iiIJxP3Xy7niOUOPPHeoLCLSlzJy14p9P5KCuv2My3EWxtSMQtb",
	"iZq581VZLomLP3/1BdlAt7Y3xOtSWcOObnq2zpcaV9iUnpY5OFUTo+LeYQkd5crGJiNrfNHqJ75DlYPR",
	"+bYjjxvjoAmGKGlJJBpSaDzuGkued6BqFE+LWWjUIJRyE/ft7W0BcbuwblmOd335Zv7i1bvLV/nTYlbU",
	"3DaxVYibYO4tLUcmfGjAGHTi/MNcZnKFzid0e6NxQQZ1uGY7NNCRrOQvxaw4C10AXMeahOzL1VnpYzuO",
	"ZWqQJ1oirQsQyjYNqg09NzejmxTVXMtKvozHL7e7Dn1nQ2bB8tPZLM4baxhNFA3ouoZUvF5+HjslacW/",
	"KkmqUazA/Yjf/x6yfzY7ezBfaURPuPpkoOfaOvo7QB5KBUsfNDPBE6fuEvkY1YY8n8QwKOp/geBOvr9/",
	"FDvrJ2BMg1DACOURkmnwXW42w1RDz8+tXj8wiuOEHe7PTnY9DkcVPHtg31OQpnh0KuHs8Uv4HLS4SOh+",
	"R7QZskOlK+9ID18ldycYta9vUVEdtMjoguNDW/OX2zfD5jyF9SDDm+dflZ6C9zmT7UFzOIKuH10RvqQG",
	"/wsqBafPHt/pO8uvbW++bXCE52KiVIcqvNX0KaZeIOgfPP3B00fmaVj06FYbaqVnbimH6+GfAAAA//8I",
	"/L7E9RAAAA==",
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