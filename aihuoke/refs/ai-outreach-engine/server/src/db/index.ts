import { Database } from "bun:sqlite";
import { dirname, resolve } from "node:path";
import { mkdirSync, readFileSync } from "node:fs";
import { env } from "../env";

const dbPath = resolve(process.cwd(), env.databasePath);
mkdirSync(dirname(dbPath), { recursive: true });

export const db = new Database(dbPath, { create: true });

export function migrate(): void {
  const schema = readFileSync(resolve(import.meta.dir, "schema.sql"), "utf8");
  db.exec(schema);
  ensureColumn("audiences", "template_subject", "TEXT NOT NULL DEFAULT ''");
  ensureColumn("audiences", "template_body", "TEXT NOT NULL DEFAULT ''");
  ensureColumn("audiences", "template_html", "TEXT NOT NULL DEFAULT ''");
  ensureColumn("audiences", "next_search_page", "INTEGER NOT NULL DEFAULT 1");
  ensureColumn("audiences", "source_adapters", "TEXT NOT NULL DEFAULT '[]'");
  ensureColumn("audiences", "funded_after_year", "INTEGER NOT NULL DEFAULT 0");
  ensureColumn("audiences", "target_roles", "TEXT NOT NULL DEFAULT '[]'");
  ensureColumn("jobs", "dismissed_at", "TEXT");
  ensureColumn("projects", "email_format", "TEXT NOT NULL DEFAULT 'html'");
  ensureColumn("projects", "workflow_type", "TEXT NOT NULL DEFAULT 'customer_outreach'");
  ensureColumn("projects", "workflow_data", "TEXT NOT NULL DEFAULT '{}'");
  ensureColumn("messages", "html_body", "TEXT NOT NULL DEFAULT ''");
  ensureColumn("messages", "email_format", "TEXT NOT NULL DEFAULT ''");
  ensureColumn("messages", "sent_text_body", "TEXT NOT NULL DEFAULT ''");
  ensureColumn("messages", "sent_html_body", "TEXT NOT NULL DEFAULT ''");
  ensureColumn("app_settings", "include_unsubscribe_link", "INTEGER NOT NULL DEFAULT 1");
}

function ensureColumn(table: string, column: string, definition: string): void {
  const existing = db
    .query<{ name: string }, []>(`PRAGMA table_info(${table})`)
    .all()
    .some((row) => row.name === column);
  if (!existing) db.exec(`ALTER TABLE ${table} ADD COLUMN ${column} ${definition}`);
}

migrate();
