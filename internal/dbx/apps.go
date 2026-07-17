package dbx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/apps"
	"golang.org/x/net/websocket"
)

// App log streaming timeouts. The /logz/stream socket sends the current buffer
// after we send our filter, then goes quiet until new logs arrive; firstRead
// waits for that initial burst, idleRead treats a subsequent gap as "backlog
// drained" so a one-shot fetch returns promptly.
const (
	appLogsFirstTimeout = 6 * time.Second
	appLogsIdleTimeout  = 1500 * time.Millisecond
)

// appLogsNoLogsSentinel is the single-byte frame the server sends to mean
// "there are currently no logs".
const appLogsNoLogsSentinel = "\x00"

// appsDAO implements AppsDAO against the SDK. Like the other DAOs it stays
// thin: pagination and type mapping only.
type appsDAO struct {
	w *databricks.WorkspaceClient
}

func (d appsDAO) List(ctx context.Context) ([]App, error) {
	items, err := d.w.Apps.ListAll(ctx, apps.ListAppsRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing apps: %w", err)
	}
	out := make([]App, 0, len(items))
	for i := range items {
		out = append(out, appFromSDK(&items[i]))
	}
	sortByName(out, func(a App) string { return a.Name })
	return out, nil
}

func appFromSDK(a *apps.App) App {
	app := App{
		Name:                 a.Name,
		Description:          a.Description,
		URL:                  a.Url,
		Creator:              a.Creator,
		ServicePrincipalName: a.ServicePrincipalName,
		CreatedAt:            parseISOTime(a.CreateTime),
		UpdatedAt:            parseISOTime(a.UpdateTime),
	}
	if a.ComputeStatus != nil {
		app.ComputeState = string(a.ComputeStatus.State)
		app.ComputeMessage = a.ComputeStatus.Message
	}
	if a.AppStatus != nil {
		app.AppState = string(a.AppStatus.State)
		app.AppMessage = a.AppStatus.Message
	}
	if a.ActiveDeployment != nil {
		app.ActiveDeploymentID = a.ActiveDeployment.DeploymentId
		if a.ActiveDeployment.Status != nil {
			app.ActiveDeploymentState = string(a.ActiveDeployment.Status.State)
		}
	}
	return app
}

// GetLogs fetches the app's runtime logs from its own host, streamed over the
// WebSocket at appURL + "/logz/stream". There is deliberately no SDK call for
// this: the Apps service exposes no logs API, and the logs live on the app's
// host rather than the workspace host, so this is the one sanctioned raw
// authenticated connection in dbx.
//
// Protocol (matching the /logz web client): connect, send the search filter
// (empty = all logs), then read frames. Each frame is a JSON object
// {timestamp, source, message}; a lone NUL byte means "no logs". The server
// sends the current buffer then streams live, so this drains until an idle gap
// and returns a snapshot — the log viewer's poll re-invokes it to refresh.
//
// Auth is attached via the SDK's unified auth (Config.Authenticate) by copying
// the headers it sets onto the handshake; the /logz host may require app-scoped
// OAuth, so a plain PAT profile can be rejected — the error is surfaced to the
// log viewer rather than swallowed.
func (d appsDAO) GetLogs(ctx context.Context, appURL string) ([]AppLogEntry, error) {
	appURL = strings.TrimRight(appURL, "/")
	if appURL == "" {
		return nil, nil
	}

	config, err := websocket.NewConfig(appLogsWSURL(appURL), appURL)
	if err != nil {
		return nil, fmt.Errorf("building app logs request: %w", err)
	}
	authReq, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL+"/logz/stream", nil)
	if err != nil {
		return nil, fmt.Errorf("building app logs request: %w", err)
	}
	if err := d.w.Config.Authenticate(authReq); err != nil {
		return nil, fmt.Errorf("authenticating app logs request: %w", err)
	}
	config.Header = authReq.Header

	ws, err := websocket.DialConfig(config)
	if err != nil {
		return nil, fmt.Errorf("connecting to app logs: %w", err)
	}
	defer func() { _ = ws.Close() }()

	// Close the socket when ctx is cancelled to unblock a pending read.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = ws.Close()
		case <-stop:
		}
	}()

	// The server only streams once it receives the client's filter.
	if err := websocket.Message.Send(ws, ""); err != nil {
		return nil, fmt.Errorf("requesting app logs: %w", err)
	}

	var entries []AppLogEntry
	for i := 0; ; i++ {
		deadline := appLogsIdleTimeout
		if i == 0 {
			deadline = appLogsFirstTimeout
		}
		_ = ws.SetReadDeadline(time.Now().Add(deadline))

		var frame string
		if err := websocket.Message.Receive(ws, &frame); err != nil {
			// A read gap (idle) or clean close ends the drain; return what we
			// have. A hard error before any logs is worth surfacing.
			var nerr net.Error
			if errors.Is(err, io.EOF) || (errors.As(err, &nerr) && nerr.Timeout()) {
				break
			}
			if len(entries) == 0 {
				return nil, fmt.Errorf("reading app logs: %w", err)
			}
			break
		}
		if frame == appLogsNoLogsSentinel {
			continue
		}
		entries = append(entries, parseAppLogFrame(frame))
	}

	return entries, nil
}

// appLogsWSURL maps an app's base URL to its /logz/stream WebSocket URL,
// upgrading the scheme (https→wss, http→ws for tests).
func appLogsWSURL(appURL string) string {
	switch {
	case strings.HasPrefix(appURL, "https://"):
		return "wss://" + strings.TrimPrefix(appURL, "https://") + "/logz/stream"
	case strings.HasPrefix(appURL, "http://"):
		return "ws://" + strings.TrimPrefix(appURL, "http://") + "/logz/stream"
	default:
		return appURL + "/logz/stream"
	}
}

// appLogFrame is the JSON shape of one /logz/stream frame. timestamp is epoch
// seconds (an integer), not an ISO string.
type appLogFrame struct {
	Timestamp int64  `json:"timestamp"`
	Source    string `json:"source"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
}

// parseAppLogFrame decodes one stream frame into an AppLogEntry, keeping the
// raw JSON for the drill-down. A non-JSON frame becomes a bare message entry
// (matching the web client's fallback).
func parseAppLogFrame(frame string) AppLogEntry {
	var f appLogFrame
	if err := json.Unmarshal([]byte(frame), &f); err != nil {
		return AppLogEntry{Message: strings.TrimRight(frame, "\r\n"), Raw: frame}
	}
	return AppLogEntry{
		Time:     epochToTime(f.Timestamp),
		Severity: f.Severity,
		Source:   f.Source,
		Message:  strings.TrimRight(f.Message, "\r\n"),
		Raw:      frame,
	}
}

// epochToTime converts an epoch timestamp to UTC, tolerating seconds,
// milliseconds, or microseconds (the stream uses seconds, but be defensive).
func epochToTime(ts int64) time.Time {
	switch {
	case ts <= 0:
		return time.Time{}
	case ts > 1e15:
		return time.UnixMicro(ts).UTC()
	case ts > 1e12:
		return time.UnixMilli(ts).UTC()
	default:
		return time.Unix(ts, 0).UTC()
	}
}

// parseISOTime parses an ISO-8601 timestamp string (as the Apps API returns),
// falling back to the zero time on empty/unparseable input.
func parseISOTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
