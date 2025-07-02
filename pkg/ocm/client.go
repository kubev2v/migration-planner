package ocm

import (
	"context"
	"errors"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

type Client struct {
	logger sdk.Logger
}

func NewClient() (*Client, error) {
	logger, err := sdk.NewGoLoggerBuilder().
		Debug(true).
		Build()
	if err != nil {
		return nil, err
	}

	return &Client{logger: logger}, nil
}

func (c *Client) GetOrganization(ctx context.Context, authToken string, orgID string) (string, error) {
	// Create the connection, and remember to close it:
	connection, err := sdk.NewConnectionBuilder().
		Logger(c.logger).
		Tokens(authToken).
		Build()
	if err != nil {
		return "", err
	}
	defer connection.Close()

	response, err := connection.AccountsMgmt().
		V1().
		Organizations().
		Organization(orgID).
		Get().
		SendContext(ctx)
	if err != nil {
		return "", err
	}

	orgName, ok := response.Body().GetName()
	if !ok {
		return "", errors.New("failed to get organization name from response")
	}

	return orgName, nil
}
