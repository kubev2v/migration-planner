package v1alpha1

import "github.com/kubev2v/migration-planner/internal/service"

type ServiceHandler struct {
	sourceSrv     *service.SourceService
	assessmentSrv service.AssessmentServicer
	jobSrv        *service.JobService
	sizerSrv      service.SizerServicer
	estimationSrv service.EstimationServicer
	partnerSrv    service.PartnerServicer
	accountsSrv   service.AccountsServicer
}

func NewServiceHandler(
	sourceService *service.SourceService,
	a service.AssessmentServicer,
	j *service.JobService,
	sizer service.SizerServicer,
	estimation service.EstimationServicer,
	partner service.PartnerServicer,
	accounts service.AccountsServicer,
) *ServiceHandler {
	return &ServiceHandler{
		sourceSrv:     sourceService,
		assessmentSrv: a,
		jobSrv:        j,
		sizerSrv:      sizer,
		estimationSrv: estimation,
		accountsSrv:   accounts,
		partnerSrv:    partner,
	}
}
