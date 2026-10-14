package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type connectorInput struct {
	Name       string            `json:"name"`
	Label      string            `json:"label"`
	Credential string            `json:"credential"`
	Object     string            `json:"object"`
	URL        string            `json:"url"`
	ExternalID string            `json:"external_id"`
	Mapping    map[string]string `json:"mapping"`
}

type connectorView struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Label      string            `json:"label"`
	Credential string            `json:"credential"`
	Object     string            `json:"object"`
	URL        string            `json:"url"`
	ExternalID string            `json:"external_id"`
	Mapping    map[string]string `json:"mapping"`
}

type connectorSync struct {
	ID         string
	Name       string
	URL        string
	ExternalID string
	Mapping    map[string]string
	ActorID    string
	ObjectID   string
	ObjectName string
	Secret     string
}

func (s *server) createConnector(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body connectorInput
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	credentialID, objectID, err := s.validateConnector(r.Context(), a.OrgID, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	raw, err := json.Marshal(body.Mapping)
	if err != nil {
		writeErr(w, err)
		return
	}
	view := connectorView{
		ID: uuid.NewString(), Name: body.Name, Label: body.Label, Credential: body.Credential,
		Object: body.Object, URL: body.URL, ExternalID: body.ExternalID, Mapping: body.Mapping,
	}
	_, err = s.pool.Exec(r.Context(), `
		insert into connector_syncs
			(id, organization_id, name, label, credential_id, object_id, url, external_id, mapping, actor_id)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10)
	`, view.ID, a.OrgID, view.Name, view.Label, credentialID, objectID, view.URL, view.ExternalID, string(raw), a.UserID)
	if isUnique(err) {
		writeErr(w, apiErr(http.StatusConflict, "connector already exists"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) listConnectors(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		select s.id::text, s.name, s.label, c.name, o.name, s.url, s.external_id, s.mapping
		from connector_syncs s
		join credentials c on c.id = s.credential_id
		join object_definitions o on o.id = s.object_id
		where s.organization_id = $1::uuid
		order by s.name
	`, a.OrgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	views := []connectorView{}
	for rows.Next() {
		view, err := scanConnector(rows)
		if err != nil {
			writeErr(w, err)
			return
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": views})
}

func (s *server) getConnector(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	view, err := s.loadConnectorView(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) deleteConnector(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `
		delete from connector_syncs
		where organization_id = $1::uuid and name = $2
	`, a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requestSync stores the work. The request does not call the other system.
func (s *server) requestSync(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.loadConnectorView(r.Context(), a.OrgID, r.PathValue("name")); err != nil {
		writeErr(w, err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	eventID, err := insertEvent(r.Context(), tx, a.OrgID, eventWrite{
		Name:    "sync.requested",
		ActorID: a.UserID,
		Source:  sourceAPI,
		Payload: map[string]any{"connector": r.PathValue("name")},
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := enqueueJob(r.Context(), tx, a.OrgID, eventID, "connector", "", ""); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.committed(r.Context(), tx); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"event": eventID})
}

func (s *server) runConnector(ctx context.Context, job dueJob) error {
	event, err := s.loadWebhookEvent(ctx, job.EventID)
	if err != nil {
		return err
	}
	name, _ := event.Payload["connector"].(string)
	sync, err := s.loadConnectorSync(ctx, job.OrgID, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if sync.Secret == "" {
		return apiErr(http.StatusBadRequest, "secret is required")
	}
	actor, err := s.memberActor(ctx, job.OrgID, sync.ActorID)
	if err != nil {
		return err
	}
	if err := s.authorize(ctx, actor, sync.ObjectID, true); err != nil {
		return err
	}
	rows, err := fetchConnector(ctx, sync.URL, sync.Secret)
	if err != nil {
		return err
	}
	defs, err := s.fieldDefs(ctx, sync.ObjectID)
	if err != nil {
		return err
	}
	for _, item := range rows {
		if err := s.applyConnectorRecord(ctx, actor, sync, defs, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) applyConnectorRecord(ctx context.Context, a actor, sync connectorSync, defs []fieldDef, item map[string]any) error {
	externalID, _ := item[sync.ExternalID].(string)
	if externalID == "" {
		return fmt.Errorf("invalid connector response")
	}
	data := map[string]any{}
	for remote, local := range sync.Mapping {
		if value, ok := item[remote]; ok {
			data[local] = value
		}
	}
	var recordID string
	err := s.pool.QueryRow(ctx, `
		select record_id::text from connector_links
		where sync_id = $1::uuid and external_id = $2
	`, sync.ID, externalID).Scan(&recordID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := validateData(defs, data, true); err != nil {
			return err
		}
		if err := s.checkRelations(ctx, a.OrgID, sync.ObjectID, defs, data); err != nil {
			return err
		}
		view, err := s.insertRecord(ctx, a, sync.ObjectID, sync.ObjectName, defs, data, sourceIntegration, "")
		if err != nil {
			return err
		}
		_, err = s.pool.Exec(ctx, `
			insert into connector_links (organization_id, sync_id, external_id, record_id)
			values ($1, $2, $3, $4)
		`, a.OrgID, sync.ID, externalID, view.ID)
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.writeRecordPatch(ctx, a, sync.ObjectID, sync.ObjectName, recordID, data, sourceIntegration, "")
	if api, ok := err.(*apiError); ok && api.status == http.StatusNotFound {
		if err := validateData(defs, data, true); err != nil {
			return err
		}
		if err := s.checkRelations(ctx, a.OrgID, sync.ObjectID, defs, data); err != nil {
			return err
		}
		view, err := s.insertRecord(ctx, a, sync.ObjectID, sync.ObjectName, defs, data, sourceIntegration, "")
		if err != nil {
			return err
		}
		_, err = s.pool.Exec(ctx, `
			update connector_links
			set record_id = $1
			where sync_id = $2::uuid and external_id = $3
		`, view.ID, sync.ID, externalID)
		return err
	}
	return err
}

func fetchConnector(ctx context.Context, rawURL, secret string) ([]map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid connector response")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Openforce-Connector")
	resp, err := webhookClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("connector status %d", resp.StatusCode)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil || rows == nil {
		return nil, fmt.Errorf("invalid connector response")
	}
	return rows, nil
}

func (s *server) validateConnector(ctx context.Context, orgID string, body connectorInput) (string, string, error) {
	if !namePattern.MatchString(body.Name) {
		return "", "", apiErr(http.StatusBadRequest, "invalid name")
	}
	if body.Label == "" {
		return "", "", apiErr(http.StatusBadRequest, "label is required")
	}
	if err := validHTTPURL(body.URL); err != nil {
		return "", "", err
	}
	if !namePattern.MatchString(body.ExternalID) {
		return "", "", apiErr(http.StatusBadRequest, "external id is required")
	}
	if len(body.Mapping) == 0 {
		return "", "", apiErr(http.StatusBadRequest, "mapping is required")
	}
	var credentialID, secret string
	err := s.pool.QueryRow(ctx, `
		select id::text, secret from credentials
		where organization_id = $1::uuid and name = $2
	`, orgID, body.Credential).Scan(&credentialID, &secret)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", apiErr(http.StatusNotFound, "not found")
	}
	if err != nil {
		return "", "", err
	}
	if secret == "" {
		return "", "", apiErr(http.StatusBadRequest, "secret is required")
	}
	objectID, err := s.objectByName(ctx, orgID, body.Object)
	if err != nil {
		return "", "", err
	}
	defs, err := s.fieldDefs(ctx, objectID)
	if err != nil {
		return "", "", err
	}
	byName := indexFields(defs)
	seen := map[string]struct{}{}
	for remote, local := range body.Mapping {
		if !namePattern.MatchString(remote) {
			return "", "", apiErr(http.StatusBadRequest, "invalid name")
		}
		if _, ok := byName[local]; !ok {
			return "", "", apiErrf(http.StatusBadRequest, "unknown field %s", local)
		}
		if _, ok := seen[local]; ok {
			return "", "", apiErr(http.StatusBadRequest, "duplicate field")
		}
		seen[local] = struct{}{}
	}
	return credentialID, objectID, nil
}

func (s *server) loadConnectorView(ctx context.Context, orgID, name string) (connectorView, error) {
	row := s.pool.QueryRow(ctx, `
		select s.id::text, s.name, s.label, c.name, o.name, s.url, s.external_id, s.mapping
		from connector_syncs s
		join credentials c on c.id = s.credential_id
		join object_definitions o on o.id = s.object_id
		where s.organization_id = $1::uuid and s.name = $2
	`, orgID, name)
	view, err := scanConnector(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return connectorView{}, apiErr(http.StatusNotFound, "not found")
	}
	return view, err
}

func (s *server) loadConnectorSync(ctx context.Context, orgID, name string) (connectorSync, error) {
	var sync connectorSync
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		select s.id::text, s.name, s.url, s.external_id, s.mapping, s.actor_id::text,
			s.object_id::text, o.name, c.secret
		from connector_syncs s
		join credentials c on c.id = s.credential_id
		join object_definitions o on o.id = s.object_id
		where s.organization_id = $1::uuid and s.name = $2
	`, orgID, name).Scan(&sync.ID, &sync.Name, &sync.URL, &sync.ExternalID, &raw, &sync.ActorID, &sync.ObjectID, &sync.ObjectName, &sync.Secret)
	if err != nil {
		return connectorSync{}, err
	}
	sync.Mapping = map[string]string{}
	if err := json.Unmarshal(raw, &sync.Mapping); err != nil {
		return connectorSync{}, err
	}
	return sync, nil
}

func scanConnector(row pgx.Row) (connectorView, error) {
	var view connectorView
	var raw []byte
	if err := row.Scan(&view.ID, &view.Name, &view.Label, &view.Credential, &view.Object, &view.URL, &view.ExternalID, &raw); err != nil {
		return view, err
	}
	view.Mapping = map[string]string{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &view.Mapping); err != nil {
			return view, err
		}
	}
	return view, nil
}
