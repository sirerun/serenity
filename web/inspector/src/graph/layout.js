const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5));

function hashText(value) {
  let hash = 2166136261;
  for (const char of value) hash = Math.imul(hash ^ char.charCodeAt(0), 16777619);
  return hash >>> 0;
}

// Stable book-like clustered layout adapted from the accepted constellation prototype.
export function layoutGraph(nodes, edges = []) {
  const positions = new Map();
  const entities = nodes.filter((node) => node.type === "entity").sort((a, b) => a.id.localeCompare(b.id));
  const centers = new Map();
  entities.forEach((node, index) => {
    const angle = index * Math.PI * 2 / Math.max(1, entities.length) - Math.PI / 2;
    const center = [Math.cos(angle) * 0.39, Math.sin(angle) * 0.31];
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
      const phase = hashText(node.id) / 4294967296 * Math.PI * 2;
      const radius = 0.1 + Math.sqrt(index + 1) * 0.055;
      const angle = index * GOLDEN_ANGLE + phase;
      positions.set(node.id, [center[0] + Math.cos(angle) * radius, center[1] + Math.sin(angle) * radius * 0.72]);
    });
  }
  const orphans = nodes.filter((node) => !positions.has(node.id)).sort((a, b) => a.id.localeCompare(b.id));
  orphans.forEach((node, index) => {
    const phase = index * GOLDEN_ANGLE + hashText(node.id) % 1000 / 1000;
    const radius = 0.24 + Math.sqrt(index + 1) * 0.035;
    positions.set(node.id, [Math.cos(phase) * radius, Math.sin(phase) * radius * 0.7]);
  });
  const ids = new Set(positions.keys());
  return { positions, edges: edges.filter((edge) => ids.has(edge.from) && ids.has(edge.to)) };
}

function creationOrder(node) { return node.createdAt || node.capturedAt || "9999"; }
