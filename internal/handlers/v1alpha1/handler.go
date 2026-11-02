package v1alpha1

import "github.com/kubev2v/migration-planner/internal/service"

type ServiceHandler struct {
	sourceSrv          *service.SourceService
	assessmentSrv      service.AssessmentServicer
	jobSrv             *service.JobService
	sizerSrv           service.SizerServicer
	estimationSrv      service.EstimationServicer
	partnerSrv         service.PartnerServicer
	accountsSrv        service.AccountsServicer
	enhancementDataSrv service.AssessmentEnhancementDataServicer
	applianceCache     *service.ApplianceImageCache
}

func NewServiceHandler(
	sourceService *service.SourceService,
	a service.AssessmentServicer,
	j *service.JobService,
	sizer service.SizerServicer,
	estimation service.EstimationServicer,
	partner service.PartnerServicer,
	accounts service.AccountsServicer,
	enhancementData service.AssessmentEnhancementDataServicer,
	applianceCache *service.ApplianceImageCache,
) *ServiceHandler {
	return &ServiceHandler{
		sourceSrv:          sourceService,
		assessmentSrv:      a,
		jobSrv:             j,
		sizerSrv:           sizer,
		estimationSrv:      estimation,
		accountsSrv:        accounts,
		partnerSrv:         partner,
		enhancementDataSrv: enhancementData,
		applianceCache:     applianceCache,
	}
}

// servedApplianceVersion returns the version of the appliance image the
// service currently serves, or "" when the cache has none.
func servedApplianceVersion(cache *service.ApplianceImageCache) string {
	if cache == nil {
		return ""
	}
	image, err := cache.GetImage()
	if err != nil {
		return ""
	}
	return image.Version
}
