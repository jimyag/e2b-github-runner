package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/qiniu/ci-runner/internal/sandboxrunner"
	"github.com/qiniu/ci-runner/internal/state"
	qnsandbox "github.com/qiniu/go-sdk/v7/sandbox"
)

const profileTemplateValidationTimeout = 5 * time.Second

// validateAdminProfileTemplate runs before the audited write transaction. Admin
// configuration is the only credential source: runtime audience/enabled flags
// govern fallback for jobs, not an administrator's ability to inspect templates.
func (s *Server) validateAdminProfileTemplate(w http.ResponseWriter, r *http.Request, templateID string) bool {
	defaultConfig, err := s.store.GetSandboxServiceDefault()
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		writeErrorCode(w, http.StatusInternalServerError, "sandbox_service_config_error", "Cannot load the admin Sandbox service configuration")
		return false
	}
	if strings.TrimSpace(defaultConfig.APIURL) == "" || strings.TrimSpace(defaultConfig.APIKeyEncrypted) == "" {
		writeErrorCode(w, http.StatusConflict, "sandbox_service_not_configured", "Configure the admin Sandbox service before creating a custom Runner Spec or changing its template; managed default specs remain available")
		return false
	}
	svc, err := s.sandboxServiceForConfig(sandboxServiceConfigSnapshot{
		APIURL: defaultConfig.APIURL, EncryptedAPIKey: defaultConfig.APIKeyEncrypted,
		Source: sandboxConfigSourceAdminDefault,
	})
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "sandbox_service_config_error", "Cannot initialize the admin Sandbox service; check its endpoint and credentials")
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), profileTemplateValidationTimeout)
	defer cancel()
	err = svc.ValidateTemplate(ctx, templateID)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		return true
	}
	// Never forward provider bodies or transport errors: they may contain
	// credentials or other information outside the admin template contract.
	var apiErr *qnsandbox.APIError
	status, code, message := http.StatusBadGateway, "template_validation_unavailable", "Cannot validate the template with the admin Sandbox service; try again"
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		status, code, message = http.StatusGatewayTimeout, "template_validation_timeout", "Template validation timed out or was canceled; try again"
	case errors.Is(err, sandboxrunner.ErrTemplateNotFound):
		status, code, message = http.StatusBadRequest, "template_not_found", "Template was not found in the admin Sandbox service"
	case errors.Is(err, sandboxrunner.ErrTemplateNotReady):
		status, code, message = http.StatusBadRequest, "template_not_ready", "Template has no usable default build in the admin Sandbox service"
	case errors.Is(err, sandboxrunner.ErrTemplateStateUnavailable):
		code, message = "template_state_unavailable", "Template exists, but its usable default build cannot be confirmed in the owned or public default catalog"
	case errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden):
		code, message = "sandbox_template_access_denied", "The admin Sandbox credentials cannot access this template; check the API key and template permissions"
	}
	writeErrorCode(w, status, code, message)
	return false
}

func writeProfileConflict(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, state.ErrConflict) {
		return false
	}
	writeErrorCode(w, http.StatusConflict, "runner_spec_conflict", "Runner Spec changed while saving; refresh and try again")
	return true
}
