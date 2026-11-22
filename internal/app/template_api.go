package app

import "github.com/st-mich43l/apexvoid-photobooth/internal/templates"

// TemplatesResult is the legacy layout listing (kept for older callers).
type TemplatesResult struct {
	Templates []templates.Template `json:"templates"`
	Errors    []string             `json:"errors"`
	Dir       string               `json:"dir"`
}

// ListTemplates returns every frame definition including disabled and legacy ones.
func (s *Service) ListTemplates() TemplatesResult {
	errs := s.tpl.Errs
	if errs == nil {
		errs = []string{}
	}
	return TemplatesResult{Templates: s.tpl.All(), Errors: errs, Dir: s.db.Layout.TemplatesDir()}
}
