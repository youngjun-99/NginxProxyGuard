package repository

import (
	"context"
	"fmt"
	"strings"

	"nginx-proxy-guard/internal/database/dialect"
	"nginx-proxy-guard/internal/model"
)

// proxyHostDomainBucketExpr is the one definition of a host's "parent domain"
// bucket. The /groups aggregate and the ?domain= list filter must use the same
// expression — if they drift, clicking a group returns zero rows.
//
//	app.example.com  (3+ labels)      -> example.com     (strip the first label)
//	example.com      (exactly 2)      -> example.com     (an apex host is its own
//	                                                      bucket; stripping
//	                                                      unconditionally filed it
//	                                                      under "com")
//	listener / "" / NULL (0-1 labels) -> NULL            (a stream listener name or
//	                                                      a domainless row has no
//	                                                      parent domain; NULL is
//	                                                      excluded from the
//	                                                      aggregate and never
//	                                                      matches the filter)
//
// NULL matters twice: the aggregate scans into a string, so a NULL bucket would
// fail with "converting NULL to string is unsupported" and turn GET
// /proxy-hosts/groups into a 500 for every caller.
//
//npg:pg-only
const proxyHostDomainBucketExpr = `CASE WHEN cardinality(string_to_array(domain_names[1], '.')) > 2 THEN regexp_replace(domain_names[1], '^[^.]+\.', '') WHEN cardinality(string_to_array(domain_names[1], '.')) = 2 THEN domain_names[1] END`

// proxyHostDomainBucketExprMySQL is proxyHostDomainBucketExpr on MySQL, where
// domain_names is the lib/pq array literal: its first element read through
// the array's JSON view, the labels counted by the dots, and the first label
// cut the way the regexp cuts it (a name starting with "." is left alone, as
// '^[^.]+\.' does not match it). It uses no PostgreSQL syntax, so it is valid
// both in raw MySQL and inside a statement the translator rewrites.
var proxyHostDomainBucketExprMySQL = strings.NewReplacer("{f}",
	"JSON_UNQUOTE(JSON_EXTRACT("+mysqlArrayJSON("domain_names")+", '$[0]'))").Replace(
	`CASE WHEN CHAR_LENGTH({f}) - CHAR_LENGTH(REPLACE({f}, '.', '')) + 1 > 2 THEN (CASE WHEN LEFT({f}, 1) <> '.' THEN SUBSTRING({f}, LOCATE('.', {f}) + 1) ELSE {f} END) WHEN CHAR_LENGTH({f}) - CHAR_LENGTH(REPLACE({f}, '.', '')) + 1 = 2 THEN {f} END`)

// proxyHostDomainBucketExprActive is the bucket expression for the active
// engine.
func proxyHostDomainBucketExprActive() string {
	if dialect.IsMySQL() {
		return proxyHostDomainBucketExprMySQL
	}
	return proxyHostDomainBucketExpr
}

// proxyHostDomainGroupQuery buckets every host by parent domain, dropping the
// rows that have none. Package-level so the DB-backed test runs this exact
// statement.
var proxyHostDomainGroupQuery = fmt.Sprintf(
	`SELECT bucket, COUNT(*) FROM (SELECT %s AS bucket FROM proxy_hosts) b WHERE bucket IS NOT NULL AND bucket <> '' GROUP BY 1 ORDER BY 2 DESC, 1 ASC`,
	proxyHostDomainBucketExpr,
)

// Groups returns the buckets the list's filter panel is drawn from. Four
// small aggregates; tag and domain cardinality is bounded by the host count,
// so there is no pagination. Slices start non-nil so the JSON is [] not null.
func (r *ProxyHostRepository) Groups(ctx context.Context) (*model.ProxyHostGroups, error) {
	out := &model.ProxyHostGroups{
		Tags:      []model.ProxyHostGroupCount{},
		Domains:   []model.ProxyHostGroupCount{},
		Upstreams: []model.ProxyHostGroupCount{},
	}
	//npg:pg-only
	tagQuery, domainQuery := `SELECT t, COUNT(*) FROM proxy_hosts, unnest(tags) AS t GROUP BY t ORDER BY 2 DESC, 1 ASC`, proxyHostDomainGroupQuery
	if dialect.IsMySQL() {
		// No unnest: JSON_TABLE over the tags array's JSON view.
		tagQuery = dialect.Raw(`SELECT n.t, COUNT(*) FROM proxy_hosts p CROSS JOIN JSON_TABLE(` +
			mysqlArrayJSON("p.tags") + `, '$[*]' COLUMNS (t VARCHAR(255) PATH '$')) AS n GROUP BY n.t ORDER BY 2 DESC, 1 ASC`)
		domainQuery = dialect.Raw(fmt.Sprintf(
			`SELECT bucket, COUNT(*) FROM (SELECT %s AS bucket FROM proxy_hosts) b WHERE bucket IS NOT NULL AND bucket <> '' GROUP BY 1 ORDER BY 2 DESC, 1 ASC`,
			proxyHostDomainBucketExprMySQL))
	}
	if err := r.groupCounts(ctx, tagQuery, &out.Tags); err != nil {
		return nil, fmt.Errorf("group by tag: %w", err)
	}
	if err := r.groupCounts(ctx, domainQuery, &out.Domains); err != nil {
		return nil, fmt.Errorf("group by domain: %w", err)
	}
	if err := r.groupCounts(ctx, `SELECT forward_host, COUNT(*) FROM proxy_hosts GROUP BY 1 ORDER BY 2 DESC, 1 ASC`, &out.Upstreams); err != nil {
		return nil, fmt.Errorf("group by upstream: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE enabled), COUNT(*) FILTER (WHERE NOT enabled) FROM proxy_hosts`).
		Scan(&out.Status.Enabled, &out.Status.Disabled); err != nil {
		return nil, fmt.Errorf("group by status: %w", err)
	}
	return out, nil
}

func (r *ProxyHostRepository) groupCounts(ctx context.Context, query string, dst *[]model.ProxyHostGroupCount) error {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c model.ProxyHostGroupCount
		if err := rows.Scan(&c.Name, &c.Count); err != nil {
			return err
		}
		*dst = append(*dst, c)
	}
	return rows.Err()
}
