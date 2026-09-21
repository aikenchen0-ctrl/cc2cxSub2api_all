import { env } from "../env";

export type TemplateContext = {
  firstName: string;
  fullName: string;
  title: string;
  company: string;
  companyDomain: string;
  observation: string;
  painPoint: string;
  valueProp: string;
  senderFirstName: string;
  senderFullName: string;
  /** Jobs: role title from the listing. */
  role: string;
  /** Jobs: tech stack from the JD. */
  techStack: string;
  /** Jobs: what the company/product focuses on. */
  companyFocus: string;
  /** Jobs: Remote / Hybrid / On-site / "". */
  workStyle: string;
};

/** Merge fields allowed in job-search lane templates. */
export const JOB_LANE_MERGE_FIELDS = [
  "firstName",
  "company",
  "senderFullName",
  "role",
  "techStack",
  "companyFocus",
  "workStyle",
] as const;

function senderFirstName(): string {
  return env.sender.fromName.split(/\s+/)[0] || "Rohit";
}

function senderFullName(): string {
  return env.sender.fromName.trim() || "Rohit";
}

export function contextFromParts(args: {
  fullName: string;
  title: string;
  company: string;
  companyDomain: string;
  companyDescription: string;
  painPoint: string;
  valueProp: string;
  role?: string;
  techStack?: string;
  companyFocus?: string;
  workStyle?: string;
}): TemplateContext {
  const firstName = args.fullName.split(/\s+/)[0] || "there";
  return {
    firstName,
    fullName: args.fullName,
    title: args.title,
    company: args.company,
    companyDomain: args.companyDomain,
    observation:
      args.companyDescription ||
      `${args.company} looks relevant for this outreach category.`,
    painPoint: args.painPoint || "keep track of important details during calls",
    valueProp: args.valueProp || "turn meetings into useful notes and live context",
    senderFirstName: senderFirstName(),
    senderFullName: senderFullName(),
    role: args.role ?? "",
    techStack: args.techStack ?? "",
    companyFocus: args.companyFocus ?? "",
    workStyle: args.workStyle ?? "",
  };
}

export function renderTemplate(template: string, context: TemplateContext): string {
  return template.replace(/\{\{\s*([a-zA-Z0-9_]+)\s*\}\}/g, (_match, key: keyof TemplateContext) => {
    const value = context[key];
    return typeof value === "string" ? value : "";
  });
}
