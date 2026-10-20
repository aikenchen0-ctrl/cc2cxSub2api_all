import argparse
import fcntl
import json
import os
import shutil
import sqlite3
import subprocess
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path

ROOT = Path('/opt/cc2cx-audit-validation')
CONTAINER = 'cc2cx-helicone-audit'
PERMANENT_USER = '11'
USER_FIELD = 'helicone-property-sub2api-user-id'
DAYS = 2
LIMIT = 2000
os.umask(0o077)


def run(args, data=None):
    result = subprocess.run(args, input=data, capture_output=True, text=True, timeout=180)
    if result.returncode:
        raise RuntimeError('审计存储操作失败，停止本轮清理；不会扩大删除范围')
    return result.stdout.strip()


def ch(sql):
    return run(['docker', 'exec', CONTAINER, 'clickhouse-client', '--query', sql])


def pg(sql):
    return run(['docker', 'exec', '-i', '-u', 'postgres', CONTAINER, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-d', 'helicone_test'], sql)


def pg_rows(sql):
    return json.loads(pg(f"SELECT COALESCE(json_agg(t),'[]'::json) FROM ({sql}) t;"))


def ch_rows(sql):
    return [json.loads(line) for line in ch(sql + ' FORMAT JSONEachRow').splitlines() if line]


def ids_sql(ids):
    return ','.join("'" + str(uuid.UUID(value)) + "'" for value in sorted(ids))


def protected_ids(org, ids):
    if not ids:
        return set()
    values = ids_sql(ids)
    protected = set()
    for table in ['request_response_rmt', 'request_response_versioned']:
        rows = ch_rows(f"SELECT DISTINCT toString(request_id) AS id FROM {table} WHERE organization_id='{org}' AND request_id IN ({values}) AND properties['{USER_FIELD}']='{PERMANENT_USER}'")
        protected.update(row['id'] for row in rows)
    rows = pg_rows(f"SELECT id::text FROM request WHERE helicone_org_id='{org}' AND id IN ({values}) AND properties->>'{USER_FIELD}'='{PERMANENT_USER}'")
    protected.update(row['id'] for row in rows)
    return protected


def candidates(org, cutoff):
    selected = set()
    stamp = cutoff.strftime('%Y-%m-%d %H:%M:%S')
    for table in ['request_response_rmt', 'request_response_versioned']:
        rows = ch_rows(f"SELECT toString(request_id) AS id FROM {table} WHERE organization_id='{org}' GROUP BY request_id HAVING max(request_created_at)<toDateTime('{stamp}','UTC') AND max(properties['{USER_FIELD}']='{PERMANENT_USER}')=0 ORDER BY max(request_created_at) LIMIT {LIMIT}")
        selected.update(row['id'] for row in rows)
    rows = pg_rows(f"SELECT id::text FROM request WHERE helicone_org_id='{org}' AND created_at<'{cutoff.isoformat()}' AND COALESCE(properties->>'{USER_FIELD}','')<>'{PERMANENT_USER}' ORDER BY created_at LIMIT {LIMIT}")
    selected.update(row['id'] for row in rows)
    return selected


def strip_body_ttl():
    # 由外部保留策略统一管理，解除原有三个月正文清空规则。
    for table in ['request_response_rmt', 'request_response_versioned']:
        for column in ch_rows(f'DESCRIBE TABLE {table}'):
            if column['name'] in ['request_body', 'response_body'] and column.get('ttl_expression'):
                ch(f"ALTER TABLE {table} MODIFY COLUMN {column['name']} REMOVE TTL")


def verify_schema():
    allowed = {'cache_hits', 'properties', 'feedback', 'request_job_task', 'job_node_request',
               'finetune_dataset_data', 'asset', 'prompt_input_record', 'experiment_output',
               'score_value', 'experiment_v2_hypothesis_run', 'response'}
    rows = pg_rows("SELECT c.relname AS name FROM pg_constraint f JOIN pg_class c ON c.oid=f.conrelid WHERE f.contype='f' AND f.confrelid IN('public.request'::regclass,'public.response'::regclass)")
    if any(row['name'] not in allowed for row in rows):
        raise RuntimeError('发现新增关联表，停止清理，需先复核升级后的结构')
    lifecycle = subprocess.run(['docker', 'exec', CONTAINER, 'mc', 'ilm', 'rule', 'ls', '--json', 'localminio/request-response-storage'], capture_output=True, text=True, timeout=20)
    if lifecycle.returncode == 0:
        raise RuntimeError('发现独立对象过期规则，停止清理，需先确认不会影响永久记录')


def delete_metadata(org, ids):
    values = ids_sql(ids)
    scoped = f"SELECT id FROM request WHERE helicone_org_id='{org}' AND id IN ({values})"
    statements = ['BEGIN;', "SET LOCAL lock_timeout='5s';", "SET LOCAL statement_timeout='30s';"]
    statements.append(f'DELETE FROM feedback WHERE response_id IN (SELECT id FROM response WHERE request IN ({scoped}));')
    for table, column in [('cache_hits', 'request_id'), ('properties', 'request_id'),
                          ('request_job_task', 'request_id'), ('job_node_request', 'request_id'),
                          ('prompt_input_record', 'source_request'), ('experiment_output', 'request_id'),
                          ('score_value', 'request_id')]:
        statements.append(f'DELETE FROM {table} WHERE {column} IN ({scoped});')
    statements += [f'DELETE FROM response WHERE request IN ({scoped});',
                   f'DELETE FROM request WHERE helicone_org_id=\'{org}\' AND id IN ({values});', 'COMMIT;']
    pg('\n'.join(statements))
    columns = ch_rows("SELECT table,groupArray(name) AS names FROM system.columns WHERE database='default' GROUP BY table HAVING has(names,'request_id')")
    for item in columns:
        table = item['table']
        if not table.replace('_', '').isalnum() or table.startswith('cc2_'):
            continue
        condition = f'request_id IN ({values})'
        if 'organization_id' in item['names']:
            condition += f" AND organization_id='{org}'"
        if int(ch(f'SELECT count(*) FROM {table} WHERE {condition}')):
            ch(f'ALTER TABLE {table} DELETE WHERE {condition} SETTINGS mutations_sync=1')


def clean_backups(cutoff):
    removed = []
    # 仅清理由本次部署产生的两份已知临时备份，不触及生产数据库备份。
    for name in ['audit-db-before-volume-fix.dump', 'postgres-volume-copy']:
        path = ROOT / name
        if path.exists() and not path.is_symlink() and path.stat().st_mtime < cutoff.timestamp():
            if path.resolve().parent != ROOT.resolve():
                raise RuntimeError('备份路径越界，停止清理')
            if path.is_dir():
                shutil.rmtree(path)
            else:
                path.unlink()
            removed.append(name)
    return removed


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--remove-default-ttl', action='store_true')
    args = parser.parse_args()
    lock = (ROOT / 'retention.lock').open('w')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    org = str(uuid.UUID(json.loads((ROOT / 'dashboard-credentials.json').read_text())['organization_id']))
    cutoff = datetime.now(timezone.utc) - timedelta(days=DAYS)
    verify_schema()
    if args.remove_default_ttl:
        if not args.apply:
            raise RuntimeError('移除默认过期规则需要同时指定 --apply')
        strip_body_ttl()
    ledger = sqlite3.connect(ROOT / 'retention-state.sqlite3')
    ledger.execute('CREATE TABLE IF NOT EXISTS pending (id TEXT PRIMARY KEY, organization TEXT NOT NULL)')
    queued = {row[0] for row in ledger.execute('SELECT id FROM pending WHERE organization=?', (org,))}
    selected = candidates(org, cutoff) | queued
    protected = protected_ids(org, selected)
    selected -= protected
    selected = set(sorted(selected)[:LIMIT])
    stats = {'time': datetime.now(timezone.utc).isoformat(), 'apply': args.apply, 'retention_days': DAYS,
             'permanent_user_id': PERMANENT_USER, 'candidates': len(selected), 'protected_from_candidates': len(protected), 'deleted': 0}
    if args.apply:
        for request_id in protected:
            ledger.execute('DELETE FROM pending WHERE id=?', (request_id,))
        for request_id in selected:
            ledger.execute('INSERT OR IGNORE INTO pending VALUES (?,?)', (request_id, org))
        ledger.commit()
        if selected:
            # 删除前再跨数据库校验，先留可恢复任务编号，保证中断后仍可继续清理。
            latest_protected = protected_ids(org, selected)
            selected -= latest_protected
            for request_id in latest_protected:
                ledger.execute('DELETE FROM pending WHERE id=?', (request_id,))
            ledger.commit()
        if selected:
            delete_metadata(org, selected)
            ordered = sorted(selected)
            # 每次仅传入已校验的精确请求目录，减少逐条启动容器命令的开销。
            for offset in range(0, len(ordered), 50):
                batch = ordered[offset:offset + 50]
                prefixes = [f'localminio/request-response-storage/organizations/{org}/requests/{uuid.UUID(request_id)}/' for request_id in batch]
                run(['docker', 'exec', CONTAINER, 'mc', 'rm', '--recursive', '--force', *prefixes])
                ledger.executemany('DELETE FROM pending WHERE id=?', [(request_id,) for request_id in batch])
                ledger.commit()
                stats['deleted'] += len(batch)
        stats['temporary_backups_removed'] = clean_backups(cutoff)
    stats['pending'] = ledger.execute('SELECT count(*) FROM pending').fetchone()[0]
    ledger.close()
    if args.apply:
        (ROOT / 'retention-status.json').write_text(json.dumps(stats, ensure_ascii=False))
    print(json.dumps(stats, ensure_ascii=False))


if __name__ == '__main__':
    main()
