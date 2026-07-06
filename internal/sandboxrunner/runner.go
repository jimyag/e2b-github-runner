package sandboxrunner

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	qnsandbox "github.com/qiniu/go-sdk/v7/sandbox"
)

type StartInput struct {
	RequestID         string
	RunnerName        string
	RepositoryURL     string
	RegistrationToken string
	Labels            []string
	RunnerGroup       string
	TemplateID        string
	Timeout           time.Duration
	CommandContext    context.Context
	OnStdout          func([]byte)
	OnStderr          func([]byte)
	OnExit            func(ExitResult, error)
}

type StartResult struct {
	SandboxID string
	PID       uint32
}

type PtySize struct {
	Cols uint32
	Rows uint32
}

type ExitResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Error    string
}

type TerminalSession interface {
	PID() uint32
	SendInput(ctx context.Context, data []byte) error
	Resize(ctx context.Context, size PtySize) error
	Close(ctx context.Context) error
}

type Service interface {
	ValidateTemplate(ctx context.Context, templateID string) error
	StartRunner(ctx context.Context, input StartInput) (StartResult, error)
	StopRunner(ctx context.Context, sandboxID string, pid uint32) error
	StartTerminal(ctx context.Context, sandboxID string, size PtySize, onData func([]byte)) (TerminalSession, error)
}

var (
	ErrTemplateRequired = errors.New("template_id is required")
	ErrTemplateNotFound = errors.New("template not found")
	ErrTemplateNotReady = errors.New("template is not ready")
)

type E2BService struct {
	client *qnsandbox.Client
}

//go:embed scripts/start-github-runner.sh
var startRunnerScriptTemplate string

func NewE2BService(apiKey, endpoint string, httpClient *http.Client) (*E2BService, error) {
	cfg := &qnsandbox.Config{APIKey: apiKey, Endpoint: endpoint, HTTPClient: httpClient}
	client, err := qnsandbox.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &E2BService{client: client}, nil
}

func (s *E2BService) ValidateTemplate(ctx context.Context, templateID string) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return ErrTemplateRequired
	}
	template, err := s.client.GetTemplate(ctx, templateID, nil)
	if err != nil {
		var apiErr *qnsandbox.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return ErrTemplateNotFound
		}
		return err
	}
	if len(template.Builds) == 0 {
		return nil
	}
	for _, build := range template.Builds {
		if templateBuildUsable(build.Status) {
			return nil
		}
	}
	return ErrTemplateNotReady
}

func templateBuildUsable(status qnsandbox.TemplateBuildStatus) bool {
	return status == qnsandbox.BuildStatusReady || status == qnsandbox.BuildStatusUploaded
}

func (s *E2BService) StartRunner(ctx context.Context, input StartInput) (StartResult, error) {
	timeout := int32(input.Timeout.Seconds())
	allowInternet := true
	metadata := qnsandbox.Metadata{
		"app":        "e2b-github-runner",
		"request_id": input.RequestID,
	}
	sb, _, err := s.client.CreateAndWait(ctx, qnsandbox.CreateParams{
		TemplateID:          input.TemplateID,
		Timeout:             &timeout,
		AllowInternetAccess: &allowInternet,
		Metadata:            &metadata,
	}, qnsandbox.WithPollInterval(500*time.Millisecond))
	if err != nil {
		return StartResult{}, err
	}

	if _, err := sb.Files().Write(ctx, "/tmp/start-github-runner.sh", []byte(startScript(input, sb.ID()))); err != nil {
		_ = sb.Kill(ctx)
		return StartResult{}, fmt.Errorf("write runner script: %w", err)
	}
	commandCtx := input.CommandContext
	if commandCtx == nil {
		commandCtx = context.Background()
	}
	cmd := "chmod +x /tmp/start-github-runner.sh && /tmp/start-github-runner.sh"
	handle, err := sb.Commands().Start(
		commandCtx, cmd,
		qnsandbox.WithTag("github-runner"),
		qnsandbox.WithOnStdout(input.OnStdout),
		qnsandbox.WithOnStderr(input.OnStderr),
	)
	if err != nil {
		_ = sb.Kill(ctx)
		return StartResult{}, fmt.Errorf("start runner command: %w", err)
	}
	pid, err := handle.WaitPID(ctx)
	if err != nil {
		_ = sb.Kill(ctx)
		return StartResult{}, fmt.Errorf("wait runner pid: %w", err)
	}
	if input.OnExit != nil {
		go func() {
			result, err := handle.Wait()
			if result == nil {
				input.OnExit(ExitResult{}, err)
				return
			}
			input.OnExit(ExitResult{
				ExitCode: result.ExitCode,
				Stdout:   result.Stdout,
				Stderr:   result.Stderr,
				Error:    result.Error,
			}, err)
		}()
	}
	return StartResult{SandboxID: sb.ID(), PID: pid}, nil
}

func (s *E2BService) StopRunner(ctx context.Context, sandboxID string, pid uint32) error {
	sb, err := s.client.Connect(ctx, sandboxID, qnsandbox.ConnectParams{Timeout: 30})
	if err != nil {
		return err
	}
	if pid != 0 {
		_ = sb.Commands().Kill(ctx, pid)
	}
	return sb.Kill(ctx)
}

type e2bTerminalSession struct {
	sandboxID string
	pty       interface {
		SendInput(context.Context, uint32, []byte) error
		Resize(context.Context, uint32, qnsandbox.PtySize) error
		Kill(context.Context, uint32) error
	}
	pid uint32
}

func (s *E2BService) StartTerminal(ctx context.Context, sandboxID string, size PtySize, onData func([]byte)) (TerminalSession, error) {
	sb, err := s.client.Connect(ctx, sandboxID, qnsandbox.ConnectParams{Timeout: 30})
	if err != nil {
		return nil, err
	}
	if size.Cols == 0 {
		size.Cols = 100
	}
	if size.Rows == 0 {
		size.Rows = 28
	}
	handle, err := sb.Pty().Create(
		ctx,
		qnsandbox.PtySize{Cols: size.Cols, Rows: size.Rows},
		qnsandbox.WithTag("runnerd-web-terminal"),
		qnsandbox.WithOnPtyData(onData),
	)
	if err != nil {
		return nil, err
	}
	pid, err := handle.WaitPID(ctx)
	if err != nil {
		return nil, err
	}
	return &e2bTerminalSession{sandboxID: sandboxID, pty: sb.Pty(), pid: pid}, nil
}

func (s *e2bTerminalSession) PID() uint32 {
	return s.pid
}

func (s *e2bTerminalSession) SendInput(ctx context.Context, data []byte) error {
	return s.pty.SendInput(ctx, s.pid, data)
}

func (s *e2bTerminalSession) Resize(ctx context.Context, size PtySize) error {
	if size.Cols == 0 || size.Rows == 0 {
		return nil
	}
	return s.pty.Resize(ctx, s.pid, qnsandbox.PtySize{Cols: size.Cols, Rows: size.Rows})
}

func (s *e2bTerminalSession) Close(ctx context.Context) error {
	return s.pty.Kill(ctx, s.pid)
}

func startScript(input StartInput, sandboxID string) string {
	labels := strings.Join(input.Labels, ",")
	return fmt.Sprintf(
		startRunnerScriptTemplate,
		base64.StdEncoding.EncodeToString([]byte(input.RepositoryURL)),
		base64.StdEncoding.EncodeToString([]byte(input.RegistrationToken)),
		base64.StdEncoding.EncodeToString([]byte(input.RunnerName)),
		base64.StdEncoding.EncodeToString([]byte(labels)),
		base64.StdEncoding.EncodeToString([]byte(input.RunnerGroup)),
		base64.StdEncoding.EncodeToString([]byte(input.RequestID)),
		base64.StdEncoding.EncodeToString([]byte(sandboxID)),
	)
}
