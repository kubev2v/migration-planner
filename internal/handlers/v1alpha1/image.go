package v1alpha1

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	imageServer "github.com/kubev2v/migration-planner/internal/api/server/image"
	imageService "github.com/kubev2v/migration-planner/internal/service"
)

type Key int

// Keys for values stored in request context (openapi strict handlers).
const (
	ResponseWriterKey Key = iota
	RequestKey
)

var jwtPayloadRegexp = regexp.MustCompile(`^.+\.(.+)\..+`)

type ImageHandler struct {
	imageService *imageService.ImageSvc
}

// Make sure we conform to servers Service interface
var _ imageServer.Service = (*ImageHandler)(nil)

func NewImageHandler(srv *imageService.ImageSvc) *ImageHandler {
	return &ImageHandler{
		imageService: srv,
	}
}

func (h *ImageHandler) Health(ctx context.Context, request imageServer.HealthRequestObject) (imageServer.HealthResponseObject, error) {
	return nil, nil
}

func (h *ImageHandler) GenerateDownloadURL(ctx context.Context, request imageServer.GenerateDownloadURLRequestObject) (imageServer.GenerateDownloadURLResponseObject, error) {
	url, expireAt, err := h.imageService.GenerateDownloadURL(ctx, request.Body.Id)
	if err != nil {
		var errResourceNotFound *imageService.ErrResourceNotFound
		switch {
		case errors.As(err, &errResourceNotFound):
			return imageServer.GenerateDownloadURL400JSONResponse{Message: err.Error()}, nil
		default:
			return imageServer.GenerateDownloadURL500JSONResponse{}, nil
		}
	}
	return imageServer.GenerateDownloadURL200JSONResponse{Url: url, ExpiresAt: &expireAt}, nil
}

func (h *ImageHandler) HeadImageByToken(ctx context.Context, req imageServer.HeadImageByTokenRequestObject) (imageServer.HeadImageByTokenResponseObject, error) {
	token, _, err := h.imageService.ParseTokenETag(req.Token)
	if err != nil {
		return imageServer.HeadImageByToken400JSONResponse{Message: err.Error()}, nil
	}

	if err := h.imageService.ValidateToken(ctx, token); err != nil {
		return imageServer.HeadImageByToken401JSONResponse{Message: err.Error()}, nil
	}

	return imageServer.HeadImageByToken200Response{}, nil
}

func (h *ImageHandler) GetImageByToken(ctx context.Context, req imageServer.GetImageByTokenRequestObject) (imageServer.GetImageByTokenResponseObject, error) {
	writer, ok := ctx.Value(ResponseWriterKey).(http.ResponseWriter)
	r, ok2 := ctx.Value(RequestKey).(*http.Request)
	if !ok || !ok2 {
		return imageServer.GetImageByToken500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}

	jwt, etag, err := h.imageService.ParseTokenETag(req.Token)
	if err != nil {
		return imageServer.GetImageByToken400JSONResponse{Message: err.Error()}, nil
	}

	if err := h.imageService.ValidateToken(ctx, jwt); err != nil {
		return imageServer.GetImageByToken401JSONResponse{Message: err.Error()}, nil
	}

	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", req.Name))
	writer.Header().Set("Content-Type", "application/ovf")
	writer.Header().Set("ETag", fmt.Sprintf("%q", etag))

	http.ServeFile(writer, r, h.imageService.FilePath(etag))

	sourceId, err := IdFromJWT(jwt)
	if err != nil {
		return imageServer.GetImageByToken401JSONResponse{Message: err.Error()}, nil
	}

	h.imageService.UpdateAgentVersion(sourceId)

	return nil, nil
}

func IdFromJWT(jwt string) (string, error) {
	match := jwtPayloadRegexp.FindStringSubmatch(jwt)

	if len(match) != 2 {
		return "", fmt.Errorf("failed to parse JWT from URL")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(match[1])
	if err != nil {
		return "", err
	}

	var p struct {
		Sub string `json:"sub"`
	}

	err = json.Unmarshal(decoded, &p)
	if err != nil {
		return "", err
	}

	switch {
	case p.Sub != "":
		return p.Sub, nil
	}

	return "", fmt.Errorf("sub ID not found in token")
}
