package iam

// findUserPath is the User Service endpoint that resolves a single user.
const findUserPath = "/v2/findUser"

// findAccountPath is the User Service endpoint that resolves account details.
const findAccountPath = "/v2/findAccount"

// authProvider is the authentication provider used when looking a user up by
// login/principal. Console SSO logins are issued by "Red Hat".
const authProvider = "Red Hat"

// accountTypeOrganization is the expected account type when looking up organizations.
const accountTypeOrganization = "organization"

// UserInfo contains user identity and organization details.
type UserInfo struct {
	OrgID     string // accountId from accountRelationships
	FirstName string // from personalInformation
	LastName  string // from personalInformation (lastNames)
}

// OrgInfo contains organization account details.
type OrgInfo struct {
	ID               string // account id
	Name             string // organization name
	EBSAccountNumber string // ebsAccountNumber
	Status           string // enabled/disabled/etc
	Type             string // organization/individual/etc
}

// findUserRequest is the POST /v2/findUser body. We look the user up by their
// SSO login/principal and ask for relationship summary and personal information.
type findUserRequest struct {
	By      findUserBy      `json:"by"`
	Include findUserInclude `json:"include"`
}

type findUserBy struct {
	Authentication findUserAuthentication `json:"authentication"`
}

type findUserAuthentication struct {
	Principal string `json:"principal"`
	Provider  string `json:"provider"`
}

type findUserInclude struct {
	AllOf []string `json:"allOf"`
}

// findUserResponse is the subset of the /v2/findUser response we consume.
type findUserResponse struct {
	AccountRelationships []accountRelationship `json:"accountRelationships"`
	PersonalInformation  personalInformation   `json:"personalInformation"`
}

type accountRelationship struct {
	AccountID string `json:"accountId"`
}

type personalInformation struct {
	FirstName       string `json:"firstName"`
	LastNames       string `json:"lastNames"`
	CountryIso2Code string `json:"countryIso2Code"`
	TimeZone        string `json:"timeZone"`
}

// findAccountRequest is the POST /v2/findAccount body.
type findAccountRequest struct {
	By findAccountBy `json:"by"`
}

type findAccountBy struct {
	ID string `json:"id"`
}

// findAccountResponse is the /v2/findAccount response.
type findAccountResponse struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	EBSAccountNumber string `json:"ebsAccountNumber"`
	Status           string `json:"status"`
	Type             string `json:"type"`
}
