import { useEffect, useMemo, useRef, useState } from "react";
import { adjacentFacetYear, creationDate, dedupeNodes, facetCount, sortedFacetYears } from "./time.js";
import { layoutGraph, normalizeEdge } from "./graph/layout.js";
import Graph3D from "./graph/Graph3D.jsx";
import "./styles.css";

/** onSelect emits a stable node ID; the app adapter owns data loading and auth. */
const TYPES = ["entity", "fact", "claim", "source"];
const labelFor = (node) => node?.type === "entity" ? node.label || node.text : node?.text || node?.label;
const selectedKey = (selected) => typeof selected === "string" ? selected : selected?.id;
const text = (value) => typeof value === "string" ? value : "";

export function Explorer({
  nodes = [], edges = [], coreIds = [], facets = {}, filters = { scope: "all", year: "all", q: "", type: "all" },
  onFilter = () => {}, onMore = () => {}, hasMore = false, loading = false, totalMatching = 0,
  selected, onSelect = () => {}, detail, detailLoading = false, error, mode = "private",
  brainName = "Your memory", onBrainChange, brains = [], onRetry,
}) {
  const [presentation, setPresentation] = useState("graph");
  const [graph3DReady, setGraph3DReady] = useState(false);
  const [reducedMotion, setReducedMotion] = useState(false);
  const searchRef = useRef(null);
  useEffect(() => {
    const focusSearch = (event) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        searchRef.current?.focus();
      }
    };
    document.addEventListener("keydown", focusSearch);
    return () => document.removeEventListener("keydown", focusSearch);
  }, []);
  useEffect(() => {
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setReducedMotion(media.matches);
    update();
    media.addEventListener?.("change", update);
    return () => media.removeEventListener?.("change", update);
  }, []);
  const safeNodes = useMemo(() => dedupeNodes(nodes, coreIds), [nodes, coreIds]);
  const coreSet = useMemo(() => new Set(coreIds), [coreIds]);
  const years = useMemo(() => sortedFacetYears(facets), [facets]);
  const rawYear = filters.year;
  const year = !rawYear || String(rawYear).toLowerCase() === "all" ? "" : String(rawYear).toLowerCase() === "unknown" ? "unknown" : String(rawYear);
  const selectedId = selectedKey(selected);
  const visibleNodes = safeNodes;
  const visibleIds = useMemo(() => new Set(visibleNodes.map((node) => node.id)), [visibleNodes]);
  const visibleEdges = useMemo(() => edges.map(normalizeEdge).filter((edge) => visibleIds.has(edge.from) && visibleIds.has(edge.to)), [edges, visibleIds]);
  const { positions } = useMemo(() => layoutGraph(visibleNodes, visibleEdges), [visibleNodes, visibleEdges]);
  const coreVisible = visibleNodes.filter((node) => coreSet.has(node.id));
  const contextVisible = visibleNodes.length - coreVisible.length;
  const selectedNode = safeNodes.find((node) => node.id === selectedId) || (selected && typeof selected === "object" ? selected : null);
  const change = (patch) => onFilter({ ...filters, ...patch });
  const moveYear = (direction) => {
    const next = adjacentFacetYear(years, year || null, direction);
    if (next) change({ year: next });
  };
  const yearsWithCounts = years.map((value) => ({ year: value, count: facetCount(facets, value) }));
  const allFacetCount = yearsWithCounts.reduce((sum, facet) => sum + facet.count, facetCount(facets, "Unknown"));
  const selectedDetailNode = detail?.node || detail || null;
  const relatedSources = (detail?.relatedNodes || []).filter((node) => node.type === "source");
  const sourceEntries = (selectedDetailNode?.sourceIds || selectedDetailNode?.sources || []).map((source) => {
    const sourceNode = typeof source === "object" && source?.node ? source.node : source;
    const id = typeof sourceNode === "string" ? sourceNode : sourceNode?.id;
    return relatedSources.find((node) => node.id === id) || sourceNode;
  });

  return <main className={`explorer ${mode === "demo" ? "explorer-demo" : "explorer-private"}`}>
    <header className="explorer-topbar">
      <a className="explorer-brand" href="#explore" aria-label="Serenity memory explorer">
        <img className="brand-mark" src="/assets/brand.svg" alt="" />
        <span><strong>Serenity</strong><small>MEMORY EXPLORER</small></span>
      </a>
      <div className="topbar-center">{mode === "demo" ? <span className="demo-label">SYNTHETIC DEMO</span> : <><span className="private-dot" aria-hidden="true" /> PRIVATE SPACE</>}</div>
      <div className="brain-picker">
        {onBrainChange && brains.length > 1 ? <label className="sr-only" htmlFor="explorer-brain">Choose memory space</label> : null}
        {onBrainChange && brains.length > 1
          ? <select id="explorer-brain" value={selectedBrainValue(brains, brainName)} onChange={(event) => onBrainChange(event.target.value)}>{brains.map((brain) => <option key={brain.id || brain.name} value={brain.id || brain.name}>{brain.name || brain.label || brain.id}</option>)}</select>
          : <span>{brainName}</span>}
        <span className="avatar" aria-hidden="true">{(brainName || "M").slice(0, 1).toUpperCase()}</span>
      </div>
    </header>

    <section className="explorer-intro">
      <div className="intro-copy"><div className="eyebrow"><span /> A PERSONAL LIBRARY OF IDEAS</div>
        <h1>Memory, <em>in its place.</em></h1>
        <p>Explore the people, moments and ideas connected across {mode === "demo" ? "this fictional collection" : "your memory"}.</p>
      </div>
      <div className="intro-stats"><strong>{formatCount(totalMatching)}</strong><span>memories in this view</span><div className="stat-rule" /><span className="stat-note">{mode === "demo" ? <><span className="demo-stats-dot" /> Entirely fictional</> : <><span className="private-dot" /> Private to you</>}</span></div>
    </section>

    <section className="filter-panel" aria-label="Memory filters">
      <label className="search-box"><span aria-hidden="true" className="search-icon" /><span className="sr-only">Search memories</span><input ref={searchRef} value={filters.q || ""} onChange={(event) => change({ q: event.target.value })} placeholder="Search your memories…" />{filters.q ? <button type="button" className="clear-search" onClick={() => change({ q: "" })} aria-label="Clear search">×</button> : <kbd>⌘ K</kbd>}</label>
      <label className="filter-select"><span className="sr-only">Memory scope</span><select aria-label="Memory scope" value={filters.scope || "all"} onChange={(event) => change({ scope: event.target.value })}><option value="all">All scopes</option><option value="private">Private</option><option value="world">World</option></select></label>
      <label className="filter-select type-select"><span className="sr-only">Memory type</span><select aria-label="Memory type" value={filters.type || "all"} onChange={(event) => change({ type: event.target.value })}><option value="all">All types</option>{TYPES.map((type) => <option value={type} key={type}>{type[0].toUpperCase() + type.slice(1)}s</option>)}</select></label>
      <div className="view-switch" role="group" aria-label="Presentation"><button type="button" aria-pressed={presentation === "graph"} onClick={() => setPresentation("graph")}><span aria-hidden="true">◌</span> Map</button><button type="button" aria-pressed={presentation === "list"} onClick={() => { setGraph3DReady(false); setPresentation("list"); }}><span aria-hidden="true">☷</span> List</button></div>
    </section>

    <section className="year-rail" aria-label="Browse by year">
      <div className="rail-heading"><div><span className="rail-overline">TIME ATLAS</span><strong>Added to memory</strong></div><span className="rail-total">{formatCount(allFacetCount)} RECORDS</span></div>
      <div className="rail-track">
        <button className={`year-all ${year === "" ? "active" : ""}`} aria-pressed={year === ""} type="button" onClick={() => change({ year: "" })}>All time <small>{formatCount(allFacetCount)}</small></button>
        <button className="year-arrow" aria-label="Previous year" type="button" onClick={() => moveYear(-1)} disabled={!years.length || year === years[0] || year === "unknown"}>‹</button>
        <div className="year-items">{yearsWithCounts.map(({ year: value, count }) => <button className={`year-item ${year === value ? "active" : ""}`} type="button" aria-pressed={year === value} key={value} onClick={() => change({ year: value })}><span className="year-dot" />{value}<small>{formatCount(count)}</small></button>)}</div>
        <button className="year-arrow" aria-label="Next year" type="button" onClick={() => moveYear(1)} disabled={!years.length || year === years.at(-1) || year === "unknown"}>›</button>
        <button className={`year-item unknown-year ${year === "unknown" ? "active" : ""}`} type="button" aria-pressed={year === "unknown"} onClick={() => change({ year: "unknown" })}><span className="year-dot" />Unknown<small>{formatCount(facetCount(facets, "unknown"))}</small></button>
      </div>
    </section>

    <div className="workspace-heading"><div><span className="workspace-kicker">YOUR COLLECTION</span><h2>{year === "" ? "A connected library" : year === "unknown" ? "Without a date" : `The ${year} collection`}</h2></div><div className="workspace-count"><strong>{formatCount(totalMatching)}</strong><span>matching memories</span></div></div>
    {error ? <div className="explorer-state error-state" role="alert"><span className="state-seal">!</span><h3>We couldn’t open this collection</h3><p>{text(error?.message || error) || "Your memories are still private. Try again when the connection is ready."}</p>{onRetry && <button type="button" className="rose-button" onClick={onRetry}>Try again</button>}</div>
      : loading && !safeNodes.length ? <div className="explorer-state" role="status"><span className="loader-orbit" /><p>Opening your library…</p></div>
      : !totalMatching ? <div className="explorer-state empty-state"><span className="state-seal">✧</span><h3>No matching memories</h3><p>Try another year or clear a filter to explore more of your library.{mode === "private" ? " Remote-private memories remain excluded from this view." : ""}</p><button type="button" className="quiet-button" onClick={() => onFilter({ ...filters, scope: "all", type: "all", q: "", year: "" })}>Clear filters</button></div>
      : <section className={`collection ${presentation === "list" ? "list-mode" : "graph-mode"}`} aria-label="Memory collection">
        <div className="collection-main">
          {presentation === "graph" ? <div className="map-stage">
            <div className="map-caption"><span className="caption-pip" /> INTERCONNECTED IDEAS <span className="caption-divider">·</span> SELECT A NODE TO INSPECT</div>
            <span id="map-keyboard-help" className="sr-only">Drag to rotate the map; scroll or pinch to zoom. Use the arrow keys to move the selection, Enter to inspect a memory, and Home to reset the view.</span>
            <Graph3D nodes={visibleNodes} edges={visibleEdges} positions={positions} selectedId={selectedId} onSelect={onSelect} reducedMotion={reducedMotion} onReady={setGraph3DReady} onFailure={() => { setGraph3DReady(false); setPresentation("list"); }} />
            {!graph3DReady && <svg className="constellation" viewBox="0 0 1000 580" role="group" aria-label={`Memory constellation with ${visibleNodes.length} loaded records`}>
              <defs><radialGradient id="rose-halo"><stop offset="0" stopColor="#f1c4cf" stopOpacity=".19"/><stop offset="1" stopColor="#f1c4cf" stopOpacity="0"/></radialGradient><filter id="soft-shadow" x="-80%" y="-80%" width="260%" height="260%"><feDropShadow dx="0" dy="7" stdDeviation="7" floodColor="#442630" floodOpacity=".14"/></filter></defs>
              <ellipse cx="505" cy="300" rx="485" ry="255" fill="url(#rose-halo)" />
              {[0,1,2].map((ring) => <ellipse key={ring} className="atlas-ring" cx="500" cy="296" rx={145 + ring * 112} ry={79 + ring * 61} transform={`rotate(${-13 + ring * 8} 500 296)`} />)}
              {visibleEdges.map((edge) => { const a = positions.get(edge.from); const b = positions.get(edge.to); if (!a || !b) return null; const x1 = 500 + a[0] * 940; const y1 = 290 + a[1] * 520; const x2 = 500 + b[0] * 940; const y2 = 290 + b[1] * 520; return <path key={edge.id || `${edge.from}:${edge.to}`} className={`memory-edge edge-${safeClass(edge.kind)}`} d={`M ${x1} ${y1} Q ${(x1+x2)/2 + (y2-y1)*.07} ${(y1+y2)/2 - (x2-x1)*.06} ${x2} ${y2}`} />; })}
              {visibleNodes.map((node, index) => { const point = positions.get(node.id) || [0, 0]; const x = 500 + point[0] * 940; const y = 290 + point[1] * 520; const context = !coreSet.has(node.id); const active = node.id === selectedId; const label = labelFor(node) || node.kind || node.type; const radius = node.type === "entity" ? 16 : node.type === "claim" ? 8 : 7; return <g key={node.id} className={`map-node node-${safeClass(node.type)} ${context ? "context-node" : ""} ${active ? "selected-node" : ""}`} transform={`translate(${x} ${y})`} role="button" tabIndex="0" aria-label={`${label}${context ? ", related context" : ""}; ${node.type}`} aria-pressed={active} onClick={() => onSelect(node.id)} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); onSelect(node.id); } }}>
                <circle className="node-halo" r={radius * 2.5} /><circle className="node-core" r={radius} filter="url(#soft-shadow)" />{node.type === "entity" && <circle className="node-rim" r={radius + 4} />}
                {index < 14 && <text className="node-label" x={radius + 9} y="4">{shortLabel(label, 28)}</text>}
                {context && <title>Related context: {label}</title>}
              </g>; })}
            </svg>}
            {graph3DReady && <div className="orbit-hint" aria-hidden="true">DRAG TO ORBIT <span>·</span> PINCH TO ZOOM</div>}
            <div className="map-legend"><span><i className="legend-core" /> Matching memory</span><span><i className="legend-context" /> Related context</span><span><i className="legend-link" /> Source connection</span></div>
          </div> : <ul className="memory-list" aria-label="Loaded memories">{visibleNodes.map((node) => <li className="memory-list-item" key={node.id}><button type="button" className={`memory-row ${node.id === selectedId ? "selected" : ""}`} aria-pressed={node.id === selectedId} onClick={() => onSelect(node.id)}><span className={`type-mark type-${safeClass(node.type)}`} aria-hidden="true">{typeGlyph(node.type)}</span><span className="row-copy"><span className="row-title">{labelFor(node) || "Untitled memory"}</span><span className="row-meta">{node.kind || node.type} · {node.scope || "scope unavailable"} · {formatDate(node)}</span></span><span className={`context-tag ${coreSet.has(node.id) ? "" : "visible"}`}>{coreSet.has(node.id) ? "MATCH" : "RELATED"}</span><span className="row-chevron" aria-hidden="true">›</span></button></li>)}</ul>}
          <div className="collection-footer"><span>{formatCount(coreVisible.length)} matching loaded <span className="footer-sep">·</span> {contextVisible} related context</span>{hasMore && <button type="button" className="load-more" onClick={onMore} disabled={loading}>{loading ? "Loading…" : "Load more memories"}<span aria-hidden="true"> ↓</span></button>}</div>
        </div>
        <aside className="detail-card" aria-label="Selected memory" aria-live="polite">
          <div className="detail-topline"><span>MEMORY NOTE</span><span className="detail-index">{selectedNode ? String(Math.max(1, safeNodes.findIndex((node) => node.id === selectedId) + 1)).padStart(2, "0") : "—"}</span></div>
          {detailLoading ? <div className="detail-placeholder"><span className="loader-orbit small" /><span>Opening note…</span></div>
            : selectedNode && selectedDetailNode ? <><div className="detail-kind"><span className={`type-mark type-${safeClass(selectedNode.type)}`}>{typeGlyph(selectedNode.type)}</span>{selectedNode.type} <span className="detail-status">{text(selectedDetailNode.status || selectedNode.status || "available")}</span></div><h3>{labelFor(selectedDetailNode) || labelFor(selectedNode) || "Untitled memory"}</h3><p className="detail-body">{text(selectedDetailNode.text) || "No text is available for this memory."}</p><div className="detail-rule"/><dl className="detail-meta"><div><dt>ADDED</dt><dd>{formatDate(selectedDetailNode)}</dd></div><div><dt>SCOPE</dt><dd>{text(selectedDetailNode.scope || selectedNode.scope) || "Unknown"}</dd></div>{selectedDetailNode.confidence != null && <div><dt>CONFIDENCE</dt><dd>{Math.round(Number(selectedDetailNode.confidence) * 100)}%</dd></div>}</dl>{sourceEntries.length > 0 && <div className="source-block"><span className="source-heading">SOURCES <span>{sourceEntries.length}</span></span>{sourceEntries.slice(0, 4).map((source, index) => { const sourceNode = typeof source === "object" && source?.node ? source.node : source; const key = typeof sourceNode === "string" ? sourceNode : sourceNode?.id || index; const label = typeof sourceNode === "string" ? sourceNode : sourceNode?.label || sourceNode?.text || sourceNode?.id; return <button type="button" className="source-item" key={key} onClick={() => onSelect(sourceNode)}><span className="source-icon">↗</span>{text(label)}</button>; })}</div>}{selectedDetailNode.supersedes && <div className="source-block"><span className="source-heading">SUPERSEDES</span><div className="source-item">{text(selectedDetailNode.supersedes)}</div></div>}{selectedDetailNode.supersededBy && <div className="source-block"><span className="source-heading">SUPERSEDED BY</span><div className="source-item">{text(selectedDetailNode.supersededBy)}</div></div>}{!coreSet.has(selectedId) && <span className="related-note">RELATED CONTEXT · OUTSIDE MATCH COUNT</span>}</>
              : <div className="detail-empty"><span className="detail-book">⌑</span><h3>Select a memory</h3><p>Choose a point in the map or a note in the list to see its details and sources.</p></div>}
        </aside>
      </section>}
    <footer className="explorer-foot"><span>Serenity keeps memory in context.</span><span>BUILT FOR CAREFUL RECOLLECTION</span></footer>
  </main>;
}

export default Explorer;

function selectedBrainValue(brains, name) { const brain = brains.find((item) => item.name === name || item.label === name || item.id === name); return brain?.id || brain?.name || brains[0]?.id || ""; }
function formatCount(value) { return new Intl.NumberFormat().format(Number(value) || 0); }
function safeClass(value) { return String(value || "memory").toLowerCase().replace(/[^a-z0-9_-]/g, "-"); }
function shortLabel(value, limit) { const label = String(value); return label.length > limit ? `${label.slice(0, limit - 1)}…` : label; }
function formatDate(node) { const date = creationDate(node); return date ? String(date).slice(0, 10) : "Unknown date"; }
function typeGlyph(type) { return ({ entity: "✧", fact: "•", claim: "◇", source: "▧" })[type] || "•"; }
