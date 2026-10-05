/**
 * Entirely synthetic sample data for the Serenity memory inspector prototype.
 * Nothing in this file is loaded from, or written back to, a Serenity store.
 */

export const demoBrain = {
  label: "Demo brain",
  generatedAt: "2026-10-03T12:00:00Z",
  entities: [
    { id: "person-maya", label: "Maya Chen", kind: "person", scope: "private", summary: "A sample person used to demonstrate linked memories.", position: [-4.8, 1.4, 0.2] },
    { id: "project-serenity", label: "Serenity", kind: "project", scope: "world", summary: "A sample project with evolving design and retrieval notes.", position: [-1.9, 0.1, -1.1] },
    { id: "project-fieldnotes", label: "Fieldnotes", kind: "project", scope: "world", summary: "A small research project in the demo graph.", position: [1.4, 1.8, -0.3] },
    { id: "place-studio", label: "North Studio", kind: "place", scope: "private", summary: "A fictional work room and meeting place.", position: [4.2, 0.5, 0.5] },
    { id: "concept-retrieval", label: "Memory retrieval", kind: "concept", scope: "world", summary: "An idea connected to several design claims.", position: [-0.6, -2.1, 1.2] },
    { id: "concept-garden", label: "Quiet garden", kind: "concept", scope: "private", summary: "A personal preference included in this synthetic brain.", position: [2.9, -1.8, -1.1] },
  ],
  facts: [
    { id: "fact-maya-role", entityId: "person-maya", text: "Maya is a product designer.", scope: "private", status: "current", kind: "profile", sourceIds: ["source-intro"], createdAt: "2024-05-12", attribution: "Sample introduction" },
    { id: "fact-maya-pronouns", entityId: "person-maya", text: "Maya uses she/her pronouns.", scope: "private", status: "current", kind: "profile", sourceIds: ["source-intro"], createdAt: "2024-05-12", attribution: "Sample introduction" },
    { id: "fact-maya-concise", entityId: "person-maya", text: "Prefers concise notes.", scope: "world", status: "current", kind: "preference", sourceIds: ["source-preferences"], createdAt: "2024-06-04", attribution: "Sample preferences", context: "Maya prefers a short written recap after each planning session." },
    { id: "fact-serenity-purpose", entityId: "project-serenity", text: "Serenity is an on-device memory system.", scope: "world", status: "current", kind: "description", sourceIds: ["source-brief"], createdAt: "2024-03-01", attribution: "Synthetic project brief" },
    { id: "fact-serenity-owner", entityId: "project-serenity", text: "Maya is prototyping the inspector experience.", scope: "private", status: "current", kind: "activity", sourceIds: ["source-session"], createdAt: "2025-11-10", attribution: "Demo conversation · Nov 10" },
    { id: "fact-fieldnotes-purpose", entityId: "project-fieldnotes", text: "Fieldnotes collects short research observations.", scope: "world", status: "current", kind: "description", sourceIds: ["source-brief"], createdAt: "2025-05-16", attribution: "Synthetic project brief" },
    { id: "fact-studio-location", entityId: "place-studio", text: "North Studio is in the fictional Harbor District.", scope: "private", status: "current", kind: "location", sourceIds: ["source-calendar"], createdAt: "2024-11-10", attribution: "Sample calendar entry" },
    { id: "fact-meeting-room", entityId: "place-studio", text: "The sample team meets in the small library room.", scope: "private", status: "current", kind: "place detail", sourceIds: ["source-session"], createdAt: "2025-11-10", attribution: "Demo conversation · Nov 10" },
    { id: "fact-retrieval-exact", entityId: "concept-retrieval", text: "A sample retrieval note describes full-dimensional vector comparison.", scope: "world", status: "current", kind: "research note", sourceIds: ["source-brief"], createdAt: "2026-09-04", attribution: "Synthetic project brief" },
    { id: "fact-retrieval-fusion", entityId: "concept-retrieval", text: "A sample note describes combining vector and text matches.", scope: "world", status: "current", kind: "research note", sourceIds: ["source-session"], createdAt: "2026-09-04", attribution: "Demo conversation · Nov 10" },
    { id: "fact-garden-preference", entityId: "concept-garden", text: "Maya prefers a quiet outdoor break after lunch.", scope: "private", status: "current", kind: "preference", sourceIds: ["source-preferences"], createdAt: "2024-06-04", attribution: "Sample preferences" },
    { id: "fact-garden-distance", entityId: "concept-garden", text: "The garden path is a short walk from North Studio.", scope: "private", status: "current", kind: "place detail", sourceIds: ["source-calendar"], createdAt: "2024-11-10", attribution: "Sample calendar entry" },
    { id: "fact-demo-origin", entityId: "project-serenity", text: "All names and details in this view are synthetic.", scope: "world", status: "current", kind: "demo note", sourceIds: ["source-fixture"], createdAt: "2026-10-03", attribution: "Demo fixture" },
    { id: "fact-private-authority", entityId: "person-maya", text: "Private memories are visible only in this local demo.", scope: "private", status: "current", kind: "demo note", sourceIds: ["source-fixture"], createdAt: "2026-10-03", attribution: "Demo fixture" },
    { id: "fact-expired-preference", entityId: "person-maya", text: "Maya planned to work from North Studio this week.", scope: "private", status: "expired", kind: "temporary plan", sourceIds: ["source-calendar"], createdAt: "2024-11-10", validUntil: "2026-08-17", attribution: "Sample calendar entry" },
  ],
  claims: [
    { id: "claim-serenity-scope", entityId: "project-serenity", text: "Serenity should keep private memories under local user control.", scope: "world", status: "current", confidence: 0.82, sourceIds: ["source-brief"], createdAt: "2025-05-16", validUntil: null, supersedes: null, supersededBy: null },
    { id: "claim-memory-source", entityId: "project-serenity", text: "Every remembered statement should expose where it came from.", scope: "world", status: "current", confidence: 0.91, sourceIds: ["source-session", "source-brief"], createdAt: "2025-11-10", validUntil: null, supersedes: null, supersededBy: null },
    { id: "claim-serenity-cloud", entityId: "project-serenity", text: "Serenity will sync all memories to a shared cloud by default.", scope: "world", status: "superseded", confidence: 0.45, sourceIds: ["source-old-notes"], createdAt: "2024-02-14", validUntil: "2025-02-20", supersedes: null, supersededBy: "claim-serenity-local" },
    { id: "claim-serenity-local", entityId: "project-serenity", text: "Serenity should keep memory local unless the user chooses to share it.", scope: "world", status: "current", confidence: 0.93, sourceIds: ["source-session"], createdAt: "2025-11-10", validUntil: null, supersedes: "claim-serenity-cloud", supersededBy: null },
    { id: "claim-fast-search", entityId: "concept-retrieval", text: "A clustered index may reduce work for some queries.", scope: "world", status: "current", confidence: 0.63, sourceIds: ["source-retrieval-review"], createdAt: "2026-09-04", validUntil: null, supersedes: null, supersededBy: null },
    { id: "claim-lossless-index", entityId: "concept-retrieval", text: "The clustered index always returns exact results in constant time.", scope: "world", status: "superseded", confidence: 0.28, sourceIds: ["source-old-notes"], createdAt: "2024-06-09", validUntil: "2026-09-04", supersedes: null, supersededBy: "claim-fast-search" },
    { id: "claim-break-timing", entityId: "concept-garden", text: "A short walk helps Maya reset between afternoon sessions.", scope: "private", status: "current", confidence: 0.59, sourceIds: ["source-preferences"], createdAt: "2024-06-04", validUntil: null, supersedes: null, supersededBy: null },
    { id: "claim-review-room", entityId: "place-studio", text: "The library room is easiest to use on Wednesdays.", scope: "private", status: "current", confidence: 0.55, sourceIds: ["source-calendar"], createdAt: "2024-11-10", validUntil: null, supersedes: null, supersededBy: null },
    { id: "claim-fieldnotes-format", entityId: "project-fieldnotes", text: "Short observations are easier to review than long session transcripts.", scope: "world", status: "current", confidence: 0.75, sourceIds: ["source-brief"], createdAt: "2025-05-16", validUntil: null, supersedes: null, supersededBy: null },
    { id: "claim-missing-reference", entityId: "project-serenity", text: "A sample note references one available source and one that is absent from this fixture.", scope: "private", status: "current", confidence: 0.4, sourceIds: ["source-session", "source-not-present"], createdAt: "2026-09-04", validUntil: null, supersedes: null, supersededBy: null },
  ],
  sources: [
    { id: "source-intro", label: "Sample introduction", kind: "conversation", scope: "private", capturedAt: "2024-05-12", excerpt: "A fictional profile used only for this prototype." },
    { id: "source-brief", label: "Synthetic project brief", kind: "document", scope: "world", capturedAt: "2024-03-01", excerpt: "A generated summary of the fictional Serenity and Fieldnotes projects." },
    { id: "source-session", label: "Demo conversation · Nov 10", kind: "conversation", scope: "private", capturedAt: "2025-11-10", excerpt: "A fictional conversation about control, provenance, and the inspector." },
    { id: "source-calendar", label: "Sample calendar entry", kind: "calendar", scope: "private", capturedAt: "2024-11-10", excerpt: "A fictional event mentioning North Studio." },
    { id: "source-preferences", label: "Sample preferences", kind: "conversation", scope: "private", capturedAt: "2024-06-04", excerpt: "Fictional notes mention concise planning recaps and a quiet outdoor break." },
    { id: "source-old-notes", label: "Outdated concept note", kind: "document", scope: "world", capturedAt: "2024-02-14", excerpt: "An older fictional proposal retained to show expiry and supersession." },
    { id: "source-retrieval-review", label: "Retrieval review note", kind: "document", scope: "world", capturedAt: "2026-09-04", excerpt: "A fictional note describing a hypothesis to benchmark." },
    { id: "source-fixture", label: "Demo fixture", kind: "fixture", scope: "world", capturedAt: "2026-10-03", excerpt: "This screen uses synthetic data. No live Serenity store is connected." },
  ],
};

// Entity creation is the earliest linked memory date. This produces honest demo cohorts.
for (const entity of demoBrain.entities) {
  const firstLinkedMemory = [...demoBrain.facts, ...demoBrain.claims]
    .filter((record) => record.entityId === entity.id)
    .map((record) => record.createdAt)
    .filter(Boolean)
    .sort()[0];
  entity.createdAt = firstLinkedMemory;
}

const nodeIndex = new Map([
  ...demoBrain.entities.map((item) => [item.id, { ...item, type: "entity" }]),
  ...demoBrain.facts.map((item) => [item.id, { ...item, type: "fact" }]),
  ...demoBrain.claims.map((item) => [item.id, { ...item, type: "claim" }]),
  ...demoBrain.sources.map((item) => [item.id, { ...item, type: "source" }]),
]);

export const demoNodeCount = nodeIndex.size;
export const getNode = (id) => nodeIndex.get(id) ?? null;

export function getEntityRecords(entityId) {
  return {
    entity: getNode(entityId),
    facts: demoBrain.facts.filter((record) => record.entityId === entityId),
    claims: demoBrain.claims.filter((record) => record.entityId === entityId),
  };
}

export function getSources(record) {
  return (record.sourceIds ?? []).map((sourceId) => ({ id: sourceId, node: getNode(sourceId) }));
}

export function getNeighborhood(entityId) {
  const { entity, facts, claims } = getEntityRecords(entityId);
  const records = [...facts, ...claims];
  const sourceIds = new Set(records.flatMap((record) => record.sourceIds));
  const relatedIds = new Set();
  for (const record of records) {
    if (record.supersedes) relatedIds.add(record.supersedes);
    if (record.supersededBy) relatedIds.add(record.supersededBy);
  }
  return {
    entity,
    records,
    sources: [...sourceIds].map((id) => getNode(id)).filter(Boolean),
    relatedRecords: [...relatedIds].map((id) => getNode(id)).filter(Boolean),
    missingReferences: records.flatMap((record) => getSources(record).filter(({ node }) => !node).map(({ id }) => ({ from: record.id, id }))),
  };
}

export function getSearchableNodes() {
  return [...nodeIndex.values()];
}
