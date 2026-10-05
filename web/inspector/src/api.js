export class SessionExpired extends Error {}
export const defaults={scope:'all',year:'',q:'',type:'all'};
export function query(filters,cursor){const p=new URLSearchParams();for(const key of ['scope','year','q','type'])if(filters[key])p.set(key,filters[key]);p.set('limit','100');if(cursor)p.set('cursor',cursor);return p.toString();}
export async function readJSON(path,{signal,fetcher=fetch}={}){
 const r=await fetcher(path,{credentials:'same-origin',cache:'no-store',signal,headers:{Accept:'application/json'}});
 if(r.status===401)throw new SessionExpired('Your session has ended. Sign in to continue.');
 if(!r.ok)throw new Error(r.status===503?'Memories are temporarily unavailable. Please retry.':'Unable to load memories. Please retry.');
 return r.json();
}
export function mergePage(previous,page){
 const core=new Map(previous.core.map(n=>[n.id,n]));for(const n of page.nodes||[])core.set(n.id,n);
 const context=new Map(previous.context.map(n=>[n.id,n]));for(const n of page.contextNodes||[])context.set(n.id,n);for(const id of core.keys())context.delete(id);
 const ordered=[...core.values(),...context.values()].slice(0,500),included=new Set(ordered.map(n=>n.id));
 const edges=new Map(previous.edges.map(e=>[e.id,e]));for(const e of page.edges||[])edges.set(e.id,e);
 return {core:[...core.values()].filter(n=>included.has(n.id)),context:[...context.values()].filter(n=>included.has(n.id)),edges:[...edges.values()].filter(e=>included.has(e.source)&&included.has(e.target)),cursor:page.nextCursor||'',total:page.totalMatching||0,truncated:!!page.contextTruncated};
}
export class RequestEpoch {
 constructor(){this.value=0;this.controller=new AbortController();}
 reset(){this.controller.abort();this.controller=new AbortController();return ++this.value;}
 current(value){return value===this.value&&!this.controller.signal.aborted;}
}
export function captureYear(node){const raw=node.capturedAt||(node.type==='entity'&&node.dateKind==='earliest-linked-memory'?node.createdAt:null);return raw?String(new Date(raw).getUTCFullYear()):'unknown';}
export function localDataset(nodes,edges,filters){
 const q=filters.q.trim().toLowerCase();const eligible=nodes.filter(n=>(filters.scope==='all'||n.scope===filters.scope)&&(filters.type==='all'||n.type===filters.type)&&(!q||[n.label,n.text,n.id].filter(Boolean).join(' ').toLowerCase().includes(q)));
 const counts=new Map();for(const n of eligible){const y=captureYear(n);counts.set(y,(counts.get(y)||0)+1);}
 const selected=eligible.filter(n=>!filters.year||captureYear(n)===filters.year),ids=new Set(selected.map(n=>n.id));
 return {nodes:selected,edges:edges.filter(e=>ids.has(e.source)&&ids.has(e.target)),totalMatching:selected.length,facets:{years:[...counts].map(([year,count])=>({year,count})).sort((a,b)=>a.year.localeCompare(b.year)),totalMatching:eligible.length}};
}
