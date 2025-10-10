package store

import (
	"time"

	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// InitSpiceDbClient initializes and returns a SpiceDB client with retry configuration
func InitSpiceDbClient(endpoint, token string) (*authzed.Client, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcutil.WithInsecureBearerToken(token),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultServiceConfig(`{
			"methodConfig": [{
				"name": [{"service": "authzed.api.v1.PermissionsService"}],
				"retryPolicy": {
					"MaxAttempts": 3,
					"InitialBackoff": ".1s",
					"MaxBackoff": "1s",
					"BackoffMultiplier": 1.0,
					"RetryableStatusCodes": [ "UNAVAILABLE" ]
				}
			}]
		}`),
	}

	return authzed.NewClient(endpoint, opts...)
}
