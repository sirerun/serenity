export function creationDate(node) {
  if (node?.type === "entity" && node?.dateKind === "earliest-linked-memory") return node.createdAt || null;
  return node?.capturedAt || null;
}

export function creationYear(node) {
  const date = creationDate(node);
  if (typeof date !== "string") return "unknown";
  const match = date.match(/^(\d{4})(?:-|$)/);
  return match ? match[1] : "unknown";
}

export function sortedFacetYears(facets) {
  const raw = facets?.years ?? facets?.yearCounts ?? facets ?? {};
  const values = Array.isArray(raw)
    ? raw.map((item) => typeof item === "string" ? item : item?.year).filter(Boolean)
    : Object.keys(raw || {});
  return [...new Set(values.map(String).filter((year) => year.toLowerCase() !== "unknown"))].sort((a, b) => a.localeCompare(b));
}

export function facetCount(facets, year) {
  const raw = facets?.years ?? facets?.yearCounts ?? facets ?? {};
  if (String(year).toLowerCase() === "unknown") {
    if (Array.isArray(raw)) {
      const entry = raw.find((item) => String(typeof item === "string" ? item : item?.year).toLowerCase() === "unknown");
      return typeof entry === "object" ? Number(entry.count) || 0 : 0;
    }
    return Number(facets?.unknown ?? facets?.unknownCount ?? raw?.Unknown ?? raw?.unknown) || 0;
  }
  if (Array.isArray(raw)) {
    const item = raw.find((entry) => String(typeof entry === "string" ? entry : entry?.year) === String(year));
    return typeof item === "object" ? Number(item.count) || 0 : 0;
  }
  return Number(raw?.[year]) || 0;
}

export function adjacentFacetYear(years, year, direction) {
  if (!years.length) return null;
  if (String(year).toLowerCase() === "unknown") return null;
  const index = years.indexOf(year);
  if (index === -1) return direction > 0 ? years.at(-1) : years[0];
  return years[index + direction] ?? null;
}

export function dedupeNodes(nodes, coreIds = [], limit = 500) {
  const core = new Set(coreIds);
  const seen = new Map();
  const candidates = nodes || [];
  for (const node of [...candidates.filter((item) => core.has(item?.id)), ...candidates.filter((item) => !core.has(item?.id))]) {
    if (!node?.id || seen.has(node.id)) continue;
    seen.set(node.id, { ...node, isContext: !core.has(node.id) });
    if (seen.size >= limit) break;
  }
  return [...seen.values()];
}
