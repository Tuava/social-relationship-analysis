package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/secrets"
)

type Repository struct {
	DB            *pgxpool.Pool
	EncryptionKey string
}

func (r Repository) encryptCredential(value string) (string, error) {
	if value == "" || secrets.IsEncrypted(value) {
		return value, nil
	}
	return secrets.Encrypt(value, r.EncryptionKey)
}

func (r Repository) decryptCredential(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return secrets.Decrypt(value, r.EncryptionKey)
}

// DecryptCredential is used by the MCP live-read bridge, which cannot expose
// credentials but still needs to call a configured local adapter.
func (r Repository) DecryptCredential(value string) (string, error) {
	return r.decryptCredential(value)
}

func (r Repository) EnsureAdmin(ctx context.Context, username, hash string) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO app_users(username,password_hash) VALUES($1,$2)
		ON CONFLICT(username) DO NOTHING`, username, hash)
	return err
}

// ProtectSystemConfigSecrets upgrades legacy plaintext credentials in place.
// It is idempotent and keeps the encryption key outside PostgreSQL.
func (r Repository) ProtectSystemConfigSecrets(ctx context.Context, masterKey string) error {
	rows, err := r.DB.Query(ctx, `SELECT key, value #>> '{}' FROM system_configs WHERE key IN ('ai.llm_api_key','vision.api_key') AND COALESCE(value #>> '{}','') <> ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type item struct{ key, value string }
	items := []item{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.key, &it.value); err != nil {
			return err
		}
		if !secrets.IsEncrypted(it.value) {
			items = append(items, it)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, it := range items {
		protected, err := secrets.Encrypt(it.value, masterKey)
		if err != nil {
			return fmt.Errorf("protect %s: %w", it.key, err)
		}
		if _, err := r.DB.Exec(ctx, `UPDATE system_configs SET value=to_jsonb($2::text),updated_at=now() WHERE key=$1`, it.key, protected); err != nil {
			return fmt.Errorf("save protected %s: %w", it.key, err)
		}
	}
	botRows, err := r.DB.Query(ctx, `SELECT id::text, llm_api_key FROM bot_instances WHERE COALESCE(llm_api_key,'') <> ''`)
	if err != nil {
		return err
	}
	type botSecret struct{ id, value string }
	bots := []botSecret{}
	for botRows.Next() {
		var it botSecret
		if err := botRows.Scan(&it.id, &it.value); err != nil {
			botRows.Close()
			return err
		}
		if !secrets.IsEncrypted(it.value) {
			bots = append(bots, it)
		}
	}
	if err := botRows.Err(); err != nil {
		botRows.Close()
		return err
	}
	botRows.Close()
	for _, it := range bots {
		protected, err := secrets.Encrypt(it.value, masterKey)
		if err != nil {
			return fmt.Errorf("protect bot %s: %w", it.id, err)
		}
		if _, err := r.DB.Exec(ctx, `UPDATE bot_instances SET llm_api_key=$2 WHERE id=$1::uuid`, it.id, protected); err != nil {
			return fmt.Errorf("save protected bot %s: %w", it.id, err)
		}
	}
	accountRows, err := r.DB.Query(ctx, `SELECT id::text,ws_token,http_token FROM napcat_accounts WHERE COALESCE(ws_token,'') <> '' OR COALESCE(http_token,'') <> ''`)
	if err != nil {
		return err
	}
	type accountSecret struct{ id, wsToken, httpToken string }
	accounts := []accountSecret{}
	for accountRows.Next() {
		var item accountSecret
		if err := accountRows.Scan(&item.id, &item.wsToken, &item.httpToken); err != nil {
			accountRows.Close()
			return err
		}
		if !secrets.IsEncrypted(item.wsToken) || !secrets.IsEncrypted(item.httpToken) {
			accounts = append(accounts, item)
		}
	}
	if err := accountRows.Err(); err != nil {
		accountRows.Close()
		return err
	}
	accountRows.Close()
	for _, item := range accounts {
		wsToken, err := r.encryptCredential(item.wsToken)
		if err != nil {
			return fmt.Errorf("protect NapCat websocket credential %s: %w", item.id, err)
		}
		httpToken, err := r.encryptCredential(item.httpToken)
		if err != nil {
			return fmt.Errorf("protect NapCat HTTP credential %s: %w", item.id, err)
		}
		if _, err := r.DB.Exec(ctx, `UPDATE napcat_accounts SET ws_token=$2,http_token=$3 WHERE id=$1::uuid`, item.id, wsToken, httpToken); err != nil {
			return fmt.Errorf("save protected NapCat credential %s: %w", item.id, err)
		}
	}
	qzoneRows, err := r.DB.Query(ctx, `SELECT id::text,access_token FROM qzone_connections WHERE COALESCE(access_token,'') <> ''`)
	if err != nil {
		return err
	}
	type qzoneSecret struct{ id, token string }
	qzones := []qzoneSecret{}
	for qzoneRows.Next() {
		var item qzoneSecret
		if err := qzoneRows.Scan(&item.id, &item.token); err != nil {
			qzoneRows.Close()
			return err
		}
		if !secrets.IsEncrypted(item.token) {
			qzones = append(qzones, item)
		}
	}
	if err := qzoneRows.Err(); err != nil {
		qzoneRows.Close()
		return err
	}
	qzoneRows.Close()
	for _, item := range qzones {
		token, err := r.encryptCredential(item.token)
		if err != nil {
			return fmt.Errorf("protect QZone credential %s: %w", item.id, err)
		}
		if _, err := r.DB.Exec(ctx, `UPDATE qzone_connections SET access_token=$2,updated_at=now() WHERE id=$1::uuid`, item.id, token); err != nil {
			return fmt.Errorf("save protected QZone credential %s: %w", item.id, err)
		}
	}
	return nil
}

func (r Repository) FindUser(ctx context.Context, username string) (domain.AppUser, error) {
	var u domain.AppUser
	err := r.DB.QueryRow(ctx, `SELECT id,username,password_hash,created_at FROM app_users WHERE username=$1`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	return u, err
}

func (r Repository) ListAccounts(ctx context.Context) ([]domain.NapCatAccount, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,name,qq_uin,ws_url,http_url,enabled,status,last_connected_at,created_at FROM napcat_accounts ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := []domain.NapCatAccount{}
	for rows.Next() {
		var a domain.NapCatAccount
		if err := rows.Scan(&a.ID, &a.Name, &a.QQUIN, &a.WSURL, &a.HTTPURL, &a.Enabled, &a.Status, &a.LastConnectedAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (r Repository) CreateAccount(ctx context.Context, a domain.NapCatAccount) (domain.NapCatAccount, error) {
	wsToken, err := r.encryptCredential(a.WSToken)
	if err != nil {
		return a, err
	}
	httpToken, err := r.encryptCredential(a.HTTPToken)
	if err != nil {
		return a, err
	}
	err = r.DB.QueryRow(ctx, `INSERT INTO napcat_accounts(name,qq_uin,ws_url,ws_token,http_url,http_token,enabled) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,created_at,status`, a.Name, a.QQUIN, a.WSURL, wsToken, a.HTTPURL, httpToken, a.Enabled).Scan(&a.ID, &a.CreatedAt, &a.Status)
	return a, err
}

func (r Repository) GetAccount(ctx context.Context, id string) (domain.NapCatAccount, error) {
	var a domain.NapCatAccount
	var wsToken, httpToken string
	err := r.DB.QueryRow(ctx, `SELECT id,name,qq_uin,ws_url,ws_token,http_url,http_token,enabled,status,last_connected_at,created_at FROM napcat_accounts WHERE id=$1`, id).Scan(&a.ID, &a.Name, &a.QQUIN, &a.WSURL, &wsToken, &a.HTTPURL, &httpToken, &a.Enabled, &a.Status, &a.LastConnectedAt, &a.CreatedAt)
	if err != nil {
		return a, err
	}
	a.WSToken, err = r.decryptCredential(wsToken)
	if err != nil {
		return a, fmt.Errorf("decrypt websocket credential: %w", err)
	}
	a.HTTPToken, err = r.decryptCredential(httpToken)
	if err != nil {
		return a, fmt.Errorf("decrypt HTTP credential: %w", err)
	}
	return a, err
}

func (r Repository) UpdateAccount(ctx context.Context, id string, a domain.NapCatAccount) error {
	wsToken, err := r.encryptCredential(a.WSToken)
	if err != nil {
		return err
	}
	httpToken, err := r.encryptCredential(a.HTTPToken)
	if err != nil {
		return err
	}
	_, err = r.DB.Exec(ctx, `UPDATE napcat_accounts SET name=$2,qq_uin=$3,ws_url=$4,ws_token=COALESCE(NULLIF($5,''),ws_token),http_url=$6,http_token=COALESCE(NULLIF($7,''),http_token),enabled=$8 WHERE id=$1`, id, a.Name, a.QQUIN, a.WSURL, wsToken, a.HTTPURL, httpToken, a.Enabled)
	return err
}

func (r Repository) DeleteAccount(ctx context.Context, id string) error {
	_, err := r.DB.Exec(ctx, `DELETE FROM napcat_accounts WHERE id=$1`, id)
	return err
}

func (r Repository) GetQZoneConnection(ctx context.Context, accountID string) (domain.QZoneConnection, error) {
	var v domain.QZoneConnection
	var token string
	err := r.DB.QueryRow(ctx, `SELECT id,account_id,http_url,access_token,ws_url,enabled,status,last_connected_at,last_event_at,last_error,created_at,updated_at FROM qzone_connections WHERE account_id=$1`, accountID).
		Scan(&v.ID, &v.AccountID, &v.HTTPURL, &token, &v.WSURL, &v.Enabled, &v.Status, &v.LastConnectedAt, &v.LastEventAt, &v.LastError, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, err
	}
	v.AccessToken, err = r.decryptCredential(token)
	return v, err
}

func (r Repository) ListQZoneConnections(ctx context.Context) ([]domain.QZoneConnection, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,account_id,http_url,access_token,ws_url,enabled,status,last_connected_at,last_event_at,last_error,created_at,updated_at FROM qzone_connections ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.QZoneConnection{}
	for rows.Next() {
		var v domain.QZoneConnection
		if err := rows.Scan(&v.ID, &v.AccountID, &v.HTTPURL, &v.AccessToken, &v.WSURL, &v.Enabled, &v.Status, &v.LastConnectedAt, &v.LastEventAt, &v.LastError, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r Repository) UpsertQZoneConnection(ctx context.Context, v domain.QZoneConnection) (domain.QZoneConnection, error) {
	token, err := r.encryptCredential(v.AccessToken)
	if err != nil {
		return v, err
	}
	err = r.DB.QueryRow(ctx, `INSERT INTO qzone_connections(account_id,http_url,access_token,ws_url,enabled)
		VALUES($1,$2,$3,$4,$5)
		ON CONFLICT(account_id) DO UPDATE SET http_url=EXCLUDED.http_url,
		access_token=COALESCE(NULLIF(EXCLUDED.access_token,''),qzone_connections.access_token),
		ws_url=EXCLUDED.ws_url,enabled=EXCLUDED.enabled,updated_at=now()
		RETURNING id,account_id,http_url,access_token,ws_url,enabled,status,last_connected_at,last_event_at,last_error,created_at,updated_at`,
		v.AccountID, v.HTTPURL, token, v.WSURL, v.Enabled).
		Scan(&v.ID, &v.AccountID, &v.HTTPURL, &token, &v.WSURL, &v.Enabled, &v.Status, &v.LastConnectedAt, &v.LastEventAt, &v.LastError, &v.CreatedAt, &v.UpdatedAt)
	if err == nil {
		v.AccessToken, err = r.decryptCredential(token)
	}
	return v, err
}

func (r Repository) SetQZoneStatus(ctx context.Context, accountID, status string, eventAt *time.Time, connectionError *string) error {
	_, err := r.DB.Exec(ctx, `UPDATE qzone_connections SET status=$2,
		last_connected_at=CASE WHEN $2='connected' THEN now() ELSE last_connected_at END,
		last_event_at=COALESCE($3,last_event_at),last_error=$4,updated_at=now() WHERE account_id=$1`,
		accountID, status, eventAt, connectionError)
	return err
}

func (r Repository) CreateOperation(ctx context.Context, accountID, endpoint string, parameters any, previewHash, username string) (domain.OperationRequest, error) {
	var op domain.OperationRequest
	var params []byte
	if parameters != nil {
		var err error
		params, err = json.Marshal(parameters)
		if err != nil {
			return op, err
		}
	} else {
		params = []byte(`{}`)
	}
	var userID *string
	var id string
	if username != "" && r.DB.QueryRow(ctx, `SELECT id FROM app_users WHERE username=$1`, username).Scan(&id) == nil {
		userID = &id
	}
	err := r.DB.QueryRow(ctx, `INSERT INTO operation_requests(account_id,endpoint,parameters,preview_hash,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id,account_id,endpoint,parameters,status,preview_hash,created_at,confirmed_at`, accountID, endpoint, params, previewHash, userID).Scan(&op.ID, &op.AccountID, &op.Endpoint, &params, &op.Status, &op.PreviewHash, &op.CreatedAt, &op.ConfirmedAt)
	if err == nil {
		_ = json.Unmarshal(params, &op.Parameters)
	}
	return op, err
}

func (r Repository) GetOperation(ctx context.Context, id string) (domain.OperationRequest, error) {
	var op domain.OperationRequest
	var params []byte
	err := r.DB.QueryRow(ctx, `SELECT id,account_id,endpoint,parameters,status,preview_hash,created_at,confirmed_at FROM operation_requests WHERE id=$1`, id).Scan(&op.ID, &op.AccountID, &op.Endpoint, &params, &op.Status, &op.PreviewHash, &op.CreatedAt, &op.ConfirmedAt)
	if err == nil {
		_ = json.Unmarshal(params, &op.Parameters)
	}
	return op, err
}

func (r Repository) SetOperationStatus(ctx context.Context, id, status string) error {
	_, err := r.DB.Exec(ctx, `UPDATE operation_requests SET status=$2,confirmed_at=CASE WHEN $2 IN ('executed','failed') THEN now() ELSE confirmed_at END WHERE id=$1`, id, status)
	return err
}

func (r Repository) SaveOperationAudit(ctx context.Context, operationID string, request, response any, status string) error {
	req, err := json.Marshal(request)
	if err != nil {
		return err
	}
	res, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = r.DB.Exec(ctx, `INSERT INTO operation_audits(operation_id,request,response,status) VALUES($1,$2,$3,$4)`, operationID, req, res, status)
	return err
}

func (r Repository) SetAccountStatus(ctx context.Context, id, status string) error {
	_, err := r.DB.Exec(ctx, `UPDATE napcat_accounts SET status=$2,last_connected_at=CASE WHEN $2='connected' THEN now() ELSE last_connected_at END WHERE id=$1`, id, status)
	return err
}

func (r Repository) TouchConnection(ctx context.Context, accountID, kind string, status string, eventAt *time.Time, connectionError *string) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO source_connections(account_id,kind,status,last_event_at,error) VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id,kind) DO UPDATE SET status=EXCLUDED.status,last_event_at=COALESCE(EXCLUDED.last_event_at,source_connections.last_event_at),error=EXCLUDED.error`, accountID, kind, status, eventAt, connectionError)
	return err
}

func (r Repository) SaveRaw(ctx context.Context, accountID, source, endpoint string, payload []byte) (string, error) {
	h := sha256.Sum256(payload)
	hash := hex.EncodeToString(h[:])
	var id string
	var value any = json.RawMessage(payload)
	if !json.Valid(payload) {
		value = map[string]any{"raw": string(payload)}
	}
	err := r.DB.QueryRow(ctx, `INSERT INTO raw_records(account_id,source,endpoint_or_event_type,payload,payload_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id,source,endpoint_or_event_type,payload_hash) DO UPDATE SET collected_at=raw_records.collected_at RETURNING id`, accountID, source, endpoint, value, hash).Scan(&id)
	return id, err
}

func (r Repository) SaveRawEvent(ctx context.Context, rawID, eventID, postType, messageType string, occurredAt *time.Time, sequence int64) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO raw_events(raw_record_id,event_id,post_type,message_type,occurred_at,sequence) VALUES($1,$2,$3,$4,$5,$6)`, rawID, nullable(eventID), nullable(postType), nullable(messageType), occurredAt, sequence)
	return err
}

func (r Repository) Count(ctx context.Context, table string) (int64, error) {
	allowed := map[string]bool{
		"napcat_accounts": true,
		"raw_records":     true,
		"messages":        true,
		"relation_events": true,
		"contents":        true,
		"persons":         true,
		"groups":          true,
	}
	if !allowed[table] {
		return 0, fmt.Errorf("unsupported count table: %s", table)
	}
	var n int64
	err := r.DB.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	return n, err
}

func (r Repository) CreateCollectionRun(ctx context.Context, accountID *string, runType string) (domain.CollectionRun, error) {
	var run domain.CollectionRun
	err := r.DB.QueryRow(ctx, `INSERT INTO collection_runs(account_id,type) VALUES($1,$2) RETURNING id,account_id,type,status,progress,error,started_at,ended_at,created_at,config`, accountID, runType).Scan(&run.ID, &run.AccountID, &run.Type, &run.Status, &run.Progress, &run.Error, &run.StartedAt, &run.EndedAt, &run.CreatedAt, &run.Config)
	return run, err
}

func (r Repository) ListCollectionRuns(ctx context.Context) ([]domain.CollectionRun, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,account_id,type,status,progress,error,started_at,ended_at,created_at,config FROM collection_runs ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.CollectionRun{}
	for rows.Next() {
		var v domain.CollectionRun
		if err := rows.Scan(&v.ID, &v.AccountID, &v.Type, &v.Status, &v.Progress, &v.Error, &v.StartedAt, &v.EndedAt, &v.CreatedAt, &v.Config); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r Repository) UpdateCollectionRun(ctx context.Context, id, status string, progress int, runErr *string) error {
	_, err := r.DB.Exec(ctx, `UPDATE collection_runs SET status=$2, progress=$3, error=$4, started_at=CASE WHEN $2='running' AND started_at IS NULL THEN now() ELSE started_at END, ended_at=CASE WHEN $2 IN ('completed','partial','failed','cancelled') THEN now() ELSE ended_at END WHERE id=$1`, id, status, progress, runErr)
	return err
}

func (r Repository) GetCollectionRun(ctx context.Context, id string) (domain.CollectionRun, error) {
	var v domain.CollectionRun
	err := r.DB.QueryRow(ctx, `SELECT id,account_id,type,status,progress,error,started_at,ended_at,created_at,config FROM collection_runs WHERE id=$1`, id).Scan(&v.ID, &v.AccountID, &v.Type, &v.Status, &v.Progress, &v.Error, &v.StartedAt, &v.EndedAt, &v.CreatedAt, &v.Config)
	return v, err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r Repository) ListCapabilities(ctx context.Context, capabilities []domain.Capability) []domain.Capability {
	return capabilities
}

func IsNotFound(err error) bool { return err == pgx.ErrNoRows }

func PreviewHash(endpoint string, params any) (string, error) {
	b, err := json.Marshal(struct {
		Endpoint   string `json:"endpoint"`
		Parameters any    `json:"parameters"`
	}{endpoint, params})
	if err != nil {
		return "", fmt.Errorf("marshal preview: %w", err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
