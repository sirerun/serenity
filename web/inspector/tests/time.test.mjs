import test from "node:test";
import assert from "node:assert/strict";
import { adjacentFacetYear, creationDate, creationYear, dedupeNodes, facetCount, sortedFacetYears } from "../src/time.js";

test("capture time derives a year without treating observed time as capture time", () => {
  assert.equal(creationDate({ createdAt: "2024-08-09", captureTime: "2025-01-02" }), "2024-08-09");
  assert.equal(creationYear({ captureTime: "2025-01-02T00:00:00Z" }), "2025");
  assert.equal(creationYear({ observedAt: "2026-03-04" }), "Unknown");
  assert.equal(creationYear({ createdAt: "invalid" }), "Unknown");
});

test("full-dataset facets remain chronological and keep unknown explicit", () => {
  const facets = { years: { "2025": 3, "2023": 2, "2024": 4 }, unknown: 1 };
  const years = sortedFacetYears(facets);
  assert.deepEqual(years, ["2023", "2024", "2025"]);
  assert.equal(facetCount(facets, "2024"), 4);
  assert.equal(facetCount(facets, "Unknown"), 1);
  assert.equal(adjacentFacetYear(years, "2024", -1), "2023");
  assert.equal(adjacentFacetYear(years, "2024", 1), "2025");
  assert.equal(adjacentFacetYear(years, "2025", 1), null);
});

test("node merge deduplicates by stable ID, prioritizes core nodes and caps context", () => {
  const nodes = Array.from({ length: 503 }, (_, index) => ({ id: `node-${index}`, type: "fact" }));
  nodes.push({ id: "node-1", type: "entity" });
  const merged = dedupeNodes(nodes, ["node-501", "node-1"], 3);
  assert.deepEqual(merged.map((node) => node.id), ["node-1", "node-501", "node-0"]);
  assert.deepEqual(merged.map((node) => node.isContext), [false, false, true]);
});
