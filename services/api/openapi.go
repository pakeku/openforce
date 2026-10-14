package main

import (
	"context"
	"net/http"
)

// Pointer fields encode a zero value. omitempty would drop a plain bool false or a plain 0.
type openAPISchema struct {
	Ref                  string                    `json:"$ref,omitempty"`
	Type                 string                    `json:"type,omitempty"`
	Format               string                    `json:"format,omitempty"`
	Description          string                    `json:"description,omitempty"`
	Enum                 []string                  `json:"enum,omitempty"`
	Properties           map[string]*openAPISchema `json:"properties,omitempty"`
	Required             []string                  `json:"required,omitempty"`
	Items                *openAPISchema            `json:"items,omitempty"`
	AdditionalProperties *bool                     `json:"additionalProperties,omitempty"`
	Minimum              *int                      `json:"minimum,omitempty"`
	Maximum              *int                      `json:"maximum,omitempty"`
	Default              any                       `json:"default,omitempty"`
}

type openAPIParameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Style       string         `json:"style,omitempty"`
	Explode     *bool          `json:"explode,omitempty"`
	Schema      *openAPISchema `json:"schema,omitempty"`
}

type openAPIMedia struct {
	Schema *openAPISchema `json:"schema,omitempty"`
}

type openAPIRequestBody struct {
	Required bool                    `json:"required"`
	Content  map[string]openAPIMedia `json:"content"`
}

type openAPIResponse struct {
	Description string                  `json:"description"`
	Content     map[string]openAPIMedia `json:"content,omitempty"`
}

type openAPIOperation struct {
	Summary     string                     `json:"summary,omitempty"`
	OperationID string                     `json:"operationId,omitempty"`
	Parameters  []openAPIParameter         `json:"parameters,omitempty"`
	RequestBody *openAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]openAPIResponse `json:"responses"`
}

type openAPIPathItem struct {
	Get    *openAPIOperation `json:"get,omitempty"`
	Post   *openAPIOperation `json:"post,omitempty"`
	Patch  *openAPIOperation `json:"patch,omitempty"`
	Delete *openAPIOperation `json:"delete,omitempty"`
}

type openAPISecurityScheme struct {
	Type   string `json:"type"`
	Scheme string `json:"scheme"`
}

type openAPIComponents struct {
	Schemas         map[string]*openAPISchema        `json:"schemas,omitempty"`
	SecuritySchemes map[string]openAPISecurityScheme `json:"securitySchemes"`
}

type openAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

type openAPIDocument struct {
	OpenAPI    string                     `json:"openapi"`
	Info       openAPIInfo                `json:"info"`
	Paths      map[string]openAPIPathItem `json:"paths"`
	Components openAPIComponents          `json:"components"`
	Security   []map[string][]string      `json:"security"`
}

type openObject struct {
	ID     string
	Name   string
	Label  string
	Fields []fieldDef
}

func (s *server) openAPI(w http.ResponseWriter, r *http.Request) {
	objects, err := s.readableObjects(r.Context(), actorFrom(r.Context()))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, buildOpenAPI(objects))
}

// readableObjects is every object for an owner, and only objects with can_read for a member or a service key.
func (s *server) readableObjects(ctx context.Context, a actor) ([]openObject, error) {
	query := `
		select id::text, name, label
		from object_definitions
		where organization_id = $1`
	args := []any{a.OrgID}
	switch a.Role {
	case "owner":
	case "member":
		query += `
			and id in (
				select object_id from object_permissions
				where organization_id = $1 and role = 'member' and can_read
			)`
	case "service":
		if a.KeyID == "" {
			return []openObject{}, nil
		}
		args = append(args, a.KeyID)
		query += `
			and id in (
				select object_id from api_key_permissions
				where organization_id = $1 and api_key_id = $2 and can_read
			)`
	default:
		return []openObject{}, nil
	}
	query += ` order by name`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	objects := []openObject{}
	for rows.Next() {
		var object openObject
		if err := rows.Scan(&object.ID, &object.Name, &object.Label); err != nil {
			return nil, err
		}
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range objects {
		defs, err := s.fieldDefs(ctx, objects[i].ID)
		if err != nil {
			return nil, err
		}
		objects[i].Fields = defs
	}
	return objects, nil
}

func buildOpenAPI(objects []openObject) openAPIDocument {
	doc := openAPIDocument{
		OpenAPI: "3.0.3",
		Info: openAPIInfo{
			Title:       "Openforce Record API",
			Version:     "1.0.0",
			Description: "Record CRUD for the objects this caller can read.",
		},
		Paths: map[string]openAPIPathItem{},
		Components: openAPIComponents{
			SecuritySchemes: map[string]openAPISecurityScheme{
				"bearerAuth": {Type: "http", Scheme: "bearer"},
			},
		},
		Security: []map[string][]string{{"bearerAuth": {}}},
	}
	for _, object := range objects {
		addObjectPaths(&doc, object)
	}
	return doc
}

func addObjectPaths(doc *openAPIDocument, object openObject) {
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = map[string]*openAPISchema{}
	}
	doc.Components.Schemas[object.Name] = fieldObjectSchema(object, true)
	doc.Components.Schemas[object.Name+".patch"] = fieldObjectSchema(object, false)
	doc.Components.Schemas[object.Name+".record"] = recordSchema(object.Name)
	collection := "/api/objects/" + object.Name
	doc.Paths[collection] = openAPIPathItem{
		Get:  listOperation(object),
		Post: createOperation(object),
	}
	doc.Paths[collection+"/{id}"] = openAPIPathItem{
		Get:    readOperation(object),
		Patch:  patchOperation(object),
		Delete: deleteOperation(object),
	}
}

func fieldObjectSchema(object openObject, require bool) *openAPISchema {
	props := map[string]*openAPISchema{}
	var required []string
	for _, field := range object.Fields {
		props[field.Name] = schemaForField(field)
		if require && field.Required {
			required = append(required, field.Name)
		}
	}
	return &openAPISchema{
		Type:                 "object",
		Description:          object.Label,
		Properties:           props,
		Required:             required,
		AdditionalProperties: boolPtr(false),
	}
}

func schemaForField(field fieldDef) *openAPISchema {
	switch field.Type {
	case "text", "relation", "date":
		return &openAPISchema{Type: "string"}
	case "number", "currency":
		return &openAPISchema{Type: "number"}
	case "boolean":
		return &openAPISchema{Type: "boolean"}
	case "enum":
		options := append([]string(nil), field.Options...)
		return &openAPISchema{Type: "string", Enum: options}
	default:
		return &openAPISchema{Type: "string"}
	}
}

func recordSchema(name string) *openAPISchema {
	return &openAPISchema{
		Type:                 "object",
		Required:             []string{"id", "version", "data"},
		AdditionalProperties: boolPtr(false),
		Properties: map[string]*openAPISchema{
			"id":      {Type: "string", Format: "uuid"},
			"version": {Type: "integer"},
			"data":    refSchema(name),
		},
	}
}

func listOperation(object openObject) *openAPIOperation {
	return &openAPIOperation{
		Summary:     "List " + object.Label + " records",
		OperationID: "list_" + object.Name,
		Parameters:  listParameters(),
		Responses: withErrors("200", jsonResponse("Live records.", &openAPISchema{
			Type:                 "object",
			Required:             []string{"records"},
			AdditionalProperties: boolPtr(false),
			Properties: map[string]*openAPISchema{
				"records": {Type: "array", Items: refSchema(object.Name + ".record")},
			},
		}), true, false),
	}
}

func createOperation(object openObject) *openAPIOperation {
	return &openAPIOperation{
		Summary:     "Create a " + object.Label + " record",
		OperationID: "create_" + object.Name,
		RequestBody: jsonBody(refSchema(object.Name), true),
		Responses:   withErrors("201", jsonResponse("Created record.", refSchema(object.Name+".record")), true, false),
	}
}

func readOperation(object openObject) *openAPIOperation {
	return &openAPIOperation{
		Summary:     "Read a " + object.Label + " record",
		OperationID: "get_" + object.Name,
		Parameters:  []openAPIParameter{idParameter()},
		Responses:   withErrors("200", jsonResponse("One live record.", refSchema(object.Name+".record")), false, false),
	}
}

func patchOperation(object openObject) *openAPIOperation {
	return &openAPIOperation{
		Summary:     "Update a " + object.Label + " record",
		OperationID: "patch_" + object.Name,
		Parameters:  []openAPIParameter{idParameter()},
		RequestBody: jsonBody(&openAPISchema{
			Type:                 "object",
			Required:             []string{"version"},
			AdditionalProperties: boolPtr(false),
			Properties: map[string]*openAPISchema{
				"version": {Type: "integer", Minimum: intPtr(1)},
				"data":    refSchema(object.Name + ".patch"),
			},
		}, true),
		Responses: withErrors("200", jsonResponse("Updated record.", refSchema(object.Name+".record")), true, true),
	}
}

func deleteOperation(object openObject) *openAPIOperation {
	return &openAPIOperation{
		Summary:     "Delete a " + object.Label + " record",
		OperationID: "delete_" + object.Name,
		Parameters:  []openAPIParameter{idParameter()},
		Responses: withErrors("204", openAPIResponse{
			Description: "Deleted. The response is empty.",
		}, false, false),
	}
}

func listParameters() []openAPIParameter {
	return []openAPIParameter{
		{
			Name:        "limit",
			In:          "query",
			Description: "1 to 200. The default is 50.",
			Schema:      &openAPISchema{Type: "integer", Minimum: intPtr(1), Maximum: intPtr(200), Default: 50},
		},
		{
			Name:        "filter",
			In:          "query",
			Description: "Repeatable field:op:value.",
			Style:       "form",
			Explode:     boolPtr(true),
			Schema:      &openAPISchema{Type: "array", Items: &openAPISchema{Type: "string"}},
		},
		{
			Name:        "sort",
			In:          "query",
			Description: "Repeatable field:asc or field:desc.",
			Style:       "form",
			Explode:     boolPtr(true),
			Schema:      &openAPISchema{Type: "array", Items: &openAPISchema{Type: "string"}},
		},
		{
			Name:        "view",
			In:          "query",
			Description: "Saved view name.",
			Schema:      &openAPISchema{Type: "string"},
		},
	}
}

func idParameter() openAPIParameter {
	return openAPIParameter{
		Name:        "id",
		In:          "path",
		Required:    true,
		Description: "Record id.",
		Schema:      &openAPISchema{Type: "string", Format: "uuid"},
	}
}

func withErrors(success string, body openAPIResponse, badRequest, conflict bool) map[string]openAPIResponse {
	out := map[string]openAPIResponse{
		success: body,
		"401":   errorResponse("Missing or invalid bearer token.", "unauthorized"),
		"403":   errorResponse("Forbidden.", "forbidden"),
		"404":   errorResponse("Not found.", "not found"),
	}
	if badRequest {
		out["400"] = errorResponse("Invalid request.", "")
	}
	if conflict {
		out["409"] = errorResponse("Stale version.", "stale version")
	}
	return out
}

func jsonResponse(desc string, schema *openAPISchema) openAPIResponse {
	return openAPIResponse{
		Description: desc,
		Content: map[string]openAPIMedia{
			"application/json": {Schema: schema},
		},
	}
}

func errorResponse(desc, msg string) openAPIResponse {
	prop := &openAPISchema{Type: "string"}
	if msg != "" {
		prop.Enum = []string{msg}
	}
	return jsonResponse(desc, &openAPISchema{
		Type:                 "object",
		Required:             []string{"error"},
		AdditionalProperties: boolPtr(false),
		Properties:           map[string]*openAPISchema{"error": prop},
	})
}

func jsonBody(schema *openAPISchema, required bool) *openAPIRequestBody {
	return &openAPIRequestBody{
		Required: required,
		Content: map[string]openAPIMedia{
			"application/json": {Schema: schema},
		},
	}
}

func refSchema(name string) *openAPISchema {
	return &openAPISchema{Ref: "#/components/schemas/" + name}
}

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }
