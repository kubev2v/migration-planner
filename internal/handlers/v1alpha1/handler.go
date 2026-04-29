package v1alpha1

import (
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
)

type ServiceHandler struct {
	sourceSrv     *service.SourceService
	imageSrv      *service.ImageService
	assessmentSrv service.AssessmentServicer
	jobSrv        *service.JobService
	sizerSrv      *service.SizerService
	estimationSrv *service.EstimationService
	accountsSrv   *service.AccountsService
	partnerSrv    service.PartnerServicer
	S3            *config.S3
}

func NewServiceHandler(
	sourceService *service.SourceService,
	a service.AssessmentServicer,
	j *service.JobService,
	sizer *service.SizerService,
	estimation *service.EstimationService,
	accounts *service.AccountsService,
	partner service.PartnerServicer,
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

func (s *ServiceHandler) WithS3Cfg(cfg *config.S3) *ServiceHandler {
	s.S3 = cfg
	return s
}

func (s *ServiceHandler) WithImageSrv(is *service.ImageService) *ServiceHandler {
	s.imageSrv = is
	return s
}
