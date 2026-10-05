import React,{useCallback,useEffect,useMemo,useRef,useState} from 'react';
import {createRoot} from 'react-dom/client';
import {Explorer} from './Explorer.jsx';
import {defaults,readJSON,SessionExpired,mergePage,RequestEpoch,query,localDataset} from './api.js';
import './styles.css';
const EMPTY={core:[],context:[],edges:[],cursor:'',total:0,truncated:false};
const privateMode=location.pathname.startsWith('/dashboard/');
function App(){
 const [brains,setBrains]=useState([]),[brain,setBrain]=useState(''),[filters,setFilters]=useState(defaults),[debouncedQ,setDebouncedQ]=useState('');
 const [data,setData]=useState(EMPTY),[facets,setFacets]=useState({years:[]}),[loading,setLoading]=useState(true),[error,setError]=useState(''),[expired,setExpired]=useState(false),[selected,setSelected]=useState(null),[detail,setDetail]=useState(null),[detailLoading,setDetailLoading]=useState(false),[demo,setDemo]=useState(null),[retry,setRetry]=useState(0);
 const epoch=useRef(new RequestEpoch()),busy=useRef(false),detailRequest=useRef(null);
 const apiBase=useMemo(()=>'/api/inspector/v1/brains/'+encodeURIComponent(brain),[brain]);
 const fail=useCallback(err=>{if(err.name==='AbortError')return;if(err instanceof SessionExpired){epoch.current.reset();detailRequest.current?.abort();setData(EMPTY);setFacets({years:[]});setDetailLoading(false);setDetail(null);setSelected(null);setBrains([]);setBrain('');setExpired(true);}setError(err.message);setLoading(false);busy.current=false;},[]);
 useEffect(()=>{const timer=setTimeout(()=>setDebouncedQ(filters.q),180);return()=>clearTimeout(timer);},[filters.q]);
 useEffect(()=>{let active=true;const controller=new AbortController();if(privateMode){setError('');setLoading(true);readJSON('/api/inspector/v1/brains',{signal:controller.signal}).then(body=>{if(!active)return;setBrains(body.brains);setBrain(body.brains[0]?.id||'');if(!body.brains.length)setLoading(false);}).catch(err=>{if(active)fail(err);});}else{import('./demo.js').then(module=>{if(active){setDemo({nodes:module.demoNodes,edges:module.demoEdges});setLoading(false);}}).catch(fail);}return()=>{active=false;controller.abort();};},[fail,retry]);
 useEffect(()=>{const onPop=e=>{detailRequest.current?.abort();setDetailLoading(false);setSelected(null);setDetail(null);setFilters({...defaults,...(e.state?.explorerFilters||{})});};addEventListener('popstate',onPop);return()=>removeEventListener('popstate',onPop);},[]);
 const effective=useMemo(()=>({...filters,q:debouncedQ}),[filters.scope,filters.year,filters.type,debouncedQ]);
 const onFilter=useCallback(change=>{detailRequest.current?.abort();setDetailLoading(false);setSelected(null);setDetail(null);setFilters(previous=>{const next={...previous,...change};if(next.year==='all')next.year='';if(next.year==='Unknown')next.year='unknown';if(['scope','year','type'].some(key=>next[key]!==previous[key]))history.pushState({explorerFilters:{scope:next.scope,year:next.year,type:next.type}},'',location.pathname);return next;});},[]);
 useEffect(()=>{
  detailRequest.current?.abort();setDetailLoading(false);setSelected(null);setDetail(null);setError('');setData(EMPTY);setFacets({years:[]});busy.current=false;const generation=epoch.current.reset();
  if(!privateMode){if(demo){const d=localDataset(demo.nodes,demo.edges,effective);setData({...EMPTY,core:d.nodes,edges:d.edges,total:d.totalMatching});setFacets(d.facets);}return;}
  if(!brain||expired)return;setLoading(true);busy.current=true;const signal=epoch.current.controller.signal;
  Promise.all([readJSON(apiBase+'/graph?'+query(effective),{signal}),readJSON(apiBase+'/facets?'+query(effective),{signal})]).then(([page,years])=>{if(!epoch.current.current(generation))return;setData(mergePage(EMPTY,page));setFacets(years);setLoading(false);busy.current=false;}).catch(err=>{if(epoch.current.current(generation))fail(err);});
  return()=>epoch.current.controller.abort();
 },[brain,effective,apiBase,demo,retry,expired,fail]);
 const more=async()=>{if(busy.current||!data.cursor||data.core.length+data.context.length>=500)return;busy.current=true;setLoading(true);const generation=epoch.current.value;try{const page=await readJSON(apiBase+'/graph?'+query(effective,data.cursor),{signal:epoch.current.controller.signal});if(epoch.current.current(generation)){setData(previous=>mergePage(previous,page));setLoading(false);busy.current=false;}}catch(err){if(epoch.current.current(generation))fail(err);}};
 const select=async node=>{if(typeof node==='string')node=[...data.core,...data.context].find(item=>item.id===node)||null;detailRequest.current?.abort();setDetailLoading(false);setSelected(node);setDetail(null);if(!node)return;if(!privateMode){const adjacent=(demo?.edges||[]).filter(edge=>edge.source===node.id||edge.target===node.id);const ids=new Set(adjacent.flatMap(edge=>[edge.source,edge.target]));setDetail({node,relatedNodes:(demo?.nodes||[]).filter(item=>item.id!==node.id&&ids.has(item.id)),edges:adjacent,truncated:false});return;}const controller=new AbortController();detailRequest.current=controller;setDetailLoading(true);try{const body=await readJSON(apiBase+'/nodes/'+encodeURIComponent(node.id),{signal:controller.signal});if(!controller.signal.aborted){setDetail(body);setDetailLoading(false);}}catch(err){if(!controller.signal.aborted){setDetailLoading(false);fail(err);}}};
 const switchBrain=id=>{epoch.current.reset();detailRequest.current?.abort();setDetailLoading(false);setData(EMPTY);setFacets({years:[]});setDetail(null);setSelected(null);setFilters(defaults);setBrain(id);};
 const sceneNodes=useMemo(()=>[...data.core,...data.context],[data.core,data.context]);
 const coreIds=useMemo(()=>new Set(data.core.map(node=>node.id)),[data.core]);
 if(expired)return <main className="session-message"><h1>Your session has ended</h1><p>Sign in to explore your memories.</p><a href="/login">Sign in</a></main>;
 return <Explorer nodes={sceneNodes} edges={data.edges} coreIds={coreIds} facets={facets} filters={filters} onFilter={onFilter} onMore={more} hasMore={!!data.cursor&&data.core.length+data.context.length<500} loading={loading} totalMatching={data.total} selected={selected} onSelect={select} detail={detail} detailLoading={detailLoading} error={error} mode={privateMode?'private':'demo'} brainId={brain} brainName={privateMode?(brains.find(b=>b.id===brain)?.name||'Your memories'):'Synthetic demo'} onBrainChange={switchBrain} brains={brains} onRetry={()=>setRetry(x=>x+1)} />;
}
createRoot(document.getElementById('root')).render(<App/>);
