const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5));

export function normalizeEdge(edge) {
  return { ...edge, from: edge?.from ?? edge?.source, to: edge?.to ?? edge?.target, kind: edge?.kind ?? edge?.type };
}

function hashText(value) {
  let hash = 2166136261;
  for (const char of value) hash = Math.imul(hash ^ char.charCodeAt(0), 16777619);
  return hash >>> 0;
}

function unitDirection(index, count, phase = 0) {
  if (count <= 1) return [Math.cos(phase), .24, Math.sin(phase)];
  const vertical = 1 - 2 * ((index + .5) / count);
  const ring = Math.sqrt(Math.max(0, 1 - vertical * vertical));
  const angle = index * GOLDEN_ANGLE + phase;
  return [ring * Math.cos(angle), vertical, ring * Math.sin(angle)];
}

// Stable book-like clustered layout adapted from the accepted constellation prototype.
export function layoutGraph(nodes, edges = []) {
  const positions = new Map();
  const entities = nodes.filter((node) => node.type === "entity").sort((a, b) => a.id.localeCompare(b.id));
  const centers = new Map();
  entities.forEach((node, index) => {
    const angle = index * Math.PI * 2 / Math.max(1, entities.length) - Math.PI / 2;
    const center = [Math.cos(angle) * 0.36, Math.sin(angle) * 0.27, Math.sin(angle * 2 + .45) * 0.13];
    centers.set(node.id, center);
    positions.set(node.id, center);
  });
  const byEntity = new Map(entities.map((entity) => [entity.id, []]));
  for (const node of nodes) {
    if (byEntity.has(node.entityId)) byEntity.get(node.entityId).push(node);
  }
  for (const [entityId, records] of byEntity) {
    const center = centers.get(entityId);
    records.sort((a, b) => creationOrder(a).localeCompare(creationOrder(b)) || a.id.localeCompare(b.id));
    records.forEach((node, index) => {
      const direction = unitDirection(index, records.length, hashText(node.id) % 6283 / 1000);
      const radius = 0.12 + Math.sqrt(index + 1) * 0.043;
      positions.set(node.id, center.map((value, axis) => value + direction[axis] * radius));
    });
  }
  const orphans = nodes.filter((node) => !positions.has(node.id)).sort((a, b) => a.id.localeCompare(b.id));
  orphans.forEach((node, index) => {
    const direction = unitDirection(index, orphans.length, hashText(node.id) % 6283 / 1000);
    const radius = 0.2 + Math.sqrt(index + 1) * 0.045;
    positions.set(node.id, direction.map((value) => value * radius));
  });
  const ids = new Set(positions.keys());
  return { positions, edges: edges.map(normalizeEdge).filter((edge) => ids.has(edge.from) && ids.has(edge.to)) };
}

function creationOrder(node) { return node.createdAt || node.capturedAt || "9999"; }
