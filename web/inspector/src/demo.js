import { demoBrain } from "./graph/demoFixture.js";
import { creationYear } from "./time.js";

const types = ["entity", "fact", "claim", "source"];
const buckets = { entity: demoBrain.entities, fact: demoBrain.facts, claim: demoBrain.claims, source: demoBrain.sources };
// The accepted prototype uses a six-entity, 39-record fictional brain.
const earliestFacts = new Map();
for (const fact of buckets.fact) {
  if (!fact.createdAt) continue;
  const prior = earliestFacts.get(fact.entityId);
  if (!prior || fact.createdAt < prior) earliestFacts.set(fact.entityId, fact.createdAt);
}

export const demoNodes = types.flatMap((type) => buckets[type].map((item) => {
  if (type === "entity") {
    const { position, summary, ...rest } = item;
    return { ...rest, type, text: summary || "", dateKind: "earliest-linked-memory", createdAt: earliestFacts.get(item.id), capturedAt: undefined };
  }
  const { position, attribution, context, ...rest } = item;
  if (type === "source") return { ...rest, type, text: item.excerpt || "", capturedAt: item.capturedAt || null };
  return { ...rest, type, capturedAt: item.createdAt || null };
}));

const demoNodeIds = new Set(demoNodes.map((node) => node.id));
export const demoEdges = [];
const edgeIds = new Set();
function addEdge(edge) { if (!edgeIds.has(edge.id)) { edgeIds.add(edge.id); demoEdges.push(edge); } }
for (const node of demoNodes) {
  if (node.type !== "entity" && node.entityId && demoNodeIds.has(node.entityId)) {
    addEdge({ id: `memory:${node.entityId}:${node.id}`, source: node.entityId, target: node.id, type: "memory" });
  }
  for (const sourceId of node.sourceIds || []) {
    if (demoNodeIds.has(sourceId)) addEdge({ id: `source:${node.id}:${sourceId}`, source: node.id, target: sourceId, type: "source" });
  }
  if (node.supersedes && demoNodeIds.has(node.supersedes)) addEdge({ id: `history:${node.id}:${node.supersedes}`, source: node.id, target: node.supersedes, type: "history" });
  if (node.supersededBy && demoNodeIds.has(node.supersededBy)) addEdge({ id: `history:${node.id}:${node.supersededBy}`, source: node.id, target: node.supersededBy, type: "history" });
}

export const demoFacets = demoNodes.reduce((facets, node) => {
  const year = creationYear(node);
  if (year === "unknown") facets.unknown += 1;
  else facets.years.set(year, (facets.years.get(year) || 0) + 1);
  return facets;
}, { years: new Map(), unknown: 0 });
demoFacets.years = [...demoFacets.years].map(([year, count]) => ({ year, count }));

export const demoCoreIds = demoNodes.map((node) => node.id);
