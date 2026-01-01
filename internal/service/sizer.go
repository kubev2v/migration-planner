package service

// SizerService handles odf sizer related operations.
type SizerService struct {
}

func NewSizerService() *SizerService {
	return &SizerService{}
}

func (s *SizerService) CalculateSizing() error {
	return nil
}
