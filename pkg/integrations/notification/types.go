package notification

const (
	// bundle is the Console Notifications bundle this application belongs to.
	bundle = "openshift"

	// application is the technical name registered with the notification service.
	application = "migration-advisor"

	// schemaVersion is the version of the notification message schema.
	schemaVersion = "v2.0.0"
)

const (
	SeverityImportant = "IMPORTANT"
)

const (
	// PartnershipRequestEventType fires when a partner organization receives a request to establish a partnership.
	PartnershipRequestEventType = "partnership-request"

	// PartnershipResponseEventType fires when a partner organization responds to a partnership request.
	PartnershipResponseEventType = "partnership-response"

	// AssessmentSharedEventType fires when a migration assessment is shared with a partner organization.
	AssessmentSharedEventType = "assessment-shared"

	// TODO: implement the mechnisem of notify the customer when a partner creates an assessment on their behalf.
	// This is disabled because there is currently no "create on behalf of a customer"
	// AssessmentCreatedEventType fires when a new assessment is created on behalf of a customer by a partner organization.
	AssessmentCreatedEventType = "assessment-created"
)
