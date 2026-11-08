package dialect

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// RETURNING emulation.
//
// MySQL has no RETURNING. A write that asks for rows back is run as the write
// plus a read of exactly the rows it touched, inside one transaction (the
// caller's, or one opened for the purpose), so the rows returned are the rows
// written:
//
//   - INSERT ... VALUES: the key of every row is known before the write — the
//     caller supplied it, or the driver generates the UUID the column's
//     DEFAULT (UUID()) would have, or AUTO_INCREMENT hands it out — and the
//     rows are read back by key. An upsert reads back by its conflict target,
//     because an updated row keeps its original key.
//   - UPDATE: the target rows' keys are read and locked (SELECT ... FOR
//     UPDATE) with the statement's own WHERE, the update is applied to exactly
//     those keys, and they are read again.
//   - DELETE: the rows are read and locked first, then deleted by key.
//
// Anything else with RETURNING — INSERT ... SELECT, UPDATE ... FROM — is
// reported as untranslatable rather than guessed at.

type tableMeta struct {
	pk          []string
	autoInc     string
	uuidDefault map[string]bool
}

var metaCache sync.Map // table name -> *tableMeta

func (c *conn) tableMeta(ctx context.Context, table string) (*tableMeta, error) {
	if m, ok := metaCache.Load(table); ok {
		return m.(*tableMeta), nil
	}
	m := &tableMeta{uuidDefault: map[string]bool{}}
	rows, err := c.queryRaw(ctx,
		`SELECT COLUMN_NAME, EXTRA, COALESCE(COLUMN_DEFAULT, '') FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`,
		[]driver.NamedValue{{Ordinal: 1, Value: table}})
	if err != nil {
		return nil, err
	}
	_, data, err := readAll(rows, -1)
	if err != nil {
		return nil, err
	}
	for _, r := range data {
		name, extra, def := asString(r[0]), strings.ToLower(asString(r[1])), strings.ToLower(asString(r[2]))
		if strings.Contains(extra, "auto_increment") {
			m.autoInc = name
		}
		if strings.Contains(def, "uuid(") {
			m.uuidDefault[name] = true
		}
	}
	rows, err = c.queryRaw(ctx,
		`SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY'
		 ORDER BY ORDINAL_POSITION`,
		[]driver.NamedValue{{Ordinal: 1, Value: table}})
	if err != nil {
		return nil, err
	}
	_, data, err = readAll(rows, -1)
	if err != nil {
		return nil, err
	}
	for _, r := range data {
		m.pk = append(m.pk, asString(r[0]))
	}
	if len(m.pk) == 0 {
		return nil, fmt.Errorf("RETURNING on %s: table has no primary key to read the rows back by", table)
	}
	metaCache.Store(table, m)
	return m, nil
}

func (c *conn) indexColumns(ctx context.Context, table, index string) ([]string, error) {
	rows, err := c.queryRaw(ctx,
		`SELECT COLUMN_NAME FROM information_schema.STATISTICS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ? ORDER BY SEQ_IN_INDEX`,
		[]driver.NamedValue{{Ordinal: 1, Value: table}, {Ordinal: 2, Value: index}})
	if err != nil {
		return nil, err
	}
	_, data, err := readAll(rows, -1)
	if err != nil {
		return nil, err
	}
	var cols []string
	for _, r := range data {
		if r[0] == nil {
			return nil, untranslatable("ON CONFLICT ON CONSTRAINT %s: an expression index", index)
		}
		cols = append(cols, asString(r[0]))
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("ON CONFLICT ON CONSTRAINT %s: no such index on %s", index, table)
	}
	return cols, nil
}

func asString(v driver.Value) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

// runReturning executes a statement with RETURNING and returns its rows.
func (c *conn) runReturning(ctx context.Context, p *Plan, args []driver.NamedValue) (driver.Rows, error) {
	own := !c.inTx
	if own {
		if _, err := c.execRaw(ctx, "START TRANSACTION", nil); err != nil {
			return nil, err
		}
	}
	rows, err := c.returningBody(ctx, p, args)
	if own {
		if err != nil {
			_, _ = c.execRaw(context.WithoutCancel(ctx), "ROLLBACK", nil)
			return nil, err
		}
		if _, err := c.execRaw(ctx, "COMMIT", nil); err != nil {
			return nil, err
		}
	}
	return rows, err
}

// keySet accumulates key tuples as synthesised parameters.
type keySet struct {
	synth []driver.Value
	keys  [][]int // per row: synth slot of each key column
}

func (k *keySet) slot(v driver.Value) int {
	k.synth = append(k.synth, v)
	return len(k.synth) - 1
}

// cond renders `"a" IN (?, ?)` or `("a", "b") IN ((?, ?), (?, ?))`.
func (k *keySet) cond(cols []string) []node {
	if len(k.keys) == 0 {
		return []node{kw("FALSE")}
	}
	var lhs []node
	if len(cols) == 1 {
		lhs = []node{quotedIdent(cols[0])}
	} else {
		var parts [][]node
		for _, c := range cols {
			parts = append(parts, []node{quotedIdent(c)})
		}
		lhs = []node{paren(joinTop(parts)...)}
	}
	var tuples [][]node
	for _, row := range k.keys {
		var vals [][]node
		for _, s := range row {
			vals = append(vals, []node{{param: &paramRef{kind: paramSynth, index: s}}})
		}
		if len(cols) == 1 {
			tuples = append(tuples, vals[0])
		} else {
			tuples = append(tuples, []node{paren(joinTop(vals)...)})
		}
	}
	return append(lhs, kw("IN"), paren(joinTop(tuples)...))
}

func (c *conn) returningBody(ctx context.Context, p *Plan, args []driver.NamedValue) (driver.Rows, error) {
	ret := p.ret
	meta, err := c.tableMeta(ctx, ret.table)
	if err != nil {
		return nil, err
	}
	switch ret.kind {
	case stmtInsert:
		return c.insertReturning(ctx, ret, meta, args)
	case stmtUpdate:
		return c.updateReturning(ctx, ret, meta, args)
	case stmtDelete:
		return c.deleteReturning(ctx, ret, meta, args)
	}
	return nil, untranslatable("RETURNING on this statement")
}

func (c *conn) query(ctx context.Context, items []node, args []driver.NamedValue, synth []driver.Value) (driver.Rows, error) {
	t, err := emit(items)
	if err != nil {
		return nil, err
	}
	q, bound, err := render(t, args, synth)
	if err != nil {
		return nil, err
	}
	return c.queryRaw(ctx, q, bound)
}

func (c *conn) exec(ctx context.Context, items []node, args []driver.NamedValue, synth []driver.Value) (driver.Result, error) {
	t, err := emit(items)
	if err != nil {
		return nil, err
	}
	q, bound, err := render(t, args, synth)
	if err != nil {
		return nil, err
	}
	return c.execRaw(ctx, q, bound)
}

// selectBack reads the RETURNING list for the rows matching cond.
func (c *conn) selectBack(ctx context.Context, ret *returning, cond []node, args []driver.NamedValue, synth []driver.Value) (driver.Rows, error) {
	q := append([]node{kw("SELECT")}, ret.exprs...)
	q = append(q, kw("FROM"))
	q = append(q, ret.ref...)
	q = append(q, kw("WHERE"))
	q = append(q, cond...)
	rows, err := c.query(ctx, q, args, synth)
	if err != nil {
		return nil, err
	}
	buf, _, err := readAll(rows, -1)
	return buf, err
}

// resolveKey evaluates a key value written in the statement: a bound
// argument or a literal.
func resolveKey(v []node, args []driver.NamedValue) (driver.Value, bool) {
	if len(v) != 1 {
		return nil, false
	}
	n := v[0]
	switch {
	case n.param != nil && n.param.kind == paramArg && n.param.index < len(args):
		cv, err := convertArg(args[n.param.index].Value)
		return cv, err == nil
	case n.isKind(tkString):
		return unquoteString(n.tok.text), true
	case n.isKind(tkNumber):
		return n.tok.text, true
	}
	return nil, false
}

func isUUIDCall(v []node) bool {
	return len(v) == 2 && v[0].is("UUID") && v[1].isParen() && len(v[1].group.items) == 0
}

func indexOf(cols []string, name string) int {
	for i, c := range cols {
		if strings.EqualFold(c, name) {
			return i
		}
	}
	return -1
}

func (c *conn) insertReturning(ctx context.Context, ret *returning, meta *tableMeta, args []driver.NamedValue) (driver.Rows, error) {
	if ret.rows == nil {
		return nil, untranslatable("INSERT ... SELECT ... RETURNING")
	}
	cols := append([]string(nil), ret.columns...)
	if cols == nil && !(len(ret.rows) == 1 && len(ret.rows[0]) == 0) {
		return nil, untranslatable("INSERT ... RETURNING without a column list")
	}
	rows := make([][][]node, len(ret.rows))
	for r := range ret.rows {
		rows[r] = append([][]node(nil), ret.rows[r]...)
	}

	ks := &keySet{keys: make([][]int, len(rows))}
	generated := true // every key is one the driver made up
	useLastID := false
	for _, p := range meta.pk {
		ci := indexOf(cols, p)
		switch {
		case ci >= 0:
			for r := range rows {
				v := rows[r][ci]
				if isUUIDCall(v) {
					s := ks.slot(uuid.NewString())
					rows[r][ci] = []node{{param: &paramRef{kind: paramSynth, index: s}}}
					ks.keys[r] = append(ks.keys[r], s)
					continue
				}
				val, ok := resolveKey(v, args)
				if !ok {
					return nil, untranslatable("INSERT ... RETURNING with a computed key for %s", p)
				}
				ks.keys[r] = append(ks.keys[r], ks.slot(val))
				generated = false
			}
		case meta.uuidDefault[p]:
			cols = append(cols, p)
			for r := range rows {
				s := ks.slot(uuid.NewString())
				rows[r] = append(rows[r], []node{{param: &paramRef{kind: paramSynth, index: s}}})
				ks.keys[r] = append(ks.keys[r], s)
			}
		case meta.autoInc == p && len(meta.pk) == 1:
			useLastID = true
			generated = false
		default:
			return nil, untranslatable("INSERT ... RETURNING on %s: no value for key column %s", ret.table, p)
		}
	}

	stmt := clone(ret.head)
	if len(cols) > 0 {
		var cl [][]node
		for _, c := range cols {
			cl = append(cl, []node{quotedIdent(c)})
		}
		stmt = append(stmt, paren(joinTop(cl)...))
	} else {
		stmt = append(stmt, paren())
	}
	stmt = append(stmt, kw("VALUES"))
	var tuples [][]node
	for _, row := range rows {
		tuples = append(tuples, []node{paren(joinTop(row)...)})
	}
	stmt = append(stmt, joinTop(tuples)...)
	stmt = append(stmt, ret.tail...)

	res, err := c.exec(ctx, stmt, args, ks.synth)
	if err != nil {
		return nil, err
	}
	affected, _ := res.RowsAffected()

	keyCols := meta.pk
	switch {
	case ret.upsert && ret.conflictExprs != nil:
		// (lower(name)): find each row as `lower(name) = lower(<its value>)`.
		var alts [][]node
		for r := range rows {
			var conj []node
			for k, e := range ret.conflictExprs {
				rhs, used := substituteColumns(e, cols, rows[r])
				if !used {
					return nil, untranslatable("upsert RETURNING: conflict expression reads no inserted column")
				}
				if k > 0 {
					conj = append(conj, kw("AND"))
				}
				conj = append(conj, paren(clone(e)...), op("="), paren(rhs...))
			}
			alts = append(alts, []node{paren(conj...)})
		}
		var cond []node
		for k, a := range alts {
			if k > 0 {
				cond = append(cond, kw("OR"))
			}
			cond = append(cond, a...)
		}
		cond = []node{paren(cond...)}
		if len(ret.conflictWhere) > 0 {
			cond = append(cond, kw("AND"), paren(clone(ret.conflictWhere)...))
		}
		return c.selectBack(ctx, ret, cond, args, ks.synth)
	case ret.upsert:
		target := ret.conflict
		if ret.constrain != "" {
			if target, err = c.indexColumns(ctx, ret.table, ret.constrain); err != nil {
				return nil, err
			}
		}
		if len(target) == 0 {
			return nil, untranslatable("upsert RETURNING without a conflict target")
		}
		if !sameColumns(target, meta.pk) || useLastID || generated {
			// An updated row keeps its key: find it by the conflict target.
			lk := &keySet{synth: ks.synth, keys: make([][]int, len(rows))}
			for _, t := range target {
				ci := indexOf(cols, t)
				if ci < 0 {
					return nil, untranslatable("upsert RETURNING: conflict column %s is not inserted", t)
				}
				for r := range rows {
					val, ok := resolveKey(rows[r][ci], args)
					if !ok {
						if n := rows[r][ci]; len(n) == 1 && n[0].param != nil && n[0].param.kind == paramSynth {
							lk.keys[r] = append(lk.keys[r], n[0].param.index)
							continue
						}
						return nil, untranslatable("upsert RETURNING: computed value for %s", t)
					}
					lk.keys[r] = append(lk.keys[r], lk.slot(val))
				}
			}
			ks, keyCols = lk, target
		}
		if len(ret.conflictWhere) > 0 {
			// The arbiter is a partial unique index: the key is unique only
			// among rows matching its predicate.
			cond := append(ks.cond(keyCols), kw("AND"), paren(clone(ret.conflictWhere)...))
			return c.selectBack(ctx, ret, cond, args, ks.synth)
		}
	case useLastID:
		res2, err := c.queryRaw(ctx, "SELECT LAST_INSERT_ID()", nil)
		if err != nil {
			return nil, err
		}
		_, data, err := readAll(res2, -1)
		if err != nil || len(data) == 0 {
			return nil, fmt.Errorf("reading LAST_INSERT_ID: %v", err)
		}
		first := asString(data[0][0])
		var base int64
		fmt.Sscan(first, &base)
		for r := range rows {
			ks.keys[r] = []int{ks.slot(base + int64(r))}
		}
		if ret.ignore && affected < int64(len(rows)) {
			return nil, untranslatable("INSERT IGNORE ... RETURNING with AUTO_INCREMENT keys")
		}
	case ret.ignore && !generated:
		// The caller chose the keys, so a skipped row's key matches the row
		// that was already there; only the row count can tell them apart.
		switch {
		case affected == 0:
			ks.keys = nil
		case affected != int64(len(rows)):
			return nil, untranslatable("multi-row INSERT ... ON CONFLICT DO NOTHING RETURNING with caller-chosen keys")
		}
	}
	return c.selectBack(ctx, ret, ks.cond(keyCols), args, ks.synth)
}

func sameColumns(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

// lockedKeys reads and locks the keys of the rows the statement's WHERE
// matches.
func (c *conn) lockedKeys(ctx context.Context, ret *returning, meta *tableMeta, args []driver.NamedValue) (*keySet, error) {
	var sel [][]node
	for _, p := range meta.pk {
		sel = append(sel, []node{quotedIdent(p)})
	}
	q := append([]node{kw("SELECT")}, joinTop(sel)...)
	q = append(q, kw("FROM"))
	q = append(q, ret.ref...)
	if ret.where != nil {
		q = append(q, kw("WHERE"))
		q = append(q, ret.where...)
	}
	q = append(q, kw("FOR"), kw("UPDATE"))
	rows, err := c.query(ctx, q, args, nil)
	if err != nil {
		return nil, err
	}
	_, data, err := readAll(rows, -1)
	if err != nil {
		return nil, err
	}
	ks := &keySet{}
	for _, r := range data {
		var key []int
		for _, v := range r {
			key = append(key, ks.slot(v))
		}
		ks.keys = append(ks.keys, key)
	}
	return ks, nil
}

func (c *conn) updateReturning(ctx context.Context, ret *returning, meta *tableMeta, args []driver.NamedValue) (driver.Rows, error) {
	ks, err := c.lockedKeys(ctx, ret, meta, args)
	if err != nil {
		return nil, err
	}
	if len(ks.keys) > 0 {
		stmt := append([]node{kw("UPDATE")}, ret.ref...)
		stmt = append(stmt, kw("SET"))
		stmt = append(stmt, ret.set...)
		stmt = append(stmt, kw("WHERE"))
		stmt = append(stmt, ks.cond(meta.pk)...)
		if _, err := c.exec(ctx, stmt, args, ks.synth); err != nil {
			return nil, err
		}
	}
	return c.selectBack(ctx, ret, ks.cond(meta.pk), args, ks.synth)
}

func (c *conn) deleteReturning(ctx context.Context, ret *returning, meta *tableMeta, args []driver.NamedValue) (driver.Rows, error) {
	q := append([]node{kw("SELECT")}, ret.exprs...)
	for _, p := range meta.pk {
		q = append(q, comma(), quotedIdent(p))
	}
	q = append(q, kw("FROM"))
	q = append(q, ret.ref...)
	if ret.where != nil {
		q = append(q, kw("WHERE"))
		q = append(q, ret.where...)
	}
	q = append(q, kw("FOR"), kw("UPDATE"))
	rows, err := c.query(ctx, q, args, nil)
	if err != nil {
		return nil, err
	}
	keep := len(rows.Columns()) - len(meta.pk)
	buf, full, err := readAll(rows, keep)
	if err != nil {
		return nil, err
	}
	if len(full) == 0 {
		return buf, nil
	}
	ks := &keySet{}
	for _, r := range full {
		var key []int
		for _, v := range r[keep:] {
			key = append(key, ks.slot(v))
		}
		ks.keys = append(ks.keys, key)
	}
	stmt := []node{kw("DELETE"), kw("FROM"), quotedIdent(ret.table), kw("WHERE")}
	stmt = append(stmt, ks.cond(meta.pk)...)
	if _, err := c.exec(ctx, stmt, args, ks.synth); err != nil {
		return nil, err
	}
	return buf, nil
}

// forgetTableMeta drops cached table metadata, for tests that recreate tables.
func forgetTableMeta() {
	metaCache.Range(func(k, _ any) bool { metaCache.Delete(k); return true })
}

// substituteColumns replaces references to inserted columns in expr with the
// values the row inserts for them.
func substituteColumns(expr []node, cols []string, row [][]node) ([]node, bool) {
	used := false
	var walk func(items []node) []node
	walk = func(items []node) []node {
		out := make([]node, 0, len(items))
		for i, n := range items {
			if n.group != nil {
				g := *n.group
				g.items = walk(g.items)
				out = append(out, node{group: &g})
				continue
			}
			if n.isIdentNode() && n.param == nil && !(i+1 < len(items) && items[i+1].isParen()) &&
				!(i > 0 && items[i-1].isPunct(".")) && !(i+1 < len(items) && items[i+1].isPunct(".")) {
				if ci := indexOf(cols, identName(n)); ci >= 0 && ci < len(row) {
					out = append(out, paren(clone(row[ci])...))
					used = true
					continue
				}
			}
			out = append(out, n)
		}
		return out
	}
	return walk(expr), used
}
