export function rescanWhatHappens(alsoRegenerate: boolean, preserveProspecting: boolean) {
  if (!alsoRegenerate) {
    return {
      will: [
        "Re-crawl the website and refresh product analysis",
        "Update competitors and features from the latest pages",
      ],
      willNot: [
        "Change audiences",
        "Find or regenerate companies / contacts",
        "Touch drafts or sent emails",
      ],
    };
  }
  if (preserveProspecting) {
    return {
      will: [
        "Re-crawl the website and refresh product analysis",
        "Invent a new set of AI audiences from that analysis",
        "Keep old AI audiences that already have companies/contacts (disabled as “kept”)",
        "Replace empty AI audience suggestions that have no contacts yet",
      ],
      willNot: [
        "Delete companies, contacts, drafts, or sent emails",
        "Re-find contacts automatically — you still click “Find next batch” per audience",
        "Change audiences you added manually",
      ],
    };
  }
  return {
    will: [
      "Re-crawl the website and refresh product analysis",
      "Delete AI-generated audiences and their companies, contacts, drafts, and sent emails",
      "Invent a fresh set of AI audiences",
    ],
    willNot: [
      "Delete audiences you added manually",
      "Re-find contacts automatically — you still click “Find next batch” per audience",
    ],
  };
}

export function regenWhatHappens(preserveProspecting: boolean) {
  if (preserveProspecting) {
    return {
      will: [
        "Invent a new set of AI audiences from the current site analysis (no re-scan)",
        "Keep old AI audiences that already have companies/contacts (disabled as “kept”)",
        "Replace empty AI audience suggestions that have no contacts yet",
      ],
      willNot: [
        "Re-crawl or change the website analysis",
        "Delete companies, contacts, drafts, or sent emails",
        "Re-find contacts automatically — you still click “Find next batch” per audience",
        "Change audiences you added manually",
      ],
    };
  }
  return {
    will: [
      "Delete AI-generated audiences and their companies, contacts, drafts, and sent emails",
      "Invent a fresh set of AI audiences from the current site analysis",
    ],
    willNot: [
      "Re-crawl or change the website analysis",
      "Delete audiences you added manually",
      "Re-find contacts automatically — you still click “Find next batch” per audience",
    ],
  };
}
