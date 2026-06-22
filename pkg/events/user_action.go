package events

import "time"

type UserActionEventPayload struct {
	UserAction UserActionData `json:"user_action"`
}

type UserActionData struct {
	Username  string    `json:"username"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

type ShareAssessmentActionData struct {
	AssessmentID string `json:"assessment_id"`
	PartnerID    string `json:"partner_id"`
}

type UnshareAssessmentActionData struct {
	AssessmentID string `json:"assessment_id"`
}

type SizingActionData struct {
	AssessmentID string `json:"assessment_id"`
}

type ComplexityActionData struct {
	AssessmentID string `json:"assessment_id"`
}

type OVADownloadActionData struct {
	SourceID string `json:"source_id"`
}

type VisitorActionData struct {
	OrgID string `json:"org_id"`
}

func NewShareAssessmentPayload(username, assessmentID, partnerID string) UserActionEventPayload {
	return UserActionEventPayload{
		UserAction: UserActionData{
			Username:  username,
			Timestamp: time.Now().UTC(),
			Data: ShareAssessmentActionData{
				AssessmentID: assessmentID,
				PartnerID:    partnerID,
			},
		},
	}
}

func NewUnshareAssessmentPayload(username, assessmentID string) UserActionEventPayload {
	return UserActionEventPayload{
		UserAction: UserActionData{
			Username:  username,
			Timestamp: time.Now().UTC(),
			Data: UnshareAssessmentActionData{
				AssessmentID: assessmentID,
			},
		},
	}
}

func NewSizingPayload(username, assessmentID string) UserActionEventPayload {
	return UserActionEventPayload{
		UserAction: UserActionData{
			Username:  username,
			Timestamp: time.Now().UTC(),
			Data: SizingActionData{
				AssessmentID: assessmentID,
			},
		},
	}
}

func NewComplexityPayload(username, assessmentID string) UserActionEventPayload {
	return UserActionEventPayload{
		UserAction: UserActionData{
			Username:  username,
			Timestamp: time.Now().UTC(),
			Data: ComplexityActionData{
				AssessmentID: assessmentID,
			},
		},
	}
}

func NewOVADownloadPayload(username, sourceID string) UserActionEventPayload {
	return UserActionEventPayload{
		UserAction: UserActionData{
			Username:  username,
			Timestamp: time.Now().UTC(),
			Data: OVADownloadActionData{
				SourceID: sourceID,
			},
		},
	}
}

func NewVisitorPayload(username, orgID string) UserActionEventPayload {
	return UserActionEventPayload{
		UserAction: UserActionData{
			Username:  username,
			Timestamp: time.Now().UTC(),
			Data: VisitorActionData{
				OrgID: orgID,
			},
		},
	}
}
