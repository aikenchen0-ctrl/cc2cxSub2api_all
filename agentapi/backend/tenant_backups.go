package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const tenantBackupVersion = 1

var tenantBackupParts = []string{
	"branding", "model_policy", "announcements", "content_pages", "plan_policies",
}

type TenantBackupRecord struct {
	ID          int64    `json:"id"`
	Status      string   `json:"status"`
	FileName    string   `json:"file_name"`
	SizeBytes   int64    `json:"size_bytes"`
	Parts       []string `json:"parts"`
	TriggeredBy string   `json:"triggered_by"`
	CreatedBy   string   `json:"created_by,omitempty"`
	StartedAt   string   `json:"started_at"`
	RestoredAt  string   `json:"restored_at,omitempty"`
}

type tenantBackupBranding struct {
	AgentHomeSettings
	Name        string `json:"name"`
	SiteName    string `json:"site_name"`
	SiteLogo    string `json:"site_logo,omitempty"`
	DocURL      string `json:"doc_url,omitempty"`
	ContactInfo string `json:"contact_info,omitempty"`
}

type tenantBackupModelPolicy struct {
	Customized bool     `json:"customized"`
	Enabled    []string `json:"enabled"`
}

type tenantBackupAnnouncement struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	NotifyMode string `json:"notify_mode"`
	StartsAt   string `json:"starts_at,omitempty"`
	EndsAt     string `json:"ends_at,omitempty"`
}

type tenantBackupContentPage struct {
	Slug      string `json:"slug"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	SortOrder int    `json:"sort_order"`
}

type TenantBackupSnapshot struct {
	Version       int                        `json:"version"`
	SourceAgentID string                     `json:"source_agent_id"`
	CreatedAt     string                     `json:"created_at"`
	Branding      tenantBackupBranding       `json:"branding"`
	ModelPolicy   tenantBackupModelPolicy    `json:"model_policy"`
	Announcements []tenantBackupAnnouncement `json:"announcements"`
	ContentPages  []tenantBackupContentPage  `json:"content_pages"`
	PlanPolicies  []AgentPlanPolicy          `json:"plan_policies"`
}

func (s *Store) buildTenantBackupSnapshot(agentID string) (TenantBackupSnapshot, error) {
	agentID = strings.TrimSpace(agentID)
	agent, err := s.Agent(agentID)
	if err != nil {
		return TenantBackupSnapshot{}, err
	}
	modelPolicy, err := s.AgentModelPolicy(agentID)
	if err != nil {
		return TenantBackupSnapshot{}, err
	}
	announcements, err := s.AdminAnnouncements(agentID)
	if err != nil {
		return TenantBackupSnapshot{}, err
	}
	pages, err := s.ContentPages(agentID, false)
	if err != nil {
		return TenantBackupSnapshot{}, err
	}
	plans, err := s.AgentPlanPolicies(agentID)
	if err != nil {
		return TenantBackupSnapshot{}, err
	}
	snapshot := TenantBackupSnapshot{
		Version:       tenantBackupVersion,
		SourceAgentID: agentID,
		CreatedAt:     s.clock().UTC().Format(time.RFC3339),
		Branding: tenantBackupBranding{
			AgentHomeSettings: agent.AgentHomeSettings,
			Name:              agent.Name, SiteName: agent.SiteName, SiteLogo: agent.SiteLogo, DocURL: agent.DocURL, ContactInfo: agent.ContactInfo,
		},
		ModelPolicy:   tenantBackupModelPolicy{Customized: modelPolicy.Customized, Enabled: append([]string(nil), modelPolicy.Enabled...)},
		Announcements: make([]tenantBackupAnnouncement, 0, len(announcements)),
		ContentPages:  make([]tenantBackupContentPage, 0, len(pages)),
		PlanPolicies:  append([]AgentPlanPolicy(nil), plans...),
	}
	for _, item := range announcements {
		snapshot.Announcements = append(snapshot.Announcements, tenantBackupAnnouncement{
			Title: item.Title, Content: item.Content, Status: item.Status, NotifyMode: item.NotifyMode,
			StartsAt: item.StartsAt, EndsAt: item.EndsAt,
		})
	}
	for _, item := range pages {
		snapshot.ContentPages = append(snapshot.ContentPages, tenantBackupContentPage{
			Slug: item.Slug, Kind: item.Kind, Title: item.Title, Content: item.Content,
			Status: item.Status, SortOrder: item.SortOrder,
		})
	}
	return snapshot, nil
}

func backupRecordFromScanner(scanner interface{ Scan(...any) error }) (TenantBackupRecord, error) {
	var item TenantBackupRecord
	var createdAt, restoredAt int64
	if err := scanner.Scan(&item.ID, &item.FileName, &item.SizeBytes, &item.CreatedBy, &createdAt, &restoredAt); err != nil {
		return TenantBackupRecord{}, err
	}
	item.Status = "completed"
	item.Parts = append([]string(nil), tenantBackupParts...)
	item.TriggeredBy = "manual"
	item.StartedAt = time.Unix(createdAt, 0).UTC().Format(time.RFC3339)
	if restoredAt > 0 {
		item.RestoredAt = time.Unix(restoredAt, 0).UTC().Format(time.RFC3339)
	}
	return item, nil
}

func (s *Store) CreateTenantBackup(agentID, actorID string) (TenantBackupRecord, error) {
	snapshot, err := s.buildTenantBackupSnapshot(agentID)
	if err != nil {
		return TenantBackupRecord{}, err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return TenantBackupRecord{}, err
	}
	return s.ImportTenantBackup(agentID, actorID, snapshot, data)
}

func (s *Store) ImportTenantBackup(agentID, actorID string, snapshot TenantBackupSnapshot, original []byte) (TenantBackupRecord, error) {
	if err := validateTenantBackupSnapshot(agentID, &snapshot); err != nil {
		return TenantBackupRecord{}, err
	}
	data := original
	if len(data) == 0 {
		var err error
		data, err = json.Marshal(snapshot)
		if err != nil {
			return TenantBackupRecord{}, err
		}
	}
	if len(data) > maxJSONBody {
		return TenantBackupRecord{}, fmt.Errorf("backup exceeds the maximum allowed size")
	}
	now := s.clock().UTC()
	fileName := fmt.Sprintf("agentapi-%s-%s.json", safeBackupFilePart(agentID), now.Format("20060102-150405"))
	result, err := s.db.Exec(`INSERT INTO agent_config_backups(agent_id,file_name,snapshot_json,size_bytes,created_by,created_at,restored_at) VALUES(?,?,?,?,?,?,0)`,
		strings.TrimSpace(agentID), fileName, string(data), len(data), strings.TrimSpace(actorID), now.Unix())
	if err != nil {
		return TenantBackupRecord{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return TenantBackupRecord{}, err
	}
	return s.TenantBackupRecord(agentID, id)
}

func safeBackupFilePart(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	if builder.Len() == 0 {
		return "tenant"
	}
	return builder.String()
}

func (s *Store) TenantBackupRecord(agentID string, id int64) (TenantBackupRecord, error) {
	item, err := backupRecordFromScanner(s.db.QueryRow(`SELECT id,file_name,size_bytes,created_by,created_at,restored_at FROM agent_config_backups WHERE agent_id=? AND id=?`, strings.TrimSpace(agentID), id))
	if errors.Is(err, sql.ErrNoRows) {
		return TenantBackupRecord{}, errNotFound
	}
	return item, err
}

func (s *Store) TenantBackups(agentID string) ([]TenantBackupRecord, error) {
	rows, err := s.db.Query(`SELECT id,file_name,size_bytes,created_by,created_at,restored_at FROM agent_config_backups WHERE agent_id=? ORDER BY id DESC`, strings.TrimSpace(agentID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TenantBackupRecord, 0)
	for rows.Next() {
		item, err := backupRecordFromScanner(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) TenantBackupSnapshot(agentID string, id int64) (TenantBackupRecord, TenantBackupSnapshot, error) {
	var raw string
	item, err := backupRecordFromScanner(s.db.QueryRow(`SELECT id,file_name,size_bytes,created_by,created_at,restored_at FROM agent_config_backups WHERE agent_id=? AND id=?`, strings.TrimSpace(agentID), id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = errNotFound
		}
		return TenantBackupRecord{}, TenantBackupSnapshot{}, err
	}
	if err := s.db.QueryRow(`SELECT snapshot_json FROM agent_config_backups WHERE agent_id=? AND id=?`, strings.TrimSpace(agentID), id).Scan(&raw); err != nil {
		return TenantBackupRecord{}, TenantBackupSnapshot{}, err
	}
	var snapshot TenantBackupSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return TenantBackupRecord{}, TenantBackupSnapshot{}, fmt.Errorf("decode tenant backup: %w", err)
	}
	return item, snapshot, nil
}

func (s *Store) DeleteTenantBackup(agentID string, id int64) error {
	result, err := s.db.Exec(`DELETE FROM agent_config_backups WHERE agent_id=? AND id=?`, strings.TrimSpace(agentID), id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return errNotFound
	}
	return nil
}

func validateTenantBackupSnapshot(agentID string, snapshot *TenantBackupSnapshot) error {
	if snapshot == nil || snapshot.Version != tenantBackupVersion {
		return fmt.Errorf("unsupported backup version")
	}
	if strings.TrimSpace(snapshot.SourceAgentID) != strings.TrimSpace(agentID) {
		return fmt.Errorf("backup belongs to a different agent")
	}
	if err := validateBrandingText(snapshot.Branding.Name, "name", 100); err != nil {
		return err
	}
	if err := validateBrandingText(snapshot.Branding.SiteName, "site_name", 160); err != nil {
		return err
	}
	if err := validateBrandLogo(snapshot.Branding.SiteLogo); err != nil {
		return err
	}
	if err := validateBrandDocURL(snapshot.Branding.DocURL); err != nil {
		return err
	}
	if err := validateBrandContactInfo(snapshot.Branding.ContactInfo); err != nil {
		return err
	}
	if err := validateAgentHomeSettings(snapshot.Branding.AgentHomeSettings); err != nil {
		return err
	}
	seenModels := map[string]struct{}{}
	for _, raw := range snapshot.ModelPolicy.Enabled {
		model := strings.TrimSpace(raw)
		if _, ok := publicModelNames[model]; !ok {
			return fmt.Errorf("model %q is not in the public catalog", model)
		}
		if _, duplicate := seenModels[model]; duplicate {
			return fmt.Errorf("model %q is duplicated", model)
		}
		seenModels[model] = struct{}{}
	}
	if len(snapshot.Announcements) > 500 || len(snapshot.ContentPages) > 500 || len(snapshot.PlanPolicies) > 500 {
		return fmt.Errorf("backup contains too many tenant records")
	}
	for index, item := range snapshot.Announcements {
		normalized, _, _, err := validateAnnouncementInput(announcementInput{Title: item.Title, Content: item.Content, Status: item.Status, NotifyMode: item.NotifyMode, StartsAt: item.StartsAt, EndsAt: item.EndsAt})
		if err != nil {
			return fmt.Errorf("announcement %d: %w", index+1, err)
		}
		snapshot.Announcements[index] = tenantBackupAnnouncement{Title: normalized.Title, Content: normalized.Content, Status: normalized.Status, NotifyMode: normalized.NotifyMode, StartsAt: strings.TrimSpace(item.StartsAt), EndsAt: strings.TrimSpace(item.EndsAt)}
	}
	pageKeys := map[string]struct{}{}
	for index, item := range snapshot.ContentPages {
		normalized, err := validateContentPageInput(contentPageInput{Slug: item.Slug, Kind: item.Kind, Title: item.Title, Content: item.Content, Status: item.Status, SortOrder: item.SortOrder})
		if err != nil {
			return fmt.Errorf("content page %d: %w", index+1, err)
		}
		key := normalized.Kind + "/" + normalized.Slug
		if _, exists := pageKeys[key]; exists {
			return fmt.Errorf("content page %q is duplicated", key)
		}
		pageKeys[key] = struct{}{}
		snapshot.ContentPages[index] = tenantBackupContentPage(normalized)
	}
	planIDs := map[int64]struct{}{}
	for index, item := range snapshot.PlanPolicies {
		if item.PlanID <= 0 {
			return fmt.Errorf("plan policy %d has an invalid plan id", index+1)
		}
		enabled := item.Enabled
		normalized, err := normalizeAgentPlanPolicyInput(agentPlanPolicyInput{Enabled: &enabled, SortOrder: item.SortOrder, DisplayName: item.DisplayName, Description: item.Description, Features: item.Features})
		if err != nil {
			return fmt.Errorf("plan policy %d: %w", index+1, err)
		}
		if _, exists := planIDs[item.PlanID]; exists {
			return fmt.Errorf("plan policy %d is duplicated", item.PlanID)
		}
		planIDs[item.PlanID] = struct{}{}
		normalized.PlanID = item.PlanID
		snapshot.PlanPolicies[index] = normalized
	}
	return nil
}

func (s *Store) RestoreTenantBackup(agentID string, id int64) (TenantBackupRecord, error) {
	item, snapshot, err := s.TenantBackupSnapshot(agentID, id)
	if err != nil {
		return TenantBackupRecord{}, err
	}
	if err := validateTenantBackupSnapshot(agentID, &snapshot); err != nil {
		return TenantBackupRecord{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TenantBackupRecord{}, err
	}
	defer tx.Rollback()
	now := s.clock().UTC().Unix()
	result, err := tx.Exec(`UPDATE agent_config SET name=?,site_name=?,site_logo=?,doc_url=?,contact_info=?,site_subtitle=?,compact_home_enabled=?,home_content=?,updated_at=? WHERE agent_id=?`, snapshot.Branding.Name, snapshot.Branding.SiteName, snapshot.Branding.SiteLogo, snapshot.Branding.DocURL, snapshot.Branding.ContactInfo, snapshot.Branding.SiteSubtitle, snapshot.Branding.CompactHomeEnabled, snapshot.Branding.HomeContent, now, agentID)
	if err != nil {
		return TenantBackupRecord{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return TenantBackupRecord{}, errNotFound
	}
	if _, err = tx.Exec(`DELETE FROM agent_model_policies WHERE agent_id=?`, agentID); err != nil {
		return TenantBackupRecord{}, err
	}
	if snapshot.ModelPolicy.Customized {
		models, _ := json.Marshal(snapshot.ModelPolicy.Enabled)
		if _, err = tx.Exec(`INSERT INTO agent_model_policies(agent_id,allowed_models_json,updated_at) VALUES(?,?,?)`, agentID, string(models), now); err != nil {
			return TenantBackupRecord{}, err
		}
	}
	if _, err = tx.Exec(`DELETE FROM agent_announcement_reads WHERE agent_id=?`, agentID); err != nil {
		return TenantBackupRecord{}, err
	}
	if _, err = tx.Exec(`DELETE FROM agent_announcements WHERE agent_id=?`, agentID); err != nil {
		return TenantBackupRecord{}, err
	}
	for _, announcement := range snapshot.Announcements {
		_, startsAt, endsAt, _ := validateAnnouncementInput(announcementInput{Title: announcement.Title, Content: announcement.Content, Status: announcement.Status, NotifyMode: announcement.NotifyMode, StartsAt: announcement.StartsAt, EndsAt: announcement.EndsAt})
		if _, err = tx.Exec(`INSERT INTO agent_announcements(agent_id,title,content,status,notify_mode,starts_at,ends_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, agentID, announcement.Title, announcement.Content, announcement.Status, announcement.NotifyMode, unixOrZero(startsAt), unixOrZero(endsAt), now, now); err != nil {
			return TenantBackupRecord{}, err
		}
	}
	if _, err = tx.Exec(`DELETE FROM agent_content_pages WHERE agent_id=?`, agentID); err != nil {
		return TenantBackupRecord{}, err
	}
	for _, page := range snapshot.ContentPages {
		if _, err = tx.Exec(`INSERT INTO agent_content_pages(agent_id,slug,kind,title,content,status,sort_order,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, agentID, page.Slug, page.Kind, page.Title, page.Content, page.Status, page.SortOrder, now, now); err != nil {
			return TenantBackupRecord{}, err
		}
	}
	if _, err = tx.Exec(`DELETE FROM agent_plan_policies WHERE agent_id=?`, agentID); err != nil {
		return TenantBackupRecord{}, err
	}
	for _, policy := range snapshot.PlanPolicies {
		features, _ := json.Marshal(policy.Features)
		if _, err = tx.Exec(`INSERT INTO agent_plan_policies(agent_id,plan_id,enabled,sort_order,display_name,description,features_json,updated_at) VALUES(?,?,?,?,?,?,?,?)`, agentID, policy.PlanID, boolInt(policy.Enabled), policy.SortOrder, policy.DisplayName, policy.Description, string(features), now); err != nil {
			return TenantBackupRecord{}, err
		}
	}
	if _, err = tx.Exec(`UPDATE agent_config_backups SET restored_at=? WHERE agent_id=? AND id=?`, now, agentID, id); err != nil {
		return TenantBackupRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return TenantBackupRecord{}, err
	}
	item.RestoredAt = time.Unix(now, 0).UTC().Format(time.RFC3339)
	return item, nil
}

func backupPathID(path, suffix string) (int64, bool) {
	const prefix = "/api/v1/agent/admin/backups/"
	if !strings.HasPrefix(path, prefix) || suffix != "" && !strings.HasSuffix(path, suffix) {
		return 0, false
	}
	raw := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		raw = strings.TrimSuffix(raw, suffix)
	}
	raw = strings.Trim(raw, "/")
	if raw == "" || strings.Contains(raw, "/") {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0
}

func (s *Server) handleAgentAdminBackups(w http.ResponseWriter, r *http.Request, requestID string, actor Session) {
	w.Header().Set("Cache-Control", "no-store")
	const base = "/api/v1/agent/admin/backups"
	if r.URL.Path == base {
		switch r.Method {
		case http.MethodGet:
			items, err := s.store.TenantBackups(s.cfg.AgentID)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "BACKUP_LIST_FAILED", "failed to load tenant backups")
				return
			}
			s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items), "parts": tenantBackupParts})
		case http.MethodPost:
			if !sameOrigin(r) {
				s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
				return
			}
			item, err := s.store.CreateTenantBackup(s.cfg.AgentID, actor.MainUserID)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "BACKUP_CREATE_FAILED", "failed to create tenant backup")
				return
			}
			s.recordAudit("agent_admin", actor.MainUserID, "backup.create", "tenant_backup", strconv.FormatInt(item.ID, 10), requestID, "success", "")
			s.writeData(w, http.StatusCreated, requestID, item)
		default:
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported backup operation")
		}
		return
	}
	if r.URL.Path == base+"/import" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported backup import operation")
			return
		}
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		var snapshot TenantBackupSnapshot
		if err := decodeJSON(r, &snapshot, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BACKUP", err.Error())
			return
		}
		item, err := s.store.ImportTenantBackup(s.cfg.AgentID, actor.MainUserID, snapshot, nil)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BACKUP", err.Error())
			return
		}
		s.recordAudit("agent_admin", actor.MainUserID, "backup.import", "tenant_backup", strconv.FormatInt(item.ID, 10), requestID, "success", "")
		s.writeData(w, http.StatusCreated, requestID, item)
		return
	}
	if id, ok := backupPathID(r.URL.Path, "/download"); ok {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported backup download operation")
			return
		}
		item, snapshot, err := s.store.TenantBackupSnapshot(s.cfg.AgentID, id)
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "BACKUP_NOT_FOUND", "backup not found")
			return
		}
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "BACKUP_READ_FAILED", "failed to read tenant backup")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"record": item, "snapshot": snapshot})
		return
	}
	if id, ok := backupPathID(r.URL.Path, "/restore"); ok {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported backup restore operation")
			return
		}
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		// A restored model policy must follow the same managed-runtime bridge as
		// an ordinary policy edit. Serialize the operation, validate the target
		// before changing Sub2API, and compensate the main-site scope if the
		// local transaction cannot commit.
		s.modelPolicyMu.Lock()
		defer s.modelPolicyMu.Unlock()
		_, target, err := s.store.TenantBackupSnapshot(s.cfg.AgentID, id)
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "BACKUP_NOT_FOUND", "backup not found")
			return
		}
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "BACKUP_READ_FAILED", "failed to read tenant backup")
			return
		}
		if err := validateTenantBackupSnapshot(s.cfg.AgentID, &target); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "BACKUP_RESTORE_FAILED", err.Error())
			return
		}
		prior, err := s.store.AgentModelPolicy(s.cfg.AgentID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "BACKUP_RESTORE_FAILED", "failed to load current model policy")
			return
		}
		targetModels := append([]string(nil), target.ModelPolicy.Enabled...)
		if !target.ModelPolicy.Customized {
			targetModels = append([]string(nil), publicModelCatalog...)
		}
		runtimeUpdated := false
		if s.managedRuntimeBridge() {
			if s.main == nil {
				s.writeError(w, http.StatusServiceUnavailable, requestID, "MODEL_SCOPE_SYNC_UNAVAILABLE", "main-site model scope control is unavailable")
				return
			}
			if err := s.main.RuntimeUpdateModelAllowlist(r.Context(), targetModels); err != nil {
				s.writeError(w, http.StatusBadGateway, requestID, "MODEL_SCOPE_SYNC_FAILED", "main site did not confirm the restored model scope")
				return
			}
			runtimeUpdated = true
		}
		item, err := s.store.RestoreTenantBackup(s.cfg.AgentID, id)
		if err != nil {
			if runtimeUpdated {
				if rollbackErr := s.main.RuntimeUpdateModelAllowlist(r.Context(), prior.Enabled); rollbackErr != nil {
					slog.Error("failed to restore main-site model scope after tenant backup rollback", "agent_id", s.cfg.AgentID, "error", rollbackErr)
				}
			}
			s.writeError(w, http.StatusBadRequest, requestID, "BACKUP_RESTORE_FAILED", err.Error())
			return
		}
		s.recordAudit("agent_admin", actor.MainUserID, "backup.restore", "tenant_backup", strconv.FormatInt(item.ID, 10), requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, item)
		return
	}
	if id, ok := backupPathID(r.URL.Path, ""); ok {
		if r.Method != http.MethodDelete {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported backup operation")
			return
		}
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		if err := s.store.DeleteTenantBackup(s.cfg.AgentID, id); errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "BACKUP_NOT_FOUND", "backup not found")
			return
		} else if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "BACKUP_DELETE_FAILED", "failed to delete tenant backup")
			return
		}
		s.recordAudit("agent_admin", actor.MainUserID, "backup.delete", "tenant_backup", strconv.FormatInt(id, 10), requestID, "success", "")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.writeError(w, http.StatusNotFound, requestID, "BACKUP_NOT_FOUND", "backup not found")
}
