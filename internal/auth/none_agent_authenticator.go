package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

type NoneAgentAuthenticator struct{}

func NewNoneAgentAuthenticator() *NoneAgentAuthenticator {
	return &NoneAgentAuthenticator{}
}

var sourcePathRe = regexp.MustCompile(`/api/v1/sources/([^/]+)/`)

func (n *NoneAgentAuthenticator) Authenticator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()

		var req struct {
			SourceID string `json:"sourceId"`
		}

		_ = json.Unmarshal(bodyBytes, &req)

		if req.SourceID == "" {
			if matches := sourcePathRe.FindStringSubmatch(r.URL.Path); len(matches) > 1 {
				req.SourceID = matches[1]
			}
		}

		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		agentJWT := AgentJWT{
			ExpireAt: time.Now().Add(defaultExpirationPeriod * time.Hour),
			IssueAt:  time.Now(),
			Issuer:   "none",
			OrgID:    "internal",
			SourceID: req.SourceID,
		}
		ctx := NewTokenContext(r.Context(), agentJWT)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
