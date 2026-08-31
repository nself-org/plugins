package internal

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

type Handlers struct {
    pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers {
    return &Handlers{pool: pool}
}

func (h *Handlers) sourceAccount(r *http.Request) string {
    if sa := r.Header.Get("X-Source-Account-ID"); sa != "" {
        return sa
    }
    return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "workflows"})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()
    if err := h.pool.Ping(ctx); err != nil {
        writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": err.Error()})
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handlers) ListWorkflows(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    rows, err := h.pool.Query(ctx,
        `SELECT id, source_account_id, name, description, status, version, created_at, updated_at
         FROM np_workflows_workflows WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query workflows: %v", err)})
        return
    }
    defer rows.Close()
    var workflows []Workflow
    for rows.Next() {
        var wf Workflow
        if err := rows.Scan(&wf.ID, &wf.SourceAccountID, &wf.Name, &wf.Description, &wf.Status, &wf.Version, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
            return
        }
        workflows = append(workflows, wf)
    }
    if workflows == nil {
        workflows = []Workflow{}
    }
    writeJSON(w, http.StatusOK, map[string]any{"data": workflows, "total": len(workflows)})
}

func (h *Handlers) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    var body struct {
        Name        string         `json:"name"`
        Description string         `json:"description"`
        Definition  map[string]any `json:"definition"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }
    if body.Name == "" {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
        return
    }
    defJSON, _ := json.Marshal(body.Definition)
    var wf Workflow
    err := h.pool.QueryRow(ctx,
        `INSERT INTO np_workflows_workflows (source_account_id, name, description, status, definition, version)
         VALUES ($1, $2, $3, 'draft', $4, 1)
         RETURNING id, source_account_id, name, description, status, version, created_at, updated_at`,
        sa, body.Name, body.Description, defJSON,
    ).Scan(&wf.ID, &wf.SourceAccountID, &wf.Name, &wf.Description, &wf.Status, &wf.Version, &wf.CreatedAt, &wf.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert workflow: %v", err)})
        return
    }
    writeJSON(w, http.StatusCreated, map[string]any{"workflow": wf})
}

func (h *Handlers) GetWorkflow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var wf Workflow
    err := h.pool.QueryRow(ctx,
        `SELECT id, source_account_id, name, description, status, version, created_at, updated_at
         FROM np_workflows_workflows WHERE id=$1 AND source_account_id=$2`, id, sa,
    ).Scan(&wf.ID, &wf.SourceAccountID, &wf.Name, &wf.Description, &wf.Status, &wf.Version, &wf.CreatedAt, &wf.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

func (h *Handlers) UpdateWorkflow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var body struct {
        Name        *string `json:"name"`
        Description *string `json:"description"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }
    var wf Workflow
    err := h.pool.QueryRow(ctx,
        `UPDATE np_workflows_workflows SET
            name = COALESCE($3, name),
            description = COALESCE($4, description),
            updated_at = NOW()
         WHERE id=$1 AND source_account_id=$2
         RETURNING id, source_account_id, name, description, status, version, created_at, updated_at`,
        id, sa, body.Name, body.Description,
    ).Scan(&wf.ID, &wf.SourceAccountID, &wf.Name, &wf.Description, &wf.Status, &wf.Version, &wf.CreatedAt, &wf.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

func (h *Handlers) DeleteWorkflow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    tag, err := h.pool.Exec(ctx, `DELETE FROM np_workflows_workflows WHERE id=$1 AND source_account_id=$2`, id, sa)
    if err != nil || tag.RowsAffected() == 0 {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) PublishWorkflow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var wf Workflow
    err := h.pool.QueryRow(ctx,
        `UPDATE np_workflows_workflows SET status='active', updated_at=NOW() WHERE id=$1 AND source_account_id=$2
         RETURNING id, source_account_id, name, description, status, version, created_at, updated_at`,
        id, sa,
    ).Scan(&wf.ID, &wf.SourceAccountID, &wf.Name, &wf.Description, &wf.Status, &wf.Version, &wf.CreatedAt, &wf.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

func (h *Handlers) UnpublishWorkflow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var wf Workflow
    err := h.pool.QueryRow(ctx,
        `UPDATE np_workflows_workflows SET status='draft', updated_at=NOW() WHERE id=$1 AND source_account_id=$2
         RETURNING id, source_account_id, name, description, status, version, created_at, updated_at`,
        id, sa,
    ).Scan(&wf.ID, &wf.SourceAccountID, &wf.Name, &wf.Description, &wf.Status, &wf.Version, &wf.CreatedAt, &wf.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

func (h *Handlers) DuplicateWorkflow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var body struct {
        Name string `json:"name"`
    }
    json.NewDecoder(r.Body).Decode(&body)
    var srcName string
    h.pool.QueryRow(ctx, `SELECT name FROM np_workflows_workflows WHERE id=$1 AND source_account_id=$2`, id, sa).Scan(&srcName)
    if srcName == "" {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"})
        return
    }
    newName := body.Name
    if newName == "" {
        newName = srcName + " (copy)"
    }
    var wf Workflow
    err := h.pool.QueryRow(ctx,
        `INSERT INTO np_workflows_workflows (source_account_id, name, description, status, definition, version)
         SELECT source_account_id, $3, description, 'draft', definition, 1 FROM np_workflows_workflows WHERE id=$1 AND source_account_id=$2
         RETURNING id, source_account_id, name, description, status, version, created_at, updated_at`,
        id, sa, newName,
    ).Scan(&wf.ID, &wf.SourceAccountID, &wf.Name, &wf.Description, &wf.Status, &wf.Version, &wf.CreatedAt, &wf.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("duplicate: %v", err)})
        return
    }
    writeJSON(w, http.StatusCreated, map[string]any{"workflow": wf})
}

func (h *Handlers) ExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    wfID := chi.URLParam(r, "id")
    var exec Execution
    err := h.pool.QueryRow(ctx,
        `INSERT INTO np_workflows_executions (source_account_id, workflow_id, status, triggered_by)
         VALUES ($1, $2, 'pending', 'manual')
         RETURNING id, source_account_id, workflow_id, status, triggered_by, started_at, completed_at, created_at`,
        sa, wfID,
    ).Scan(&exec.ID, &exec.SourceAccountID, &exec.WorkflowID, &exec.Status, &exec.TriggeredBy, &exec.StartedAt, &exec.CompletedAt, &exec.CreatedAt)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("create execution: %v", err)})
        return
    }
    writeJSON(w, http.StatusCreated, map[string]any{"execution": exec})
}

func (h *Handlers) TestWorkflow(w http.ResponseWriter, r *http.Request) {
    h.ExecuteWorkflow(w, r)
}

func (h *Handlers) ListExecutions(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    wfID := chi.URLParam(r, "id")
    rows, err := h.pool.Query(ctx,
        `SELECT id, source_account_id, workflow_id, status, triggered_by, started_at, completed_at, created_at
         FROM np_workflows_executions WHERE workflow_id=$1 AND source_account_id=$2 ORDER BY created_at DESC LIMIT 50`,
        wfID, sa)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query executions: %v", err)})
        return
    }
    defer rows.Close()
    var execs []Execution
    for rows.Next() {
        var e Execution
        if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.WorkflowID, &e.Status, &e.TriggeredBy, &e.StartedAt, &e.CompletedAt, &e.CreatedAt); err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
            return
        }
        execs = append(execs, e)
    }
    if execs == nil {
        execs = []Execution{}
    }
    writeJSON(w, http.StatusOK, map[string]any{"data": execs, "total": len(execs)})
}

func (h *Handlers) ListAllExecutions(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    rows, err := h.pool.Query(ctx,
        `SELECT id, source_account_id, workflow_id, status, triggered_by, started_at, completed_at, created_at
         FROM np_workflows_executions WHERE source_account_id=$1 ORDER BY created_at DESC LIMIT 100`, sa)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query executions: %v", err)})
        return
    }
    defer rows.Close()
    var execs []Execution
    for rows.Next() {
        var e Execution
        if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.WorkflowID, &e.Status, &e.TriggeredBy, &e.StartedAt, &e.CompletedAt, &e.CreatedAt); err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
            return
        }
        execs = append(execs, e)
    }
    if execs == nil {
        execs = []Execution{}
    }
    writeJSON(w, http.StatusOK, map[string]any{"data": execs, "total": len(execs)})
}

func (h *Handlers) GetExecution(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var e Execution
    err := h.pool.QueryRow(ctx,
        `SELECT id, source_account_id, workflow_id, status, triggered_by, started_at, completed_at, created_at
         FROM np_workflows_executions WHERE id=$1 AND source_account_id=$2`, id, sa,
    ).Scan(&e.ID, &e.SourceAccountID, &e.WorkflowID, &e.Status, &e.TriggeredBy, &e.StartedAt, &e.CompletedAt, &e.CreatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "execution not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"execution": e})
}

func (h *Handlers) CancelExecution(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    tag, err := h.pool.Exec(ctx,
        `UPDATE np_workflows_executions SET status='cancelled', updated_at=NOW() WHERE id=$1 AND source_account_id=$2 AND status IN ('pending','running')`,
        id, sa)
    if err != nil || tag.RowsAffected() == 0 {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "execution not found or already completed"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) ListExecutionSteps(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    id := chi.URLParam(r, "id")
    rows, err := h.pool.Query(ctx,
        `SELECT id, execution_id, step_name, status, started_at, completed_at, created_at
         FROM np_workflows_execution_steps WHERE execution_id=$1 ORDER BY created_at ASC`, id)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query steps: %v", err)})
        return
    }
    defer rows.Close()
    var steps []ExecutionStep
    for rows.Next() {
        var s ExecutionStep
        if err := rows.Scan(&s.ID, &s.ExecutionID, &s.StepName, &s.Status, &s.StartedAt, &s.CompletedAt, &s.CreatedAt); err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
            return
        }
        steps = append(steps, s)
    }
    if steps == nil {
        steps = []ExecutionStep{}
    }
    writeJSON(w, http.StatusOK, map[string]any{"data": steps, "total": len(steps)})
}

func (h *Handlers) ListTriggers(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    rows, err := h.pool.Query(ctx,
        `SELECT id, source_account_id, workflow_id, trigger_type, is_active, created_at, updated_at
         FROM np_workflows_triggers WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query triggers: %v", err)})
        return
    }
    defer rows.Close()
    var triggers []Trigger
    for rows.Next() {
        var t Trigger
        if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.WorkflowID, &t.TriggerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
            return
        }
        triggers = append(triggers, t)
    }
    if triggers == nil {
        triggers = []Trigger{}
    }
    writeJSON(w, http.StatusOK, map[string]any{"data": triggers, "total": len(triggers)})
}

func (h *Handlers) CreateTrigger(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    var body struct {
        WorkflowID  string         `json:"workflow_id"`
        TriggerType string         `json:"trigger_type"`
        Config      map[string]any `json:"config"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }
    if body.WorkflowID == "" || body.TriggerType == "" {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workflow_id and trigger_type are required"})
        return
    }
    cfgJSON, _ := json.Marshal(body.Config)
    var t Trigger
    err := h.pool.QueryRow(ctx,
        `INSERT INTO np_workflows_triggers (source_account_id, workflow_id, trigger_type, config)
         VALUES ($1, $2, $3, $4)
         RETURNING id, source_account_id, workflow_id, trigger_type, is_active, created_at, updated_at`,
        sa, body.WorkflowID, body.TriggerType, cfgJSON,
    ).Scan(&t.ID, &t.SourceAccountID, &t.WorkflowID, &t.TriggerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert trigger: %v", err)})
        return
    }
    writeJSON(w, http.StatusCreated, map[string]any{"trigger": t})
}

func (h *Handlers) GetTrigger(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var t Trigger
    err := h.pool.QueryRow(ctx,
        `SELECT id, source_account_id, workflow_id, trigger_type, is_active, created_at, updated_at
         FROM np_workflows_triggers WHERE id=$1 AND source_account_id=$2`, id, sa,
    ).Scan(&t.ID, &t.SourceAccountID, &t.WorkflowID, &t.TriggerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "trigger not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"trigger": t})
}

func (h *Handlers) UpdateTrigger(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var body struct {
        IsActive *bool `json:"is_active"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }
    var t Trigger
    err := h.pool.QueryRow(ctx,
        `UPDATE np_workflows_triggers SET is_active=COALESCE($3, is_active), updated_at=NOW()
         WHERE id=$1 AND source_account_id=$2
         RETURNING id, source_account_id, workflow_id, trigger_type, is_active, created_at, updated_at`,
        id, sa, body.IsActive,
    ).Scan(&t.ID, &t.SourceAccountID, &t.WorkflowID, &t.TriggerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "trigger not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"trigger": t})
}

func (h *Handlers) DeleteTrigger(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    tag, err := h.pool.Exec(ctx, `DELETE FROM np_workflows_triggers WHERE id=$1 AND source_account_id=$2`, id, sa)
    if err != nil || tag.RowsAffected() == 0 {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "trigger not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    rows, err := h.pool.Query(ctx,
        `SELECT id, source_account_id, name, description, category, is_public, created_at, updated_at
         FROM np_workflows_templates WHERE source_account_id=$1 OR is_public=true ORDER BY created_at DESC`, sa)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query templates: %v", err)})
        return
    }
    defer rows.Close()
    var templates []Template
    for rows.Next() {
        var t Template
        if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.Name, &t.Description, &t.Category, &t.IsPublic, &t.CreatedAt, &t.UpdatedAt); err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
            return
        }
        templates = append(templates, t)
    }
    if templates == nil {
        templates = []Template{}
    }
    writeJSON(w, http.StatusOK, map[string]any{"data": templates, "total": len(templates)})
}

func (h *Handlers) CreateTemplate(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    var body struct {
        Name        string         `json:"name"`
        Description string         `json:"description"`
        Category    string         `json:"category"`
        Definition  map[string]any `json:"definition"`
        IsPublic    bool           `json:"is_public"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }
    if body.Name == "" {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
        return
    }
    defJSON, _ := json.Marshal(body.Definition)
    var t Template
    err := h.pool.QueryRow(ctx,
        `INSERT INTO np_workflows_templates (source_account_id, name, description, category, definition, is_public)
         VALUES ($1, $2, $3, $4, $5, $6)
         RETURNING id, source_account_id, name, description, category, is_public, created_at, updated_at`,
        sa, body.Name, body.Description, body.Category, defJSON, body.IsPublic,
    ).Scan(&t.ID, &t.SourceAccountID, &t.Name, &t.Description, &t.Category, &t.IsPublic, &t.CreatedAt, &t.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert template: %v", err)})
        return
    }
    writeJSON(w, http.StatusCreated, map[string]any{"template": t})
}

func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var t Template
    err := h.pool.QueryRow(ctx,
        `SELECT id, source_account_id, name, description, category, is_public, created_at, updated_at
         FROM np_workflows_templates WHERE id=$1 AND (source_account_id=$2 OR is_public=true)`, id, sa,
    ).Scan(&t.ID, &t.SourceAccountID, &t.Name, &t.Description, &t.Category, &t.IsPublic, &t.CreatedAt, &t.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"template": t})
}

func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var body struct {
        Name     *string `json:"name"`
        IsPublic *bool   `json:"is_public"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }
    var t Template
    err := h.pool.QueryRow(ctx,
        `UPDATE np_workflows_templates SET name=COALESCE($3,name), is_public=COALESCE($4,is_public), updated_at=NOW()
         WHERE id=$1 AND source_account_id=$2
         RETURNING id, source_account_id, name, description, category, is_public, created_at, updated_at`,
        id, sa, body.Name, body.IsPublic,
    ).Scan(&t.ID, &t.SourceAccountID, &t.Name, &t.Description, &t.Category, &t.IsPublic, &t.CreatedAt, &t.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"template": t})
}

func (h *Handlers) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    tag, err := h.pool.Exec(ctx, `DELETE FROM np_workflows_templates WHERE id=$1 AND source_account_id=$2`, id, sa)
    if err != nil || tag.RowsAffected() == 0 {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) ListVariables(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    rows, err := h.pool.Query(ctx,
        `SELECT id, source_account_id, workflow_id, name, is_secret, created_at, updated_at
         FROM np_workflows_variables WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query variables: %v", err)})
        return
    }
    defer rows.Close()
    var variables []Variable
    for rows.Next() {
        var v Variable
        var wfID *string
        if err := rows.Scan(&v.ID, &v.SourceAccountID, &wfID, &v.Name, &v.IsSecret, &v.CreatedAt, &v.UpdatedAt); err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
            return
        }
        if wfID != nil {
            v.WorkflowID = *wfID
        }
        variables = append(variables, v)
    }
    if variables == nil {
        variables = []Variable{}
    }
    writeJSON(w, http.StatusOK, map[string]any{"data": variables, "total": len(variables)})
}

func (h *Handlers) CreateVariable(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    var body struct {
        WorkflowID string `json:"workflow_id"`
        Name       string `json:"name"`
        Value      string `json:"value"`
        IsSecret   bool   `json:"is_secret"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }
    if body.Name == "" {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
        return
    }
    var v Variable
    var wfID *string
    if body.WorkflowID != "" {
        wfID = &body.WorkflowID
    }
    err := h.pool.QueryRow(ctx,
        `INSERT INTO np_workflows_variables (source_account_id, workflow_id, name, value, is_secret)
         VALUES ($1, $2, $3, $4, $5)
         RETURNING id, source_account_id, workflow_id, name, is_secret, created_at, updated_at`,
        sa, wfID, body.Name, body.Value, body.IsSecret,
    ).Scan(&v.ID, &v.SourceAccountID, &wfID, &v.Name, &v.IsSecret, &v.CreatedAt, &v.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert variable: %v", err)})
        return
    }
    if wfID != nil {
        v.WorkflowID = *wfID
    }
    writeJSON(w, http.StatusCreated, map[string]any{"variable": v})
}

func (h *Handlers) GetVariable(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var v Variable
    var wfID *string
    err := h.pool.QueryRow(ctx,
        `SELECT id, source_account_id, workflow_id, name, is_secret, created_at, updated_at
         FROM np_workflows_variables WHERE id=$1 AND source_account_id=$2`, id, sa,
    ).Scan(&v.ID, &v.SourceAccountID, &wfID, &v.Name, &v.IsSecret, &v.CreatedAt, &v.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "variable not found"})
        return
    }
    if wfID != nil {
        v.WorkflowID = *wfID
    }
    writeJSON(w, http.StatusOK, map[string]any{"variable": v})
}

func (h *Handlers) UpdateVariable(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    var body struct {
        Value *string `json:"value"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }
    var v Variable
    var wfID *string
    err := h.pool.QueryRow(ctx,
        `UPDATE np_workflows_variables SET value=COALESCE($3,value), updated_at=NOW()
         WHERE id=$1 AND source_account_id=$2
         RETURNING id, source_account_id, workflow_id, name, is_secret, created_at, updated_at`,
        id, sa, body.Value,
    ).Scan(&v.ID, &v.SourceAccountID, &wfID, &v.Name, &v.IsSecret, &v.CreatedAt, &v.UpdatedAt)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "variable not found"})
        return
    }
    if wfID != nil {
        v.WorkflowID = *wfID
    }
    writeJSON(w, http.StatusOK, map[string]any{"variable": v})
}

func (h *Handlers) DeleteVariable(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    sa := h.sourceAccount(r)
    id := chi.URLParam(r, "id")
    tag, err := h.pool.Exec(ctx, `DELETE FROM np_workflows_variables WHERE id=$1 AND source_account_id=$2`, id, sa)
    if err != nil || tag.RowsAffected() == 0 {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "variable not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) HandleWebhook(w http.ResponseWriter, r *http.Request) {
    token := chi.URLParam(r, "token")
    ctx := r.Context()
    h.pool.Exec(ctx,
        `INSERT INTO np_workflows_webhook_logs (token, method, headers, body_size)
         VALUES ($1, $2, $3, $4)`,
        token, r.Method, "{}", r.ContentLength)
    writeJSON(w, http.StatusOK, map[string]any{"received": true})
}
