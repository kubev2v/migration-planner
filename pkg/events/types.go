package events

const (
	// GenericTopic is the Kafka topic where all events are produced
	GenericTopic = "assisted.migration.events"

	// DefaultEventSource is the CloudEvent source field
	DefaultEventSource = "migration-planner"

	// Event types identify the CloudEvent type, used for Kafka routing
	// Following the pattern <domain>.<entity>.<action>

	AssessmentCreatedEventType = "assisted.migration.assessment.created"
	AssessmentDeletedEventType = "assisted.migration.assessment.deleted"
	// VisitorEventType tracks user visits to the assessment list
	VisitorEventType = "assisted.migration.visitor.visited"
	// PartnerCustomerEventType covers partner-customer relationship changes (request, accept, cancel, etc.)
	PartnerCustomerEventType = "assisted.migration.partner_customer.updated"

	// User action event types track discrete user actions (share, unshare, sizing, OVA download, etc.)
	ShareAssessmentEventType     = "assisted.migration.user_action.assessment_shared"
	UnshareAssessmentEventType   = "assisted.migration.user_action.assessment_unshared"
	SizingEventType              = "assisted.migration.user_action.sizing_requested"
	MigrationComplexityEventType = "assisted.migration.user_action.complexity_estimated"
	DownloadOVAEventType         = "assisted.migration.user_action.ova_downloaded"
)
