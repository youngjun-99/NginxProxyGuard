package dialect

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

// DriverName is the database/sql driver that translates PostgreSQL SQL for
// MySQL. Opening plain "mysql" instead would send PostgreSQL to the server.
const DriverName = "npg-mysql"

func init() {
	sql.Register(DriverName, &translatingDriver{})
}

// translatingDriver wraps go-sql-driver/mysql. Every statement passes through
// Translate on its way to the server; RETURNING is emulated (returning.go).
type translatingDriver struct{}

func (d *translatingDriver) Open(dsn string) (driver.Conn, error) {
	c, err := d.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return c.Connect(context.Background())
}

func (d *translatingDriver) OpenConnector(dsn string) (driver.Connector, error) {
	inner, err := (mysql.MySQLDriver{}).OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return &connector{inner: inner, drv: d}, nil
}

type connector struct {
	inner driver.Connector
	drv   *translatingDriver
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	inner, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &conn{inner: inner}, nil
}

func (c *connector) Driver() driver.Driver { return c.drv }

// conn is one MySQL connection seen through the translator.
type conn struct {
	inner driver.Conn
	inTx  bool
}

var (
	_ driver.ConnBeginTx        = (*conn)(nil)
	_ driver.ExecerContext      = (*conn)(nil)
	_ driver.QueryerContext     = (*conn)(nil)
	_ driver.ConnPrepareContext = (*conn)(nil)
	_ driver.NamedValueChecker  = (*conn)(nil)
	_ driver.Pinger             = (*conn)(nil)
	_ driver.SessionResetter    = (*conn)(nil)
	_ driver.Validator          = (*conn)(nil)
)

func (c *conn) Close() error { return c.inner.Close() }

func (c *conn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.inner.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	c.inTx = true
	return &txWrap{inner: tx, c: c}, nil
}

type txWrap struct {
	inner driver.Tx
	c     *conn
}

func (t *txWrap) Commit() error   { t.c.inTx = false; return t.inner.Commit() }
func (t *txWrap) Rollback() error { t.c.inTx = false; return t.inner.Rollback() }

func (c *conn) Ping(ctx context.Context) error { return c.inner.(driver.Pinger).Ping(ctx) }

func (c *conn) ResetSession(ctx context.Context) error {
	if r, ok := c.inner.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *conn) IsValid() bool {
	if v, ok := c.inner.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

// CheckNamedValue accepts every argument as is. database/sql would otherwise
// reduce a pq.StringArray bound to `= ANY($1)` to its text form before the
// translator could expand it into an IN list; bind converts values for
// go-sql-driver once the statement's shape is known.
func (c *conn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

// PrepareContext returns a statement that is translated and bound on each
// execution. Nothing is prepared on the server: arguments are interpolated
// client-side (interpolateParams), which costs one round trip instead of three.
func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if strings.HasPrefix(query, RawPrefix) {
		return &stmt{c: c, query: query}, nil
	}
	if _, err := Translate(query); err != nil {
		return nil, wrapTranslateError(err, query)
	}
	return &stmt{c: c, query: query}, nil
}

func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if raw, ok := strings.CutPrefix(query, RawPrefix); ok {
		bound, err := bindAll(args)
		if err != nil {
			return nil, err
		}
		return c.execRaw(ctx, raw, bound)
	}
	p, err := Translate(query)
	if err != nil {
		return nil, wrapTranslateError(err, query)
	}
	sqlText, bound, err := render(p.sql, args, nil)
	if err != nil {
		return nil, err
	}
	return c.execRaw(ctx, sqlText, bound)
}

func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if raw, ok := strings.CutPrefix(query, RawPrefix); ok {
		bound, err := bindAll(args)
		if err != nil {
			return nil, err
		}
		return c.queryRaw(ctx, raw, bound)
	}
	p, err := Translate(query)
	if err != nil {
		return nil, wrapTranslateError(err, query)
	}
	if p.ret != nil {
		return c.runReturning(ctx, p, args)
	}
	sqlText, bound, err := render(p.sql, args, nil)
	if err != nil {
		return nil, err
	}
	return c.queryRaw(ctx, sqlText, bound)
}

// wrapTranslateError reports a statement the translator could not rewrite.
// The SQL goes to the log, not into the error: errors reach API responses and
// stored error columns, and the scrubber recognises the ErrUntranslatable text
// every such error carries.
func wrapTranslateError(err error, query string) error {
	q := strings.Join(strings.Fields(query), " ")
	if len(q) > 600 {
		q = q[:600] + "..."
	}
	log.Printf("[DB] MySQL translation failed: %v -- %s", err, q)
	if errors.Is(err, ErrUntranslatable) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrUntranslatable, err)
}

// logFailures, set by NPG_DB_LOG_ERRORS=1, logs every statement the server
// rejects together with its translated text (never the arguments). Several
// repositories degrade quietly on a failed query — a dashboard widget comes
// back empty — so this is how a MySQL-specific failure gets reported.
var logFailures = os.Getenv("NPG_DB_LOG_ERRORS") == "1"

func noteFailure(query string, err error) {
	if err == nil || !logFailures || errors.Is(err, driver.ErrSkip) {
		return
	}
	q := strings.Join(strings.Fields(query), " ")
	if len(q) > 600 {
		q = q[:600] + "..."
	}
	log.Printf("[DB] MySQL rejected a statement: %v -- %s", err, q)
}

// execRaw runs MySQL SQL on the underlying connection. go-sql-driver answers
// driver.ErrSkip when it cannot interpolate (a literal '?' in the text, for
// one); the statement is then prepared on the server instead.
func (c *conn) execRaw(ctx context.Context, query string, args []driver.NamedValue) (res driver.Result, err error) {
	defer func() { noteFailure(query, err) }()
	res, err = c.inner.(driver.ExecerContext).ExecContext(ctx, query, args)
	if !errors.Is(err, driver.ErrSkip) {
		return res, err
	}
	st, err := c.inner.(driver.ConnPrepareContext).PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	return st.(driver.StmtExecContext).ExecContext(ctx, args)
}

func (c *conn) queryRaw(ctx context.Context, query string, args []driver.NamedValue) (_ driver.Rows, err error) {
	defer func() { noteFailure(query, err) }()
	rows, err := c.inner.(driver.QueryerContext).QueryContext(ctx, query, args)
	if !errors.Is(err, driver.ErrSkip) {
		return rows, err
	}
	st, err := c.inner.(driver.ConnPrepareContext).PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err = st.(driver.StmtQueryContext).QueryContext(ctx, args)
	if err != nil {
		st.Close()
		return nil, err
	}
	return &stmtRows{Rows: rows, st: st}, nil
}

// stmtRows closes the server-side statement with its rows.
type stmtRows struct {
	driver.Rows
	st driver.Stmt
}

func (r *stmtRows) Close() error {
	err := r.Rows.Close()
	r.st.Close()
	return err
}

// stmt is a lazily translated prepared statement.
type stmt struct {
	c     *conn
	query string
}

func (s *stmt) Close() error                             { return nil }
func (s *stmt) NumInput() int                            { return -1 }
func (s *stmt) CheckNamedValue(*driver.NamedValue) error { return nil }

func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), valuesToNamed(args))
}

func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), valuesToNamed(args))
}

func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.c.ExecContext(ctx, s.query, args)
}

func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.c.QueryContext(ctx, s.query, args)
}

func valuesToNamed(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	return out
}

// ---------------------------------------------------------------------------
// Binding
// ---------------------------------------------------------------------------

// render emits a template as MySQL text with `?` placeholders and the
// argument list those placeholders bind, in order.
func render(t *template, args []driver.NamedValue, synth []driver.Value) (string, []driver.NamedValue, error) {
	var b strings.Builder
	var bound []driver.NamedValue
	add := func(v any) error {
		cv, err := convertArg(v)
		if err != nil {
			return err
		}
		bound = append(bound, driver.NamedValue{Ordinal: len(bound) + 1, Value: cv})
		b.WriteByte('?')
		return nil
	}
	for _, s := range t.segs {
		if s.param == nil {
			b.WriteString(s.text)
			continue
		}
		switch s.param.kind {
		case paramArg:
			if s.param.index >= len(args) {
				return "", nil, fmt.Errorf("statement uses $%d but only %d argument(s) were given", s.param.index+1, len(args))
			}
			if err := add(args[s.param.index].Value); err != nil {
				return "", nil, err
			}
		case paramArray:
			if s.param.index >= len(args) {
				return "", nil, fmt.Errorf("statement uses $%d but only %d argument(s) were given", s.param.index+1, len(args))
			}
			elems, err := arrayElements(args[s.param.index].Value)
			if err != nil {
				return "", nil, fmt.Errorf("$%d: %w", s.param.index+1, err)
			}
			if len(elems) == 0 {
				b.WriteString("SELECT NULL FROM DUAL WHERE FALSE")
				continue
			}
			for i, e := range elems {
				if i > 0 {
					b.WriteString(", ")
				}
				if err := add(e); err != nil {
					return "", nil, err
				}
			}
		case paramSynth:
			if err := add(synth[s.param.index]); err != nil {
				return "", nil, err
			}
		case paramInterval:
			if s.param.index >= len(args) {
				return "", nil, fmt.Errorf("statement uses $%d but only %d argument(s) were given", s.param.index+1, len(args))
			}
			v, err := convertArg(args[s.param.index].Value)
			if err != nil {
				return "", nil, err
			}
			text, ok := v.(string)
			if !ok {
				return "", nil, fmt.Errorf("$%d::interval: want an interval string, got %T", s.param.index+1, v)
			}
			nodes, err := intervalNodes(text)
			if err != nil {
				return "", nil, fmt.Errorf("$%d::interval: %w", s.param.index+1, err)
			}
			// INTERVAL <number> <UNIT>: both parts come out of intervalNodes'
			// parser, so nothing of the argument is copied through as text.
			b.WriteString(nodes[0].tok.text + " " + nodes[1].tok.text + " " + nodes[2].tok.text)
		case paramJSONArray:
			if s.param.index >= len(args) {
				return "", nil, fmt.Errorf("statement uses $%d but only %d argument(s) were given", s.param.index+1, len(args))
			}
			elems, err := arrayElements(args[s.param.index].Value)
			if err != nil {
				return "", nil, fmt.Errorf("$%d: %w", s.param.index+1, err)
			}
			vals := make([]any, len(elems))
			for i, e := range elems {
				if vals[i], err = convertArg(e); err != nil {
					return "", nil, err
				}
			}
			js, err := json.Marshal(vals)
			if err != nil {
				return "", nil, err
			}
			if err := add(string(js)); err != nil {
				return "", nil, err
			}
		}
	}
	return b.String(), bound, nil
}

// bindAll converts arguments of a raw statement.
func bindAll(args []driver.NamedValue) ([]driver.NamedValue, error) {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		v, err := convertArg(a.Value)
		if err != nil {
			return nil, err
		}
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return out, nil
}

// convertArg reduces a Go value to one go-sql-driver binds.
func convertArg(v any) (driver.Value, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case []byte:
		// JSON columns refuse a binary-charset string, and that is how a
		// []byte (json.RawMessage, marshalled JSON) is sent. Text is sent as
		// text; genuinely binary data stays binary.
		if utf8.Valid(x) {
			return string(x), nil
		}
		return x, nil
	case string, int64, float64, bool:
		return x, nil
	case driver.Valuer:
		val, err := x.Value()
		if err != nil {
			return nil, err
		}
		return convertArg(val)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil
		}
		return convertArg(rv.Elem().Interface())
	}
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() != reflect.Uint8 {
		// A bare slice outside ANY(): store it the way pq.Array would.
		val, err := pq.Array(v).Value()
		if err != nil {
			return nil, err
		}
		return convertArg(val)
	}
	val, err := driver.DefaultParameterConverter.ConvertValue(v)
	if err != nil {
		return nil, err
	}
	if b, ok := val.([]byte); ok {
		return convertArg(b)
	}
	return val, nil
}

// arrayElements returns the elements of an array argument bound to
// `= ANY($n)`: a pq array type, a Go slice, or a PostgreSQL array literal.
func arrayElements(v any) ([]any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case pq.StringArray:
		return toAny([]string(x)), nil
	case pq.Int64Array:
		return toAny([]int64(x)), nil
	case pq.Int32Array:
		return toAny([]int32(x)), nil
	case pq.Float64Array:
		return toAny([]float64(x)), nil
	case pq.BoolArray:
		return toAny([]bool(x)), nil
	case pq.GenericArray:
		return arrayElements(x.A)
	case *pq.GenericArray:
		return arrayElements(x.A)
	case string:
		elems, ok := parsePGArrayLiteral(x)
		if !ok {
			return nil, fmt.Errorf("not an array literal: %q", x)
		}
		return toAny(elems), nil
	case []byte:
		return arrayElements(string(x))
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, nil
		}
		return arrayElements(rv.Elem().Interface())
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = rv.Index(i).Interface()
		}
		return out, nil
	}
	if vr, ok := v.(driver.Valuer); ok {
		val, err := vr.Value()
		if err != nil {
			return nil, err
		}
		return arrayElements(val)
	}
	return nil, fmt.Errorf("unsupported array argument %T", v)
}

func toAny[T any](s []T) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Buffered rows
// ---------------------------------------------------------------------------

// bufferedRows serves rows read ahead of time: the result of an emulated
// RETURNING, read inside the transaction that wrote them.
type bufferedRows struct {
	cols []string
	data [][]driver.Value
	pos  int
}

func (r *bufferedRows) Columns() []string { return r.cols }
func (r *bufferedRows) Close() error      { return nil }

func (r *bufferedRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.pos])
	r.pos++
	return nil
}

// readAll drains rows into memory, keeping the first keep columns (all when
// keep < 0).
func readAll(rows driver.Rows, keep int) (*bufferedRows, [][]driver.Value, error) {
	defer rows.Close()
	cols := rows.Columns()
	if keep < 0 || keep > len(cols) {
		keep = len(cols)
	}
	out := &bufferedRows{cols: append([]string(nil), cols[:keep]...)}
	var full [][]driver.Value
	for {
		dest := make([]driver.Value, len(cols))
		if err := rows.Next(dest); err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, err
		}
		// go-sql-driver reuses its buffers between rows.
		for i, v := range dest {
			if b, ok := v.([]byte); ok {
				dest[i] = append([]byte(nil), b...)
			}
		}
		full = append(full, dest)
		out.data = append(out.data, dest[:keep])
	}
	return out, full, nil
}
