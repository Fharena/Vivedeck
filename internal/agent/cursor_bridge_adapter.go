package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

type CursorBridgeProcessConfig struct {
	Command        string
	Args           []string
	WorkingDir     string
	Env            []string
	StartupTimeout time.Duration
	CallTimeout    time.Duration
}

type CursorBridgeTCPConfig struct {
	Address        string
	DialTimeout    time.Duration
	StartupTimeout time.Duration
	CallTimeout    time.Duration
}

func DefaultCursorBridgeProcessConfig() (CursorBridgeProcessConfig, error) {
	workingDir := strings.TrimSpace(os.Getenv("CURSOR_BRIDGE_WORKDIR"))
	if workingDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return CursorBridgeProcessConfig{}, fmt.Errorf("get working directory: %w", err)
		}
		workingDir = cwd
	}

	args, err := defaultCursorBridgeArgs(workingDir)
	if err != nil {
		return CursorBridgeProcessConfig{}, err
	}

	extraEnv, err := jsonStringArrayEnv("CURSOR_BRIDGE_ENV_JSON")
	if err != nil {
		return CursorBridgeProcessConfig{}, err
	}

	command := strings.TrimSpace(os.Getenv("CURSOR_BRIDGE_BIN"))
	if command == "" {
		command = "node"
	}

	return CursorBridgeProcessConfig{
		Command:        command,
		Args:           args,
		WorkingDir:     workingDir,
		Env:            extraEnv,
		StartupTimeout: durationEnv("CURSOR_BRIDGE_STARTUP_TIMEOUT", 5*time.Second),
		CallTimeout:    durationEnv("CURSOR_BRIDGE_CALL_TIMEOUT", 10*time.Second),
	}, nil
}

func DefaultCursorBridgeTCPConfig() (CursorBridgeTCPConfig, error) {
	address := strings.TrimSpace(os.Getenv("CURSOR_BRIDGE_TCP_ADDR"))
	if address == "" {
		return CursorBridgeTCPConfig{}, errors.New("CURSOR_BRIDGE_TCP_ADDR is empty")
	}

	return CursorBridgeTCPConfig{
		Address:        address,
		DialTimeout:    durationEnv("CURSOR_BRIDGE_TCP_DIAL_TIMEOUT", 3*time.Second),
		StartupTimeout: durationEnv("CURSOR_BRIDGE_STARTUP_TIMEOUT", 5*time.Second),
		CallTimeout:    durationEnv("CURSOR_BRIDGE_CALL_TIMEOUT", 10*time.Second),
	}, nil
}

func NewWorkspaceAdapterFromEnv(ctx context.Context) (WorkspaceAdapter, io.Closer, error) {
	switch strings.TrimSpace(os.Getenv("WORKSPACE_ADAPTER_MODE")) {
	case "", "bridge":
	case "cursor_agent_cli":
		cfg, err := DefaultCursorAgentCLIConfig()
		if err != nil {
			return nil, nil, err
		}

		adapter, err := NewCursorAgentCLIAdapter(ctx, cfg)
		if err != nil {
			return nil, nil, err
		}
		return adapter, nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported WORKSPACE_ADAPTER_MODE %q", os.Getenv("WORKSPACE_ADAPTER_MODE"))
	}

	if strings.TrimSpace(os.Getenv("CURSOR_BRIDGE_TCP_ADDR")) != "" {
		cfg, err := DefaultCursorBridgeTCPConfig()
		if err != nil {
			return nil, nil, err
		}

		adapter, err := NewCursorBridgeTCPAdapter(ctx, cfg)
		if err != nil {
			return nil, nil, err
		}
		return adapter, adapter, nil
	}

	cfg, err := DefaultCursorBridgeProcessConfig()
	if err != nil {
		return nil, nil, err
	}

	adapter, err := NewCursorBridgeAdapter(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	return adapter, adapter, nil
}

type CursorBridgeAdapter struct {
	name         string
	capabilities AdapterCapabilities
	callTimeout  time.Duration

	stdin io.WriteCloser

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan cursorBridgeResponse

	stderrMu   sync.Mutex
	stderrTail []string

	requestSeq atomic.Int64

	done            chan struct{}
	waitErr         error
	waitErrMu       sync.RWMutex
	finishOnce      sync.Once
	closeOnce       sync.Once
	closeFn         func() error
	finishOnReadEOF bool
}

type cursorBridgeRequest struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type cursorBridgeResponse struct {
	ID     string             `json:"id"`
	Result json.RawMessage    `json:"result,omitempty"`
	Error  *cursorBridgeError `json:"error,omitempty"`
}

type cursorBridgeError struct {
	Message string `json:"message"`
}

func NewCursorBridgeAdapter(ctx context.Context, cfg CursorBridgeProcessConfig) (*CursorBridgeAdapter, error) {
	if cfg.Command == "" {
		return nil, errors.New("cursor bridge command is required")
	}

	cmd := exec.Command(cfg.Command, cfg.Args...)
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), cfg.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("cursor bridge stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("cursor bridge stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("cursor bridge stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start cursor bridge command %q: %w", cfg.Command, err)
	}

	adapter := newCursorBridgeAdapter(stdin, cfg.CallTimeout)
	adapter.closeFn = func() error {
		if adapter.stdin != nil {
			_ = adapter.stdin.Close()
		}

		select {
		case <-adapter.done:
			return nil
		case <-time.After(2 * time.Second):
		}

		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil
	}

	go adapter.readStdout(stdout)
	go adapter.readStderr(stderr)
	go adapter.waitForExit(cmd)

	if err := adapter.bootstrap(ctx, cfg.StartupTimeout); err != nil {
		_ = adapter.Close()
		return nil, err
	}

	return adapter, nil
}

func NewCursorBridgeTCPAdapter(ctx context.Context, cfg CursorBridgeTCPConfig) (*CursorBridgeAdapter, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, errors.New("cursor bridge tcp address is required")
	}

	dialCtx, cancel := withOptionalTimeout(ctx, cfg.DialTimeout)
	defer cancel()

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(dialCtx, "tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("dial cursor bridge tcp %q: %w", cfg.Address, err)
	}

	adapter := newCursorBridgeAdapter(conn, cfg.CallTimeout)
	adapter.closeFn = conn.Close
	adapter.finishOnReadEOF = true

	go adapter.readStdout(conn)

	if err := adapter.bootstrap(ctx, cfg.StartupTimeout); err != nil {
		_ = adapter.Close()
		return nil, fmt.Errorf("cursor bridge tcp handshake: %w", err)
	}

	return adapter, nil
}

func newCursorBridgeAdapter(stdin io.WriteCloser, callTimeout time.Duration) *CursorBridgeAdapter {
	return &CursorBridgeAdapter{
		callTimeout: callTimeout,
		stdin:       stdin,
		pending:     make(map[string]chan cursorBridgeResponse),
		done:        make(chan struct{}),
	}
}

func (a *CursorBridgeAdapter) bootstrap(ctx context.Context, startupTimeout time.Duration) error {
	startupCtx, cancel := withOptionalTimeout(ctx, startupTimeout)
	defer cancel()

	var name string
	if err := a.call(startupCtx, "name", nil, &name); err != nil {
		return fmt.Errorf("cursor bridge handshake(name): %w", err)
	}

	var capabilities AdapterCapabilities
	if err := a.call(startupCtx, "capabilities", nil, &capabilities); err != nil {
		return fmt.Errorf("cursor bridge handshake(capabilities): %w", err)
	}

	a.name = name
	a.capabilities = capabilities
	return nil
}

func (a *CursorBridgeAdapter) Name() string {
	return a.name
}

func (a *CursorBridgeAdapter) Capabilities() AdapterCapabilities {
	return a.capabilities
}

func (a *CursorBridgeAdapter) GetContext(ctx context.Context, input ContextRequest) (WorkspaceContext, error) {
	var out WorkspaceContext
	if err := a.call(ctx, "getContext", input, &out); err != nil {
		return WorkspaceContext{}, err
	}
	return out, nil
}

func (a *CursorBridgeAdapter) SubmitTask(ctx context.Context, input SubmitTaskInput) (TaskHandle, error) {
	var out TaskHandle
	if err := a.call(ctx, "submitTask", input, &out); err != nil {
		return TaskHandle{}, err
	}
	return out, nil
}

func (a *CursorBridgeAdapter) GetPatch(ctx context.Context, taskID string) (*protocol.PatchReadyPayload, error) {
	var out *protocol.PatchReadyPayload
	if err := a.call(ctx, "getPatch", map[string]string{"taskId": taskID}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *CursorBridgeAdapter) ApplyPatch(ctx context.Context, input ApplyPatchInput) (ApplyPatchResult, error) {
	var out ApplyPatchResult
	if err := a.call(ctx, "applyPatch", input, &out); err != nil {
		return ApplyPatchResult{}, err
	}
	return out, nil
}

func (a *CursorBridgeAdapter) RunProfile(ctx context.Context, input RunProfileInput) (RunHandle, error) {
	var out RunHandle
	if err := a.call(ctx, "runProfile", input, &out); err != nil {
		return RunHandle{}, err
	}
	return out, nil
}

func (a *CursorBridgeAdapter) GetRunResult(ctx context.Context, runID string) (*RunResult, error) {
	var out *RunResult
	if err := a.call(ctx, "getRunResult", map[string]string{"runId": runID}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *CursorBridgeAdapter) OpenLocation(ctx context.Context, input OpenLocationInput) error {
	return a.call(ctx, "openLocation", input, nil)
}

func (a *CursorBridgeAdapter) Close() error {
	a.closeOnce.Do(func() {
		if a.closeFn != nil {
			_ = a.closeFn()
			return
		}
		if a.stdin != nil {
			_ = a.stdin.Close()
		}
	})

	<-a.done
	return nil
}

func (a *CursorBridgeAdapter) call(ctx context.Context, method string, params any, out any) error {
	callCtx, cancel := withOptionalTimeout(ctx, a.callTimeout)
	defer cancel()

	requestID := fmt.Sprintf("bridge_%d", a.requestSeq.Add(1))
	responseCh := make(chan cursorBridgeResponse, 1)

	a.pendingMu.Lock()
	a.pending[requestID] = responseCh
	a.pendingMu.Unlock()

	request := cursorBridgeRequest{
		ID:     requestID,
		Method: method,
		Params: params,
	}

	payload, err := json.Marshal(request)
	if err != nil {
		a.dropPending(requestID)
		return fmt.Errorf("marshal cursor bridge request: %w", err)
	}

	a.writeMu.Lock()
	_, err = a.stdin.Write(append(payload, '\n'))
	a.writeMu.Unlock()
	if err != nil {
		a.dropPending(requestID)
		return fmt.Errorf("write cursor bridge request: %w", err)
	}

	select {
	case response, ok := <-responseCh:
		if !ok {
			return a.bridgeStoppedError()
		}
		if response.Error != nil {
			return errors.New(response.Error.Message)
		}
		if out == nil || len(response.Result) == 0 || bytes.Equal(response.Result, []byte("null")) {
			return nil
		}
		if err := json.Unmarshal(response.Result, out); err != nil {
			return fmt.Errorf("decode cursor bridge response: %w", err)
		}
		return nil
	case <-callCtx.Done():
		a.dropPending(requestID)
		return callCtx.Err()
	case <-a.done:
		a.dropPending(requestID)
		return a.bridgeStoppedError()
	}
}

func (a *CursorBridgeAdapter) readStdout(stdout io.Reader) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			var response cursorBridgeResponse
			if unmarshalErr := json.Unmarshal(trimmed, &response); unmarshalErr != nil {
				a.finish(fmt.Errorf("decode cursor bridge stdout: %w", unmarshalErr))
				return
			}
			a.deliver(response)
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				if a.finishOnReadEOF {
					a.finish(nil)
				}
				return
			}
			a.finish(fmt.Errorf("read cursor bridge stdout: %w", err))
			return
		}
	}
}

func (a *CursorBridgeAdapter) readStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
	for scanner.Scan() {
		a.appendStderr(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		a.appendStderr("stderr read failed: " + err.Error())
	}
}

func (a *CursorBridgeAdapter) waitForExit(cmd *exec.Cmd) {
	err := cmd.Wait()
	if err == nil {
		a.finish(nil)
		return
	}
	a.finish(fmt.Errorf("cursor bridge process exited: %w", err))
}

func (a *CursorBridgeAdapter) deliver(response cursorBridgeResponse) {
	a.pendingMu.Lock()
	ch, ok := a.pending[response.ID]
	if ok {
		delete(a.pending, response.ID)
	}
	a.pendingMu.Unlock()

	if ok {
		ch <- response
		close(ch)
	}
}

func (a *CursorBridgeAdapter) dropPending(requestID string) {
	a.pendingMu.Lock()
	delete(a.pending, requestID)
	a.pendingMu.Unlock()
}

func (a *CursorBridgeAdapter) finish(err error) {
	a.finishOnce.Do(func() {
		a.waitErrMu.Lock()
		a.waitErr = err
		a.waitErrMu.Unlock()

		close(a.done)

		a.pendingMu.Lock()
		pending := a.pending
		a.pending = make(map[string]chan cursorBridgeResponse)
		a.pendingMu.Unlock()

		for _, ch := range pending {
			close(ch)
		}
	})
}

func (a *CursorBridgeAdapter) appendStderr(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	a.stderrMu.Lock()
	defer a.stderrMu.Unlock()

	a.stderrTail = append(a.stderrTail, trimmed)
	if len(a.stderrTail) > 20 {
		a.stderrTail = a.stderrTail[len(a.stderrTail)-20:]
	}
}

func (a *CursorBridgeAdapter) bridgeStoppedError() error {
	a.waitErrMu.RLock()
	err := a.waitErr
	a.waitErrMu.RUnlock()

	if err == nil {
		err = errors.New("cursor bridge connection closed")
	}

	stderr := a.stderrSummary()
	if stderr == "" {
		return err
	}
	return fmt.Errorf("%w (stderr: %s)", err, stderr)
}

func (a *CursorBridgeAdapter) stderrSummary() string {
	a.stderrMu.Lock()
	defer a.stderrMu.Unlock()

	return strings.Join(a.stderrTail, " | ")
}

func defaultCursorBridgeArgs(workingDir string) ([]string, error) {
	rawArgs := strings.TrimSpace(os.Getenv("CURSOR_BRIDGE_ARGS_JSON"))
	if rawArgs != "" {
		args, err := jsonStringArray(rawArgs)
		if err != nil {
			return nil, fmt.Errorf("parse CURSOR_BRIDGE_ARGS_JSON: %w", err)
		}
		return args, nil
	}

	entry := strings.TrimSpace(os.Getenv("CURSOR_BRIDGE_ENTRY"))
	if entry == "" {
		entry = filepath.Join("adapters", "cursor-bridge", "dist", "fixtureBridgeMain.js")
	}

	entryPath := entry
	if !filepath.IsAbs(entryPath) {
		entryPath = filepath.Join(workingDir, entryPath)
	}
	if _, err := os.Stat(entryPath); err != nil {
		return nil, fmt.Errorf(
			"cursor bridge entry not found: %s (run `npm --prefix adapters/cursor-bridge install` and `npm --prefix adapters/cursor-bridge run build`)",
			entryPath,
		)
	}

	return []string{entry}, nil
}

func jsonStringArrayEnv(key string) ([]string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil, nil
	}
	result, err := jsonStringArray(value)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", key, err)
	}
	return result, nil
}

func jsonStringArray(value string) ([]string, error) {
	var result []string
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

func withOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return ctx, func() {}
}
