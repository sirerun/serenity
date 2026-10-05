// Public demo fixture shape: nodes use the read API node fields and edges use
// { id, from, to, kind }. All records below are fictional and safe for public use.
export const demoNodes = [
  { id: "demo-project", type: "entity", kind: "project", label: "Book Binder", text: "", scope: "world", status: "current", captureTime: "2024-03-18" },
  { id: "demo-fact", type: "fact", kind: "design-note", label: "", text: "A quiet, tactile space makes important ideas easier to revisit.", entityId: "demo-project", scope: "world", status: "current", captureTime: "2024-03-18", sourceIds: ["demo-source"] },
  { id: "demo-claim", type: "claim", kind: "product-principle", label: "", text: "Good tools should keep people in control of what they remember.", entityId: "demo-project", scope: "world", status: "current", observedAt: "2025-06-09", sourceIds: ["demo-source"] },
  { id: "demo-source", type: "source", kind: "conversation", label: "A fictional design conversation", text: "", scope: "world", status: "current", captureTime: "2025-06-09" },
  { id: "demo-unknown", type: "fact", kind: "observation", label: "", text: "Some notes have no trustworthy date and remain grouped as unknown.", entityId: "demo-project", scope: "world", status: "current" },
];

export const demoEdges = [
  { id: "demo-e1", from: "demo-project", to: "demo-fact", kind: "memory" },
  { id: "demo-e2", from: "demo-project", to: "demo-claim", kind: "memory" },
  { id: "demo-e3", from: "demo-fact", to: "demo-source", kind: "source" },
  { id: "demo-e4", from: "demo-claim", to: "demo-source", kind: "source" },
  { id: "demo-e5", from: "demo-project", to: "demo-unknown", kind: "memory" },
];

export const demoFacets = { years: { "2025": 1, "2024": 2 }, unknown: 2 };
