import { useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  BadgeCheck,
  Download,
  Loader2,
  Mail,
  ShieldAlert,
  Users,
} from "lucide-react";
import { api, LicensedFinderLead } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/i18n/I18nProvider";

const INSTANTLY_CSV_COLUMNS = [
  "email", "first_name", "last_name", "company", "title",
  "website", "linkedin_url", "reason", "lead_id", "qualified_at",
] as const;

function splitList(raw: string): string[] {
  return raw
    .split(/[\n,;]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function csvEscape(value: string): string {
  return /[",\n\r]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
}

function leadEmail(lead: LicensedFinderLead): string {
  if (typeof lead.email === "string" && lead.email.includes("@")) return lead.email;
  const emails = Array.isArray(lead.emails) ? lead.emails : [];
  const first = emails.find((item) => typeof item === "string" && item.includes("@"));
  return first ? String(first) : "";
}

function leadCompany(lead: LicensedFinderLead): string {
  return String(lead.company_name || lead.company || "");
}

function leadToCsvRow(lead: LicensedFinderLead): Record<(typeof INSTANTLY_CSV_COLUMNS)[number], string> {
  return {
    email: leadEmail(lead),
    first_name: String(lead.first_name || ""),
    last_name: String(lead.last_name || ""),
    company: leadCompany(lead),
    title: String(lead.title || ""),
    website: String(lead.website || ""),
    linkedin_url: String(lead.linkedin_url || ""),
    reason: String(lead.reason || ""),
    lead_id: String(lead.lead_id || ""),
    qualified_at: String(lead.qualified_at || ""),
  };
}

function buildInstantlyCsv(leads: LicensedFinderLead[]): string {
  const rows = leads.map((lead) =>
    INSTANTLY_CSV_COLUMNS.map((col) => csvEscape(leadToCsvRow(lead)[col])).join(",")
  );
  return [INSTANTLY_CSV_COLUMNS.join(","), ...rows].join("\n");
}

function triggerDownload(text: string, filename: string) {
  const blob = new Blob([text], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function Field({
  id, label, hint, value, onChange, placeholder,
}: {
  id: string;
  label: string;
  hint?: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
      />
    </div>
  );
}

export function LicensedFinderPage() {
  const t = useT();
  const [domains, setDomains] = useState("");
  const [industries, setIndustries] = useState("");
  const [titles, setTitles] = useState("");
  const [seniorities, setSeniorities] = useState("");
  const [countries, setCountries] = useState("");
  const [technologies, setTechnologies] = useState("");
  const [reason, setReason] = useState("");
  const [maxLeads, setMaxLeads] = useState(10);
  const [enrichEmails, setEnrichEmails] = useState(false);
  const [enrichPhones, setEnrichPhones] = useState(false);
  const [leads, setLeads] = useState<LicensedFinderLead[]>([]);
  const [lastMeta, setLastMeta] = useState<{
    request_id?: string;
    credits_consumed?: number;
    credits_left?: number | null;
    leads_found?: number;
    raw_status?: string;
    mode: "search" | "enrich";
  } | null>(null);

  const statusQuery = useQuery({
    queryKey: ["licensed-finder-status"],
    queryFn: api.getLicensedFinderStatus,
    retry: false,
  });

  const searchMutation = useMutation({
    mutationFn: api.searchLicensedFinder,
    onSuccess: (result) => {
      setLeads(result.leads || []);
      setLastMeta({
        request_id: result.request_id,
        credits_consumed: result.credits_consumed,
        credits_left: result.credits_left,
        leads_found: result.leads_found ?? result.leads?.length,
        raw_status: result.raw_status,
        mode: "search",
      });
    },
  });

  const enrichMutation = useMutation({
    mutationFn: api.enrichLicensedFinder,
    onSuccess: (result) => {
      setLeads(result.leads || []);
      setLastMeta({
        request_id: result.request_id,
        credits_consumed: result.credits_consumed,
        credits_left: result.credits_left,
        leads_found: result.leads?.length,
        raw_status: result.raw_status,
        mode: "enrich",
      });
    },
  });

  const status = statusQuery.data;
  const ready = Boolean(status?.enabled && status?.has_api_key);
  const busy = searchMutation.isPending || enrichMutation.isPending;
  const emailCount = useMemo(
    () => leads.filter((lead) => Boolean(leadEmail(lead))).length,
    [leads],
  );

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    searchMutation.mutate({
      company_domains: splitList(domains),
      industries: splitList(industries),
      job_titles: splitList(titles),
      seniorities: splitList(seniorities),
      countries: splitList(countries),
      technologies: splitList(technologies),
      max_leads: Math.max(1, Math.min(200, Number(maxLeads) || 10)),
      enrich_emails: enrichEmails,
      enrich_phones: enrichPhones,
      reason: reason.trim(),
    });
  };

  const handleEnrich = () => {
    if (!leads.length) return;
    enrichMutation.mutate({
      leads: leads as Record<string, unknown>[],
      enrich_emails: true,
      enrich_phones: false,
      verify_catch_all: false,
    });
  };

  const handleExport = () => {
    if (!leads.length) return;
    triggerDownload(buildInstantlyCsv(leads), "licensed-finder-leads.csv");
  };

  return (
    <div className="space-y-6 max-w-6xl">
      <div>
        <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
          <Users className="h-6 w-6 text-primary" />
          持牌找人旁路
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          不进主 hunt 图。描述筛选条件，走 BetterContact 持牌源。搜索默认不买邮箱；CSV 仍是 Instantly/Smartlead 10 列。
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">就绪状态</CardTitle>
          <CardDescription>只读探测，不花 credits。开关和 Key 在系统设置里。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {statusQuery.isLoading ? (
            <p className="text-sm text-muted-foreground flex items-center gap-2">
              <Loader2 className="h-4 w-4 animate-spin" /> 正在读取状态…
            </p>
          ) : statusQuery.isError ? (
            <p className="text-sm text-destructive">
              {statusQuery.error instanceof Error ? statusQuery.error.message : "无法读取状态"}
            </p>
          ) : (
            <>
              <div className="flex flex-wrap gap-2">
                <Badge variant={status?.enabled ? "success" : "secondary"}>
                  {status?.enabled ? "已开启" : "默认关闭"}
                </Badge>
                <Badge variant={status?.has_api_key ? "success" : "warning"}>
                  {status?.has_api_key ? "已配置 API Key" : "缺少 BetterContact Key"}
                </Badge>
                <Badge variant="outline">不进主图</Badge>
              </div>
              {!ready && (
                <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">
                  <ShieldAlert className="h-4 w-4 mt-0.5 flex-shrink-0" />
                  <div>
                    <p>旁路还没就绪：先到设置页打开开关并填 Key。</p>
                    <Link to="/settings" className="text-primary underline underline-offset-2">
                      去系统设置
                    </Link>
                  </div>
                </div>
              )}
              {status?.next_action && (
                <p className="text-xs text-muted-foreground font-mono">{status.next_action}</p>
              )}
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{t("finder.search")}</CardTitle>
          <CardDescription>
            {t("finder.searchHint")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSearch} className="space-y-4">
            <div className="grid gap-4 md:grid-cols-2">
              <Field id="lf-domains" label="公司域名" hint="例如 acme.com" value={domains} onChange={setDomains} placeholder="acme.com, tools.de" />
              <Field id="lf-industries" label="行业" value={industries} onChange={setIndustries} placeholder="Industrial machinery" />
              <Field id="lf-titles" label="职位" value={titles} onChange={setTitles} placeholder="Purchasing Manager, Buyer" />
              <Field id="lf-seniorities" label="职级" value={seniorities} onChange={setSeniorities} placeholder="Director, Manager" />
              <Field id="lf-countries" label="国家 / 地区" value={countries} onChange={setCountries} placeholder="Germany, Netherlands" />
              <Field id="lf-tech" label="技术栈" value={technologies} onChange={setTechnologies} placeholder="Shopify, SAP" />
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="lf-max">条数上限</Label>
                <Input
                  id="lf-max"
                  type="number"
                  min={1}
                  max={200}
                  value={maxLeads}
                  onChange={(e) => setMaxLeads(Number(e.target.value))}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="lf-reason">资格理由（写入 CSV reason）</Label>
                <Input
                  id="lf-reason"
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder="可选。空则后端按职位/公司补一句。"
                />
              </div>
            </div>
            <div className="rounded-md border px-3 py-3 space-y-2">
              <p className="text-sm font-medium">花费门</p>
              <label className="flex items-start gap-2 text-sm">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={enrichEmails}
                  onChange={(e) => setEnrichEmails(e.target.checked)}
                />
                <span>
                  搜索时同时买邮箱（enrich_emails）
                  <span className="block text-xs text-muted-foreground">默认关闭。只有明确要地址时才勾。</span>
                </span>
              </label>
              <label className="flex items-start gap-2 text-sm">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={enrichPhones}
                  onChange={(e) => setEnrichPhones(e.target.checked)}
                />
                <span>
                  搜索时同时买电话
                  <span className="block text-xs text-muted-foreground">默认关闭。</span>
                </span>
              </label>
            </div>
            {searchMutation.isError && (
              <p className="text-sm text-destructive">
                {searchMutation.error instanceof Error ? searchMutation.error.message : "搜索失败"}
              </p>
            )}
            <Button type="submit" disabled={!ready || busy}>
              {searchMutation.isPending ? (
                <><Loader2 className="h-4 w-4 me-2 animate-spin" />{t("finder.searching")}</>
              ) : (
                t("finder.searchBtn")
              )}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4">
          <div>
            <CardTitle className="text-lg">结果</CardTitle>
            <CardDescription>
              {leads.length
                ? `${leads.length} 条，其中 ${emailCount} 条已有邮箱。导出为 Instantly 10 列（含 reason，无 score）。`
                : "还没有结果。搜完会显示在这里。"}
            </CardDescription>
            {lastMeta && (
              <p className="text-xs text-muted-foreground mt-1">
                {lastMeta.mode === "enrich" ? "enrich" : "search"}
                {typeof lastMeta.credits_consumed === "number" ? ` · 消耗 ${lastMeta.credits_consumed}` : ""}
                {typeof lastMeta.credits_left === "number" ? ` · 剩余 ${lastMeta.credits_left}` : ""}
                {lastMeta.request_id ? ` · ${lastMeta.request_id}` : ""}
              </p>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleEnrich}
              disabled={!ready || busy || leads.length === 0}
            >
              {enrichMutation.isPending ? (
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              ) : (
                <Mail className="h-4 w-4 mr-2" />
              )}
              补邮箱（花钱）
            </Button>
            <Button variant="outline" size="sm" onClick={handleExport} disabled={!leads.length}>
              <Download className="h-4 w-4 mr-2" />
              导出 CSV
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {enrichMutation.isError && (
            <p className="text-sm text-destructive mb-3">
              {enrichMutation.error instanceof Error ? enrichMutation.error.message : t("finder.enrichFail")}
            </p>
          )}
          {leads.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">暂无线索。</p>
          ) : (
            <div className="rounded-md border overflow-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="h-10 px-3 text-start font-medium">{t("finder.name")}</th>
                    <th className="h-10 px-3 text-start font-medium">{t("finder.company")}</th>
                    <th className="h-10 px-3 text-start font-medium hidden md:table-cell">{t("finder.titles")}</th>
                    <th className="h-10 px-3 text-start font-medium">{t("finder.email")}</th>
                    <th className="h-10 px-3 text-start font-medium hidden lg:table-cell">{t("finder.reasonCol")}</th>
                  </tr>
                </thead>
                <tbody>
                  {leads.map((lead, i) => {
                    const name = [lead.first_name, lead.last_name].filter(Boolean).join(" ") || "—";
                    const email = leadEmail(lead);
                    return (
                      <tr key={String(lead.lead_id || i)} className="border-b">
                        <td className="p-3">
                          <div className="font-medium">{name}</div>
                          {lead.linkedin_url ? (
                            <a href={String(lead.linkedin_url)} target="_blank" rel="noopener noreferrer" className="text-xs text-primary hover:underline">
                              LinkedIn
                            </a>
                          ) : null}
                        </td>
                        <td className="p-3">
                          <div className="truncate max-w-[180px]">{leadCompany(lead) || "—"}</div>
                          {lead.website ? (
                            <div className="text-xs text-muted-foreground truncate max-w-[180px]">
                              {String(lead.website).replace(/^https?:\/\/(www\.)?/, "")}
                            </div>
                          ) : null}
                        </td>
                        <td className="p-3 hidden md:table-cell text-muted-foreground">{String(lead.title || "—")}</td>
                        <td className="p-3">
                          {email ? (
                            <span className="inline-flex items-center gap-1 text-xs">
                              <BadgeCheck className="h-3 w-3 text-green-700" />
                              <span className="truncate max-w-[180px]">{email}</span>
                            </span>
                          ) : (
                            <span className="text-xs text-muted-foreground">空邮箱仍是线索</span>
                          )}
                        </td>
                        <td className="p-3 hidden lg:table-cell">
                          <span className="text-xs text-muted-foreground line-clamp-2 max-w-[280px] block">
                            {String(lead.reason || "—")}
                          </span>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
