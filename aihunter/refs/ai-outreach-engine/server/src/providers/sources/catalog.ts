/**
 * Company discovery adapters. Each pulls startups from a known directory/portfolio
 * (YC JSON directory, Techstars/EF/etc via search). "web" is open Serper search.
 */
export type SourceAdapterOption = {
  id: string;
  label: string;
  hint: string;
};

export const SOURCE_ADAPTER_OPTIONS: SourceAdapterOption[] = [
  {
    id: "yc",
    label: "Y Combinator",
    hint: "Public YC company directory — hiring + industry slices.",
  },
  {
    id: "techstars",
    label: "Techstars",
    hint: "Techstars portfolio companies via web search.",
  },
  {
    id: "500global",
    label: "500 Global",
    hint: "500 Global / 500 Startups portfolio companies.",
  },
  {
    id: "antler",
    label: "Antler",
    hint: "Antler portfolio companies.",
  },
  {
    id: "ef",
    label: "Entrepreneur First",
    hint: "EF / JoinEF portfolio companies.",
  },
  {
    id: "surge",
    label: "Surge",
    hint: "Sequoia Surge portfolio companies.",
  },
  {
    id: "producthunt",
    label: "Product Hunt",
    hint: "Recent AI/SaaS launches on Product Hunt.",
  },
  {
    id: "web",
    label: "Open web",
    hint: "Serper Google search using audience queries + markets/signals.",
  },
];

export const DEFAULT_SOURCE_ADAPTERS = SOURCE_ADAPTER_OPTIONS.map((option) => option.id);

export const STRUCTURED_SOURCE_IDS = SOURCE_ADAPTER_OPTIONS.map((o) => o.id).filter(
  (id) => id !== "web",
);
