# 手动应用官方迁移 SQL 操作手册

> 适用场景:每次从官方 Tencent/WeKnora 同步更新后,部署到服务器前需要手工执行的数据库迁移步骤。
> 最后更新:2026-09-20(本次需应用 000096–000107)

---

## 一、为什么需要"手动"应用

WeKnora 启动时由 `golang-migrate` 自动执行 `migrations/versioned/` 下的迁移,但它**只执行版本号大于当前水位的迁移**。

我们的 fork 把自己的迁移重编号到了 `000900–000903`(语义建模 / 自动化模块),数据库水位因此被顶到 **903**。官方后来新增的迁移编号(000096、000097……)永远小于 903,`golang-migrate` **不会再自动执行它们**——所以每次官方同步,新增的低编号迁移都要用 psql 手动灌进数据库。

因此三条铁律:

1. **按版本号从小到大逐个执行**,不能乱序;
2. **不要动 `schema_migrations` 表**——它是单行水位设计(一行 903),手动执行过的 SQL 不登记,加了反而会破坏 golang-migrate;
3. 官方迁移全部是幂等 SQL(`IF NOT EXISTS` / `IF EXISTS` / 守卫 DO 块),**重复执行无害**,中断后重跑即可。

## 二、标准操作步骤

以下命令假设在服务器仓库根目录执行,compose 组合与日常启动一致。

```bash
DC="docker compose -f docker-compose.yml -f docker-compose.cube.yml -f docker-compose.override.yml \
  --env-file .env --env-file .env.local --env-file .env.docker"
```

### 第 1 步:拉代码 + 备份

```bash
git pull origin feat/jett-0.8.0

# 备份(出错可回退,强烈建议)
$DC exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > backup_$(date +%F).sql
```

### 第 2 步:升级 postgres 镜像(如有)

部分迁移依赖新版扩展(如 `000099` 要求 pg_search ≥ 0.22.6),**必须先升镜像再灌 SQL**。
仓库 `docker-compose.yml` 已固定为 `paradedb/paradedb:v0.22.6-pg17`,数据在卷里,重建容器不丢数据:

```bash
$DC images postgres   # 确认镜像已是 v0.22.6-pg17;若 override/env 钉了旧镜像,先改掉
$DC up -d postgres
```

### 第 3 步:按序手动执行待补的 .up.sql

把 `NNN` 换成本次待补的版本号区间(见"三、本次清单"):

```bash
for n in {096..107}; do
  f=$(ls migrations/versioned/000${n}_*.up.sql)
  echo "== applying $f"
  $DC exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < "$f" \
    || { echo "!!! FAILED: $f"; break; }
done
```

说明:

- 账号密码不用填,`POSTGRES_USER` / `POSTGRES_DB` 是容器内现成的环境变量;
- `ON_ERROR_STOP=1` 保证出错立刻停,不会带病往下跑;
- 循环用 `{096..107}` 这种补零区间写法,bash 会自动展开成 096→107。

### 第 4 步:验证 → 重建应用

```bash
# 验证(按本次迁移内容挑检查项,下方清单里每批给了现成 SQL)
$DC exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
SELECT to_regclass('mcp_endpoints') IS NOT NULL            AS has_mcp_endpoints,
       to_regclass('fork_snapshot_leases') IS NOT NULL     AS has_fork_leases,
       EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name='message_artifacts' AND column_name='deleted_at') AS has_deleted_at;
SQL
# 期望输出: t | t | t

# 新代码依赖新列/新表,所以必须 SQL 全部落地后再重建应用
$DC build frontend app
$DC up -d
```

**顺序要点:先升镜像 → 再灌 SQL → 最后重建应用。**
旧版后端在加列/建表期间继续跑没有问题(全部在线 DDL);反过来先上新代码会报"列不存在"。

## 三、本次清单(2026-09-20 同步,共 12 个)

| 版本 | 文件 | 内容 |
|------|------|------|
| 000096 | dingtalk_stream_only | 钉钉回调 webhook → websocket(纯 UPDATE) |
| 000097 | session_fork | 会话 forks 列 + 索引 |
| 000098 | fork_snapshot_lease | fork_snapshot_leases 表 |
| 000099 | pg_search_0226 | pg_search 升 0.22.6(依赖第 2 步的新镜像) |
| 000100 | chunks_index_diet | 删除 3 个 chunks 冗余索引 |
| 000101 | knowledge_profiles | knowledge profiles JSONB 列 |
| 000102 | mcp_endpoints | mcp_endpoints 表 |
| 000103 | message_artifacts_table | message_artifacts 表 + 回填 |
| 000104 | skill_served_version | tenant_skills.served |
| 000105 | message_context_checkpoint | messages.context_checkpoint |
| 000106 | messages_session_created_index | messages (session_id, created_at) 索引 |
| 000107 | message_artifacts_deleted_at | message_artifacts.deleted_at 软删列 + 2 个部分索引 |

## 四、以后官方同步怎么做

1. `git pull` 后看官方新增了哪些迁移:
   ```bash
   ls migrations/versioned/ | grep -vE "0009[0-9][0-9]" | sort | tail
   ```
   新增的、编号小于 900 的,就是要手动补的(900–903 是我们自己的,自动执行)。
2. 把第 3 步循环的区间改成新范围(如 `{108..112}`),其余步骤完全一样;
3. 开发库(本机 WeKnora-postgres-dev)同样要手动执行一遍这套 SQL;
4. 有疑问先看 `migrations/versioned/NNNNNN_*.up.sql` 开头的注释,官方每个迁移都写清楚了用途和风险。

## 五、排障

- **某一步报错停止**:错误信息会带具体 SQL 语句;修复后**从报错那个文件起重跑**(幂等,已执行过的等于空操作)。
- **000099 报 extension 版本不对**:说明第 2 步的镜像没升成功,回 `$DC images postgres` 检查。
- **启动后功能报"列不存在"**:大概率是 SQL 没灌完就重建了应用,补跑第 3 步后 `$DC up -d` 重启即可。
- 其他数据库问题参考 `docs/migration-troubleshooting.md`(golang-migrate 启动失败排障)。
