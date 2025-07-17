package server

import "net/http"

// HTTPStatusCode methods for auto-generated OpenAPI error types
// This allows them to implement the HTTPError interface

func (e *UnescapedCookieParamError) HTTPStatusCode() int {
	return http.StatusBadRequest
}

func (e *UnmarshalingParamError) HTTPStatusCode() int {
	return http.StatusUnprocessableEntity
}

func (e *RequiredParamError) HTTPStatusCode() int {
	return http.StatusBadRequest
}

func (e *RequiredHeaderError) HTTPStatusCode() int {
	return http.StatusBadRequest
}

func (e *InvalidParamFormatError) HTTPStatusCode() int {
	return http.StatusUnprocessableEntity
}

func (e *TooManyValuesForParamError) HTTPStatusCode() int {
	return http.StatusBadRequest
}
