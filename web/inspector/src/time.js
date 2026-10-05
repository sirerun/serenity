export function creationDate(node) {
  return node?.createdAt || node?.captureTime || node?.capturedAt || null;
}

export function creationYear(node) {
  const date = creationDate(node);
  if (typeof date !== "string") return "Unknown";
  const match = date.match(/^(\d{4})(?:-|$)/);
  return match ? match[1] : "Unknown";
}

export function sortedFacetYears(facets) {
  const raw = facets?.years ?? facets?.yearCounts ?? facets ?? {};
  const values = Array.isArray(raw)
    ? raw.map((item) => typeof item === "string" ? item : item?.year).filter(Boolean)
    : Object.keys(raw || {});
  return [...new Set(values.map(String).filter((year) => year !== "Unknown"))].sort((a, b) => a.localeCompare(b));
}

export function facetCount(facets, year) {
  const raw = facets?.years ?? facets?.yearCounts ?? facets ?? {};
  if (year === "Unknown") {
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
  if (year === "Unknown") return null;
  const index = years.indexOf(year);
  if (index === -1) return direction > 0 ? years[0] : years.at(-1);
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
