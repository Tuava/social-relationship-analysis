package mcp

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

// OpenSQLReadPool requires a separate, unprivileged login. Regex checks are
// only input hygiene; PostgreSQL grants are the actual data-access boundary.
func OpenSQLReadPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, nil
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid SQL reader configuration")
	}
	fail := func(reason string) (*pgxpool.Pool, error) {
		pool.Close()
		return nil, fmt.Errorf("SQL reader: %s", reason)
	}
	var privileged bool
	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE
   (rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls)
   AND pg_has_role(current_user,oid,'MEMBER'))`).Scan(&privileged)
	if err != nil {
		return fail("could not validate role privileges")
	}
	if privileged {
		return fail("use a dedicated login without privileged role memberships")
	}
	for _, table := range []string{"app_users", "system_configs", "bot_instances", "napcat_accounts", "qzone_connections", "source_connections"} {
		var readable bool
		err = pool.QueryRow(ctx, `SELECT COALESCE(has_table_privilege(current_user,to_regclass($1),'SELECT'),false)`, "public."+table).Scan(&readable)
		if err != nil {
			return fail("could not validate relation grants")
		}
		if readable {
			return fail("credentials/configuration tables must not be readable")
		}
	}
	return pool, nil
}
