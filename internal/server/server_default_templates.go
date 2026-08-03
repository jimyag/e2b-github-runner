package server

import (
	"fmt"
	"strings"

	"github.com/qiniu/ci-runner/internal/sandboxrunner"
)

const (
	defaultTemplateResolutionReasonMissing     = "missing"
	defaultTemplateResolutionReasonDuplicate   = "duplicate"
	defaultTemplateResolutionReasonPrivate     = "private"
	defaultTemplateResolutionReasonNonRunnable = "non_runnable"
	defaultTemplateResolutionReasonEmptyID     = "empty_template_id"
	defaultTemplateResolutionReasonNoCatalog   = "catalog_unavailable"
)

type defaultTemplateResolutionError struct {
	RequestedName string
	Reason        string
}

func (e *defaultTemplateResolutionError) Error() string {
	return fmt.Sprintf("default template %q cannot be resolved: %s", e.RequestedName, e.Reason)
}

func resolveDefaultTemplateID(requestedName string, templates []sandboxrunner.CatalogTemplate) (string, error) {
	requestedName = strings.TrimSpace(requestedName)
	if requestedName == "" {
		return "", newDefaultTemplateResolutionError(requestedName, defaultTemplateResolutionReasonMissing)
	}
	matches := make([]sandboxrunner.CatalogTemplate, 0, 1)
	for _, template := range templates {
		for _, name := range template.Names {
			name = strings.TrimSpace(name)
			if name == requestedName || strings.HasSuffix(name, "/"+requestedName) {
				matches = append(matches, template)
				break
			}
		}
	}

	if len(matches) == 0 {
		return "", newDefaultTemplateResolutionError(requestedName, defaultTemplateResolutionReasonMissing)
	}
	if len(matches) > 1 {
		return "", newDefaultTemplateResolutionError(requestedName, defaultTemplateResolutionReasonDuplicate)
	}

	match := matches[0]
	if !match.Public {
		return "", newDefaultTemplateResolutionError(requestedName, defaultTemplateResolutionReasonPrivate)
	}
	switch strings.TrimSpace(match.BuildStatus) {
	case "ready", "uploaded":
	default:
		return "", newDefaultTemplateResolutionError(requestedName, defaultTemplateResolutionReasonNonRunnable)
	}
	templateID := strings.TrimSpace(match.TemplateID)
	if templateID == "" {
		return "", newDefaultTemplateResolutionError(requestedName, defaultTemplateResolutionReasonEmptyID)
	}
	return templateID, nil
}

func newDefaultTemplateResolutionError(requestedName, reason string) error {
	return &defaultTemplateResolutionError{
		RequestedName: requestedName,
		Reason:        reason,
	}
}
