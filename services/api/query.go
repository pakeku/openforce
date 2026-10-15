package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type filterBody struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type sortBody struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type listQuery struct {
	Where   []string
	Order   []string
	Args    []any
	Columns []string
	Limit   int
}

func (s *server) resolveList(r *http.Request, orgID, objectID string, defs []fieldDef) (listQuery, error) {
	q := listQuery{Args: []any{orgID, objectID}, Limit: 50}
	limit, err := listLimit(r)
	if err != nil {
		return q, err
	}
	q.Limit = limit

	byName := indexFields(defs)
	if name := r.URL.Query().Get("view"); name != "" {
		view, err := s.loadView(r.Context(), orgID, objectID, name)
		if err != nil {
			return q, err
		}
		q.Columns = view.Columns
		if view.Kind == "kanban" || view.Kind == "calendar" {
			q.Columns = appendGroupColumn(view.Columns, view.Group)
		}
		for _, filter := range view.Filters {
			if err := q.addFilter(byName, filter.Field, filter.Operator, filter.Value); err != nil {
				return q, err
			}
		}
		for _, sort := range view.Sort {
			if err := q.addSort(byName, sort.Field, sort.Direction); err != nil {
				return q, err
			}
		}
	}

	for _, raw := range r.URL.Query()["filter"] {
		field, op, value, err := splitFilter(raw)
		if err != nil {
			return q, err
		}
		if err := q.addFilter(byName, field, op, value); err != nil {
			return q, err
		}
	}
	if sorts := r.URL.Query()["sort"]; len(sorts) > 0 {
		q.Order = nil
		for _, raw := range sorts {
			field, dir, err := splitSort(raw)
			if err != nil {
				return q, err
			}
			if err := q.addSort(byName, field, dir); err != nil {
				return q, err
			}
		}
	}
	if len(q.Order) == 0 {
		q.Order = []string{"created_at", "id"}
	} else {
		q.Order = append(q.Order, "id")
	}
	return q, nil
}

func splitFilter(raw string) (string, string, string, error) {
	field, rest, ok := strings.Cut(raw, ":")
	op, value, ok2 := strings.Cut(rest, ":")
	if !ok || !ok2 || field == "" || op == "" {
		return "", "", "", apiErr(http.StatusBadRequest, "invalid filter")
	}
	return field, op, value, nil
}

func splitSort(raw string) (string, string, error) {
	field, dir, ok := strings.Cut(raw, ":")
	if !ok {
		field = raw
		dir = "asc"
	}
	if field == "" || (dir != "asc" && dir != "desc") {
		return "", "", apiErr(http.StatusBadRequest, "invalid sort")
	}
	return field, dir, nil
}

func indexFields(defs []fieldDef) map[string]fieldDef {
	byName := map[string]fieldDef{
		"created_at": {Name: "created_at", Type: "timestamptz"},
	}
	for _, def := range defs {
		byName[def.Name] = def
	}
	return byName
}

func (q *listQuery) addFilter(byName map[string]fieldDef, field, op string, value any) error {
	def, ok := byName[field]
	if !ok {
		return apiErrf(http.StatusBadRequest, "unknown field %s", field)
	}
	text, err := valueString(value)
	if err != nil {
		return apiErrf(http.StatusBadRequest, "invalid value for %s", field)
	}
	expr, err := q.comparison(def, op, text)
	if err != nil {
		return err
	}
	q.Where = append(q.Where, expr)
	return nil
}

func (q *listQuery) addSort(byName map[string]fieldDef, field, dir string) error {
	def, ok := byName[field]
	if !ok {
		return apiErrf(http.StatusBadRequest, "unknown field %s", field)
	}
	if dir != "asc" && dir != "desc" {
		return apiErr(http.StatusBadRequest, "invalid sort")
	}
	expr, err := orderExpr(def)
	if err != nil {
		return err
	}
	if expr == "created_at" {
		q.Order = append(q.Order, "created_at "+dir)
		return nil
	}
	q.Args = append(q.Args, field)
	q.Order = append(q.Order, fmt.Sprintf(expr+" %s nulls last", len(q.Args), dir))
	return nil
}

func orderExpr(def fieldDef) (string, error) {
	if def.Name == "created_at" {
		return "created_at", nil
	}
	switch def.Type {
	case "number", "currency":
		return "(data->>$%d)::numeric", nil
	case "date":
		return "(data->>$%d)::date", nil
	case "text", "enum", "relation", "boolean":
		return "data->>$%d", nil
	default:
		return "", apiErrf(http.StatusBadRequest, "unknown field %s", def.Name)
	}
}

func (q *listQuery) comparison(def fieldDef, op, value string) (string, error) {
	if def.Name == "created_at" {
		return q.timestampCompare("created_at", op, value, def.Name)
	}
	key := q.placeholder(def.Name)
	switch def.Type {
	case "number", "currency":
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return "", apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		return compareExpr(fmt.Sprintf("(data->>%s)::numeric", key), op, q.placeholder(n), orderedOps)
	case "date":
		cast, arg, err := dateArg(value)
		if err != nil {
			return "", apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		return compareExpr(fmt.Sprintf("(data->>%s)::%s", key, cast), op, q.placeholder(arg), orderedOps)
	case "boolean":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return "", apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		return compareExpr(fmt.Sprintf("(data->>%s)::boolean", key), op, q.placeholder(b), eqOps)
	case "text":
		return q.textCompare(fmt.Sprintf("data->>%s", key), op, value, def.Name)
	case "enum", "relation":
		return compareExpr(fmt.Sprintf("data->>%s", key), op, q.placeholder(value), eqOps)
	default:
		return "", apiErrf(http.StatusBadRequest, "unknown field %s", def.Name)
	}
}

func (q *listQuery) textCompare(expr, op, value, field string) (string, error) {
	switch op {
	case "eq":
		return expr + " = " + q.placeholder(value), nil
	case "neq":
		return expr + " is distinct from " + q.placeholder(value), nil
	case "contains":
		return expr + " ilike " + q.placeholder("%"+escapeLike(value)+"%") + ` escape '\'`, nil
	default:
		return "", apiErrf(http.StatusBadRequest, "invalid operator for %s", field)
	}
}

func (q *listQuery) timestampCompare(expr, op, value, field string) (string, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", apiErrf(http.StatusBadRequest, "invalid value for %s", field)
	}
	return compareExpr(expr, op, q.placeholder(parsed), orderedOps)
}

func (q *listQuery) placeholder(value any) string {
	q.Args = append(q.Args, value)
	return fmt.Sprintf("$%d", len(q.Args))
}

var (
	eqOps = map[string]string{
		"eq":  "=",
		"neq": "is distinct from",
	}
	orderedOps = map[string]string{
		"eq":  "=",
		"neq": "is distinct from",
		"gt":  ">",
		"gte": ">=",
		"lt":  "<",
		"lte": "<=",
	}
)

func compareExpr(expr, op, placeholder string, ops map[string]string) (string, error) {
	sql, ok := ops[op]
	if !ok {
		return "", apiErr(http.StatusBadRequest, "invalid operator")
	}
	return expr + " " + sql + " " + placeholder, nil
}

func dateArg(value string) (string, any, error) {
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return "date", parsed, nil
	}
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return "timestamptz", value, nil
	}
	return "", nil, fmt.Errorf("invalid date")
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func valueString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(typed), nil
	default:
		return "", fmt.Errorf("invalid value")
	}
}

func (q listQuery) sql() (string, []any) {
	args := append([]any{}, q.Args...)
	var b strings.Builder
	b.WriteString(`
		select id::text, version, data
		from records
		where organization_id = $1::uuid and object_id = $2::uuid and deleted_at is null`)
	for _, where := range q.Where {
		b.WriteString(" and ")
		b.WriteString(where)
	}
	b.WriteString(" order by ")
	b.WriteString(strings.Join(q.Order, ", "))
	args = append(args, q.Limit)
	fmt.Fprintf(&b, " limit $%d", len(args))
	return b.String(), args
}

func appendGroupColumn(columns []string, group string) []string {
	if group == "" || hasStar(columns) {
		return columns
	}
	for _, column := range columns {
		if column == group {
			return columns
		}
	}
	copied := make([]string, len(columns), len(columns)+1)
	copy(copied, columns)
	return append(copied, group)
}

func projectRecords(records []recordView, columns []string) []recordView {
	if len(columns) == 0 || hasStar(columns) {
		return records
	}
	allow := map[string]struct{}{}
	for _, column := range columns {
		allow[column] = struct{}{}
	}
	for i := range records {
		next := map[string]any{}
		for key, value := range records[i].Data {
			if _, ok := allow[key]; ok {
				next[key] = value
			}
		}
		records[i].Data = next
	}
	return records
}

func hasStar(columns []string) bool {
	for _, column := range columns {
		if column == "*" {
			return true
		}
	}
	return false
}
