# MCP SQL 独立只读账号

`sra_sql_query` 不再使用应用数据库所有者。未配置 `SRA_SQL_DATABASE_URL` 时，仅这个自由 SQL 工具不可用；其他语义查询工具照常工作。

在完成应用迁移后，数据库管理员可创建独立登录账号：

```sql
CREATE ROLE sra_reader LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
GRANT CONNECT ON DATABASE social_relationship_analysis TO sra_reader;
GRANT USAGE ON SCHEMA public TO sra_reader;
GRANT SELECT ON persons, person_identifiers, person_profiles, groups,
  group_memberships, conversations, messages, contents, relation_events,
  media_assets, media_references, message_media TO sra_reader;
ALTER ROLE sra_reader SET default_transaction_read_only = on;
```

使用 psql 的 `\password sra_reader` 交互设置随机密码。将该连接写入受保护的 `backend/.env`：

```dotenv
SRA_SQL_DATABASE_URL=postgres://sra_reader:REPLACE_PASSWORD@127.0.0.1:5432/social_relationship_analysis?sslmode=disable
```

不要授予数据库所有者、超级用户、`pg_read_all_data`、凭据表或高权限角色成员资格。需要额外数据时逐表审查后授权，不要使用“所有表”授权。角色不能读的对象即使通过不同 SQL 编码表示也无法读取。

程序在启动时检查高权限成员资格及凭据表权限，并在每次 SQL 查询中强制只读事务、10 秒超时和最多 200 行。数据库管理员后续更改授权会改变边界，需同步管理。原始请求记录可能含登录上下文，默认不向自由 SQL 账号授权 `raw_records`。
