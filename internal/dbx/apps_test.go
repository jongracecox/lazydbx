package dbx

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/apps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

func TestAppFromSDK(t *testing.T) {
	sdkApp := &apps.App{
		Name:                 "dash",
		Description:          "a dashboard",
		Url:                  "https://dash.example.com",
		Creator:              "jon@example.com",
		ServicePrincipalName: "dash-sp",
		CreateTime:           "2026-01-02T03:04:05Z",
		ComputeStatus:        &apps.ComputeStatus{State: apps.ComputeStateActive, Message: "ready"},
		AppStatus:            &apps.ApplicationStatus{State: apps.ApplicationStateRunning},
		ActiveDeployment: &apps.AppDeployment{
			DeploymentId: "dep-1",
			Status:       &apps.AppDeploymentStatus{State: apps.AppDeploymentStateSucceeded},
		},
	}

	got := appFromSDK(sdkApp)
	assert.Equal(t, "dash", got.Name)
	assert.Equal(t, "https://dash.example.com", got.URL)
	assert.Equal(t, "ACTIVE", got.ComputeState)
	assert.Equal(t, "ready", got.ComputeMessage)
	assert.Equal(t, "RUNNING", got.AppState)
	assert.Equal(t, "dep-1", got.ActiveDeploymentID)
	assert.Equal(t, "SUCCEEDED", got.ActiveDeploymentState)
	assert.Equal(t, 2026, got.CreatedAt.Year())
}

func TestAppFromSDKNilStatuses(t *testing.T) {
	got := appFromSDK(&apps.App{Name: "bare"})
	assert.Equal(t, "bare", got.Name)
	assert.Empty(t, got.ComputeState)
	assert.Empty(t, got.AppState)
	assert.Empty(t, got.ActiveDeploymentState)
	assert.True(t, got.CreatedAt.IsZero())
}

func TestAppLogsWSURL(t *testing.T) {
	assert.Equal(t, "wss://dash.example.com/logz/stream", appLogsWSURL("https://dash.example.com"))
	assert.Equal(t, "ws://127.0.0.1:8080/logz/stream", appLogsWSURL("http://127.0.0.1:8080"))
}

func TestParseAppLogFrame(t *testing.T) {
	full := parseAppLogFrame(`{"source":"APP","timestamp":1784290968,"severity":"INFO","message":"hello\n"}`)
	assert.Equal(t, "APP", full.Source)
	assert.Equal(t, "INFO", full.Severity)
	assert.Equal(t, "hello", full.Message, "trailing newline trimmed")
	assert.Equal(t, 2026, full.Time.Year(), "epoch seconds converted")
	assert.Contains(t, full.Raw, "1784290968", "raw frame retained for drill-down")

	bare := parseAppLogFrame("not json at all")
	assert.Equal(t, "not json at all", bare.Message)
	assert.True(t, bare.Time.IsZero())
	assert.Equal(t, "not json at all", bare.Raw)
}

func TestEpochToTime(t *testing.T) {
	assert.True(t, epochToTime(0).IsZero())
	assert.Equal(t, 2026, epochToTime(1784290968).Year(), "seconds")
	assert.Equal(t, 2026, epochToTime(1784290968000).Year(), "milliseconds")
	assert.Equal(t, 2026, epochToTime(1784290968000000).Year(), "microseconds")
}

// TestGetLogsStreamsFrames exercises the full /logz/stream protocol against a
// local WebSocket server: GetLogs must send the filter, then read and parse
// frames into entries until the server closes.
func TestGetLogsStreamsFrames(t *testing.T) {
	var gotFilter string
	srv := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		// The client sends its search filter first.
		_ = websocket.Message.Receive(ws, &gotFilter)
		_ = websocket.Message.Send(ws, `{"timestamp":1784290968,"source":"APP","severity":"INFO","message":"first"}`)
		_ = websocket.Message.Send(ws, `{"timestamp":1784290969,"source":"APP","severity":"ERROR","message":"second"}`)
		// Closing the handler closes the socket → client sees EOF and returns.
	}))
	defer srv.Close()

	dao := appsDAO{w: testWorkspaceClient(t, srv.URL)}
	entries, err := dao.GetLogs(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Empty(t, gotFilter, "client sends an empty filter for all logs")
	require.Len(t, entries, 2)
	assert.Equal(t, "first", entries[0].Message)
	assert.Equal(t, "INFO", entries[0].Severity)
	assert.Equal(t, "ERROR", entries[1].Severity)
}

func TestGetLogsNoLogsSentinel(t *testing.T) {
	srv := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		var filter string
		_ = websocket.Message.Receive(ws, &filter)
		_ = websocket.Message.Send(ws, "\x00")
	}))
	defer srv.Close()

	dao := appsDAO{w: testWorkspaceClient(t, srv.URL)}
	entries, err := dao.GetLogs(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Empty(t, entries, "sentinel yields no entries")
}

func TestGetLogsEmptyURL(t *testing.T) {
	dao := appsDAO{}
	entries, err := dao.GetLogs(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// testWorkspaceClient builds an offline PAT-auth workspace client pointed at
// host, so Config.Authenticate attaches a bearer token without any network.
func testWorkspaceClient(t *testing.T, host string) *databricks.WorkspaceClient {
	t.Helper()
	w, err := databricks.NewWorkspaceClient(&databricks.Config{
		Host:  host,
		Token: "test-token",
	})
	require.NoError(t, err)
	return w
}
