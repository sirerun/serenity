import test from "node:test";
import assert from "node:assert/strict";
import { adjacentFacetYear, creationDate, creationYear, dedupeNodes, facetCount, sortedFacetYears } from "../src/time.js";
import { normalizeEdge } from "../src/graph/layout.js";
import { demoEdges, demoFacets, demoNodes } from "../src/demo.js";

test("capture year preserves the API date semantics", () => {
  assert.equal(creationDate({ type: "entity", dateKind: "earliest-linked-memory", createdAt: "2024-08-09" }), "2024-08-09");
  assert.equal(creationDate({ type: "fact", createdAt: "2024-08-09", capturedAt: "2025-01-02" }), "2025-01-02");
  assert.equal(creationYear({ type: "claim", observedAt: "2026-03-04" }), "unknown");
  assert.equal(creationYear({ type: "source", observedAt: "2026-03-04" }), "unknown");
  assert.equal(creationYear({ type: "fact", createdAt: "invalid" }), "unknown");
});

test("full-dataset facets remain chronological and keep unknown explicit", () => {
  const facets = { years: [{ year: "2025", count: 3 }, { year: "unknown", count: 1 }, { year: "2023", count: 2 }, { year: "2024", count: 4 }] };
  const years = sortedFacetYears(facets);
  assert.deepEqual(years, ["2023", "2024", "2025"]);
  assert.equal(facetCount(facets, "2024"), 4);
  assert.equal(facetCount(facets, "unknown"), 1);
  assert.equal(adjacentFacetYear(years, "2024", -1), "2023");
  assert.equal(adjacentFacetYear(years, "2024", 1), "2025");
  assert.equal(adjacentFacetYear(years, "2025", 1), null);
});

test("graph normalizes the read API edge names", () => {
  assert.deepEqual(normalizeEdge({ id: "e1", source: "a", target: "b", type: "source" }), { id: "e1", source: "a", target: "b", type: "source", from: "a", to: "b", kind: "source" });
});

test("public fixture retains the accepted 39 synthetic nodes", () => {
  assert.equal(demoNodes.length, 39);
  assert.ok(demoEdges.length >= 39);
  assert.ok(demoEdges.every((edge) => edge.source && edge.target && edge.type));
  assert.ok(Array.isArray(demoFacets.years));
});

test("node merge deduplicates by stable ID, prioritizes core nodes and caps context", () => {
  const nodes = Array.from({ length: 503 }, (_, index) => ({ id: `node-${index}`, type: "fact" }));
  nodes.push({ id: "node-1", type: "entity" });
  const merged = dedupeNodes(nodes, ["node-501", "node-1"], 3);
  assert.deepEqual(merged.map((node) => node.id), ["node-1", "node-501", "node-0"]);
  assert.deepEqual(merged.map((node) => node.isContext), [false, false, true]);
});
