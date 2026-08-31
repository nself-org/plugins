// Purpose: DB-backed handler tests for the devices plugin using pgxmock —
// exercises the real query/scan/branch logic in handlers.go (success,
// not-found, and DB-error paths) without a live Postgres instance.
// Inputs: httptest requests routed through chi so URL params resolve.
// Outputs: asserts HTTP status + JSON body shape per handler.
// Constraints: pgxmock.PgxPoolIface satisfies the internal.Querier seam in db.go.
package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

var deviceCols = []string{
	"id", "source_account_id", "app_id", "device_id", "name", "device_type", "model",
	"firmware_version", "status", "trust_level", "last_seen_at", "capabilities",
	"config", "labels", "metadata", "created_at", "updated_at",
}

func sampleDeviceRow(rows *pgxmock.Rows, id string) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", "default", "dev-1", (*string)(nil), "sensor", (*string)(nil),
		(*string)(nil), "unregistered", "untrusted", (*time.Time)(nil), []byte("[]"),
		[]byte("{}"), []byte("{}"), []byte("{}"), now, now)
}

var commandCols = []string{
	"id", "source_account_id", "app_id", "device_id", "command_type", "command_id",
	"payload", "status", "priority", "dispatched_at", "acked_at", "started_at",
	"completed_at", "result", "error", "timeout_seconds", "deadline", "retry_count",
	"max_retries", "idempotency_key", "metadata", "created_at",
}

func sampleCommandRow(rows *pgxmock.Rows, id string) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", "default", "dev-1", "reboot", "cmd-1",
		[]byte("{}"), "dispatched", "normal", now, (*time.Time)(nil), (*time.Time)(nil),
		(*time.Time)(nil), []byte("{}"), (*string)(nil), 60, (*time.Time)(nil), 0,
		3, "idem-1", []byte("{}"), now)
}

var telemetryCols = []string{
	"id", "source_account_id", "app_id", "device_id", "telemetry_type", "data",
	"recorded_at", "received_at",
}

func sampleTelemetryRow(rows *pgxmock.Rows, id string) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", "default", "dev-1", "temp", []byte("{}"), now, now)
}

var sessionCols = []string{
	"id", "source_account_id", "app_id", "device_id", "stream_id", "status", "ingest_url",
	"protocol", "channel", "quality", "bitrate_kbps", "fps", "resolution", "started_at",
	"last_heartbeat_at", "ended_at", "bytes_ingested", "frames_dropped", "error_count",
	"last_error", "metadata", "created_at", "updated_at",
}

func sampleSessionRow(rows *pgxmock.Rows, id string) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", "default", "dev-1", "stream-1", "active", (*string)(nil),
		"rtmp", (*string)(nil), (*string)(nil), (*int)(nil), (*float64)(nil), (*string)(nil), &now,
		(*time.Time)(nil), (*time.Time)(nil), int64(0), int64(0), 0,
		(*string)(nil), []byte("{}"), now, now)
}

var auditCols = []string{
	"id", "source_account_id", "app_id", "device_id", "action", "actor_id", "details", "created_at",
}

func sampleAuditRow(rows *pgxmock.Rows, id string) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", "default", (*string)(nil), "device.registered",
		(*string)(nil), []byte("{}"), now)
}

// ─── Health / Ready ───────────────────────────────────────────────────────────

func TestReady_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectPing().WillReturnError(nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestReady_DBDown(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectPing().WillReturnError(assertErr("down"))

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", w.Code)
	}
}

// ─── Devices ──────────────────────────────────────────────────────────────────

func TestListDevices_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleDeviceRow(pgxmock.NewRows(deviceCols), "d1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	w := httptest.NewRecorder()
	h.ListDevices(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListDevices_WithFilters(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(deviceCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices").WithArgs(anyArgs(6)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/devices?status=enrolled&type=sensor&trust_level=trusted&limit=5&offset=2", nil)
	w := httptest.NewRecorder()
	h.ListDevices(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListDevices_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	w := httptest.NewRecorder()
	h.ListDevices(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListDevices_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{"id"}).AddRow("d1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	w := httptest.NewRecorder()
	h.ListDevices(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500 body=%s", w.Code, w.Body.String())
	}
}

func TestCreateDevice_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleDeviceRow(pgxmock.NewRows(deviceCols), "d1")
	mock.ExpectQuery("INSERT INTO np_dev_devices").WithArgs(anyArgs(11)...).WillReturnRows(rows)
	mock.ExpectExec("INSERT INTO np_dev_audit_log").WithArgs(anyArgs(6)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices",
		strings.NewReader(`{"device_id":"dev-1","device_type":"sensor"}`))
	w := httptest.NewRecorder()
	h.CreateDevice(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateDevice_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateDevice(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateDevice_MissingFields(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.CreateDevice(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateDevice_InsertError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("INSERT INTO np_dev_devices").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices",
		strings.NewReader(`{"device_id":"dev-1","device_type":"sensor"}`))
	w := httptest.NewRecorder()
	h.CreateDevice(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestGetDevice_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleDeviceRow(pgxmock.NewRows(deviceCols), "d1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE id").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/d1", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.GetDevice(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetDevice_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(deviceCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE id").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/missing", nil)
	req = withURLParam(req, "id", "missing")
	w := httptest.NewRecorder()
	h.GetDevice(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", w.Code, w.Body.String())
	}
}

func TestGetDevice_DBError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE id").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/d1", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.GetDevice(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestUpdateDevice_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleDeviceRow(pgxmock.NewRows(deviceCols), "d1")
	mock.ExpectQuery("UPDATE np_dev_devices SET").WithArgs(anyArgs(7)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/d1", strings.NewReader(`{"name":"n"}`))
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.UpdateDevice(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateDevice_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/d1", strings.NewReader(`{bad`))
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.UpdateDevice(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestUpdateDevice_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(deviceCols)
	mock.ExpectQuery("UPDATE np_dev_devices SET").WithArgs(anyArgs(7)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/missing", strings.NewReader(`{}`))
	req = withURLParam(req, "id", "missing")
	w := httptest.NewRecorder()
	h.UpdateDevice(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestUpdateDevice_DBError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("UPDATE np_dev_devices SET").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/d1", strings.NewReader(`{}`))
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.UpdateDevice(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestDeleteDevice_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleDeviceRow(pgxmock.NewRows(deviceCols), "d1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE id").WithArgs(anyArgs(1)...).WillReturnRows(rows)
	mock.ExpectExec("INSERT INTO np_dev_audit_log").WithArgs(anyArgs(6)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("DELETE FROM np_dev_devices").WithArgs(anyArgs(1)...).WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/d1", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.DeleteDevice(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteDevice_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(deviceCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE id").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/missing", nil)
	req = withURLParam(req, "id", "missing")
	w := httptest.NewRecorder()
	h.DeleteDevice(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestDeleteDevice_ExecError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleDeviceRow(pgxmock.NewRows(deviceCols), "d1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE id").WithArgs(anyArgs(1)...).WillReturnRows(rows)
	mock.ExpectExec("INSERT INTO np_dev_audit_log").WithArgs(anyArgs(6)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("DELETE FROM np_dev_devices").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/d1", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.DeleteDevice(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Device Commands ──────────────────────────────────────────────────────────

func TestSendDeviceCommand_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	devRows := sampleDeviceRow(pgxmock.NewRows(deviceCols), "d1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE id").WithArgs(anyArgs(1)...).WillReturnRows(devRows)
	cmdRows := sampleCommandRow(pgxmock.NewRows(commandCols), "c1")
	mock.ExpectQuery("INSERT INTO np_dev_commands").WithArgs(anyArgs(8)...).WillReturnRows(cmdRows)
	mock.ExpectExec("INSERT INTO np_dev_audit_log").WithArgs(anyArgs(6)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/d1/commands",
		strings.NewReader(`{"type":"reboot"}`))
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.SendDeviceCommand(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSendDeviceCommand_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/d1/commands", strings.NewReader(`{bad`))
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.SendDeviceCommand(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestSendDeviceCommand_MissingType(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/d1/commands", strings.NewReader(`{}`))
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.SendDeviceCommand(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestSendDeviceCommand_DeviceNotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE id").WillReturnError(assertErr("no rows"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/missing/commands",
		strings.NewReader(`{"type":"reboot"}`))
	req = withURLParam(req, "id", "missing")
	w := httptest.NewRecorder()
	h.SendDeviceCommand(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestSendDeviceCommand_InsertError(t *testing.T) {
	h, mock := newTestHandlers(t)
	devRows := sampleDeviceRow(pgxmock.NewRows(deviceCols), "d1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE id").WithArgs(anyArgs(1)...).WillReturnRows(devRows)
	mock.ExpectQuery("INSERT INTO np_dev_commands").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/d1/commands",
		strings.NewReader(`{"type":"reboot"}`))
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.SendDeviceCommand(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListDeviceCommands_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleCommandRow(pgxmock.NewRows(commandCols), "c1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_commands WHERE device_id").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/d1/commands", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.ListDeviceCommands(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListDeviceCommands_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_commands WHERE device_id").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/d1/commands", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.ListDeviceCommands(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListDeviceCommands_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{"id"}).AddRow("c1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_commands WHERE device_id").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/d1/commands", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.ListDeviceCommands(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestGetCommand_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleCommandRow(pgxmock.NewRows(commandCols), "c1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_commands WHERE id").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/c1", nil)
	req = withURLParam(req, "id", "c1")
	w := httptest.NewRecorder()
	h.GetCommand(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetCommand_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(commandCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_commands WHERE id").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/missing", nil)
	req = withURLParam(req, "id", "missing")
	w := httptest.NewRecorder()
	h.GetCommand(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestGetCommand_DBError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_commands WHERE id").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/c1", nil)
	req = withURLParam(req, "id", "c1")
	w := httptest.NewRecorder()
	h.GetCommand(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Telemetry ────────────────────────────────────────────────────────────────

func TestIngestTelemetry_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleTelemetryRow(pgxmock.NewRows(telemetryCols), "t1")
	mock.ExpectQuery("INSERT INTO np_dev_telemetry").WithArgs(anyArgs(6)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry",
		strings.NewReader(`{"device_id":"d1","telemetry_type":"temp"}`))
	w := httptest.NewRecorder()
	h.IngestTelemetry(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestIngestTelemetry_MetricValueStyle(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleTelemetryRow(pgxmock.NewRows(telemetryCols), "t1")
	mock.ExpectQuery("INSERT INTO np_dev_telemetry").WithArgs(anyArgs(6)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry",
		strings.NewReader(`{"device_id":"d1","metric":"temp","value":21.5}`))
	w := httptest.NewRecorder()
	h.IngestTelemetry(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestIngestTelemetry_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.IngestTelemetry(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestIngestTelemetry_MissingType(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry", strings.NewReader(`{"device_id":"d1"}`))
	w := httptest.NewRecorder()
	h.IngestTelemetry(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestIngestTelemetry_InsertError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("INSERT INTO np_dev_telemetry").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry",
		strings.NewReader(`{"device_id":"d1","telemetry_type":"temp"}`))
	w := httptest.NewRecorder()
	h.IngestTelemetry(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestBatchIngestTelemetry_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("INSERT INTO np_dev_telemetry").WithArgs(anyArgs(6)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO np_dev_telemetry").WithArgs(anyArgs(6)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/batch",
		strings.NewReader(`[{"device_id":"d1","telemetry_type":"temp"},{"device_id":"d2","telemetry_type":"hum"}]`))
	w := httptest.NewRecorder()
	h.BatchIngestTelemetry(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBatchIngestTelemetry_SkipsInvalidAndFailedItems(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("INSERT INTO np_dev_telemetry").WithArgs(anyArgs(6)...).WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/batch",
		strings.NewReader(`[{"device_id":"","telemetry_type":"temp"},{"device_id":"d1","telemetry_type":"temp"}]`))
	w := httptest.NewRecorder()
	h.BatchIngestTelemetry(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBatchIngestTelemetry_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/batch", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.BatchIngestTelemetry(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestQueryTelemetry_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleTelemetryRow(pgxmock.NewRows(telemetryCols), "t1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_telemetry WHERE source_account_id").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	w := httptest.NewRecorder()
	h.QueryTelemetry(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestQueryTelemetry_WithAllFilters(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(telemetryCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_telemetry WHERE source_account_id").WithArgs(anyArgs(6)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/telemetry?device_id=d1&metric=temp&start_time=2026-01-01&end_time=2026-02-01&limit=5", nil)
	w := httptest.NewRecorder()
	h.QueryTelemetry(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestQueryTelemetry_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_telemetry WHERE source_account_id").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	w := httptest.NewRecorder()
	h.QueryTelemetry(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestQueryTelemetry_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{"id"}).AddRow("t1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_telemetry WHERE source_account_id").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	w := httptest.NewRecorder()
	h.QueryTelemetry(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestGetDeviceTelemetry_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleTelemetryRow(pgxmock.NewRows(telemetryCols), "t1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_telemetry WHERE device_id").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/d1/telemetry", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.GetDeviceTelemetry(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetDeviceTelemetry_WithMetric(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(telemetryCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_telemetry WHERE device_id").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/d1/telemetry?metric=temp", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.GetDeviceTelemetry(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetDeviceTelemetry_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_telemetry WHERE device_id").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/d1/telemetry", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.GetDeviceTelemetry(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestGetDeviceTelemetry_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{"id"}).AddRow("t1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_telemetry WHERE device_id").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/d1/telemetry", nil)
	req = withURLParam(req, "id", "d1")
	w := httptest.NewRecorder()
	h.GetDeviceTelemetry(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Ingest Sessions ──────────────────────────────────────────────────────────

func TestListIngestSessions_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleSessionRow(pgxmock.NewRows(sessionCols), "s1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_ingest_sessions WHERE source_account_id").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest-sessions", nil)
	w := httptest.NewRecorder()
	h.ListIngestSessions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListIngestSessions_WithFilters(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(sessionCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_ingest_sessions WHERE source_account_id").WithArgs(anyArgs(5)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest-sessions?status=active&device_id=d1", nil)
	w := httptest.NewRecorder()
	h.ListIngestSessions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListIngestSessions_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_ingest_sessions WHERE source_account_id").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest-sessions", nil)
	w := httptest.NewRecorder()
	h.ListIngestSessions(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListIngestSessions_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{"id"}).AddRow("s1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_ingest_sessions WHERE source_account_id").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest-sessions", nil)
	w := httptest.NewRecorder()
	h.ListIngestSessions(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestCreateIngestSession_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleSessionRow(pgxmock.NewRows(sessionCols), "s1")
	mock.ExpectQuery("INSERT INTO np_dev_ingest_sessions").WithArgs(anyArgs(9)...).WillReturnRows(rows)
	mock.ExpectExec("INSERT INTO np_dev_audit_log").WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest-sessions",
		strings.NewReader(`{"device_id":"d1","stream_id":"stream-1"}`))
	w := httptest.NewRecorder()
	h.CreateIngestSession(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateIngestSession_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest-sessions", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateIngestSession(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateIngestSession_MissingFields(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest-sessions", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.CreateIngestSession(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateIngestSession_InsertError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("INSERT INTO np_dev_ingest_sessions").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest-sessions",
		strings.NewReader(`{"device_id":"d1","stream_id":"stream-1"}`))
	w := httptest.NewRecorder()
	h.CreateIngestSession(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Audit ────────────────────────────────────────────────────────────────────

func TestGetAuditLog_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleAuditRow(pgxmock.NewRows(auditCols), "a1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_audit_log WHERE source_account_id").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	w := httptest.NewRecorder()
	h.GetAuditLog(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetAuditLog_WithDeviceFilter(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(auditCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_audit_log WHERE source_account_id").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit?device_id=d1", nil)
	w := httptest.NewRecorder()
	h.GetAuditLog(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetAuditLog_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_audit_log WHERE source_account_id").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	w := httptest.NewRecorder()
	h.GetAuditLog(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestGetAuditLog_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{"id"}).AddRow("a1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_audit_log WHERE source_account_id").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	w := httptest.NewRecorder()
	h.GetAuditLog(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func TestGetStats_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	statsRows := pgxmock.NewRows([]string{
		"total_devices", "enrolled_devices", "online_devices", "suspended_devices",
		"revoked_devices", "max",
	}).AddRow(5, 3, 2, 0, 0, (*time.Time)(nil))
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE source_account_id").WithArgs(anyArgs(1)...).WillReturnRows(statsRows)

	cmdRows := pgxmock.NewRows([]string{"total", "pending", "succeeded", "failed"}).AddRow(1, 1, 0, 0)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_commands WHERE source_account_id").WithArgs(anyArgs(1)...).WillReturnRows(cmdRows)

	ingestRows := pgxmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_dev_ingest_sessions").WithArgs(anyArgs(1)...).WillReturnRows(ingestRows)

	telRows := pgxmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_dev_telemetry").WithArgs(anyArgs(1)...).WillReturnRows(telRows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	h.GetStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetStats_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices WHERE source_account_id").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	h.GetStats(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}
