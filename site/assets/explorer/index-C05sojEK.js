(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const l of document.querySelectorAll('link[rel="modulepreload"]'))s(l);new MutationObserver(l=>{for(const c of l)if(c.type==="childList")for(const h of c.addedNodes)h.tagName==="LINK"&&h.rel==="modulepreload"&&s(h)}).observe(document,{childList:!0,subtree:!0});function i(l){const c={};return l.integrity&&(c.integrity=l.integrity),l.referrerPolicy&&(c.referrerPolicy=l.referrerPolicy),l.crossOrigin==="use-credentials"?c.credentials="include":l.crossOrigin==="anonymous"?c.credentials="omit":c.credentials="same-origin",c}function s(l){if(l.ep)return;l.ep=!0;const c=i(l);fetch(l.href,c)}})();const xS="modulepreload",SS=function(r){return"/assets/explorer/"+r},J_={},MS=function(t,i,s){let l=Promise.resolve();if(i&&i.length>0){let h=function(p){return Promise.all(p.map(v=>Promise.resolve(v).then(g=>({status:"fulfilled",value:g}),g=>({status:"rejected",reason:g}))))};document.getElementsByTagName("link");const d=document.querySelector("meta[property=csp-nonce]"),m=(d==null?void 0:d.nonce)||(d==null?void 0:d.getAttribute("nonce"));l=h(i.map(p=>{if(p=SS(p),p in J_)return;J_[p]=!0;const v=p.endsWith(".css"),g=v?'[rel="stylesheet"]':"";if(document.querySelector(`link[href="${p}"]${g}`))return;const x=document.createElement("link");if(x.rel=v?"stylesheet":xS,v||(x.as="script"),x.crossOrigin="",x.href=p,m&&x.setAttribute("nonce",m),document.head.appendChild(x),v)return new Promise((S,b)=>{x.addEventListener("load",S),x.addEventListener("error",()=>b(new Error(`Unable to preload CSS for ${p}`)))})}))}function c(h){const d=new Event("vite:preloadError",{cancelable:!0});if(d.payload=h,window.dispatchEvent(d),!d.defaultPrevented)throw h}return l.then(h=>{for(const d of h||[])d.status==="rejected"&&c(d.reason);return t().catch(c)})};var Mh={exports:{}},Fo={};/**
 * @license React
 * react-jsx-runtime.production.js
 *
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var $_;function ES(){if($_)return Fo;$_=1;var r=Symbol.for("react.transitional.element"),t=Symbol.for("react.fragment");function i(s,l,c){var h=null;if(c!==void 0&&(h=""+c),l.key!==void 0&&(h=""+l.key),"key"in l){c={};for(var d in l)d!=="key"&&(c[d]=l[d])}else c=l;return l=c.ref,{$$typeof:r,type:s,key:h,ref:l!==void 0?l:null,props:c}}return Fo.Fragment=t,Fo.jsx=i,Fo.jsxs=i,Fo}var t0;function TS(){return t0||(t0=1,Mh.exports=ES()),Mh.exports}var j=TS(),Eh={exports:{}},he={};/**
 * @license React
 * react.production.js
 *
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var e0;function bS(){if(e0)return he;e0=1;var r=Symbol.for("react.transitional.element"),t=Symbol.for("react.portal"),i=Symbol.for("react.fragment"),s=Symbol.for("react.strict_mode"),l=Symbol.for("react.profiler"),c=Symbol.for("react.consumer"),h=Symbol.for("react.context"),d=Symbol.for("react.forward_ref"),m=Symbol.for("react.suspense"),p=Symbol.for("react.memo"),v=Symbol.for("react.lazy"),g=Symbol.for("react.activity"),x=Symbol.iterator;function S(z){return z===null||typeof z!="object"?null:(z=x&&z[x]||z["@@iterator"],typeof z=="function"?z:null)}var b={isMounted:function(){return!1},enqueueForceUpdate:function(){},enqueueReplaceState:function(){},enqueueSetState:function(){}},R=Object.assign,M={};function _(z,at,Mt){this.props=z,this.context=at,this.refs=M,this.updater=Mt||b}_.prototype.isReactComponent={},_.prototype.setState=function(z,at){if(typeof z!="object"&&typeof z!="function"&&z!=null)throw Error("takes an object of state variables to update or a function which returns an object of state variables.");this.updater.enqueueSetState(this,z,at,"setState")},_.prototype.forceUpdate=function(z){this.updater.enqueueForceUpdate(this,z,"forceUpdate")};function L(){}L.prototype=_.prototype;function U(z,at,Mt){this.props=z,this.context=at,this.refs=M,this.updater=Mt||b}var T=U.prototype=new L;T.constructor=U,R(T,_.prototype),T.isPureReactComponent=!0;var P=Array.isArray;function B(){}var F={H:null,A:null,T:null,S:null},G=Object.prototype.hasOwnProperty;function D(z,at,Mt){var wt=Mt.ref;return{$$typeof:r,type:z,key:at,ref:wt!==void 0?wt:null,props:Mt}}function w(z,at){return D(z.type,at,z.props)}function V(z){return typeof z=="object"&&z!==null&&z.$$typeof===r}function K(z){var at={"=":"=0",":":"=2"};return"$"+z.replace(/[=:]/g,function(Mt){return at[Mt]})}var ot=/\/+/g;function ct(z,at){return typeof z=="object"&&z!==null&&z.key!=null?K(""+z.key):at.toString(36)}function ht(z){switch(z.status){case"fulfilled":return z.value;case"rejected":throw z.reason;default:switch(typeof z.status=="string"?z.then(B,B):(z.status="pending",z.then(function(at){z.status==="pending"&&(z.status="fulfilled",z.value=at)},function(at){z.status==="pending"&&(z.status="rejected",z.reason=at)})),z.status){case"fulfilled":return z.value;case"rejected":throw z.reason}}throw z}function N(z,at,Mt,wt,mt){var q=typeof z;(q==="undefined"||q==="boolean")&&(z=null);var it=!1;if(z===null)it=!0;else switch(q){case"bigint":case"string":case"number":it=!0;break;case"object":switch(z.$$typeof){case r:case t:it=!0;break;case v:return it=z._init,N(it(z._payload),at,Mt,wt,mt)}}if(it)return mt=mt(z),it=wt===""?"."+ct(z,0):wt,P(mt)?(Mt="",it!=null&&(Mt=it.replace(ot,"$&/")+"/"),N(mt,at,Mt,"",function(Bt){return Bt})):mt!=null&&(V(mt)&&(mt=w(mt,Mt+(mt.key==null||z&&z.key===mt.key?"":(""+mt.key).replace(ot,"$&/")+"/")+it)),at.push(mt)),1;it=0;var xt=wt===""?".":wt+":";if(P(z))for(var Lt=0;Lt<z.length;Lt++)wt=z[Lt],q=xt+ct(wt,Lt),it+=N(wt,at,Mt,q,mt);else if(Lt=S(z),typeof Lt=="function")for(z=Lt.call(z),Lt=0;!(wt=z.next()).done;)wt=wt.value,q=xt+ct(wt,Lt++),it+=N(wt,at,Mt,q,mt);else if(q==="object"){if(typeof z.then=="function")return N(ht(z),at,Mt,wt,mt);throw at=String(z),Error("Objects are not valid as a React child (found: "+(at==="[object Object]"?"object with keys {"+Object.keys(z).join(", ")+"}":at)+"). If you meant to render a collection of children, use an array instead.")}return it}function J(z,at,Mt){if(z==null)return z;var wt=[],mt=0;return N(z,wt,"","",function(q){return at.call(Mt,q,mt++)}),wt}function Y(z){if(z._status===-1){var at=z._result;at=at(),at.then(function(Mt){(z._status===0||z._status===-1)&&(z._status=1,z._result=Mt)},function(Mt){(z._status===0||z._status===-1)&&(z._status=2,z._result=Mt)}),z._status===-1&&(z._status=0,z._result=at)}if(z._status===1)return z._result.default;throw z._result}var Tt=typeof reportError=="function"?reportError:function(z){if(typeof window=="object"&&typeof window.ErrorEvent=="function"){var at=new window.ErrorEvent("error",{bubbles:!0,cancelable:!0,message:typeof z=="object"&&z!==null&&typeof z.message=="string"?String(z.message):String(z),error:z});if(!window.dispatchEvent(at))return}else if(typeof process=="object"&&typeof process.emit=="function"){process.emit("uncaughtException",z);return}console.error(z)},Ct={map:J,forEach:function(z,at,Mt){J(z,function(){at.apply(this,arguments)},Mt)},count:function(z){var at=0;return J(z,function(){at++}),at},toArray:function(z){return J(z,function(at){return at})||[]},only:function(z){if(!V(z))throw Error("React.Children.only expected to receive a single React element child.");return z}};return he.Activity=g,he.Children=Ct,he.Component=_,he.Fragment=i,he.Profiler=l,he.PureComponent=U,he.StrictMode=s,he.Suspense=m,he.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE=F,he.__COMPILER_RUNTIME={__proto__:null,c:function(z){return F.H.useMemoCache(z)}},he.cache=function(z){return function(){return z.apply(null,arguments)}},he.cacheSignal=function(){return null},he.cloneElement=function(z,at,Mt){if(z==null)throw Error("The argument must be a React element, but you passed "+z+".");var wt=R({},z.props),mt=z.key;if(at!=null)for(q in at.key!==void 0&&(mt=""+at.key),at)!G.call(at,q)||q==="key"||q==="__self"||q==="__source"||q==="ref"&&at.ref===void 0||(wt[q]=at[q]);var q=arguments.length-2;if(q===1)wt.children=Mt;else if(1<q){for(var it=Array(q),xt=0;xt<q;xt++)it[xt]=arguments[xt+2];wt.children=it}return D(z.type,mt,wt)},he.createContext=function(z){return z={$$typeof:h,_currentValue:z,_currentValue2:z,_threadCount:0,Provider:null,Consumer:null},z.Provider=z,z.Consumer={$$typeof:c,_context:z},z},he.createElement=function(z,at,Mt){var wt,mt={},q=null;if(at!=null)for(wt in at.key!==void 0&&(q=""+at.key),at)G.call(at,wt)&&wt!=="key"&&wt!=="__self"&&wt!=="__source"&&(mt[wt]=at[wt]);var it=arguments.length-2;if(it===1)mt.children=Mt;else if(1<it){for(var xt=Array(it),Lt=0;Lt<it;Lt++)xt[Lt]=arguments[Lt+2];mt.children=xt}if(z&&z.defaultProps)for(wt in it=z.defaultProps,it)mt[wt]===void 0&&(mt[wt]=it[wt]);return D(z,q,mt)},he.createRef=function(){return{current:null}},he.forwardRef=function(z){return{$$typeof:d,render:z}},he.isValidElement=V,he.lazy=function(z){return{$$typeof:v,_payload:{_status:-1,_result:z},_init:Y}},he.memo=function(z,at){return{$$typeof:p,type:z,compare:at===void 0?null:at}},he.startTransition=function(z){var at=F.T,Mt={};F.T=Mt;try{var wt=z(),mt=F.S;mt!==null&&mt(Mt,wt),typeof wt=="object"&&wt!==null&&typeof wt.then=="function"&&wt.then(B,Tt)}catch(q){Tt(q)}finally{at!==null&&Mt.types!==null&&(at.types=Mt.types),F.T=at}},he.unstable_useCacheRefresh=function(){return F.H.useCacheRefresh()},he.use=function(z){return F.H.use(z)},he.useActionState=function(z,at,Mt){return F.H.useActionState(z,at,Mt)},he.useCallback=function(z,at){return F.H.useCallback(z,at)},he.useContext=function(z){return F.H.useContext(z)},he.useDebugValue=function(){},he.useDeferredValue=function(z,at){return F.H.useDeferredValue(z,at)},he.useEffect=function(z,at){return F.H.useEffect(z,at)},he.useEffectEvent=function(z){return F.H.useEffectEvent(z)},he.useId=function(){return F.H.useId()},he.useImperativeHandle=function(z,at,Mt){return F.H.useImperativeHandle(z,at,Mt)},he.useInsertionEffect=function(z,at){return F.H.useInsertionEffect(z,at)},he.useLayoutEffect=function(z,at){return F.H.useLayoutEffect(z,at)},he.useMemo=function(z,at){return F.H.useMemo(z,at)},he.useOptimistic=function(z,at){return F.H.useOptimistic(z,at)},he.useReducer=function(z,at,Mt){return F.H.useReducer(z,at,Mt)},he.useRef=function(z){return F.H.useRef(z)},he.useState=function(z){return F.H.useState(z)},he.useSyncExternalStore=function(z,at,Mt){return F.H.useSyncExternalStore(z,at,Mt)},he.useTransition=function(){return F.H.useTransition()},he.version="19.2.0",he}var n0;function ep(){return n0||(n0=1,Eh.exports=bS()),Eh.exports}var oe=ep(),Th={exports:{}},Ho={},bh={exports:{}},Ah={};/**
 * @license React
 * scheduler.production.js
 *
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var i0;function AS(){return i0||(i0=1,(function(r){function t(N,J){var Y=N.length;N.push(J);t:for(;0<Y;){var Tt=Y-1>>>1,Ct=N[Tt];if(0<l(Ct,J))N[Tt]=J,N[Y]=Ct,Y=Tt;else break t}}function i(N){return N.length===0?null:N[0]}function s(N){if(N.length===0)return null;var J=N[0],Y=N.pop();if(Y!==J){N[0]=Y;t:for(var Tt=0,Ct=N.length,z=Ct>>>1;Tt<z;){var at=2*(Tt+1)-1,Mt=N[at],wt=at+1,mt=N[wt];if(0>l(Mt,Y))wt<Ct&&0>l(mt,Mt)?(N[Tt]=mt,N[wt]=Y,Tt=wt):(N[Tt]=Mt,N[at]=Y,Tt=at);else if(wt<Ct&&0>l(mt,Y))N[Tt]=mt,N[wt]=Y,Tt=wt;else break t}}return J}function l(N,J){var Y=N.sortIndex-J.sortIndex;return Y!==0?Y:N.id-J.id}if(r.unstable_now=void 0,typeof performance=="object"&&typeof performance.now=="function"){var c=performance;r.unstable_now=function(){return c.now()}}else{var h=Date,d=h.now();r.unstable_now=function(){return h.now()-d}}var m=[],p=[],v=1,g=null,x=3,S=!1,b=!1,R=!1,M=!1,_=typeof setTimeout=="function"?setTimeout:null,L=typeof clearTimeout=="function"?clearTimeout:null,U=typeof setImmediate<"u"?setImmediate:null;function T(N){for(var J=i(p);J!==null;){if(J.callback===null)s(p);else if(J.startTime<=N)s(p),J.sortIndex=J.expirationTime,t(m,J);else break;J=i(p)}}function P(N){if(R=!1,T(N),!b)if(i(m)!==null)b=!0,B||(B=!0,K());else{var J=i(p);J!==null&&ht(P,J.startTime-N)}}var B=!1,F=-1,G=5,D=-1;function w(){return M?!0:!(r.unstable_now()-D<G)}function V(){if(M=!1,B){var N=r.unstable_now();D=N;var J=!0;try{t:{b=!1,R&&(R=!1,L(F),F=-1),S=!0;var Y=x;try{e:{for(T(N),g=i(m);g!==null&&!(g.expirationTime>N&&w());){var Tt=g.callback;if(typeof Tt=="function"){g.callback=null,x=g.priorityLevel;var Ct=Tt(g.expirationTime<=N);if(N=r.unstable_now(),typeof Ct=="function"){g.callback=Ct,T(N),J=!0;break e}g===i(m)&&s(m),T(N)}else s(m);g=i(m)}if(g!==null)J=!0;else{var z=i(p);z!==null&&ht(P,z.startTime-N),J=!1}}break t}finally{g=null,x=Y,S=!1}J=void 0}}finally{J?K():B=!1}}}var K;if(typeof U=="function")K=function(){U(V)};else if(typeof MessageChannel<"u"){var ot=new MessageChannel,ct=ot.port2;ot.port1.onmessage=V,K=function(){ct.postMessage(null)}}else K=function(){_(V,0)};function ht(N,J){F=_(function(){N(r.unstable_now())},J)}r.unstable_IdlePriority=5,r.unstable_ImmediatePriority=1,r.unstable_LowPriority=4,r.unstable_NormalPriority=3,r.unstable_Profiling=null,r.unstable_UserBlockingPriority=2,r.unstable_cancelCallback=function(N){N.callback=null},r.unstable_forceFrameRate=function(N){0>N||125<N?console.error("forceFrameRate takes a positive int between 0 and 125, forcing frame rates higher than 125 fps is not supported"):G=0<N?Math.floor(1e3/N):5},r.unstable_getCurrentPriorityLevel=function(){return x},r.unstable_next=function(N){switch(x){case 1:case 2:case 3:var J=3;break;default:J=x}var Y=x;x=J;try{return N()}finally{x=Y}},r.unstable_requestPaint=function(){M=!0},r.unstable_runWithPriority=function(N,J){switch(N){case 1:case 2:case 3:case 4:case 5:break;default:N=3}var Y=x;x=N;try{return J()}finally{x=Y}},r.unstable_scheduleCallback=function(N,J,Y){var Tt=r.unstable_now();switch(typeof Y=="object"&&Y!==null?(Y=Y.delay,Y=typeof Y=="number"&&0<Y?Tt+Y:Tt):Y=Tt,N){case 1:var Ct=-1;break;case 2:Ct=250;break;case 5:Ct=1073741823;break;case 4:Ct=1e4;break;default:Ct=5e3}return Ct=Y+Ct,N={id:v++,callback:J,priorityLevel:N,startTime:Y,expirationTime:Ct,sortIndex:-1},Y>Tt?(N.sortIndex=Y,t(p,N),i(m)===null&&N===i(p)&&(R?(L(F),F=-1):R=!0,ht(P,Y-Tt))):(N.sortIndex=Ct,t(m,N),b||S||(b=!0,B||(B=!0,K()))),N},r.unstable_shouldYield=w,r.unstable_wrapCallback=function(N){var J=x;return function(){var Y=x;x=J;try{return N.apply(this,arguments)}finally{x=Y}}}})(Ah)),Ah}var a0;function RS(){return a0||(a0=1,bh.exports=AS()),bh.exports}var Rh={exports:{}},Un={};/**
 * @license React
 * react-dom.production.js
 *
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var s0;function CS(){if(s0)return Un;s0=1;var r=ep();function t(m){var p="https://react.dev/errors/"+m;if(1<arguments.length){p+="?args[]="+encodeURIComponent(arguments[1]);for(var v=2;v<arguments.length;v++)p+="&args[]="+encodeURIComponent(arguments[v])}return"Minified React error #"+m+"; visit "+p+" for the full message or use the non-minified dev environment for full errors and additional helpful warnings."}function i(){}var s={d:{f:i,r:function(){throw Error(t(522))},D:i,C:i,L:i,m:i,X:i,S:i,M:i},p:0,findDOMNode:null},l=Symbol.for("react.portal");function c(m,p,v){var g=3<arguments.length&&arguments[3]!==void 0?arguments[3]:null;return{$$typeof:l,key:g==null?null:""+g,children:m,containerInfo:p,implementation:v}}var h=r.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;function d(m,p){if(m==="font")return"";if(typeof p=="string")return p==="use-credentials"?p:""}return Un.__DOM_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE=s,Un.createPortal=function(m,p){var v=2<arguments.length&&arguments[2]!==void 0?arguments[2]:null;if(!p||p.nodeType!==1&&p.nodeType!==9&&p.nodeType!==11)throw Error(t(299));return c(m,p,null,v)},Un.flushSync=function(m){var p=h.T,v=s.p;try{if(h.T=null,s.p=2,m)return m()}finally{h.T=p,s.p=v,s.d.f()}},Un.preconnect=function(m,p){typeof m=="string"&&(p?(p=p.crossOrigin,p=typeof p=="string"?p==="use-credentials"?p:"":void 0):p=null,s.d.C(m,p))},Un.prefetchDNS=function(m){typeof m=="string"&&s.d.D(m)},Un.preinit=function(m,p){if(typeof m=="string"&&p&&typeof p.as=="string"){var v=p.as,g=d(v,p.crossOrigin),x=typeof p.integrity=="string"?p.integrity:void 0,S=typeof p.fetchPriority=="string"?p.fetchPriority:void 0;v==="style"?s.d.S(m,typeof p.precedence=="string"?p.precedence:void 0,{crossOrigin:g,integrity:x,fetchPriority:S}):v==="script"&&s.d.X(m,{crossOrigin:g,integrity:x,fetchPriority:S,nonce:typeof p.nonce=="string"?p.nonce:void 0})}},Un.preinitModule=function(m,p){if(typeof m=="string")if(typeof p=="object"&&p!==null){if(p.as==null||p.as==="script"){var v=d(p.as,p.crossOrigin);s.d.M(m,{crossOrigin:v,integrity:typeof p.integrity=="string"?p.integrity:void 0,nonce:typeof p.nonce=="string"?p.nonce:void 0})}}else p==null&&s.d.M(m)},Un.preload=function(m,p){if(typeof m=="string"&&typeof p=="object"&&p!==null&&typeof p.as=="string"){var v=p.as,g=d(v,p.crossOrigin);s.d.L(m,v,{crossOrigin:g,integrity:typeof p.integrity=="string"?p.integrity:void 0,nonce:typeof p.nonce=="string"?p.nonce:void 0,type:typeof p.type=="string"?p.type:void 0,fetchPriority:typeof p.fetchPriority=="string"?p.fetchPriority:void 0,referrerPolicy:typeof p.referrerPolicy=="string"?p.referrerPolicy:void 0,imageSrcSet:typeof p.imageSrcSet=="string"?p.imageSrcSet:void 0,imageSizes:typeof p.imageSizes=="string"?p.imageSizes:void 0,media:typeof p.media=="string"?p.media:void 0})}},Un.preloadModule=function(m,p){if(typeof m=="string")if(p){var v=d(p.as,p.crossOrigin);s.d.m(m,{as:typeof p.as=="string"&&p.as!=="script"?p.as:void 0,crossOrigin:v,integrity:typeof p.integrity=="string"?p.integrity:void 0})}else s.d.m(m)},Un.requestFormReset=function(m){s.d.r(m)},Un.unstable_batchedUpdates=function(m,p){return m(p)},Un.useFormState=function(m,p,v){return h.H.useFormState(m,p,v)},Un.useFormStatus=function(){return h.H.useHostTransitionStatus()},Un.version="19.2.0",Un}var r0;function wS(){if(r0)return Rh.exports;r0=1;function r(){if(!(typeof __REACT_DEVTOOLS_GLOBAL_HOOK__>"u"||typeof __REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE!="function"))try{__REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE(r)}catch(t){console.error(t)}}return r(),Rh.exports=CS(),Rh.exports}/**
 * @license React
 * react-dom-client.production.js
 *
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */var o0;function DS(){if(o0)return Ho;o0=1;var r=RS(),t=ep(),i=wS();function s(e){var n="https://react.dev/errors/"+e;if(1<arguments.length){n+="?args[]="+encodeURIComponent(arguments[1]);for(var a=2;a<arguments.length;a++)n+="&args[]="+encodeURIComponent(arguments[a])}return"Minified React error #"+e+"; visit "+n+" for the full message or use the non-minified dev environment for full errors and additional helpful warnings."}function l(e){return!(!e||e.nodeType!==1&&e.nodeType!==9&&e.nodeType!==11)}function c(e){var n=e,a=e;if(e.alternate)for(;n.return;)n=n.return;else{e=n;do n=e,(n.flags&4098)!==0&&(a=n.return),e=n.return;while(e)}return n.tag===3?a:null}function h(e){if(e.tag===13){var n=e.memoizedState;if(n===null&&(e=e.alternate,e!==null&&(n=e.memoizedState)),n!==null)return n.dehydrated}return null}function d(e){if(e.tag===31){var n=e.memoizedState;if(n===null&&(e=e.alternate,e!==null&&(n=e.memoizedState)),n!==null)return n.dehydrated}return null}function m(e){if(c(e)!==e)throw Error(s(188))}function p(e){var n=e.alternate;if(!n){if(n=c(e),n===null)throw Error(s(188));return n!==e?null:e}for(var a=e,o=n;;){var u=a.return;if(u===null)break;var f=u.alternate;if(f===null){if(o=u.return,o!==null){a=o;continue}break}if(u.child===f.child){for(f=u.child;f;){if(f===a)return m(u),e;if(f===o)return m(u),n;f=f.sibling}throw Error(s(188))}if(a.return!==o.return)a=u,o=f;else{for(var y=!1,A=u.child;A;){if(A===a){y=!0,a=u,o=f;break}if(A===o){y=!0,o=u,a=f;break}A=A.sibling}if(!y){for(A=f.child;A;){if(A===a){y=!0,a=f,o=u;break}if(A===o){y=!0,o=f,a=u;break}A=A.sibling}if(!y)throw Error(s(189))}}if(a.alternate!==o)throw Error(s(190))}if(a.tag!==3)throw Error(s(188));return a.stateNode.current===a?e:n}function v(e){var n=e.tag;if(n===5||n===26||n===27||n===6)return e;for(e=e.child;e!==null;){if(n=v(e),n!==null)return n;e=e.sibling}return null}var g=Object.assign,x=Symbol.for("react.element"),S=Symbol.for("react.transitional.element"),b=Symbol.for("react.portal"),R=Symbol.for("react.fragment"),M=Symbol.for("react.strict_mode"),_=Symbol.for("react.profiler"),L=Symbol.for("react.consumer"),U=Symbol.for("react.context"),T=Symbol.for("react.forward_ref"),P=Symbol.for("react.suspense"),B=Symbol.for("react.suspense_list"),F=Symbol.for("react.memo"),G=Symbol.for("react.lazy"),D=Symbol.for("react.activity"),w=Symbol.for("react.memo_cache_sentinel"),V=Symbol.iterator;function K(e){return e===null||typeof e!="object"?null:(e=V&&e[V]||e["@@iterator"],typeof e=="function"?e:null)}var ot=Symbol.for("react.client.reference");function ct(e){if(e==null)return null;if(typeof e=="function")return e.$$typeof===ot?null:e.displayName||e.name||null;if(typeof e=="string")return e;switch(e){case R:return"Fragment";case _:return"Profiler";case M:return"StrictMode";case P:return"Suspense";case B:return"SuspenseList";case D:return"Activity"}if(typeof e=="object")switch(e.$$typeof){case b:return"Portal";case U:return e.displayName||"Context";case L:return(e._context.displayName||"Context")+".Consumer";case T:var n=e.render;return e=e.displayName,e||(e=n.displayName||n.name||"",e=e!==""?"ForwardRef("+e+")":"ForwardRef"),e;case F:return n=e.displayName||null,n!==null?n:ct(e.type)||"Memo";case G:n=e._payload,e=e._init;try{return ct(e(n))}catch{}}return null}var ht=Array.isArray,N=t.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE,J=i.__DOM_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE,Y={pending:!1,data:null,method:null,action:null},Tt=[],Ct=-1;function z(e){return{current:e}}function at(e){0>Ct||(e.current=Tt[Ct],Tt[Ct]=null,Ct--)}function Mt(e,n){Ct++,Tt[Ct]=e.current,e.current=n}var wt=z(null),mt=z(null),q=z(null),it=z(null);function xt(e,n){switch(Mt(q,n),Mt(mt,e),Mt(wt,null),n.nodeType){case 9:case 11:e=(e=n.documentElement)&&(e=e.namespaceURI)?S_(e):0;break;default:if(e=n.tagName,n=n.namespaceURI)n=S_(n),e=M_(n,e);else switch(e){case"svg":e=1;break;case"math":e=2;break;default:e=0}}at(wt),Mt(wt,e)}function Lt(){at(wt),at(mt),at(q)}function Bt(e){e.memoizedState!==null&&Mt(it,e);var n=wt.current,a=M_(n,e.type);n!==a&&(Mt(mt,e),Mt(wt,a))}function le(e){mt.current===e&&(at(wt),at(mt)),it.current===e&&(at(it),Po._currentValue=Y)}var ft,I;function jt(e){if(ft===void 0)try{throw Error()}catch(a){var n=a.stack.trim().match(/\n( *(at )?)/);ft=n&&n[1]||"",I=-1<a.stack.indexOf(`
    at`)?" (<anonymous>)":-1<a.stack.indexOf("@")?"@unknown:0:0":""}return`
`+ft+e+I}var Kt=!1;function Qt(e,n){if(!e||Kt)return"";Kt=!0;var a=Error.prepareStackTrace;Error.prepareStackTrace=void 0;try{var o={DetermineComponentFrameRoot:function(){try{if(n){var St=function(){throw Error()};if(Object.defineProperty(St.prototype,"props",{set:function(){throw Error()}}),typeof Reflect=="object"&&Reflect.construct){try{Reflect.construct(St,[])}catch(ut){var st=ut}Reflect.construct(e,[],St)}else{try{St.call()}catch(ut){st=ut}e.call(St.prototype)}}else{try{throw Error()}catch(ut){st=ut}(St=e())&&typeof St.catch=="function"&&St.catch(function(){})}}catch(ut){if(ut&&st&&typeof ut.stack=="string")return[ut.stack,st.stack]}return[null,null]}};o.DetermineComponentFrameRoot.displayName="DetermineComponentFrameRoot";var u=Object.getOwnPropertyDescriptor(o.DetermineComponentFrameRoot,"name");u&&u.configurable&&Object.defineProperty(o.DetermineComponentFrameRoot,"name",{value:"DetermineComponentFrameRoot"});var f=o.DetermineComponentFrameRoot(),y=f[0],A=f[1];if(y&&A){var H=y.split(`
`),nt=A.split(`
`);for(u=o=0;o<H.length&&!H[o].includes("DetermineComponentFrameRoot");)o++;for(;u<nt.length&&!nt[u].includes("DetermineComponentFrameRoot");)u++;if(o===H.length||u===nt.length)for(o=H.length-1,u=nt.length-1;1<=o&&0<=u&&H[o]!==nt[u];)u--;for(;1<=o&&0<=u;o--,u--)if(H[o]!==nt[u]){if(o!==1||u!==1)do if(o--,u--,0>u||H[o]!==nt[u]){var vt=`
`+H[o].replace(" at new "," at ");return e.displayName&&vt.includes("<anonymous>")&&(vt=vt.replace("<anonymous>",e.displayName)),vt}while(1<=o&&0<=u);break}}}finally{Kt=!1,Error.prepareStackTrace=a}return(a=e?e.displayName||e.name:"")?jt(a):""}function Xt(e,n){switch(e.tag){case 26:case 27:case 5:return jt(e.type);case 16:return jt("Lazy");case 13:return e.child!==n&&n!==null?jt("Suspense Fallback"):jt("Suspense");case 19:return jt("SuspenseList");case 0:case 15:return Qt(e.type,!1);case 11:return Qt(e.type.render,!1);case 1:return Qt(e.type,!0);case 31:return jt("Activity");default:return""}}function Ee(e){try{var n="",a=null;do n+=Xt(e,a),a=e,e=e.return;while(e);return n}catch(o){return`
Error generating stack: `+o.message+`
`+o.stack}}var Dt=Object.prototype.hasOwnProperty,Ht=r.unstable_scheduleCallback,xe=r.unstable_cancelCallback,Se=r.unstable_shouldYield,O=r.unstable_requestPaint,E=r.unstable_now,tt=r.unstable_getCurrentPriorityLevel,gt=r.unstable_ImmediatePriority,At=r.unstable_UserBlockingPriority,dt=r.unstable_NormalPriority,Jt=r.unstable_LowPriority,Nt=r.unstable_IdlePriority,Wt=r.log,$t=r.unstable_setDisableYieldValue,Ut=null,Ot=null;function ee(e){if(typeof Wt=="function"&&$t(e),Ot&&typeof Ot.setStrictMode=="function")try{Ot.setStrictMode(Ut,e)}catch{}}var It=Math.clz32?Math.clz32:W,Ft=Math.log,fe=Math.LN2;function W(e){return e>>>=0,e===0?32:31-(Ft(e)/fe|0)|0}var _t=256,Rt=262144,Pt=4194304;function Et(e){var n=e&42;if(n!==0)return n;switch(e&-e){case 1:return 1;case 2:return 2;case 4:return 4;case 8:return 8;case 16:return 16;case 32:return 32;case 64:return 64;case 128:return 128;case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:return e&261888;case 262144:case 524288:case 1048576:case 2097152:return e&3932160;case 4194304:case 8388608:case 16777216:case 33554432:return e&62914560;case 67108864:return 67108864;case 134217728:return 134217728;case 268435456:return 268435456;case 536870912:return 536870912;case 1073741824:return 0;default:return e}}function pt(e,n,a){var o=e.pendingLanes;if(o===0)return 0;var u=0,f=e.suspendedLanes,y=e.pingedLanes;e=e.warmLanes;var A=o&134217727;return A!==0?(o=A&~f,o!==0?u=Et(o):(y&=A,y!==0?u=Et(y):a||(a=A&~e,a!==0&&(u=Et(a))))):(A=o&~f,A!==0?u=Et(A):y!==0?u=Et(y):a||(a=o&~e,a!==0&&(u=Et(a)))),u===0?0:n!==0&&n!==u&&(n&f)===0&&(f=u&-u,a=n&-n,f>=a||f===32&&(a&4194048)!==0)?n:u}function zt(e,n){return(e.pendingLanes&~(e.suspendedLanes&~e.pingedLanes)&n)===0}function ne(e,n){switch(e){case 1:case 2:case 4:case 8:case 64:return n+250;case 16:case 32:case 128:case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:return n+5e3;case 4194304:case 8388608:case 16777216:case 33554432:return-1;case 67108864:case 134217728:case 268435456:case 536870912:case 1073741824:return-1;default:return-1}}function _e(){var e=Pt;return Pt<<=1,(Pt&62914560)===0&&(Pt=4194304),e}function Re(e){for(var n=[],a=0;31>a;a++)n.push(e);return n}function En(e,n){e.pendingLanes|=n,n!==268435456&&(e.suspendedLanes=0,e.pingedLanes=0,e.warmLanes=0)}function oi(e,n,a,o,u,f){var y=e.pendingLanes;e.pendingLanes=a,e.suspendedLanes=0,e.pingedLanes=0,e.warmLanes=0,e.expiredLanes&=a,e.entangledLanes&=a,e.errorRecoveryDisabledLanes&=a,e.shellSuspendCounter=0;var A=e.entanglements,H=e.expirationTimes,nt=e.hiddenUpdates;for(a=y&~a;0<a;){var vt=31-It(a),St=1<<vt;A[vt]=0,H[vt]=-1;var st=nt[vt];if(st!==null)for(nt[vt]=null,vt=0;vt<st.length;vt++){var ut=st[vt];ut!==null&&(ut.lane&=-536870913)}a&=~St}o!==0&&qr(e,o,0),f!==0&&u===0&&e.tag!==0&&(e.suspendedLanes|=f&~(y&~n))}function qr(e,n,a){e.pendingLanes|=n,e.suspendedLanes&=~n;var o=31-It(n);e.entangledLanes|=n,e.entanglements[o]=e.entanglements[o]|1073741824|a&261930}function Ci(e,n){var a=e.entangledLanes|=n;for(e=e.entanglements;a;){var o=31-It(a),u=1<<o;u&n|e[o]&n&&(e[o]|=n),a&=~u}}function Ns(e,n){var a=n&-n;return a=(a&42)!==0?1:Os(a),(a&(e.suspendedLanes|n))!==0?0:a}function Os(e){switch(e){case 2:e=1;break;case 8:e=4;break;case 32:e=16;break;case 256:case 512:case 1024:case 2048:case 4096:case 8192:case 16384:case 32768:case 65536:case 131072:case 262144:case 524288:case 1048576:case 2097152:case 4194304:case 8388608:case 16777216:case 33554432:e=128;break;case 268435456:e=134217728;break;default:e=0}return e}function Ps(e){return e&=-e,2<e?8<e?(e&134217727)!==0?32:268435456:8:2}function Ka(){var e=J.p;return e!==0?e:(e=window.event,e===void 0?32:W_(e.type))}function Zr(e,n){var a=J.p;try{return J.p=e,n()}finally{J.p=a}}var qn=Math.random().toString(36).slice(2),on="__reactFiber$"+qn,Tn="__reactProps$"+qn,pa="__reactContainer$"+qn,Kr="__reactEvents$"+qn,mu="__reactListeners$"+qn,gu="__reactHandles$"+qn,ul="__reactResources$"+qn,Qa="__reactMarker$"+qn;function C(e){delete e[on],delete e[Tn],delete e[Kr],delete e[mu],delete e[gu]}function Z(e){var n=e[on];if(n)return n;for(var a=e.parentNode;a;){if(n=a[pa]||a[on]){if(a=n.alternate,n.child!==null||a!==null&&a.child!==null)for(e=w_(e);e!==null;){if(a=e[on])return a;e=w_(e)}return n}e=a,a=e.parentNode}return null}function rt(e){if(e=e[on]||e[pa]){var n=e.tag;if(n===5||n===6||n===13||n===31||n===26||n===27||n===3)return e}return null}function lt(e){var n=e.tag;if(n===5||n===26||n===27||n===6)return e.stateNode;throw Error(s(33))}function $(e){var n=e[ul];return n||(n=e[ul]={hoistableStyles:new Map,hoistableScripts:new Map}),n}function bt(e){e[Qa]=!0}var Gt=new Set,qt={};function kt(e,n){ie(e,n),ie(e+"Capture",n)}function ie(e,n){for(qt[e]=n,e=0;e<n.length;e++)Gt.add(n[e])}var ue=RegExp("^[:A-Z_a-z\\u00C0-\\u00D6\\u00D8-\\u00F6\\u00F8-\\u02FF\\u0370-\\u037D\\u037F-\\u1FFF\\u200C-\\u200D\\u2070-\\u218F\\u2C00-\\u2FEF\\u3001-\\uD7FF\\uF900-\\uFDCF\\uFDF0-\\uFFFD][:A-Z_a-z\\u00C0-\\u00D6\\u00D8-\\u00F6\\u00F8-\\u02FF\\u0370-\\u037D\\u037F-\\u1FFF\\u200C-\\u200D\\u2070-\\u218F\\u2C00-\\u2FEF\\u3001-\\uD7FF\\uF900-\\uFDCF\\uFDF0-\\uFFFD\\-.0-9\\u00B7\\u0300-\\u036F\\u203F-\\u2040]*$"),ae={},ve={};function Be(e){return Dt.call(ve,e)?!0:Dt.call(ae,e)?!1:ue.test(e)?ve[e]=!0:(ae[e]=!0,!1)}function Ze(e,n,a){if(Be(n))if(a===null)e.removeAttribute(n);else{switch(typeof a){case"undefined":case"function":case"symbol":e.removeAttribute(n);return;case"boolean":var o=n.toLowerCase().slice(0,5);if(o!=="data-"&&o!=="aria-"){e.removeAttribute(n);return}}e.setAttribute(n,""+a)}}function Ie(e,n,a){if(a===null)e.removeAttribute(n);else{switch(typeof a){case"undefined":case"function":case"symbol":case"boolean":e.removeAttribute(n);return}e.setAttribute(n,""+a)}}function Te(e,n,a,o){if(o===null)e.removeAttribute(a);else{switch(typeof o){case"undefined":case"function":case"symbol":case"boolean":e.removeAttribute(a);return}e.setAttributeNS(n,a,""+o)}}function Zt(e){switch(typeof e){case"bigint":case"boolean":case"number":case"string":case"undefined":return e;case"object":return e;default:return""}}function Qe(e){var n=e.type;return(e=e.nodeName)&&e.toLowerCase()==="input"&&(n==="checkbox"||n==="radio")}function Le(e,n,a){var o=Object.getOwnPropertyDescriptor(e.constructor.prototype,n);if(!e.hasOwnProperty(n)&&typeof o<"u"&&typeof o.get=="function"&&typeof o.set=="function"){var u=o.get,f=o.set;return Object.defineProperty(e,n,{configurable:!0,get:function(){return u.call(this)},set:function(y){a=""+y,f.call(this,y)}}),Object.defineProperty(e,n,{enumerable:o.enumerable}),{getValue:function(){return a},setValue:function(y){a=""+y},stopTracking:function(){e._valueTracker=null,delete e[n]}}}}function bn(e){if(!e._valueTracker){var n=Qe(e)?"checked":"value";e._valueTracker=Le(e,n,""+e[n])}}function Hi(e){if(!e)return!1;var n=e._valueTracker;if(!n)return!0;var a=n.getValue(),o="";return e&&(o=Qe(e)?e.checked?"true":"false":e.value),e=o,e!==a?(n.setValue(e),!0):!1}function yn(e){if(e=e||(typeof document<"u"?document:void 0),typeof e>"u")return null;try{return e.activeElement||e.body}catch{return e.body}}var Ja=/[\n"\\]/g;function Ce(e){return e.replace(Ja,function(n){return"\\"+n.charCodeAt(0).toString(16)+" "})}function Dn(e,n,a,o,u,f,y,A){e.name="",y!=null&&typeof y!="function"&&typeof y!="symbol"&&typeof y!="boolean"?e.type=y:e.removeAttribute("type"),n!=null?y==="number"?(n===0&&e.value===""||e.value!=n)&&(e.value=""+Zt(n)):e.value!==""+Zt(n)&&(e.value=""+Zt(n)):y!=="submit"&&y!=="reset"||e.removeAttribute("value"),n!=null?pn(e,y,Zt(n)):a!=null?pn(e,y,Zt(a)):o!=null&&e.removeAttribute("value"),u==null&&f!=null&&(e.defaultChecked=!!f),u!=null&&(e.checked=u&&typeof u!="function"&&typeof u!="symbol"),A!=null&&typeof A!="function"&&typeof A!="symbol"&&typeof A!="boolean"?e.name=""+Zt(A):e.removeAttribute("name")}function Pn(e,n,a,o,u,f,y,A){if(f!=null&&typeof f!="function"&&typeof f!="symbol"&&typeof f!="boolean"&&(e.type=f),n!=null||a!=null){if(!(f!=="submit"&&f!=="reset"||n!=null)){bn(e);return}a=a!=null?""+Zt(a):"",n=n!=null?""+Zt(n):a,A||n===e.value||(e.value=n),e.defaultValue=n}o=o??u,o=typeof o!="function"&&typeof o!="symbol"&&!!o,e.checked=A?e.checked:!!o,e.defaultChecked=!!o,y!=null&&typeof y!="function"&&typeof y!="symbol"&&typeof y!="boolean"&&(e.name=y),bn(e)}function pn(e,n,a){n==="number"&&yn(e.ownerDocument)===e||e.defaultValue===""+a||(e.defaultValue=""+a)}function an(e,n,a,o){if(e=e.options,n){n={};for(var u=0;u<a.length;u++)n["$"+a[u]]=!0;for(a=0;a<e.length;a++)u=n.hasOwnProperty("$"+e[a].value),e[a].selected!==u&&(e[a].selected=u),u&&o&&(e[a].defaultSelected=!0)}else{for(a=""+Zt(a),n=null,u=0;u<e.length;u++){if(e[u].value===a){e[u].selected=!0,o&&(e[u].defaultSelected=!0);return}n!==null||e[u].disabled||(n=e[u])}n!==null&&(n.selected=!0)}}function zs(e,n,a){if(n!=null&&(n=""+Zt(n),n!==e.value&&(e.value=n),a==null)){e.defaultValue!==n&&(e.defaultValue=n);return}e.defaultValue=a!=null?""+Zt(a):""}function wi(e,n,a,o){if(n==null){if(o!=null){if(a!=null)throw Error(s(92));if(ht(o)){if(1<o.length)throw Error(s(93));o=o[0]}a=o}a==null&&(a=""),n=a}a=Zt(n),e.defaultValue=a,o=e.textContent,o===a&&o!==""&&o!==null&&(e.value=o),bn(e)}function Bs(e,n){if(n){var a=e.firstChild;if(a&&a===e.lastChild&&a.nodeType===3){a.nodeValue=n;return}}e.textContent=n}var my=new Set("animationIterationCount aspectRatio borderImageOutset borderImageSlice borderImageWidth boxFlex boxFlexGroup boxOrdinalGroup columnCount columns flex flexGrow flexPositive flexShrink flexNegative flexOrder gridArea gridRow gridRowEnd gridRowSpan gridRowStart gridColumn gridColumnEnd gridColumnSpan gridColumnStart fontWeight lineClamp lineHeight opacity order orphans scale tabSize widows zIndex zoom fillOpacity floodOpacity stopOpacity strokeDasharray strokeDashoffset strokeMiterlimit strokeOpacity strokeWidth MozAnimationIterationCount MozBoxFlex MozBoxFlexGroup MozLineClamp msAnimationIterationCount msFlex msZoom msFlexGrow msFlexNegative msFlexOrder msFlexPositive msFlexShrink msGridColumn msGridColumnSpan msGridRow msGridRowSpan WebkitAnimationIterationCount WebkitBoxFlex WebKitBoxFlexGroup WebkitBoxOrdinalGroup WebkitColumnCount WebkitColumns WebkitFlex WebkitFlexGrow WebkitFlexPositive WebkitFlexShrink WebkitLineClamp".split(" "));function vp(e,n,a){var o=n.indexOf("--")===0;a==null||typeof a=="boolean"||a===""?o?e.setProperty(n,""):n==="float"?e.cssFloat="":e[n]="":o?e.setProperty(n,a):typeof a!="number"||a===0||my.has(n)?n==="float"?e.cssFloat=a:e[n]=(""+a).trim():e[n]=a+"px"}function yp(e,n,a){if(n!=null&&typeof n!="object")throw Error(s(62));if(e=e.style,a!=null){for(var o in a)!a.hasOwnProperty(o)||n!=null&&n.hasOwnProperty(o)||(o.indexOf("--")===0?e.setProperty(o,""):o==="float"?e.cssFloat="":e[o]="");for(var u in n)o=n[u],n.hasOwnProperty(u)&&a[u]!==o&&vp(e,u,o)}else for(var f in n)n.hasOwnProperty(f)&&vp(e,f,n[f])}function _u(e){if(e.indexOf("-")===-1)return!1;switch(e){case"annotation-xml":case"color-profile":case"font-face":case"font-face-src":case"font-face-uri":case"font-face-format":case"font-face-name":case"missing-glyph":return!1;default:return!0}}var gy=new Map([["acceptCharset","accept-charset"],["htmlFor","for"],["httpEquiv","http-equiv"],["crossOrigin","crossorigin"],["accentHeight","accent-height"],["alignmentBaseline","alignment-baseline"],["arabicForm","arabic-form"],["baselineShift","baseline-shift"],["capHeight","cap-height"],["clipPath","clip-path"],["clipRule","clip-rule"],["colorInterpolation","color-interpolation"],["colorInterpolationFilters","color-interpolation-filters"],["colorProfile","color-profile"],["colorRendering","color-rendering"],["dominantBaseline","dominant-baseline"],["enableBackground","enable-background"],["fillOpacity","fill-opacity"],["fillRule","fill-rule"],["floodColor","flood-color"],["floodOpacity","flood-opacity"],["fontFamily","font-family"],["fontSize","font-size"],["fontSizeAdjust","font-size-adjust"],["fontStretch","font-stretch"],["fontStyle","font-style"],["fontVariant","font-variant"],["fontWeight","font-weight"],["glyphName","glyph-name"],["glyphOrientationHorizontal","glyph-orientation-horizontal"],["glyphOrientationVertical","glyph-orientation-vertical"],["horizAdvX","horiz-adv-x"],["horizOriginX","horiz-origin-x"],["imageRendering","image-rendering"],["letterSpacing","letter-spacing"],["lightingColor","lighting-color"],["markerEnd","marker-end"],["markerMid","marker-mid"],["markerStart","marker-start"],["overlinePosition","overline-position"],["overlineThickness","overline-thickness"],["paintOrder","paint-order"],["panose-1","panose-1"],["pointerEvents","pointer-events"],["renderingIntent","rendering-intent"],["shapeRendering","shape-rendering"],["stopColor","stop-color"],["stopOpacity","stop-opacity"],["strikethroughPosition","strikethrough-position"],["strikethroughThickness","strikethrough-thickness"],["strokeDasharray","stroke-dasharray"],["strokeDashoffset","stroke-dashoffset"],["strokeLinecap","stroke-linecap"],["strokeLinejoin","stroke-linejoin"],["strokeMiterlimit","stroke-miterlimit"],["strokeOpacity","stroke-opacity"],["strokeWidth","stroke-width"],["textAnchor","text-anchor"],["textDecoration","text-decoration"],["textRendering","text-rendering"],["transformOrigin","transform-origin"],["underlinePosition","underline-position"],["underlineThickness","underline-thickness"],["unicodeBidi","unicode-bidi"],["unicodeRange","unicode-range"],["unitsPerEm","units-per-em"],["vAlphabetic","v-alphabetic"],["vHanging","v-hanging"],["vIdeographic","v-ideographic"],["vMathematical","v-mathematical"],["vectorEffect","vector-effect"],["vertAdvY","vert-adv-y"],["vertOriginX","vert-origin-x"],["vertOriginY","vert-origin-y"],["wordSpacing","word-spacing"],["writingMode","writing-mode"],["xmlnsXlink","xmlns:xlink"],["xHeight","x-height"]]),_y=/^[\u0000-\u001F ]*j[\r\n\t]*a[\r\n\t]*v[\r\n\t]*a[\r\n\t]*s[\r\n\t]*c[\r\n\t]*r[\r\n\t]*i[\r\n\t]*p[\r\n\t]*t[\r\n\t]*:/i;function fl(e){return _y.test(""+e)?"javascript:throw new Error('React has blocked a javascript: URL as a security precaution.')":e}function Gi(){}var vu=null;function yu(e){return e=e.target||e.srcElement||window,e.correspondingUseElement&&(e=e.correspondingUseElement),e.nodeType===3?e.parentNode:e}var Is=null,Fs=null;function xp(e){var n=rt(e);if(n&&(e=n.stateNode)){var a=e[Tn]||null;t:switch(e=n.stateNode,n.type){case"input":if(Dn(e,a.value,a.defaultValue,a.defaultValue,a.checked,a.defaultChecked,a.type,a.name),n=a.name,a.type==="radio"&&n!=null){for(a=e;a.parentNode;)a=a.parentNode;for(a=a.querySelectorAll('input[name="'+Ce(""+n)+'"][type="radio"]'),n=0;n<a.length;n++){var o=a[n];if(o!==e&&o.form===e.form){var u=o[Tn]||null;if(!u)throw Error(s(90));Dn(o,u.value,u.defaultValue,u.defaultValue,u.checked,u.defaultChecked,u.type,u.name)}}for(n=0;n<a.length;n++)o=a[n],o.form===e.form&&Hi(o)}break t;case"textarea":zs(e,a.value,a.defaultValue);break t;case"select":n=a.value,n!=null&&an(e,!!a.multiple,n,!1)}}}var xu=!1;function Sp(e,n,a){if(xu)return e(n,a);xu=!0;try{var o=e(n);return o}finally{if(xu=!1,(Is!==null||Fs!==null)&&(Jl(),Is&&(n=Is,e=Fs,Fs=Is=null,xp(n),e)))for(n=0;n<e.length;n++)xp(e[n])}}function Qr(e,n){var a=e.stateNode;if(a===null)return null;var o=a[Tn]||null;if(o===null)return null;a=o[n];t:switch(n){case"onClick":case"onClickCapture":case"onDoubleClick":case"onDoubleClickCapture":case"onMouseDown":case"onMouseDownCapture":case"onMouseMove":case"onMouseMoveCapture":case"onMouseUp":case"onMouseUpCapture":case"onMouseEnter":(o=!o.disabled)||(e=e.type,o=!(e==="button"||e==="input"||e==="select"||e==="textarea")),e=!o;break t;default:e=!1}if(e)return null;if(a&&typeof a!="function")throw Error(s(231,n,typeof a));return a}var Vi=!(typeof window>"u"||typeof window.document>"u"||typeof window.document.createElement>"u"),Su=!1;if(Vi)try{var Jr={};Object.defineProperty(Jr,"passive",{get:function(){Su=!0}}),window.addEventListener("test",Jr,Jr),window.removeEventListener("test",Jr,Jr)}catch{Su=!1}var ma=null,Mu=null,hl=null;function Mp(){if(hl)return hl;var e,n=Mu,a=n.length,o,u="value"in ma?ma.value:ma.textContent,f=u.length;for(e=0;e<a&&n[e]===u[e];e++);var y=a-e;for(o=1;o<=y&&n[a-o]===u[f-o];o++);return hl=u.slice(e,1<o?1-o:void 0)}function dl(e){var n=e.keyCode;return"charCode"in e?(e=e.charCode,e===0&&n===13&&(e=13)):e=n,e===10&&(e=13),32<=e||e===13?e:0}function pl(){return!0}function Ep(){return!1}function Fn(e){function n(a,o,u,f,y){this._reactName=a,this._targetInst=u,this.type=o,this.nativeEvent=f,this.target=y,this.currentTarget=null;for(var A in e)e.hasOwnProperty(A)&&(a=e[A],this[A]=a?a(f):f[A]);return this.isDefaultPrevented=(f.defaultPrevented!=null?f.defaultPrevented:f.returnValue===!1)?pl:Ep,this.isPropagationStopped=Ep,this}return g(n.prototype,{preventDefault:function(){this.defaultPrevented=!0;var a=this.nativeEvent;a&&(a.preventDefault?a.preventDefault():typeof a.returnValue!="unknown"&&(a.returnValue=!1),this.isDefaultPrevented=pl)},stopPropagation:function(){var a=this.nativeEvent;a&&(a.stopPropagation?a.stopPropagation():typeof a.cancelBubble!="unknown"&&(a.cancelBubble=!0),this.isPropagationStopped=pl)},persist:function(){},isPersistent:pl}),n}var $a={eventPhase:0,bubbles:0,cancelable:0,timeStamp:function(e){return e.timeStamp||Date.now()},defaultPrevented:0,isTrusted:0},ml=Fn($a),$r=g({},$a,{view:0,detail:0}),vy=Fn($r),Eu,Tu,to,gl=g({},$r,{screenX:0,screenY:0,clientX:0,clientY:0,pageX:0,pageY:0,ctrlKey:0,shiftKey:0,altKey:0,metaKey:0,getModifierState:Au,button:0,buttons:0,relatedTarget:function(e){return e.relatedTarget===void 0?e.fromElement===e.srcElement?e.toElement:e.fromElement:e.relatedTarget},movementX:function(e){return"movementX"in e?e.movementX:(e!==to&&(to&&e.type==="mousemove"?(Eu=e.screenX-to.screenX,Tu=e.screenY-to.screenY):Tu=Eu=0,to=e),Eu)},movementY:function(e){return"movementY"in e?e.movementY:Tu}}),Tp=Fn(gl),yy=g({},gl,{dataTransfer:0}),xy=Fn(yy),Sy=g({},$r,{relatedTarget:0}),bu=Fn(Sy),My=g({},$a,{animationName:0,elapsedTime:0,pseudoElement:0}),Ey=Fn(My),Ty=g({},$a,{clipboardData:function(e){return"clipboardData"in e?e.clipboardData:window.clipboardData}}),by=Fn(Ty),Ay=g({},$a,{data:0}),bp=Fn(Ay),Ry={Esc:"Escape",Spacebar:" ",Left:"ArrowLeft",Up:"ArrowUp",Right:"ArrowRight",Down:"ArrowDown",Del:"Delete",Win:"OS",Menu:"ContextMenu",Apps:"ContextMenu",Scroll:"ScrollLock",MozPrintableKey:"Unidentified"},Cy={8:"Backspace",9:"Tab",12:"Clear",13:"Enter",16:"Shift",17:"Control",18:"Alt",19:"Pause",20:"CapsLock",27:"Escape",32:" ",33:"PageUp",34:"PageDown",35:"End",36:"Home",37:"ArrowLeft",38:"ArrowUp",39:"ArrowRight",40:"ArrowDown",45:"Insert",46:"Delete",112:"F1",113:"F2",114:"F3",115:"F4",116:"F5",117:"F6",118:"F7",119:"F8",120:"F9",121:"F10",122:"F11",123:"F12",144:"NumLock",145:"ScrollLock",224:"Meta"},wy={Alt:"altKey",Control:"ctrlKey",Meta:"metaKey",Shift:"shiftKey"};function Dy(e){var n=this.nativeEvent;return n.getModifierState?n.getModifierState(e):(e=wy[e])?!!n[e]:!1}function Au(){return Dy}var Uy=g({},$r,{key:function(e){if(e.key){var n=Ry[e.key]||e.key;if(n!=="Unidentified")return n}return e.type==="keypress"?(e=dl(e),e===13?"Enter":String.fromCharCode(e)):e.type==="keydown"||e.type==="keyup"?Cy[e.keyCode]||"Unidentified":""},code:0,location:0,ctrlKey:0,shiftKey:0,altKey:0,metaKey:0,repeat:0,locale:0,getModifierState:Au,charCode:function(e){return e.type==="keypress"?dl(e):0},keyCode:function(e){return e.type==="keydown"||e.type==="keyup"?e.keyCode:0},which:function(e){return e.type==="keypress"?dl(e):e.type==="keydown"||e.type==="keyup"?e.keyCode:0}}),Ly=Fn(Uy),Ny=g({},gl,{pointerId:0,width:0,height:0,pressure:0,tangentialPressure:0,tiltX:0,tiltY:0,twist:0,pointerType:0,isPrimary:0}),Ap=Fn(Ny),Oy=g({},$r,{touches:0,targetTouches:0,changedTouches:0,altKey:0,metaKey:0,ctrlKey:0,shiftKey:0,getModifierState:Au}),Py=Fn(Oy),zy=g({},$a,{propertyName:0,elapsedTime:0,pseudoElement:0}),By=Fn(zy),Iy=g({},gl,{deltaX:function(e){return"deltaX"in e?e.deltaX:"wheelDeltaX"in e?-e.wheelDeltaX:0},deltaY:function(e){return"deltaY"in e?e.deltaY:"wheelDeltaY"in e?-e.wheelDeltaY:"wheelDelta"in e?-e.wheelDelta:0},deltaZ:0,deltaMode:0}),Fy=Fn(Iy),Hy=g({},$a,{newState:0,oldState:0}),Gy=Fn(Hy),Vy=[9,13,27,32],Ru=Vi&&"CompositionEvent"in window,eo=null;Vi&&"documentMode"in document&&(eo=document.documentMode);var ky=Vi&&"TextEvent"in window&&!eo,Rp=Vi&&(!Ru||eo&&8<eo&&11>=eo),Cp=" ",wp=!1;function Dp(e,n){switch(e){case"keyup":return Vy.indexOf(n.keyCode)!==-1;case"keydown":return n.keyCode!==229;case"keypress":case"mousedown":case"focusout":return!0;default:return!1}}function Up(e){return e=e.detail,typeof e=="object"&&"data"in e?e.data:null}var Hs=!1;function Xy(e,n){switch(e){case"compositionend":return Up(n);case"keypress":return n.which!==32?null:(wp=!0,Cp);case"textInput":return e=n.data,e===Cp&&wp?null:e;default:return null}}function Wy(e,n){if(Hs)return e==="compositionend"||!Ru&&Dp(e,n)?(e=Mp(),hl=Mu=ma=null,Hs=!1,e):null;switch(e){case"paste":return null;case"keypress":if(!(n.ctrlKey||n.altKey||n.metaKey)||n.ctrlKey&&n.altKey){if(n.char&&1<n.char.length)return n.char;if(n.which)return String.fromCharCode(n.which)}return null;case"compositionend":return Rp&&n.locale!=="ko"?null:n.data;default:return null}}var jy={color:!0,date:!0,datetime:!0,"datetime-local":!0,email:!0,month:!0,number:!0,password:!0,range:!0,search:!0,tel:!0,text:!0,time:!0,url:!0,week:!0};function Lp(e){var n=e&&e.nodeName&&e.nodeName.toLowerCase();return n==="input"?!!jy[e.type]:n==="textarea"}function Np(e,n,a,o){Is?Fs?Fs.push(o):Fs=[o]:Is=o,n=sc(n,"onChange"),0<n.length&&(a=new ml("onChange","change",null,a,o),e.push({event:a,listeners:n}))}var no=null,io=null;function Yy(e){m_(e,0)}function _l(e){var n=lt(e);if(Hi(n))return e}function Op(e,n){if(e==="change")return n}var Pp=!1;if(Vi){var Cu;if(Vi){var wu="oninput"in document;if(!wu){var zp=document.createElement("div");zp.setAttribute("oninput","return;"),wu=typeof zp.oninput=="function"}Cu=wu}else Cu=!1;Pp=Cu&&(!document.documentMode||9<document.documentMode)}function Bp(){no&&(no.detachEvent("onpropertychange",Ip),io=no=null)}function Ip(e){if(e.propertyName==="value"&&_l(io)){var n=[];Np(n,io,e,yu(e)),Sp(Yy,n)}}function qy(e,n,a){e==="focusin"?(Bp(),no=n,io=a,no.attachEvent("onpropertychange",Ip)):e==="focusout"&&Bp()}function Zy(e){if(e==="selectionchange"||e==="keyup"||e==="keydown")return _l(io)}function Ky(e,n){if(e==="click")return _l(n)}function Qy(e,n){if(e==="input"||e==="change")return _l(n)}function Jy(e,n){return e===n&&(e!==0||1/e===1/n)||e!==e&&n!==n}var Zn=typeof Object.is=="function"?Object.is:Jy;function ao(e,n){if(Zn(e,n))return!0;if(typeof e!="object"||e===null||typeof n!="object"||n===null)return!1;var a=Object.keys(e),o=Object.keys(n);if(a.length!==o.length)return!1;for(o=0;o<a.length;o++){var u=a[o];if(!Dt.call(n,u)||!Zn(e[u],n[u]))return!1}return!0}function Fp(e){for(;e&&e.firstChild;)e=e.firstChild;return e}function Hp(e,n){var a=Fp(e);e=0;for(var o;a;){if(a.nodeType===3){if(o=e+a.textContent.length,e<=n&&o>=n)return{node:a,offset:n-e};e=o}t:{for(;a;){if(a.nextSibling){a=a.nextSibling;break t}a=a.parentNode}a=void 0}a=Fp(a)}}function Gp(e,n){return e&&n?e===n?!0:e&&e.nodeType===3?!1:n&&n.nodeType===3?Gp(e,n.parentNode):"contains"in e?e.contains(n):e.compareDocumentPosition?!!(e.compareDocumentPosition(n)&16):!1:!1}function Vp(e){e=e!=null&&e.ownerDocument!=null&&e.ownerDocument.defaultView!=null?e.ownerDocument.defaultView:window;for(var n=yn(e.document);n instanceof e.HTMLIFrameElement;){try{var a=typeof n.contentWindow.location.href=="string"}catch{a=!1}if(a)e=n.contentWindow;else break;n=yn(e.document)}return n}function Du(e){var n=e&&e.nodeName&&e.nodeName.toLowerCase();return n&&(n==="input"&&(e.type==="text"||e.type==="search"||e.type==="tel"||e.type==="url"||e.type==="password")||n==="textarea"||e.contentEditable==="true")}var $y=Vi&&"documentMode"in document&&11>=document.documentMode,Gs=null,Uu=null,so=null,Lu=!1;function kp(e,n,a){var o=a.window===a?a.document:a.nodeType===9?a:a.ownerDocument;Lu||Gs==null||Gs!==yn(o)||(o=Gs,"selectionStart"in o&&Du(o)?o={start:o.selectionStart,end:o.selectionEnd}:(o=(o.ownerDocument&&o.ownerDocument.defaultView||window).getSelection(),o={anchorNode:o.anchorNode,anchorOffset:o.anchorOffset,focusNode:o.focusNode,focusOffset:o.focusOffset}),so&&ao(so,o)||(so=o,o=sc(Uu,"onSelect"),0<o.length&&(n=new ml("onSelect","select",null,n,a),e.push({event:n,listeners:o}),n.target=Gs)))}function ts(e,n){var a={};return a[e.toLowerCase()]=n.toLowerCase(),a["Webkit"+e]="webkit"+n,a["Moz"+e]="moz"+n,a}var Vs={animationend:ts("Animation","AnimationEnd"),animationiteration:ts("Animation","AnimationIteration"),animationstart:ts("Animation","AnimationStart"),transitionrun:ts("Transition","TransitionRun"),transitionstart:ts("Transition","TransitionStart"),transitioncancel:ts("Transition","TransitionCancel"),transitionend:ts("Transition","TransitionEnd")},Nu={},Xp={};Vi&&(Xp=document.createElement("div").style,"AnimationEvent"in window||(delete Vs.animationend.animation,delete Vs.animationiteration.animation,delete Vs.animationstart.animation),"TransitionEvent"in window||delete Vs.transitionend.transition);function es(e){if(Nu[e])return Nu[e];if(!Vs[e])return e;var n=Vs[e],a;for(a in n)if(n.hasOwnProperty(a)&&a in Xp)return Nu[e]=n[a];return e}var Wp=es("animationend"),jp=es("animationiteration"),Yp=es("animationstart"),tx=es("transitionrun"),ex=es("transitionstart"),nx=es("transitioncancel"),qp=es("transitionend"),Zp=new Map,Ou="abort auxClick beforeToggle cancel canPlay canPlayThrough click close contextMenu copy cut drag dragEnd dragEnter dragExit dragLeave dragOver dragStart drop durationChange emptied encrypted ended error gotPointerCapture input invalid keyDown keyPress keyUp load loadedData loadedMetadata loadStart lostPointerCapture mouseDown mouseMove mouseOut mouseOver mouseUp paste pause play playing pointerCancel pointerDown pointerMove pointerOut pointerOver pointerUp progress rateChange reset resize seeked seeking stalled submit suspend timeUpdate touchCancel touchEnd touchStart volumeChange scroll toggle touchMove waiting wheel".split(" ");Ou.push("scrollEnd");function xi(e,n){Zp.set(e,n),kt(n,[e])}var vl=typeof reportError=="function"?reportError:function(e){if(typeof window=="object"&&typeof window.ErrorEvent=="function"){var n=new window.ErrorEvent("error",{bubbles:!0,cancelable:!0,message:typeof e=="object"&&e!==null&&typeof e.message=="string"?String(e.message):String(e),error:e});if(!window.dispatchEvent(n))return}else if(typeof process=="object"&&typeof process.emit=="function"){process.emit("uncaughtException",e);return}console.error(e)},li=[],ks=0,Pu=0;function yl(){for(var e=ks,n=Pu=ks=0;n<e;){var a=li[n];li[n++]=null;var o=li[n];li[n++]=null;var u=li[n];li[n++]=null;var f=li[n];if(li[n++]=null,o!==null&&u!==null){var y=o.pending;y===null?u.next=u:(u.next=y.next,y.next=u),o.pending=u}f!==0&&Kp(a,u,f)}}function xl(e,n,a,o){li[ks++]=e,li[ks++]=n,li[ks++]=a,li[ks++]=o,Pu|=o,e.lanes|=o,e=e.alternate,e!==null&&(e.lanes|=o)}function zu(e,n,a,o){return xl(e,n,a,o),Sl(e)}function ns(e,n){return xl(e,null,null,n),Sl(e)}function Kp(e,n,a){e.lanes|=a;var o=e.alternate;o!==null&&(o.lanes|=a);for(var u=!1,f=e.return;f!==null;)f.childLanes|=a,o=f.alternate,o!==null&&(o.childLanes|=a),f.tag===22&&(e=f.stateNode,e===null||e._visibility&1||(u=!0)),e=f,f=f.return;return e.tag===3?(f=e.stateNode,u&&n!==null&&(u=31-It(a),e=f.hiddenUpdates,o=e[u],o===null?e[u]=[n]:o.push(n),n.lane=a|536870912),f):null}function Sl(e){if(50<Co)throw Co=0,jf=null,Error(s(185));for(var n=e.return;n!==null;)e=n,n=e.return;return e.tag===3?e.stateNode:null}var Xs={};function ix(e,n,a,o){this.tag=e,this.key=a,this.sibling=this.child=this.return=this.stateNode=this.type=this.elementType=null,this.index=0,this.refCleanup=this.ref=null,this.pendingProps=n,this.dependencies=this.memoizedState=this.updateQueue=this.memoizedProps=null,this.mode=o,this.subtreeFlags=this.flags=0,this.deletions=null,this.childLanes=this.lanes=0,this.alternate=null}function Kn(e,n,a,o){return new ix(e,n,a,o)}function Bu(e){return e=e.prototype,!(!e||!e.isReactComponent)}function ki(e,n){var a=e.alternate;return a===null?(a=Kn(e.tag,n,e.key,e.mode),a.elementType=e.elementType,a.type=e.type,a.stateNode=e.stateNode,a.alternate=e,e.alternate=a):(a.pendingProps=n,a.type=e.type,a.flags=0,a.subtreeFlags=0,a.deletions=null),a.flags=e.flags&65011712,a.childLanes=e.childLanes,a.lanes=e.lanes,a.child=e.child,a.memoizedProps=e.memoizedProps,a.memoizedState=e.memoizedState,a.updateQueue=e.updateQueue,n=e.dependencies,a.dependencies=n===null?null:{lanes:n.lanes,firstContext:n.firstContext},a.sibling=e.sibling,a.index=e.index,a.ref=e.ref,a.refCleanup=e.refCleanup,a}function Qp(e,n){e.flags&=65011714;var a=e.alternate;return a===null?(e.childLanes=0,e.lanes=n,e.child=null,e.subtreeFlags=0,e.memoizedProps=null,e.memoizedState=null,e.updateQueue=null,e.dependencies=null,e.stateNode=null):(e.childLanes=a.childLanes,e.lanes=a.lanes,e.child=a.child,e.subtreeFlags=0,e.deletions=null,e.memoizedProps=a.memoizedProps,e.memoizedState=a.memoizedState,e.updateQueue=a.updateQueue,e.type=a.type,n=a.dependencies,e.dependencies=n===null?null:{lanes:n.lanes,firstContext:n.firstContext}),e}function Ml(e,n,a,o,u,f){var y=0;if(o=e,typeof e=="function")Bu(e)&&(y=1);else if(typeof e=="string")y=lS(e,a,wt.current)?26:e==="html"||e==="head"||e==="body"?27:5;else t:switch(e){case D:return e=Kn(31,a,n,u),e.elementType=D,e.lanes=f,e;case R:return is(a.children,u,f,n);case M:y=8,u|=24;break;case _:return e=Kn(12,a,n,u|2),e.elementType=_,e.lanes=f,e;case P:return e=Kn(13,a,n,u),e.elementType=P,e.lanes=f,e;case B:return e=Kn(19,a,n,u),e.elementType=B,e.lanes=f,e;default:if(typeof e=="object"&&e!==null)switch(e.$$typeof){case U:y=10;break t;case L:y=9;break t;case T:y=11;break t;case F:y=14;break t;case G:y=16,o=null;break t}y=29,a=Error(s(130,e===null?"null":typeof e,"")),o=null}return n=Kn(y,a,n,u),n.elementType=e,n.type=o,n.lanes=f,n}function is(e,n,a,o){return e=Kn(7,e,o,n),e.lanes=a,e}function Iu(e,n,a){return e=Kn(6,e,null,n),e.lanes=a,e}function Jp(e){var n=Kn(18,null,null,0);return n.stateNode=e,n}function Fu(e,n,a){return n=Kn(4,e.children!==null?e.children:[],e.key,n),n.lanes=a,n.stateNode={containerInfo:e.containerInfo,pendingChildren:null,implementation:e.implementation},n}var $p=new WeakMap;function ci(e,n){if(typeof e=="object"&&e!==null){var a=$p.get(e);return a!==void 0?a:(n={value:e,source:n,stack:Ee(n)},$p.set(e,n),n)}return{value:e,source:n,stack:Ee(n)}}var Ws=[],js=0,El=null,ro=0,ui=[],fi=0,ga=null,Di=1,Ui="";function Xi(e,n){Ws[js++]=ro,Ws[js++]=El,El=e,ro=n}function tm(e,n,a){ui[fi++]=Di,ui[fi++]=Ui,ui[fi++]=ga,ga=e;var o=Di;e=Ui;var u=32-It(o)-1;o&=~(1<<u),a+=1;var f=32-It(n)+u;if(30<f){var y=u-u%5;f=(o&(1<<y)-1).toString(32),o>>=y,u-=y,Di=1<<32-It(n)+u|a<<u|o,Ui=f+e}else Di=1<<f|a<<u|o,Ui=e}function Hu(e){e.return!==null&&(Xi(e,1),tm(e,1,0))}function Gu(e){for(;e===El;)El=Ws[--js],Ws[js]=null,ro=Ws[--js],Ws[js]=null;for(;e===ga;)ga=ui[--fi],ui[fi]=null,Ui=ui[--fi],ui[fi]=null,Di=ui[--fi],ui[fi]=null}function em(e,n){ui[fi++]=Di,ui[fi++]=Ui,ui[fi++]=ga,Di=n.id,Ui=n.overflow,ga=e}var An=null,Je=null,Ue=!1,_a=null,hi=!1,Vu=Error(s(519));function va(e){var n=Error(s(418,1<arguments.length&&arguments[1]!==void 0&&arguments[1]?"text":"HTML",""));throw oo(ci(n,e)),Vu}function nm(e){var n=e.stateNode,a=e.type,o=e.memoizedProps;switch(n[on]=e,n[Tn]=o,a){case"dialog":Ae("cancel",n),Ae("close",n);break;case"iframe":case"object":case"embed":Ae("load",n);break;case"video":case"audio":for(a=0;a<Do.length;a++)Ae(Do[a],n);break;case"source":Ae("error",n);break;case"img":case"image":case"link":Ae("error",n),Ae("load",n);break;case"details":Ae("toggle",n);break;case"input":Ae("invalid",n),Pn(n,o.value,o.defaultValue,o.checked,o.defaultChecked,o.type,o.name,!0);break;case"select":Ae("invalid",n);break;case"textarea":Ae("invalid",n),wi(n,o.value,o.defaultValue,o.children)}a=o.children,typeof a!="string"&&typeof a!="number"&&typeof a!="bigint"||n.textContent===""+a||o.suppressHydrationWarning===!0||y_(n.textContent,a)?(o.popover!=null&&(Ae("beforetoggle",n),Ae("toggle",n)),o.onScroll!=null&&Ae("scroll",n),o.onScrollEnd!=null&&Ae("scrollend",n),o.onClick!=null&&(n.onclick=Gi),n=!0):n=!1,n||va(e,!0)}function im(e){for(An=e.return;An;)switch(An.tag){case 5:case 31:case 13:hi=!1;return;case 27:case 3:hi=!0;return;default:An=An.return}}function Ys(e){if(e!==An)return!1;if(!Ue)return im(e),Ue=!0,!1;var n=e.tag,a;if((a=n!==3&&n!==27)&&((a=n===5)&&(a=e.type,a=!(a!=="form"&&a!=="button")||oh(e.type,e.memoizedProps)),a=!a),a&&Je&&va(e),im(e),n===13){if(e=e.memoizedState,e=e!==null?e.dehydrated:null,!e)throw Error(s(317));Je=C_(e)}else if(n===31){if(e=e.memoizedState,e=e!==null?e.dehydrated:null,!e)throw Error(s(317));Je=C_(e)}else n===27?(n=Je,La(e.type)?(e=hh,hh=null,Je=e):Je=n):Je=An?pi(e.stateNode.nextSibling):null;return!0}function as(){Je=An=null,Ue=!1}function ku(){var e=_a;return e!==null&&(kn===null?kn=e:kn.push.apply(kn,e),_a=null),e}function oo(e){_a===null?_a=[e]:_a.push(e)}var Xu=z(null),ss=null,Wi=null;function ya(e,n,a){Mt(Xu,n._currentValue),n._currentValue=a}function ji(e){e._currentValue=Xu.current,at(Xu)}function Wu(e,n,a){for(;e!==null;){var o=e.alternate;if((e.childLanes&n)!==n?(e.childLanes|=n,o!==null&&(o.childLanes|=n)):o!==null&&(o.childLanes&n)!==n&&(o.childLanes|=n),e===a)break;e=e.return}}function ju(e,n,a,o){var u=e.child;for(u!==null&&(u.return=e);u!==null;){var f=u.dependencies;if(f!==null){var y=u.child;f=f.firstContext;t:for(;f!==null;){var A=f;f=u;for(var H=0;H<n.length;H++)if(A.context===n[H]){f.lanes|=a,A=f.alternate,A!==null&&(A.lanes|=a),Wu(f.return,a,e),o||(y=null);break t}f=A.next}}else if(u.tag===18){if(y=u.return,y===null)throw Error(s(341));y.lanes|=a,f=y.alternate,f!==null&&(f.lanes|=a),Wu(y,a,e),y=null}else y=u.child;if(y!==null)y.return=u;else for(y=u;y!==null;){if(y===e){y=null;break}if(u=y.sibling,u!==null){u.return=y.return,y=u;break}y=y.return}u=y}}function qs(e,n,a,o){e=null;for(var u=n,f=!1;u!==null;){if(!f){if((u.flags&524288)!==0)f=!0;else if((u.flags&262144)!==0)break}if(u.tag===10){var y=u.alternate;if(y===null)throw Error(s(387));if(y=y.memoizedProps,y!==null){var A=u.type;Zn(u.pendingProps.value,y.value)||(e!==null?e.push(A):e=[A])}}else if(u===it.current){if(y=u.alternate,y===null)throw Error(s(387));y.memoizedState.memoizedState!==u.memoizedState.memoizedState&&(e!==null?e.push(Po):e=[Po])}u=u.return}e!==null&&ju(n,e,a,o),n.flags|=262144}function Tl(e){for(e=e.firstContext;e!==null;){if(!Zn(e.context._currentValue,e.memoizedValue))return!0;e=e.next}return!1}function rs(e){ss=e,Wi=null,e=e.dependencies,e!==null&&(e.firstContext=null)}function Rn(e){return am(ss,e)}function bl(e,n){return ss===null&&rs(e),am(e,n)}function am(e,n){var a=n._currentValue;if(n={context:n,memoizedValue:a,next:null},Wi===null){if(e===null)throw Error(s(308));Wi=n,e.dependencies={lanes:0,firstContext:n},e.flags|=524288}else Wi=Wi.next=n;return a}var ax=typeof AbortController<"u"?AbortController:function(){var e=[],n=this.signal={aborted:!1,addEventListener:function(a,o){e.push(o)}};this.abort=function(){n.aborted=!0,e.forEach(function(a){return a()})}},sx=r.unstable_scheduleCallback,rx=r.unstable_NormalPriority,ln={$$typeof:U,Consumer:null,Provider:null,_currentValue:null,_currentValue2:null,_threadCount:0};function Yu(){return{controller:new ax,data:new Map,refCount:0}}function lo(e){e.refCount--,e.refCount===0&&sx(rx,function(){e.controller.abort()})}var co=null,qu=0,Zs=0,Ks=null;function ox(e,n){if(co===null){var a=co=[];qu=0,Zs=Jf(),Ks={status:"pending",value:void 0,then:function(o){a.push(o)}}}return qu++,n.then(sm,sm),n}function sm(){if(--qu===0&&co!==null){Ks!==null&&(Ks.status="fulfilled");var e=co;co=null,Zs=0,Ks=null;for(var n=0;n<e.length;n++)(0,e[n])()}}function lx(e,n){var a=[],o={status:"pending",value:null,reason:null,then:function(u){a.push(u)}};return e.then(function(){o.status="fulfilled",o.value=n;for(var u=0;u<a.length;u++)(0,a[u])(n)},function(u){for(o.status="rejected",o.reason=u,u=0;u<a.length;u++)(0,a[u])(void 0)}),o}var rm=N.S;N.S=function(e,n){kg=E(),typeof n=="object"&&n!==null&&typeof n.then=="function"&&ox(e,n),rm!==null&&rm(e,n)};var os=z(null);function Zu(){var e=os.current;return e!==null?e:Ke.pooledCache}function Al(e,n){n===null?Mt(os,os.current):Mt(os,n.pool)}function om(){var e=Zu();return e===null?null:{parent:ln._currentValue,pool:e}}var Qs=Error(s(460)),Ku=Error(s(474)),Rl=Error(s(542)),Cl={then:function(){}};function lm(e){return e=e.status,e==="fulfilled"||e==="rejected"}function cm(e,n,a){switch(a=e[a],a===void 0?e.push(n):a!==n&&(n.then(Gi,Gi),n=a),n.status){case"fulfilled":return n.value;case"rejected":throw e=n.reason,fm(e),e;default:if(typeof n.status=="string")n.then(Gi,Gi);else{if(e=Ke,e!==null&&100<e.shellSuspendCounter)throw Error(s(482));e=n,e.status="pending",e.then(function(o){if(n.status==="pending"){var u=n;u.status="fulfilled",u.value=o}},function(o){if(n.status==="pending"){var u=n;u.status="rejected",u.reason=o}})}switch(n.status){case"fulfilled":return n.value;case"rejected":throw e=n.reason,fm(e),e}throw cs=n,Qs}}function ls(e){try{var n=e._init;return n(e._payload)}catch(a){throw a!==null&&typeof a=="object"&&typeof a.then=="function"?(cs=a,Qs):a}}var cs=null;function um(){if(cs===null)throw Error(s(459));var e=cs;return cs=null,e}function fm(e){if(e===Qs||e===Rl)throw Error(s(483))}var Js=null,uo=0;function wl(e){var n=uo;return uo+=1,Js===null&&(Js=[]),cm(Js,e,n)}function fo(e,n){n=n.props.ref,e.ref=n!==void 0?n:null}function Dl(e,n){throw n.$$typeof===x?Error(s(525)):(e=Object.prototype.toString.call(n),Error(s(31,e==="[object Object]"?"object with keys {"+Object.keys(n).join(", ")+"}":e)))}function hm(e){function n(Q,k){if(e){var et=Q.deletions;et===null?(Q.deletions=[k],Q.flags|=16):et.push(k)}}function a(Q,k){if(!e)return null;for(;k!==null;)n(Q,k),k=k.sibling;return null}function o(Q){for(var k=new Map;Q!==null;)Q.key!==null?k.set(Q.key,Q):k.set(Q.index,Q),Q=Q.sibling;return k}function u(Q,k){return Q=ki(Q,k),Q.index=0,Q.sibling=null,Q}function f(Q,k,et){return Q.index=et,e?(et=Q.alternate,et!==null?(et=et.index,et<k?(Q.flags|=67108866,k):et):(Q.flags|=67108866,k)):(Q.flags|=1048576,k)}function y(Q){return e&&Q.alternate===null&&(Q.flags|=67108866),Q}function A(Q,k,et,yt){return k===null||k.tag!==6?(k=Iu(et,Q.mode,yt),k.return=Q,k):(k=u(k,et),k.return=Q,k)}function H(Q,k,et,yt){var se=et.type;return se===R?vt(Q,k,et.props.children,yt,et.key):k!==null&&(k.elementType===se||typeof se=="object"&&se!==null&&se.$$typeof===G&&ls(se)===k.type)?(k=u(k,et.props),fo(k,et),k.return=Q,k):(k=Ml(et.type,et.key,et.props,null,Q.mode,yt),fo(k,et),k.return=Q,k)}function nt(Q,k,et,yt){return k===null||k.tag!==4||k.stateNode.containerInfo!==et.containerInfo||k.stateNode.implementation!==et.implementation?(k=Fu(et,Q.mode,yt),k.return=Q,k):(k=u(k,et.children||[]),k.return=Q,k)}function vt(Q,k,et,yt,se){return k===null||k.tag!==7?(k=is(et,Q.mode,yt,se),k.return=Q,k):(k=u(k,et),k.return=Q,k)}function St(Q,k,et){if(typeof k=="string"&&k!==""||typeof k=="number"||typeof k=="bigint")return k=Iu(""+k,Q.mode,et),k.return=Q,k;if(typeof k=="object"&&k!==null){switch(k.$$typeof){case S:return et=Ml(k.type,k.key,k.props,null,Q.mode,et),fo(et,k),et.return=Q,et;case b:return k=Fu(k,Q.mode,et),k.return=Q,k;case G:return k=ls(k),St(Q,k,et)}if(ht(k)||K(k))return k=is(k,Q.mode,et,null),k.return=Q,k;if(typeof k.then=="function")return St(Q,wl(k),et);if(k.$$typeof===U)return St(Q,bl(Q,k),et);Dl(Q,k)}return null}function st(Q,k,et,yt){var se=k!==null?k.key:null;if(typeof et=="string"&&et!==""||typeof et=="number"||typeof et=="bigint")return se!==null?null:A(Q,k,""+et,yt);if(typeof et=="object"&&et!==null){switch(et.$$typeof){case S:return et.key===se?H(Q,k,et,yt):null;case b:return et.key===se?nt(Q,k,et,yt):null;case G:return et=ls(et),st(Q,k,et,yt)}if(ht(et)||K(et))return se!==null?null:vt(Q,k,et,yt,null);if(typeof et.then=="function")return st(Q,k,wl(et),yt);if(et.$$typeof===U)return st(Q,k,bl(Q,et),yt);Dl(Q,et)}return null}function ut(Q,k,et,yt,se){if(typeof yt=="string"&&yt!==""||typeof yt=="number"||typeof yt=="bigint")return Q=Q.get(et)||null,A(k,Q,""+yt,se);if(typeof yt=="object"&&yt!==null){switch(yt.$$typeof){case S:return Q=Q.get(yt.key===null?et:yt.key)||null,H(k,Q,yt,se);case b:return Q=Q.get(yt.key===null?et:yt.key)||null,nt(k,Q,yt,se);case G:return yt=ls(yt),ut(Q,k,et,yt,se)}if(ht(yt)||K(yt))return Q=Q.get(et)||null,vt(k,Q,yt,se,null);if(typeof yt.then=="function")return ut(Q,k,et,wl(yt),se);if(yt.$$typeof===U)return ut(Q,k,et,bl(k,yt),se);Dl(k,yt)}return null}function Yt(Q,k,et,yt){for(var se=null,Ne=null,te=k,pe=k=0,De=null;te!==null&&pe<et.length;pe++){te.index>pe?(De=te,te=null):De=te.sibling;var Oe=st(Q,te,et[pe],yt);if(Oe===null){te===null&&(te=De);break}e&&te&&Oe.alternate===null&&n(Q,te),k=f(Oe,k,pe),Ne===null?se=Oe:Ne.sibling=Oe,Ne=Oe,te=De}if(pe===et.length)return a(Q,te),Ue&&Xi(Q,pe),se;if(te===null){for(;pe<et.length;pe++)te=St(Q,et[pe],yt),te!==null&&(k=f(te,k,pe),Ne===null?se=te:Ne.sibling=te,Ne=te);return Ue&&Xi(Q,pe),se}for(te=o(te);pe<et.length;pe++)De=ut(te,Q,pe,et[pe],yt),De!==null&&(e&&De.alternate!==null&&te.delete(De.key===null?pe:De.key),k=f(De,k,pe),Ne===null?se=De:Ne.sibling=De,Ne=De);return e&&te.forEach(function(Ba){return n(Q,Ba)}),Ue&&Xi(Q,pe),se}function ce(Q,k,et,yt){if(et==null)throw Error(s(151));for(var se=null,Ne=null,te=k,pe=k=0,De=null,Oe=et.next();te!==null&&!Oe.done;pe++,Oe=et.next()){te.index>pe?(De=te,te=null):De=te.sibling;var Ba=st(Q,te,Oe.value,yt);if(Ba===null){te===null&&(te=De);break}e&&te&&Ba.alternate===null&&n(Q,te),k=f(Ba,k,pe),Ne===null?se=Ba:Ne.sibling=Ba,Ne=Ba,te=De}if(Oe.done)return a(Q,te),Ue&&Xi(Q,pe),se;if(te===null){for(;!Oe.done;pe++,Oe=et.next())Oe=St(Q,Oe.value,yt),Oe!==null&&(k=f(Oe,k,pe),Ne===null?se=Oe:Ne.sibling=Oe,Ne=Oe);return Ue&&Xi(Q,pe),se}for(te=o(te);!Oe.done;pe++,Oe=et.next())Oe=ut(te,Q,pe,Oe.value,yt),Oe!==null&&(e&&Oe.alternate!==null&&te.delete(Oe.key===null?pe:Oe.key),k=f(Oe,k,pe),Ne===null?se=Oe:Ne.sibling=Oe,Ne=Oe);return e&&te.forEach(function(yS){return n(Q,yS)}),Ue&&Xi(Q,pe),se}function Xe(Q,k,et,yt){if(typeof et=="object"&&et!==null&&et.type===R&&et.key===null&&(et=et.props.children),typeof et=="object"&&et!==null){switch(et.$$typeof){case S:t:{for(var se=et.key;k!==null;){if(k.key===se){if(se=et.type,se===R){if(k.tag===7){a(Q,k.sibling),yt=u(k,et.props.children),yt.return=Q,Q=yt;break t}}else if(k.elementType===se||typeof se=="object"&&se!==null&&se.$$typeof===G&&ls(se)===k.type){a(Q,k.sibling),yt=u(k,et.props),fo(yt,et),yt.return=Q,Q=yt;break t}a(Q,k);break}else n(Q,k);k=k.sibling}et.type===R?(yt=is(et.props.children,Q.mode,yt,et.key),yt.return=Q,Q=yt):(yt=Ml(et.type,et.key,et.props,null,Q.mode,yt),fo(yt,et),yt.return=Q,Q=yt)}return y(Q);case b:t:{for(se=et.key;k!==null;){if(k.key===se)if(k.tag===4&&k.stateNode.containerInfo===et.containerInfo&&k.stateNode.implementation===et.implementation){a(Q,k.sibling),yt=u(k,et.children||[]),yt.return=Q,Q=yt;break t}else{a(Q,k);break}else n(Q,k);k=k.sibling}yt=Fu(et,Q.mode,yt),yt.return=Q,Q=yt}return y(Q);case G:return et=ls(et),Xe(Q,k,et,yt)}if(ht(et))return Yt(Q,k,et,yt);if(K(et)){if(se=K(et),typeof se!="function")throw Error(s(150));return et=se.call(et),ce(Q,k,et,yt)}if(typeof et.then=="function")return Xe(Q,k,wl(et),yt);if(et.$$typeof===U)return Xe(Q,k,bl(Q,et),yt);Dl(Q,et)}return typeof et=="string"&&et!==""||typeof et=="number"||typeof et=="bigint"?(et=""+et,k!==null&&k.tag===6?(a(Q,k.sibling),yt=u(k,et),yt.return=Q,Q=yt):(a(Q,k),yt=Iu(et,Q.mode,yt),yt.return=Q,Q=yt),y(Q)):a(Q,k)}return function(Q,k,et,yt){try{uo=0;var se=Xe(Q,k,et,yt);return Js=null,se}catch(te){if(te===Qs||te===Rl)throw te;var Ne=Kn(29,te,null,Q.mode);return Ne.lanes=yt,Ne.return=Q,Ne}finally{}}}var us=hm(!0),dm=hm(!1),xa=!1;function Qu(e){e.updateQueue={baseState:e.memoizedState,firstBaseUpdate:null,lastBaseUpdate:null,shared:{pending:null,lanes:0,hiddenCallbacks:null},callbacks:null}}function Ju(e,n){e=e.updateQueue,n.updateQueue===e&&(n.updateQueue={baseState:e.baseState,firstBaseUpdate:e.firstBaseUpdate,lastBaseUpdate:e.lastBaseUpdate,shared:e.shared,callbacks:null})}function Sa(e){return{lane:e,tag:0,payload:null,callback:null,next:null}}function Ma(e,n,a){var o=e.updateQueue;if(o===null)return null;if(o=o.shared,(ze&2)!==0){var u=o.pending;return u===null?n.next=n:(n.next=u.next,u.next=n),o.pending=n,n=Sl(e),Kp(e,null,a),n}return xl(e,o,n,a),Sl(e)}function ho(e,n,a){if(n=n.updateQueue,n!==null&&(n=n.shared,(a&4194048)!==0)){var o=n.lanes;o&=e.pendingLanes,a|=o,n.lanes=a,Ci(e,a)}}function $u(e,n){var a=e.updateQueue,o=e.alternate;if(o!==null&&(o=o.updateQueue,a===o)){var u=null,f=null;if(a=a.firstBaseUpdate,a!==null){do{var y={lane:a.lane,tag:a.tag,payload:a.payload,callback:null,next:null};f===null?u=f=y:f=f.next=y,a=a.next}while(a!==null);f===null?u=f=n:f=f.next=n}else u=f=n;a={baseState:o.baseState,firstBaseUpdate:u,lastBaseUpdate:f,shared:o.shared,callbacks:o.callbacks},e.updateQueue=a;return}e=a.lastBaseUpdate,e===null?a.firstBaseUpdate=n:e.next=n,a.lastBaseUpdate=n}var tf=!1;function po(){if(tf){var e=Ks;if(e!==null)throw e}}function mo(e,n,a,o){tf=!1;var u=e.updateQueue;xa=!1;var f=u.firstBaseUpdate,y=u.lastBaseUpdate,A=u.shared.pending;if(A!==null){u.shared.pending=null;var H=A,nt=H.next;H.next=null,y===null?f=nt:y.next=nt,y=H;var vt=e.alternate;vt!==null&&(vt=vt.updateQueue,A=vt.lastBaseUpdate,A!==y&&(A===null?vt.firstBaseUpdate=nt:A.next=nt,vt.lastBaseUpdate=H))}if(f!==null){var St=u.baseState;y=0,vt=nt=H=null,A=f;do{var st=A.lane&-536870913,ut=st!==A.lane;if(ut?(we&st)===st:(o&st)===st){st!==0&&st===Zs&&(tf=!0),vt!==null&&(vt=vt.next={lane:0,tag:A.tag,payload:A.payload,callback:null,next:null});t:{var Yt=e,ce=A;st=n;var Xe=a;switch(ce.tag){case 1:if(Yt=ce.payload,typeof Yt=="function"){St=Yt.call(Xe,St,st);break t}St=Yt;break t;case 3:Yt.flags=Yt.flags&-65537|128;case 0:if(Yt=ce.payload,st=typeof Yt=="function"?Yt.call(Xe,St,st):Yt,st==null)break t;St=g({},St,st);break t;case 2:xa=!0}}st=A.callback,st!==null&&(e.flags|=64,ut&&(e.flags|=8192),ut=u.callbacks,ut===null?u.callbacks=[st]:ut.push(st))}else ut={lane:st,tag:A.tag,payload:A.payload,callback:A.callback,next:null},vt===null?(nt=vt=ut,H=St):vt=vt.next=ut,y|=st;if(A=A.next,A===null){if(A=u.shared.pending,A===null)break;ut=A,A=ut.next,ut.next=null,u.lastBaseUpdate=ut,u.shared.pending=null}}while(!0);vt===null&&(H=St),u.baseState=H,u.firstBaseUpdate=nt,u.lastBaseUpdate=vt,f===null&&(u.shared.lanes=0),Ra|=y,e.lanes=y,e.memoizedState=St}}function pm(e,n){if(typeof e!="function")throw Error(s(191,e));e.call(n)}function mm(e,n){var a=e.callbacks;if(a!==null)for(e.callbacks=null,e=0;e<a.length;e++)pm(a[e],n)}var $s=z(null),Ul=z(0);function gm(e,n){e=ea,Mt(Ul,e),Mt($s,n),ea=e|n.baseLanes}function ef(){Mt(Ul,ea),Mt($s,$s.current)}function nf(){ea=Ul.current,at($s),at(Ul)}var Qn=z(null),di=null;function Ea(e){var n=e.alternate;Mt(sn,sn.current&1),Mt(Qn,e),di===null&&(n===null||$s.current!==null||n.memoizedState!==null)&&(di=e)}function af(e){Mt(sn,sn.current),Mt(Qn,e),di===null&&(di=e)}function _m(e){e.tag===22?(Mt(sn,sn.current),Mt(Qn,e),di===null&&(di=e)):Ta()}function Ta(){Mt(sn,sn.current),Mt(Qn,Qn.current)}function Jn(e){at(Qn),di===e&&(di=null),at(sn)}var sn=z(0);function Ll(e){for(var n=e;n!==null;){if(n.tag===13){var a=n.memoizedState;if(a!==null&&(a=a.dehydrated,a===null||uh(a)||fh(a)))return n}else if(n.tag===19&&(n.memoizedProps.revealOrder==="forwards"||n.memoizedProps.revealOrder==="backwards"||n.memoizedProps.revealOrder==="unstable_legacy-backwards"||n.memoizedProps.revealOrder==="together")){if((n.flags&128)!==0)return n}else if(n.child!==null){n.child.return=n,n=n.child;continue}if(n===e)break;for(;n.sibling===null;){if(n.return===null||n.return===e)return null;n=n.return}n.sibling.return=n.return,n=n.sibling}return null}var Yi=0,de=null,Ve=null,cn=null,Nl=!1,tr=!1,fs=!1,Ol=0,go=0,er=null,cx=0;function en(){throw Error(s(321))}function sf(e,n){if(n===null)return!1;for(var a=0;a<n.length&&a<e.length;a++)if(!Zn(e[a],n[a]))return!1;return!0}function rf(e,n,a,o,u,f){return Yi=f,de=n,n.memoizedState=null,n.updateQueue=null,n.lanes=0,N.H=e===null||e.memoizedState===null?tg:Sf,fs=!1,f=a(o,u),fs=!1,tr&&(f=ym(n,a,o,u)),vm(e),f}function vm(e){N.H=yo;var n=Ve!==null&&Ve.next!==null;if(Yi=0,cn=Ve=de=null,Nl=!1,go=0,er=null,n)throw Error(s(300));e===null||un||(e=e.dependencies,e!==null&&Tl(e)&&(un=!0))}function ym(e,n,a,o){de=e;var u=0;do{if(tr&&(er=null),go=0,tr=!1,25<=u)throw Error(s(301));if(u+=1,cn=Ve=null,e.updateQueue!=null){var f=e.updateQueue;f.lastEffect=null,f.events=null,f.stores=null,f.memoCache!=null&&(f.memoCache.index=0)}N.H=eg,f=n(a,o)}while(tr);return f}function ux(){var e=N.H,n=e.useState()[0];return n=typeof n.then=="function"?_o(n):n,e=e.useState()[0],(Ve!==null?Ve.memoizedState:null)!==e&&(de.flags|=1024),n}function of(){var e=Ol!==0;return Ol=0,e}function lf(e,n,a){n.updateQueue=e.updateQueue,n.flags&=-2053,e.lanes&=~a}function cf(e){if(Nl){for(e=e.memoizedState;e!==null;){var n=e.queue;n!==null&&(n.pending=null),e=e.next}Nl=!1}Yi=0,cn=Ve=de=null,tr=!1,go=Ol=0,er=null}function zn(){var e={memoizedState:null,baseState:null,baseQueue:null,queue:null,next:null};return cn===null?de.memoizedState=cn=e:cn=cn.next=e,cn}function rn(){if(Ve===null){var e=de.alternate;e=e!==null?e.memoizedState:null}else e=Ve.next;var n=cn===null?de.memoizedState:cn.next;if(n!==null)cn=n,Ve=e;else{if(e===null)throw de.alternate===null?Error(s(467)):Error(s(310));Ve=e,e={memoizedState:Ve.memoizedState,baseState:Ve.baseState,baseQueue:Ve.baseQueue,queue:Ve.queue,next:null},cn===null?de.memoizedState=cn=e:cn=cn.next=e}return cn}function Pl(){return{lastEffect:null,events:null,stores:null,memoCache:null}}function _o(e){var n=go;return go+=1,er===null&&(er=[]),e=cm(er,e,n),n=de,(cn===null?n.memoizedState:cn.next)===null&&(n=n.alternate,N.H=n===null||n.memoizedState===null?tg:Sf),e}function zl(e){if(e!==null&&typeof e=="object"){if(typeof e.then=="function")return _o(e);if(e.$$typeof===U)return Rn(e)}throw Error(s(438,String(e)))}function uf(e){var n=null,a=de.updateQueue;if(a!==null&&(n=a.memoCache),n==null){var o=de.alternate;o!==null&&(o=o.updateQueue,o!==null&&(o=o.memoCache,o!=null&&(n={data:o.data.map(function(u){return u.slice()}),index:0})))}if(n==null&&(n={data:[],index:0}),a===null&&(a=Pl(),de.updateQueue=a),a.memoCache=n,a=n.data[n.index],a===void 0)for(a=n.data[n.index]=Array(e),o=0;o<e;o++)a[o]=w;return n.index++,a}function qi(e,n){return typeof n=="function"?n(e):n}function Bl(e){var n=rn();return ff(n,Ve,e)}function ff(e,n,a){var o=e.queue;if(o===null)throw Error(s(311));o.lastRenderedReducer=a;var u=e.baseQueue,f=o.pending;if(f!==null){if(u!==null){var y=u.next;u.next=f.next,f.next=y}n.baseQueue=u=f,o.pending=null}if(f=e.baseState,u===null)e.memoizedState=f;else{n=u.next;var A=y=null,H=null,nt=n,vt=!1;do{var St=nt.lane&-536870913;if(St!==nt.lane?(we&St)===St:(Yi&St)===St){var st=nt.revertLane;if(st===0)H!==null&&(H=H.next={lane:0,revertLane:0,gesture:null,action:nt.action,hasEagerState:nt.hasEagerState,eagerState:nt.eagerState,next:null}),St===Zs&&(vt=!0);else if((Yi&st)===st){nt=nt.next,st===Zs&&(vt=!0);continue}else St={lane:0,revertLane:nt.revertLane,gesture:null,action:nt.action,hasEagerState:nt.hasEagerState,eagerState:nt.eagerState,next:null},H===null?(A=H=St,y=f):H=H.next=St,de.lanes|=st,Ra|=st;St=nt.action,fs&&a(f,St),f=nt.hasEagerState?nt.eagerState:a(f,St)}else st={lane:St,revertLane:nt.revertLane,gesture:nt.gesture,action:nt.action,hasEagerState:nt.hasEagerState,eagerState:nt.eagerState,next:null},H===null?(A=H=st,y=f):H=H.next=st,de.lanes|=St,Ra|=St;nt=nt.next}while(nt!==null&&nt!==n);if(H===null?y=f:H.next=A,!Zn(f,e.memoizedState)&&(un=!0,vt&&(a=Ks,a!==null)))throw a;e.memoizedState=f,e.baseState=y,e.baseQueue=H,o.lastRenderedState=f}return u===null&&(o.lanes=0),[e.memoizedState,o.dispatch]}function hf(e){var n=rn(),a=n.queue;if(a===null)throw Error(s(311));a.lastRenderedReducer=e;var o=a.dispatch,u=a.pending,f=n.memoizedState;if(u!==null){a.pending=null;var y=u=u.next;do f=e(f,y.action),y=y.next;while(y!==u);Zn(f,n.memoizedState)||(un=!0),n.memoizedState=f,n.baseQueue===null&&(n.baseState=f),a.lastRenderedState=f}return[f,o]}function xm(e,n,a){var o=de,u=rn(),f=Ue;if(f){if(a===void 0)throw Error(s(407));a=a()}else a=n();var y=!Zn((Ve||u).memoizedState,a);if(y&&(u.memoizedState=a,un=!0),u=u.queue,mf(Em.bind(null,o,u,e),[e]),u.getSnapshot!==n||y||cn!==null&&cn.memoizedState.tag&1){if(o.flags|=2048,nr(9,{destroy:void 0},Mm.bind(null,o,u,a,n),null),Ke===null)throw Error(s(349));f||(Yi&127)!==0||Sm(o,n,a)}return a}function Sm(e,n,a){e.flags|=16384,e={getSnapshot:n,value:a},n=de.updateQueue,n===null?(n=Pl(),de.updateQueue=n,n.stores=[e]):(a=n.stores,a===null?n.stores=[e]:a.push(e))}function Mm(e,n,a,o){n.value=a,n.getSnapshot=o,Tm(n)&&bm(e)}function Em(e,n,a){return a(function(){Tm(n)&&bm(e)})}function Tm(e){var n=e.getSnapshot;e=e.value;try{var a=n();return!Zn(e,a)}catch{return!0}}function bm(e){var n=ns(e,2);n!==null&&Xn(n,e,2)}function df(e){var n=zn();if(typeof e=="function"){var a=e;if(e=a(),fs){ee(!0);try{a()}finally{ee(!1)}}}return n.memoizedState=n.baseState=e,n.queue={pending:null,lanes:0,dispatch:null,lastRenderedReducer:qi,lastRenderedState:e},n}function Am(e,n,a,o){return e.baseState=a,ff(e,Ve,typeof o=="function"?o:qi)}function fx(e,n,a,o,u){if(Hl(e))throw Error(s(485));if(e=n.action,e!==null){var f={payload:u,action:e,next:null,isTransition:!0,status:"pending",value:null,reason:null,listeners:[],then:function(y){f.listeners.push(y)}};N.T!==null?a(!0):f.isTransition=!1,o(f),a=n.pending,a===null?(f.next=n.pending=f,Rm(n,f)):(f.next=a.next,n.pending=a.next=f)}}function Rm(e,n){var a=n.action,o=n.payload,u=e.state;if(n.isTransition){var f=N.T,y={};N.T=y;try{var A=a(u,o),H=N.S;H!==null&&H(y,A),Cm(e,n,A)}catch(nt){pf(e,n,nt)}finally{f!==null&&y.types!==null&&(f.types=y.types),N.T=f}}else try{f=a(u,o),Cm(e,n,f)}catch(nt){pf(e,n,nt)}}function Cm(e,n,a){a!==null&&typeof a=="object"&&typeof a.then=="function"?a.then(function(o){wm(e,n,o)},function(o){return pf(e,n,o)}):wm(e,n,a)}function wm(e,n,a){n.status="fulfilled",n.value=a,Dm(n),e.state=a,n=e.pending,n!==null&&(a=n.next,a===n?e.pending=null:(a=a.next,n.next=a,Rm(e,a)))}function pf(e,n,a){var o=e.pending;if(e.pending=null,o!==null){o=o.next;do n.status="rejected",n.reason=a,Dm(n),n=n.next;while(n!==o)}e.action=null}function Dm(e){e=e.listeners;for(var n=0;n<e.length;n++)(0,e[n])()}function Um(e,n){return n}function Lm(e,n){if(Ue){var a=Ke.formState;if(a!==null){t:{var o=de;if(Ue){if(Je){e:{for(var u=Je,f=hi;u.nodeType!==8;){if(!f){u=null;break e}if(u=pi(u.nextSibling),u===null){u=null;break e}}f=u.data,u=f==="F!"||f==="F"?u:null}if(u){Je=pi(u.nextSibling),o=u.data==="F!";break t}}va(o)}o=!1}o&&(n=a[0])}}return a=zn(),a.memoizedState=a.baseState=n,o={pending:null,lanes:0,dispatch:null,lastRenderedReducer:Um,lastRenderedState:n},a.queue=o,a=Qm.bind(null,de,o),o.dispatch=a,o=df(!1),f=xf.bind(null,de,!1,o.queue),o=zn(),u={state:n,dispatch:null,action:e,pending:null},o.queue=u,a=fx.bind(null,de,u,f,a),u.dispatch=a,o.memoizedState=e,[n,a,!1]}function Nm(e){var n=rn();return Om(n,Ve,e)}function Om(e,n,a){if(n=ff(e,n,Um)[0],e=Bl(qi)[0],typeof n=="object"&&n!==null&&typeof n.then=="function")try{var o=_o(n)}catch(y){throw y===Qs?Rl:y}else o=n;n=rn();var u=n.queue,f=u.dispatch;return a!==n.memoizedState&&(de.flags|=2048,nr(9,{destroy:void 0},hx.bind(null,u,a),null)),[o,f,e]}function hx(e,n){e.action=n}function Pm(e){var n=rn(),a=Ve;if(a!==null)return Om(n,a,e);rn(),n=n.memoizedState,a=rn();var o=a.queue.dispatch;return a.memoizedState=e,[n,o,!1]}function nr(e,n,a,o){return e={tag:e,create:a,deps:o,inst:n,next:null},n=de.updateQueue,n===null&&(n=Pl(),de.updateQueue=n),a=n.lastEffect,a===null?n.lastEffect=e.next=e:(o=a.next,a.next=e,e.next=o,n.lastEffect=e),e}function zm(){return rn().memoizedState}function Il(e,n,a,o){var u=zn();de.flags|=e,u.memoizedState=nr(1|n,{destroy:void 0},a,o===void 0?null:o)}function Fl(e,n,a,o){var u=rn();o=o===void 0?null:o;var f=u.memoizedState.inst;Ve!==null&&o!==null&&sf(o,Ve.memoizedState.deps)?u.memoizedState=nr(n,f,a,o):(de.flags|=e,u.memoizedState=nr(1|n,f,a,o))}function Bm(e,n){Il(8390656,8,e,n)}function mf(e,n){Fl(2048,8,e,n)}function dx(e){de.flags|=4;var n=de.updateQueue;if(n===null)n=Pl(),de.updateQueue=n,n.events=[e];else{var a=n.events;a===null?n.events=[e]:a.push(e)}}function Im(e){var n=rn().memoizedState;return dx({ref:n,nextImpl:e}),function(){if((ze&2)!==0)throw Error(s(440));return n.impl.apply(void 0,arguments)}}function Fm(e,n){return Fl(4,2,e,n)}function Hm(e,n){return Fl(4,4,e,n)}function Gm(e,n){if(typeof n=="function"){e=e();var a=n(e);return function(){typeof a=="function"?a():n(null)}}if(n!=null)return e=e(),n.current=e,function(){n.current=null}}function Vm(e,n,a){a=a!=null?a.concat([e]):null,Fl(4,4,Gm.bind(null,n,e),a)}function gf(){}function km(e,n){var a=rn();n=n===void 0?null:n;var o=a.memoizedState;return n!==null&&sf(n,o[1])?o[0]:(a.memoizedState=[e,n],e)}function Xm(e,n){var a=rn();n=n===void 0?null:n;var o=a.memoizedState;if(n!==null&&sf(n,o[1]))return o[0];if(o=e(),fs){ee(!0);try{e()}finally{ee(!1)}}return a.memoizedState=[o,n],o}function _f(e,n,a){return a===void 0||(Yi&1073741824)!==0&&(we&261930)===0?e.memoizedState=n:(e.memoizedState=a,e=Wg(),de.lanes|=e,Ra|=e,a)}function Wm(e,n,a,o){return Zn(a,n)?a:$s.current!==null?(e=_f(e,a,o),Zn(e,n)||(un=!0),e):(Yi&42)===0||(Yi&1073741824)!==0&&(we&261930)===0?(un=!0,e.memoizedState=a):(e=Wg(),de.lanes|=e,Ra|=e,n)}function jm(e,n,a,o,u){var f=J.p;J.p=f!==0&&8>f?f:8;var y=N.T,A={};N.T=A,xf(e,!1,n,a);try{var H=u(),nt=N.S;if(nt!==null&&nt(A,H),H!==null&&typeof H=="object"&&typeof H.then=="function"){var vt=lx(H,o);vo(e,n,vt,ei(e))}else vo(e,n,o,ei(e))}catch(St){vo(e,n,{then:function(){},status:"rejected",reason:St},ei())}finally{J.p=f,y!==null&&A.types!==null&&(y.types=A.types),N.T=y}}function px(){}function vf(e,n,a,o){if(e.tag!==5)throw Error(s(476));var u=Ym(e).queue;jm(e,u,n,Y,a===null?px:function(){return qm(e),a(o)})}function Ym(e){var n=e.memoizedState;if(n!==null)return n;n={memoizedState:Y,baseState:Y,baseQueue:null,queue:{pending:null,lanes:0,dispatch:null,lastRenderedReducer:qi,lastRenderedState:Y},next:null};var a={};return n.next={memoizedState:a,baseState:a,baseQueue:null,queue:{pending:null,lanes:0,dispatch:null,lastRenderedReducer:qi,lastRenderedState:a},next:null},e.memoizedState=n,e=e.alternate,e!==null&&(e.memoizedState=n),n}function qm(e){var n=Ym(e);n.next===null&&(n=e.alternate.memoizedState),vo(e,n.next.queue,{},ei())}function yf(){return Rn(Po)}function Zm(){return rn().memoizedState}function Km(){return rn().memoizedState}function mx(e){for(var n=e.return;n!==null;){switch(n.tag){case 24:case 3:var a=ei();e=Sa(a);var o=Ma(n,e,a);o!==null&&(Xn(o,n,a),ho(o,n,a)),n={cache:Yu()},e.payload=n;return}n=n.return}}function gx(e,n,a){var o=ei();a={lane:o,revertLane:0,gesture:null,action:a,hasEagerState:!1,eagerState:null,next:null},Hl(e)?Jm(n,a):(a=zu(e,n,a,o),a!==null&&(Xn(a,e,o),$m(a,n,o)))}function Qm(e,n,a){var o=ei();vo(e,n,a,o)}function vo(e,n,a,o){var u={lane:o,revertLane:0,gesture:null,action:a,hasEagerState:!1,eagerState:null,next:null};if(Hl(e))Jm(n,u);else{var f=e.alternate;if(e.lanes===0&&(f===null||f.lanes===0)&&(f=n.lastRenderedReducer,f!==null))try{var y=n.lastRenderedState,A=f(y,a);if(u.hasEagerState=!0,u.eagerState=A,Zn(A,y))return xl(e,n,u,0),Ke===null&&yl(),!1}catch{}finally{}if(a=zu(e,n,u,o),a!==null)return Xn(a,e,o),$m(a,n,o),!0}return!1}function xf(e,n,a,o){if(o={lane:2,revertLane:Jf(),gesture:null,action:o,hasEagerState:!1,eagerState:null,next:null},Hl(e)){if(n)throw Error(s(479))}else n=zu(e,a,o,2),n!==null&&Xn(n,e,2)}function Hl(e){var n=e.alternate;return e===de||n!==null&&n===de}function Jm(e,n){tr=Nl=!0;var a=e.pending;a===null?n.next=n:(n.next=a.next,a.next=n),e.pending=n}function $m(e,n,a){if((a&4194048)!==0){var o=n.lanes;o&=e.pendingLanes,a|=o,n.lanes=a,Ci(e,a)}}var yo={readContext:Rn,use:zl,useCallback:en,useContext:en,useEffect:en,useImperativeHandle:en,useLayoutEffect:en,useInsertionEffect:en,useMemo:en,useReducer:en,useRef:en,useState:en,useDebugValue:en,useDeferredValue:en,useTransition:en,useSyncExternalStore:en,useId:en,useHostTransitionStatus:en,useFormState:en,useActionState:en,useOptimistic:en,useMemoCache:en,useCacheRefresh:en};yo.useEffectEvent=en;var tg={readContext:Rn,use:zl,useCallback:function(e,n){return zn().memoizedState=[e,n===void 0?null:n],e},useContext:Rn,useEffect:Bm,useImperativeHandle:function(e,n,a){a=a!=null?a.concat([e]):null,Il(4194308,4,Gm.bind(null,n,e),a)},useLayoutEffect:function(e,n){return Il(4194308,4,e,n)},useInsertionEffect:function(e,n){Il(4,2,e,n)},useMemo:function(e,n){var a=zn();n=n===void 0?null:n;var o=e();if(fs){ee(!0);try{e()}finally{ee(!1)}}return a.memoizedState=[o,n],o},useReducer:function(e,n,a){var o=zn();if(a!==void 0){var u=a(n);if(fs){ee(!0);try{a(n)}finally{ee(!1)}}}else u=n;return o.memoizedState=o.baseState=u,e={pending:null,lanes:0,dispatch:null,lastRenderedReducer:e,lastRenderedState:u},o.queue=e,e=e.dispatch=gx.bind(null,de,e),[o.memoizedState,e]},useRef:function(e){var n=zn();return e={current:e},n.memoizedState=e},useState:function(e){e=df(e);var n=e.queue,a=Qm.bind(null,de,n);return n.dispatch=a,[e.memoizedState,a]},useDebugValue:gf,useDeferredValue:function(e,n){var a=zn();return _f(a,e,n)},useTransition:function(){var e=df(!1);return e=jm.bind(null,de,e.queue,!0,!1),zn().memoizedState=e,[!1,e]},useSyncExternalStore:function(e,n,a){var o=de,u=zn();if(Ue){if(a===void 0)throw Error(s(407));a=a()}else{if(a=n(),Ke===null)throw Error(s(349));(we&127)!==0||Sm(o,n,a)}u.memoizedState=a;var f={value:a,getSnapshot:n};return u.queue=f,Bm(Em.bind(null,o,f,e),[e]),o.flags|=2048,nr(9,{destroy:void 0},Mm.bind(null,o,f,a,n),null),a},useId:function(){var e=zn(),n=Ke.identifierPrefix;if(Ue){var a=Ui,o=Di;a=(o&~(1<<32-It(o)-1)).toString(32)+a,n="_"+n+"R_"+a,a=Ol++,0<a&&(n+="H"+a.toString(32)),n+="_"}else a=cx++,n="_"+n+"r_"+a.toString(32)+"_";return e.memoizedState=n},useHostTransitionStatus:yf,useFormState:Lm,useActionState:Lm,useOptimistic:function(e){var n=zn();n.memoizedState=n.baseState=e;var a={pending:null,lanes:0,dispatch:null,lastRenderedReducer:null,lastRenderedState:null};return n.queue=a,n=xf.bind(null,de,!0,a),a.dispatch=n,[e,n]},useMemoCache:uf,useCacheRefresh:function(){return zn().memoizedState=mx.bind(null,de)},useEffectEvent:function(e){var n=zn(),a={impl:e};return n.memoizedState=a,function(){if((ze&2)!==0)throw Error(s(440));return a.impl.apply(void 0,arguments)}}},Sf={readContext:Rn,use:zl,useCallback:km,useContext:Rn,useEffect:mf,useImperativeHandle:Vm,useInsertionEffect:Fm,useLayoutEffect:Hm,useMemo:Xm,useReducer:Bl,useRef:zm,useState:function(){return Bl(qi)},useDebugValue:gf,useDeferredValue:function(e,n){var a=rn();return Wm(a,Ve.memoizedState,e,n)},useTransition:function(){var e=Bl(qi)[0],n=rn().memoizedState;return[typeof e=="boolean"?e:_o(e),n]},useSyncExternalStore:xm,useId:Zm,useHostTransitionStatus:yf,useFormState:Nm,useActionState:Nm,useOptimistic:function(e,n){var a=rn();return Am(a,Ve,e,n)},useMemoCache:uf,useCacheRefresh:Km};Sf.useEffectEvent=Im;var eg={readContext:Rn,use:zl,useCallback:km,useContext:Rn,useEffect:mf,useImperativeHandle:Vm,useInsertionEffect:Fm,useLayoutEffect:Hm,useMemo:Xm,useReducer:hf,useRef:zm,useState:function(){return hf(qi)},useDebugValue:gf,useDeferredValue:function(e,n){var a=rn();return Ve===null?_f(a,e,n):Wm(a,Ve.memoizedState,e,n)},useTransition:function(){var e=hf(qi)[0],n=rn().memoizedState;return[typeof e=="boolean"?e:_o(e),n]},useSyncExternalStore:xm,useId:Zm,useHostTransitionStatus:yf,useFormState:Pm,useActionState:Pm,useOptimistic:function(e,n){var a=rn();return Ve!==null?Am(a,Ve,e,n):(a.baseState=e,[e,a.queue.dispatch])},useMemoCache:uf,useCacheRefresh:Km};eg.useEffectEvent=Im;function Mf(e,n,a,o){n=e.memoizedState,a=a(o,n),a=a==null?n:g({},n,a),e.memoizedState=a,e.lanes===0&&(e.updateQueue.baseState=a)}var Ef={enqueueSetState:function(e,n,a){e=e._reactInternals;var o=ei(),u=Sa(o);u.payload=n,a!=null&&(u.callback=a),n=Ma(e,u,o),n!==null&&(Xn(n,e,o),ho(n,e,o))},enqueueReplaceState:function(e,n,a){e=e._reactInternals;var o=ei(),u=Sa(o);u.tag=1,u.payload=n,a!=null&&(u.callback=a),n=Ma(e,u,o),n!==null&&(Xn(n,e,o),ho(n,e,o))},enqueueForceUpdate:function(e,n){e=e._reactInternals;var a=ei(),o=Sa(a);o.tag=2,n!=null&&(o.callback=n),n=Ma(e,o,a),n!==null&&(Xn(n,e,a),ho(n,e,a))}};function ng(e,n,a,o,u,f,y){return e=e.stateNode,typeof e.shouldComponentUpdate=="function"?e.shouldComponentUpdate(o,f,y):n.prototype&&n.prototype.isPureReactComponent?!ao(a,o)||!ao(u,f):!0}function ig(e,n,a,o){e=n.state,typeof n.componentWillReceiveProps=="function"&&n.componentWillReceiveProps(a,o),typeof n.UNSAFE_componentWillReceiveProps=="function"&&n.UNSAFE_componentWillReceiveProps(a,o),n.state!==e&&Ef.enqueueReplaceState(n,n.state,null)}function hs(e,n){var a=n;if("ref"in n){a={};for(var o in n)o!=="ref"&&(a[o]=n[o])}if(e=e.defaultProps){a===n&&(a=g({},a));for(var u in e)a[u]===void 0&&(a[u]=e[u])}return a}function ag(e){vl(e)}function sg(e){console.error(e)}function rg(e){vl(e)}function Gl(e,n){try{var a=e.onUncaughtError;a(n.value,{componentStack:n.stack})}catch(o){setTimeout(function(){throw o})}}function og(e,n,a){try{var o=e.onCaughtError;o(a.value,{componentStack:a.stack,errorBoundary:n.tag===1?n.stateNode:null})}catch(u){setTimeout(function(){throw u})}}function Tf(e,n,a){return a=Sa(a),a.tag=3,a.payload={element:null},a.callback=function(){Gl(e,n)},a}function lg(e){return e=Sa(e),e.tag=3,e}function cg(e,n,a,o){var u=a.type.getDerivedStateFromError;if(typeof u=="function"){var f=o.value;e.payload=function(){return u(f)},e.callback=function(){og(n,a,o)}}var y=a.stateNode;y!==null&&typeof y.componentDidCatch=="function"&&(e.callback=function(){og(n,a,o),typeof u!="function"&&(Ca===null?Ca=new Set([this]):Ca.add(this));var A=o.stack;this.componentDidCatch(o.value,{componentStack:A!==null?A:""})})}function _x(e,n,a,o,u){if(a.flags|=32768,o!==null&&typeof o=="object"&&typeof o.then=="function"){if(n=a.alternate,n!==null&&qs(n,a,u,!0),a=Qn.current,a!==null){switch(a.tag){case 31:case 13:return di===null?$l():a.alternate===null&&nn===0&&(nn=3),a.flags&=-257,a.flags|=65536,a.lanes=u,o===Cl?a.flags|=16384:(n=a.updateQueue,n===null?a.updateQueue=new Set([o]):n.add(o),Zf(e,o,u)),!1;case 22:return a.flags|=65536,o===Cl?a.flags|=16384:(n=a.updateQueue,n===null?(n={transitions:null,markerInstances:null,retryQueue:new Set([o])},a.updateQueue=n):(a=n.retryQueue,a===null?n.retryQueue=new Set([o]):a.add(o)),Zf(e,o,u)),!1}throw Error(s(435,a.tag))}return Zf(e,o,u),$l(),!1}if(Ue)return n=Qn.current,n!==null?((n.flags&65536)===0&&(n.flags|=256),n.flags|=65536,n.lanes=u,o!==Vu&&(e=Error(s(422),{cause:o}),oo(ci(e,a)))):(o!==Vu&&(n=Error(s(423),{cause:o}),oo(ci(n,a))),e=e.current.alternate,e.flags|=65536,u&=-u,e.lanes|=u,o=ci(o,a),u=Tf(e.stateNode,o,u),$u(e,u),nn!==4&&(nn=2)),!1;var f=Error(s(520),{cause:o});if(f=ci(f,a),Ro===null?Ro=[f]:Ro.push(f),nn!==4&&(nn=2),n===null)return!0;o=ci(o,a),a=n;do{switch(a.tag){case 3:return a.flags|=65536,e=u&-u,a.lanes|=e,e=Tf(a.stateNode,o,e),$u(a,e),!1;case 1:if(n=a.type,f=a.stateNode,(a.flags&128)===0&&(typeof n.getDerivedStateFromError=="function"||f!==null&&typeof f.componentDidCatch=="function"&&(Ca===null||!Ca.has(f))))return a.flags|=65536,u&=-u,a.lanes|=u,u=lg(u),cg(u,e,a,o),$u(a,u),!1}a=a.return}while(a!==null);return!1}var bf=Error(s(461)),un=!1;function Cn(e,n,a,o){n.child=e===null?dm(n,null,a,o):us(n,e.child,a,o)}function ug(e,n,a,o,u){a=a.render;var f=n.ref;if("ref"in o){var y={};for(var A in o)A!=="ref"&&(y[A]=o[A])}else y=o;return rs(n),o=rf(e,n,a,y,f,u),A=of(),e!==null&&!un?(lf(e,n,u),Zi(e,n,u)):(Ue&&A&&Hu(n),n.flags|=1,Cn(e,n,o,u),n.child)}function fg(e,n,a,o,u){if(e===null){var f=a.type;return typeof f=="function"&&!Bu(f)&&f.defaultProps===void 0&&a.compare===null?(n.tag=15,n.type=f,hg(e,n,f,o,u)):(e=Ml(a.type,null,o,n,n.mode,u),e.ref=n.ref,e.return=n,n.child=e)}if(f=e.child,!Nf(e,u)){var y=f.memoizedProps;if(a=a.compare,a=a!==null?a:ao,a(y,o)&&e.ref===n.ref)return Zi(e,n,u)}return n.flags|=1,e=ki(f,o),e.ref=n.ref,e.return=n,n.child=e}function hg(e,n,a,o,u){if(e!==null){var f=e.memoizedProps;if(ao(f,o)&&e.ref===n.ref)if(un=!1,n.pendingProps=o=f,Nf(e,u))(e.flags&131072)!==0&&(un=!0);else return n.lanes=e.lanes,Zi(e,n,u)}return Af(e,n,a,o,u)}function dg(e,n,a,o){var u=o.children,f=e!==null?e.memoizedState:null;if(e===null&&n.stateNode===null&&(n.stateNode={_visibility:1,_pendingMarkers:null,_retryCache:null,_transitions:null}),o.mode==="hidden"){if((n.flags&128)!==0){if(f=f!==null?f.baseLanes|a:a,e!==null){for(o=n.child=e.child,u=0;o!==null;)u=u|o.lanes|o.childLanes,o=o.sibling;o=u&~f}else o=0,n.child=null;return pg(e,n,f,a,o)}if((a&536870912)!==0)n.memoizedState={baseLanes:0,cachePool:null},e!==null&&Al(n,f!==null?f.cachePool:null),f!==null?gm(n,f):ef(),_m(n);else return o=n.lanes=536870912,pg(e,n,f!==null?f.baseLanes|a:a,a,o)}else f!==null?(Al(n,f.cachePool),gm(n,f),Ta(),n.memoizedState=null):(e!==null&&Al(n,null),ef(),Ta());return Cn(e,n,u,a),n.child}function xo(e,n){return e!==null&&e.tag===22||n.stateNode!==null||(n.stateNode={_visibility:1,_pendingMarkers:null,_retryCache:null,_transitions:null}),n.sibling}function pg(e,n,a,o,u){var f=Zu();return f=f===null?null:{parent:ln._currentValue,pool:f},n.memoizedState={baseLanes:a,cachePool:f},e!==null&&Al(n,null),ef(),_m(n),e!==null&&qs(e,n,o,!0),n.childLanes=u,null}function Vl(e,n){return n=Xl({mode:n.mode,children:n.children},e.mode),n.ref=e.ref,e.child=n,n.return=e,n}function mg(e,n,a){return us(n,e.child,null,a),e=Vl(n,n.pendingProps),e.flags|=2,Jn(n),n.memoizedState=null,e}function vx(e,n,a){var o=n.pendingProps,u=(n.flags&128)!==0;if(n.flags&=-129,e===null){if(Ue){if(o.mode==="hidden")return e=Vl(n,o),n.lanes=536870912,xo(null,e);if(af(n),(e=Je)?(e=R_(e,hi),e=e!==null&&e.data==="&"?e:null,e!==null&&(n.memoizedState={dehydrated:e,treeContext:ga!==null?{id:Di,overflow:Ui}:null,retryLane:536870912,hydrationErrors:null},a=Jp(e),a.return=n,n.child=a,An=n,Je=null)):e=null,e===null)throw va(n);return n.lanes=536870912,null}return Vl(n,o)}var f=e.memoizedState;if(f!==null){var y=f.dehydrated;if(af(n),u)if(n.flags&256)n.flags&=-257,n=mg(e,n,a);else if(n.memoizedState!==null)n.child=e.child,n.flags|=128,n=null;else throw Error(s(558));else if(un||qs(e,n,a,!1),u=(a&e.childLanes)!==0,un||u){if(o=Ke,o!==null&&(y=Ns(o,a),y!==0&&y!==f.retryLane))throw f.retryLane=y,ns(e,y),Xn(o,e,y),bf;$l(),n=mg(e,n,a)}else e=f.treeContext,Je=pi(y.nextSibling),An=n,Ue=!0,_a=null,hi=!1,e!==null&&em(n,e),n=Vl(n,o),n.flags|=4096;return n}return e=ki(e.child,{mode:o.mode,children:o.children}),e.ref=n.ref,n.child=e,e.return=n,e}function kl(e,n){var a=n.ref;if(a===null)e!==null&&e.ref!==null&&(n.flags|=4194816);else{if(typeof a!="function"&&typeof a!="object")throw Error(s(284));(e===null||e.ref!==a)&&(n.flags|=4194816)}}function Af(e,n,a,o,u){return rs(n),a=rf(e,n,a,o,void 0,u),o=of(),e!==null&&!un?(lf(e,n,u),Zi(e,n,u)):(Ue&&o&&Hu(n),n.flags|=1,Cn(e,n,a,u),n.child)}function gg(e,n,a,o,u,f){return rs(n),n.updateQueue=null,a=ym(n,o,a,u),vm(e),o=of(),e!==null&&!un?(lf(e,n,f),Zi(e,n,f)):(Ue&&o&&Hu(n),n.flags|=1,Cn(e,n,a,f),n.child)}function _g(e,n,a,o,u){if(rs(n),n.stateNode===null){var f=Xs,y=a.contextType;typeof y=="object"&&y!==null&&(f=Rn(y)),f=new a(o,f),n.memoizedState=f.state!==null&&f.state!==void 0?f.state:null,f.updater=Ef,n.stateNode=f,f._reactInternals=n,f=n.stateNode,f.props=o,f.state=n.memoizedState,f.refs={},Qu(n),y=a.contextType,f.context=typeof y=="object"&&y!==null?Rn(y):Xs,f.state=n.memoizedState,y=a.getDerivedStateFromProps,typeof y=="function"&&(Mf(n,a,y,o),f.state=n.memoizedState),typeof a.getDerivedStateFromProps=="function"||typeof f.getSnapshotBeforeUpdate=="function"||typeof f.UNSAFE_componentWillMount!="function"&&typeof f.componentWillMount!="function"||(y=f.state,typeof f.componentWillMount=="function"&&f.componentWillMount(),typeof f.UNSAFE_componentWillMount=="function"&&f.UNSAFE_componentWillMount(),y!==f.state&&Ef.enqueueReplaceState(f,f.state,null),mo(n,o,f,u),po(),f.state=n.memoizedState),typeof f.componentDidMount=="function"&&(n.flags|=4194308),o=!0}else if(e===null){f=n.stateNode;var A=n.memoizedProps,H=hs(a,A);f.props=H;var nt=f.context,vt=a.contextType;y=Xs,typeof vt=="object"&&vt!==null&&(y=Rn(vt));var St=a.getDerivedStateFromProps;vt=typeof St=="function"||typeof f.getSnapshotBeforeUpdate=="function",A=n.pendingProps!==A,vt||typeof f.UNSAFE_componentWillReceiveProps!="function"&&typeof f.componentWillReceiveProps!="function"||(A||nt!==y)&&ig(n,f,o,y),xa=!1;var st=n.memoizedState;f.state=st,mo(n,o,f,u),po(),nt=n.memoizedState,A||st!==nt||xa?(typeof St=="function"&&(Mf(n,a,St,o),nt=n.memoizedState),(H=xa||ng(n,a,H,o,st,nt,y))?(vt||typeof f.UNSAFE_componentWillMount!="function"&&typeof f.componentWillMount!="function"||(typeof f.componentWillMount=="function"&&f.componentWillMount(),typeof f.UNSAFE_componentWillMount=="function"&&f.UNSAFE_componentWillMount()),typeof f.componentDidMount=="function"&&(n.flags|=4194308)):(typeof f.componentDidMount=="function"&&(n.flags|=4194308),n.memoizedProps=o,n.memoizedState=nt),f.props=o,f.state=nt,f.context=y,o=H):(typeof f.componentDidMount=="function"&&(n.flags|=4194308),o=!1)}else{f=n.stateNode,Ju(e,n),y=n.memoizedProps,vt=hs(a,y),f.props=vt,St=n.pendingProps,st=f.context,nt=a.contextType,H=Xs,typeof nt=="object"&&nt!==null&&(H=Rn(nt)),A=a.getDerivedStateFromProps,(nt=typeof A=="function"||typeof f.getSnapshotBeforeUpdate=="function")||typeof f.UNSAFE_componentWillReceiveProps!="function"&&typeof f.componentWillReceiveProps!="function"||(y!==St||st!==H)&&ig(n,f,o,H),xa=!1,st=n.memoizedState,f.state=st,mo(n,o,f,u),po();var ut=n.memoizedState;y!==St||st!==ut||xa||e!==null&&e.dependencies!==null&&Tl(e.dependencies)?(typeof A=="function"&&(Mf(n,a,A,o),ut=n.memoizedState),(vt=xa||ng(n,a,vt,o,st,ut,H)||e!==null&&e.dependencies!==null&&Tl(e.dependencies))?(nt||typeof f.UNSAFE_componentWillUpdate!="function"&&typeof f.componentWillUpdate!="function"||(typeof f.componentWillUpdate=="function"&&f.componentWillUpdate(o,ut,H),typeof f.UNSAFE_componentWillUpdate=="function"&&f.UNSAFE_componentWillUpdate(o,ut,H)),typeof f.componentDidUpdate=="function"&&(n.flags|=4),typeof f.getSnapshotBeforeUpdate=="function"&&(n.flags|=1024)):(typeof f.componentDidUpdate!="function"||y===e.memoizedProps&&st===e.memoizedState||(n.flags|=4),typeof f.getSnapshotBeforeUpdate!="function"||y===e.memoizedProps&&st===e.memoizedState||(n.flags|=1024),n.memoizedProps=o,n.memoizedState=ut),f.props=o,f.state=ut,f.context=H,o=vt):(typeof f.componentDidUpdate!="function"||y===e.memoizedProps&&st===e.memoizedState||(n.flags|=4),typeof f.getSnapshotBeforeUpdate!="function"||y===e.memoizedProps&&st===e.memoizedState||(n.flags|=1024),o=!1)}return f=o,kl(e,n),o=(n.flags&128)!==0,f||o?(f=n.stateNode,a=o&&typeof a.getDerivedStateFromError!="function"?null:f.render(),n.flags|=1,e!==null&&o?(n.child=us(n,e.child,null,u),n.child=us(n,null,a,u)):Cn(e,n,a,u),n.memoizedState=f.state,e=n.child):e=Zi(e,n,u),e}function vg(e,n,a,o){return as(),n.flags|=256,Cn(e,n,a,o),n.child}var Rf={dehydrated:null,treeContext:null,retryLane:0,hydrationErrors:null};function Cf(e){return{baseLanes:e,cachePool:om()}}function wf(e,n,a){return e=e!==null?e.childLanes&~a:0,n&&(e|=ti),e}function yg(e,n,a){var o=n.pendingProps,u=!1,f=(n.flags&128)!==0,y;if((y=f)||(y=e!==null&&e.memoizedState===null?!1:(sn.current&2)!==0),y&&(u=!0,n.flags&=-129),y=(n.flags&32)!==0,n.flags&=-33,e===null){if(Ue){if(u?Ea(n):Ta(),(e=Je)?(e=R_(e,hi),e=e!==null&&e.data!=="&"?e:null,e!==null&&(n.memoizedState={dehydrated:e,treeContext:ga!==null?{id:Di,overflow:Ui}:null,retryLane:536870912,hydrationErrors:null},a=Jp(e),a.return=n,n.child=a,An=n,Je=null)):e=null,e===null)throw va(n);return fh(e)?n.lanes=32:n.lanes=536870912,null}var A=o.children;return o=o.fallback,u?(Ta(),u=n.mode,A=Xl({mode:"hidden",children:A},u),o=is(o,u,a,null),A.return=n,o.return=n,A.sibling=o,n.child=A,o=n.child,o.memoizedState=Cf(a),o.childLanes=wf(e,y,a),n.memoizedState=Rf,xo(null,o)):(Ea(n),Df(n,A))}var H=e.memoizedState;if(H!==null&&(A=H.dehydrated,A!==null)){if(f)n.flags&256?(Ea(n),n.flags&=-257,n=Uf(e,n,a)):n.memoizedState!==null?(Ta(),n.child=e.child,n.flags|=128,n=null):(Ta(),A=o.fallback,u=n.mode,o=Xl({mode:"visible",children:o.children},u),A=is(A,u,a,null),A.flags|=2,o.return=n,A.return=n,o.sibling=A,n.child=o,us(n,e.child,null,a),o=n.child,o.memoizedState=Cf(a),o.childLanes=wf(e,y,a),n.memoizedState=Rf,n=xo(null,o));else if(Ea(n),fh(A)){if(y=A.nextSibling&&A.nextSibling.dataset,y)var nt=y.dgst;y=nt,o=Error(s(419)),o.stack="",o.digest=y,oo({value:o,source:null,stack:null}),n=Uf(e,n,a)}else if(un||qs(e,n,a,!1),y=(a&e.childLanes)!==0,un||y){if(y=Ke,y!==null&&(o=Ns(y,a),o!==0&&o!==H.retryLane))throw H.retryLane=o,ns(e,o),Xn(y,e,o),bf;uh(A)||$l(),n=Uf(e,n,a)}else uh(A)?(n.flags|=192,n.child=e.child,n=null):(e=H.treeContext,Je=pi(A.nextSibling),An=n,Ue=!0,_a=null,hi=!1,e!==null&&em(n,e),n=Df(n,o.children),n.flags|=4096);return n}return u?(Ta(),A=o.fallback,u=n.mode,H=e.child,nt=H.sibling,o=ki(H,{mode:"hidden",children:o.children}),o.subtreeFlags=H.subtreeFlags&65011712,nt!==null?A=ki(nt,A):(A=is(A,u,a,null),A.flags|=2),A.return=n,o.return=n,o.sibling=A,n.child=o,xo(null,o),o=n.child,A=e.child.memoizedState,A===null?A=Cf(a):(u=A.cachePool,u!==null?(H=ln._currentValue,u=u.parent!==H?{parent:H,pool:H}:u):u=om(),A={baseLanes:A.baseLanes|a,cachePool:u}),o.memoizedState=A,o.childLanes=wf(e,y,a),n.memoizedState=Rf,xo(e.child,o)):(Ea(n),a=e.child,e=a.sibling,a=ki(a,{mode:"visible",children:o.children}),a.return=n,a.sibling=null,e!==null&&(y=n.deletions,y===null?(n.deletions=[e],n.flags|=16):y.push(e)),n.child=a,n.memoizedState=null,a)}function Df(e,n){return n=Xl({mode:"visible",children:n},e.mode),n.return=e,e.child=n}function Xl(e,n){return e=Kn(22,e,null,n),e.lanes=0,e}function Uf(e,n,a){return us(n,e.child,null,a),e=Df(n,n.pendingProps.children),e.flags|=2,n.memoizedState=null,e}function xg(e,n,a){e.lanes|=n;var o=e.alternate;o!==null&&(o.lanes|=n),Wu(e.return,n,a)}function Lf(e,n,a,o,u,f){var y=e.memoizedState;y===null?e.memoizedState={isBackwards:n,rendering:null,renderingStartTime:0,last:o,tail:a,tailMode:u,treeForkCount:f}:(y.isBackwards=n,y.rendering=null,y.renderingStartTime=0,y.last=o,y.tail=a,y.tailMode=u,y.treeForkCount=f)}function Sg(e,n,a){var o=n.pendingProps,u=o.revealOrder,f=o.tail;o=o.children;var y=sn.current,A=(y&2)!==0;if(A?(y=y&1|2,n.flags|=128):y&=1,Mt(sn,y),Cn(e,n,o,a),o=Ue?ro:0,!A&&e!==null&&(e.flags&128)!==0)t:for(e=n.child;e!==null;){if(e.tag===13)e.memoizedState!==null&&xg(e,a,n);else if(e.tag===19)xg(e,a,n);else if(e.child!==null){e.child.return=e,e=e.child;continue}if(e===n)break t;for(;e.sibling===null;){if(e.return===null||e.return===n)break t;e=e.return}e.sibling.return=e.return,e=e.sibling}switch(u){case"forwards":for(a=n.child,u=null;a!==null;)e=a.alternate,e!==null&&Ll(e)===null&&(u=a),a=a.sibling;a=u,a===null?(u=n.child,n.child=null):(u=a.sibling,a.sibling=null),Lf(n,!1,u,a,f,o);break;case"backwards":case"unstable_legacy-backwards":for(a=null,u=n.child,n.child=null;u!==null;){if(e=u.alternate,e!==null&&Ll(e)===null){n.child=u;break}e=u.sibling,u.sibling=a,a=u,u=e}Lf(n,!0,a,null,f,o);break;case"together":Lf(n,!1,null,null,void 0,o);break;default:n.memoizedState=null}return n.child}function Zi(e,n,a){if(e!==null&&(n.dependencies=e.dependencies),Ra|=n.lanes,(a&n.childLanes)===0)if(e!==null){if(qs(e,n,a,!1),(a&n.childLanes)===0)return null}else return null;if(e!==null&&n.child!==e.child)throw Error(s(153));if(n.child!==null){for(e=n.child,a=ki(e,e.pendingProps),n.child=a,a.return=n;e.sibling!==null;)e=e.sibling,a=a.sibling=ki(e,e.pendingProps),a.return=n;a.sibling=null}return n.child}function Nf(e,n){return(e.lanes&n)!==0?!0:(e=e.dependencies,!!(e!==null&&Tl(e)))}function yx(e,n,a){switch(n.tag){case 3:xt(n,n.stateNode.containerInfo),ya(n,ln,e.memoizedState.cache),as();break;case 27:case 5:Bt(n);break;case 4:xt(n,n.stateNode.containerInfo);break;case 10:ya(n,n.type,n.memoizedProps.value);break;case 31:if(n.memoizedState!==null)return n.flags|=128,af(n),null;break;case 13:var o=n.memoizedState;if(o!==null)return o.dehydrated!==null?(Ea(n),n.flags|=128,null):(a&n.child.childLanes)!==0?yg(e,n,a):(Ea(n),e=Zi(e,n,a),e!==null?e.sibling:null);Ea(n);break;case 19:var u=(e.flags&128)!==0;if(o=(a&n.childLanes)!==0,o||(qs(e,n,a,!1),o=(a&n.childLanes)!==0),u){if(o)return Sg(e,n,a);n.flags|=128}if(u=n.memoizedState,u!==null&&(u.rendering=null,u.tail=null,u.lastEffect=null),Mt(sn,sn.current),o)break;return null;case 22:return n.lanes=0,dg(e,n,a,n.pendingProps);case 24:ya(n,ln,e.memoizedState.cache)}return Zi(e,n,a)}function Mg(e,n,a){if(e!==null)if(e.memoizedProps!==n.pendingProps)un=!0;else{if(!Nf(e,a)&&(n.flags&128)===0)return un=!1,yx(e,n,a);un=(e.flags&131072)!==0}else un=!1,Ue&&(n.flags&1048576)!==0&&tm(n,ro,n.index);switch(n.lanes=0,n.tag){case 16:t:{var o=n.pendingProps;if(e=ls(n.elementType),n.type=e,typeof e=="function")Bu(e)?(o=hs(e,o),n.tag=1,n=_g(null,n,e,o,a)):(n.tag=0,n=Af(null,n,e,o,a));else{if(e!=null){var u=e.$$typeof;if(u===T){n.tag=11,n=ug(null,n,e,o,a);break t}else if(u===F){n.tag=14,n=fg(null,n,e,o,a);break t}}throw n=ct(e)||e,Error(s(306,n,""))}}return n;case 0:return Af(e,n,n.type,n.pendingProps,a);case 1:return o=n.type,u=hs(o,n.pendingProps),_g(e,n,o,u,a);case 3:t:{if(xt(n,n.stateNode.containerInfo),e===null)throw Error(s(387));o=n.pendingProps;var f=n.memoizedState;u=f.element,Ju(e,n),mo(n,o,null,a);var y=n.memoizedState;if(o=y.cache,ya(n,ln,o),o!==f.cache&&ju(n,[ln],a,!0),po(),o=y.element,f.isDehydrated)if(f={element:o,isDehydrated:!1,cache:y.cache},n.updateQueue.baseState=f,n.memoizedState=f,n.flags&256){n=vg(e,n,o,a);break t}else if(o!==u){u=ci(Error(s(424)),n),oo(u),n=vg(e,n,o,a);break t}else{switch(e=n.stateNode.containerInfo,e.nodeType){case 9:e=e.body;break;default:e=e.nodeName==="HTML"?e.ownerDocument.body:e}for(Je=pi(e.firstChild),An=n,Ue=!0,_a=null,hi=!0,a=dm(n,null,o,a),n.child=a;a;)a.flags=a.flags&-3|4096,a=a.sibling}else{if(as(),o===u){n=Zi(e,n,a);break t}Cn(e,n,o,a)}n=n.child}return n;case 26:return kl(e,n),e===null?(a=N_(n.type,null,n.pendingProps,null))?n.memoizedState=a:Ue||(a=n.type,e=n.pendingProps,o=rc(q.current).createElement(a),o[on]=n,o[Tn]=e,wn(o,a,e),bt(o),n.stateNode=o):n.memoizedState=N_(n.type,e.memoizedProps,n.pendingProps,e.memoizedState),null;case 27:return Bt(n),e===null&&Ue&&(o=n.stateNode=D_(n.type,n.pendingProps,q.current),An=n,hi=!0,u=Je,La(n.type)?(hh=u,Je=pi(o.firstChild)):Je=u),Cn(e,n,n.pendingProps.children,a),kl(e,n),e===null&&(n.flags|=4194304),n.child;case 5:return e===null&&Ue&&((u=o=Je)&&(o=Zx(o,n.type,n.pendingProps,hi),o!==null?(n.stateNode=o,An=n,Je=pi(o.firstChild),hi=!1,u=!0):u=!1),u||va(n)),Bt(n),u=n.type,f=n.pendingProps,y=e!==null?e.memoizedProps:null,o=f.children,oh(u,f)?o=null:y!==null&&oh(u,y)&&(n.flags|=32),n.memoizedState!==null&&(u=rf(e,n,ux,null,null,a),Po._currentValue=u),kl(e,n),Cn(e,n,o,a),n.child;case 6:return e===null&&Ue&&((e=a=Je)&&(a=Kx(a,n.pendingProps,hi),a!==null?(n.stateNode=a,An=n,Je=null,e=!0):e=!1),e||va(n)),null;case 13:return yg(e,n,a);case 4:return xt(n,n.stateNode.containerInfo),o=n.pendingProps,e===null?n.child=us(n,null,o,a):Cn(e,n,o,a),n.child;case 11:return ug(e,n,n.type,n.pendingProps,a);case 7:return Cn(e,n,n.pendingProps,a),n.child;case 8:return Cn(e,n,n.pendingProps.children,a),n.child;case 12:return Cn(e,n,n.pendingProps.children,a),n.child;case 10:return o=n.pendingProps,ya(n,n.type,o.value),Cn(e,n,o.children,a),n.child;case 9:return u=n.type._context,o=n.pendingProps.children,rs(n),u=Rn(u),o=o(u),n.flags|=1,Cn(e,n,o,a),n.child;case 14:return fg(e,n,n.type,n.pendingProps,a);case 15:return hg(e,n,n.type,n.pendingProps,a);case 19:return Sg(e,n,a);case 31:return vx(e,n,a);case 22:return dg(e,n,a,n.pendingProps);case 24:return rs(n),o=Rn(ln),e===null?(u=Zu(),u===null&&(u=Ke,f=Yu(),u.pooledCache=f,f.refCount++,f!==null&&(u.pooledCacheLanes|=a),u=f),n.memoizedState={parent:o,cache:u},Qu(n),ya(n,ln,u)):((e.lanes&a)!==0&&(Ju(e,n),mo(n,null,null,a),po()),u=e.memoizedState,f=n.memoizedState,u.parent!==o?(u={parent:o,cache:o},n.memoizedState=u,n.lanes===0&&(n.memoizedState=n.updateQueue.baseState=u),ya(n,ln,o)):(o=f.cache,ya(n,ln,o),o!==u.cache&&ju(n,[ln],a,!0))),Cn(e,n,n.pendingProps.children,a),n.child;case 29:throw n.pendingProps}throw Error(s(156,n.tag))}function Ki(e){e.flags|=4}function Of(e,n,a,o,u){if((n=(e.mode&32)!==0)&&(n=!1),n){if(e.flags|=16777216,(u&335544128)===u)if(e.stateNode.complete)e.flags|=8192;else if(Zg())e.flags|=8192;else throw cs=Cl,Ku}else e.flags&=-16777217}function Eg(e,n){if(n.type!=="stylesheet"||(n.state.loading&4)!==0)e.flags&=-16777217;else if(e.flags|=16777216,!I_(n))if(Zg())e.flags|=8192;else throw cs=Cl,Ku}function Wl(e,n){n!==null&&(e.flags|=4),e.flags&16384&&(n=e.tag!==22?_e():536870912,e.lanes|=n,rr|=n)}function So(e,n){if(!Ue)switch(e.tailMode){case"hidden":n=e.tail;for(var a=null;n!==null;)n.alternate!==null&&(a=n),n=n.sibling;a===null?e.tail=null:a.sibling=null;break;case"collapsed":a=e.tail;for(var o=null;a!==null;)a.alternate!==null&&(o=a),a=a.sibling;o===null?n||e.tail===null?e.tail=null:e.tail.sibling=null:o.sibling=null}}function $e(e){var n=e.alternate!==null&&e.alternate.child===e.child,a=0,o=0;if(n)for(var u=e.child;u!==null;)a|=u.lanes|u.childLanes,o|=u.subtreeFlags&65011712,o|=u.flags&65011712,u.return=e,u=u.sibling;else for(u=e.child;u!==null;)a|=u.lanes|u.childLanes,o|=u.subtreeFlags,o|=u.flags,u.return=e,u=u.sibling;return e.subtreeFlags|=o,e.childLanes=a,n}function xx(e,n,a){var o=n.pendingProps;switch(Gu(n),n.tag){case 16:case 15:case 0:case 11:case 7:case 8:case 12:case 9:case 14:return $e(n),null;case 1:return $e(n),null;case 3:return a=n.stateNode,o=null,e!==null&&(o=e.memoizedState.cache),n.memoizedState.cache!==o&&(n.flags|=2048),ji(ln),Lt(),a.pendingContext&&(a.context=a.pendingContext,a.pendingContext=null),(e===null||e.child===null)&&(Ys(n)?Ki(n):e===null||e.memoizedState.isDehydrated&&(n.flags&256)===0||(n.flags|=1024,ku())),$e(n),null;case 26:var u=n.type,f=n.memoizedState;return e===null?(Ki(n),f!==null?($e(n),Eg(n,f)):($e(n),Of(n,u,null,o,a))):f?f!==e.memoizedState?(Ki(n),$e(n),Eg(n,f)):($e(n),n.flags&=-16777217):(e=e.memoizedProps,e!==o&&Ki(n),$e(n),Of(n,u,e,o,a)),null;case 27:if(le(n),a=q.current,u=n.type,e!==null&&n.stateNode!=null)e.memoizedProps!==o&&Ki(n);else{if(!o){if(n.stateNode===null)throw Error(s(166));return $e(n),null}e=wt.current,Ys(n)?nm(n):(e=D_(u,o,a),n.stateNode=e,Ki(n))}return $e(n),null;case 5:if(le(n),u=n.type,e!==null&&n.stateNode!=null)e.memoizedProps!==o&&Ki(n);else{if(!o){if(n.stateNode===null)throw Error(s(166));return $e(n),null}if(f=wt.current,Ys(n))nm(n);else{var y=rc(q.current);switch(f){case 1:f=y.createElementNS("http://www.w3.org/2000/svg",u);break;case 2:f=y.createElementNS("http://www.w3.org/1998/Math/MathML",u);break;default:switch(u){case"svg":f=y.createElementNS("http://www.w3.org/2000/svg",u);break;case"math":f=y.createElementNS("http://www.w3.org/1998/Math/MathML",u);break;case"script":f=y.createElement("div"),f.innerHTML="<script><\/script>",f=f.removeChild(f.firstChild);break;case"select":f=typeof o.is=="string"?y.createElement("select",{is:o.is}):y.createElement("select"),o.multiple?f.multiple=!0:o.size&&(f.size=o.size);break;default:f=typeof o.is=="string"?y.createElement(u,{is:o.is}):y.createElement(u)}}f[on]=n,f[Tn]=o;t:for(y=n.child;y!==null;){if(y.tag===5||y.tag===6)f.appendChild(y.stateNode);else if(y.tag!==4&&y.tag!==27&&y.child!==null){y.child.return=y,y=y.child;continue}if(y===n)break t;for(;y.sibling===null;){if(y.return===null||y.return===n)break t;y=y.return}y.sibling.return=y.return,y=y.sibling}n.stateNode=f;t:switch(wn(f,u,o),u){case"button":case"input":case"select":case"textarea":o=!!o.autoFocus;break t;case"img":o=!0;break t;default:o=!1}o&&Ki(n)}}return $e(n),Of(n,n.type,e===null?null:e.memoizedProps,n.pendingProps,a),null;case 6:if(e&&n.stateNode!=null)e.memoizedProps!==o&&Ki(n);else{if(typeof o!="string"&&n.stateNode===null)throw Error(s(166));if(e=q.current,Ys(n)){if(e=n.stateNode,a=n.memoizedProps,o=null,u=An,u!==null)switch(u.tag){case 27:case 5:o=u.memoizedProps}e[on]=n,e=!!(e.nodeValue===a||o!==null&&o.suppressHydrationWarning===!0||y_(e.nodeValue,a)),e||va(n,!0)}else e=rc(e).createTextNode(o),e[on]=n,n.stateNode=e}return $e(n),null;case 31:if(a=n.memoizedState,e===null||e.memoizedState!==null){if(o=Ys(n),a!==null){if(e===null){if(!o)throw Error(s(318));if(e=n.memoizedState,e=e!==null?e.dehydrated:null,!e)throw Error(s(557));e[on]=n}else as(),(n.flags&128)===0&&(n.memoizedState=null),n.flags|=4;$e(n),e=!1}else a=ku(),e!==null&&e.memoizedState!==null&&(e.memoizedState.hydrationErrors=a),e=!0;if(!e)return n.flags&256?(Jn(n),n):(Jn(n),null);if((n.flags&128)!==0)throw Error(s(558))}return $e(n),null;case 13:if(o=n.memoizedState,e===null||e.memoizedState!==null&&e.memoizedState.dehydrated!==null){if(u=Ys(n),o!==null&&o.dehydrated!==null){if(e===null){if(!u)throw Error(s(318));if(u=n.memoizedState,u=u!==null?u.dehydrated:null,!u)throw Error(s(317));u[on]=n}else as(),(n.flags&128)===0&&(n.memoizedState=null),n.flags|=4;$e(n),u=!1}else u=ku(),e!==null&&e.memoizedState!==null&&(e.memoizedState.hydrationErrors=u),u=!0;if(!u)return n.flags&256?(Jn(n),n):(Jn(n),null)}return Jn(n),(n.flags&128)!==0?(n.lanes=a,n):(a=o!==null,e=e!==null&&e.memoizedState!==null,a&&(o=n.child,u=null,o.alternate!==null&&o.alternate.memoizedState!==null&&o.alternate.memoizedState.cachePool!==null&&(u=o.alternate.memoizedState.cachePool.pool),f=null,o.memoizedState!==null&&o.memoizedState.cachePool!==null&&(f=o.memoizedState.cachePool.pool),f!==u&&(o.flags|=2048)),a!==e&&a&&(n.child.flags|=8192),Wl(n,n.updateQueue),$e(n),null);case 4:return Lt(),e===null&&nh(n.stateNode.containerInfo),$e(n),null;case 10:return ji(n.type),$e(n),null;case 19:if(at(sn),o=n.memoizedState,o===null)return $e(n),null;if(u=(n.flags&128)!==0,f=o.rendering,f===null)if(u)So(o,!1);else{if(nn!==0||e!==null&&(e.flags&128)!==0)for(e=n.child;e!==null;){if(f=Ll(e),f!==null){for(n.flags|=128,So(o,!1),e=f.updateQueue,n.updateQueue=e,Wl(n,e),n.subtreeFlags=0,e=a,a=n.child;a!==null;)Qp(a,e),a=a.sibling;return Mt(sn,sn.current&1|2),Ue&&Xi(n,o.treeForkCount),n.child}e=e.sibling}o.tail!==null&&E()>Kl&&(n.flags|=128,u=!0,So(o,!1),n.lanes=4194304)}else{if(!u)if(e=Ll(f),e!==null){if(n.flags|=128,u=!0,e=e.updateQueue,n.updateQueue=e,Wl(n,e),So(o,!0),o.tail===null&&o.tailMode==="hidden"&&!f.alternate&&!Ue)return $e(n),null}else 2*E()-o.renderingStartTime>Kl&&a!==536870912&&(n.flags|=128,u=!0,So(o,!1),n.lanes=4194304);o.isBackwards?(f.sibling=n.child,n.child=f):(e=o.last,e!==null?e.sibling=f:n.child=f,o.last=f)}return o.tail!==null?(e=o.tail,o.rendering=e,o.tail=e.sibling,o.renderingStartTime=E(),e.sibling=null,a=sn.current,Mt(sn,u?a&1|2:a&1),Ue&&Xi(n,o.treeForkCount),e):($e(n),null);case 22:case 23:return Jn(n),nf(),o=n.memoizedState!==null,e!==null?e.memoizedState!==null!==o&&(n.flags|=8192):o&&(n.flags|=8192),o?(a&536870912)!==0&&(n.flags&128)===0&&($e(n),n.subtreeFlags&6&&(n.flags|=8192)):$e(n),a=n.updateQueue,a!==null&&Wl(n,a.retryQueue),a=null,e!==null&&e.memoizedState!==null&&e.memoizedState.cachePool!==null&&(a=e.memoizedState.cachePool.pool),o=null,n.memoizedState!==null&&n.memoizedState.cachePool!==null&&(o=n.memoizedState.cachePool.pool),o!==a&&(n.flags|=2048),e!==null&&at(os),null;case 24:return a=null,e!==null&&(a=e.memoizedState.cache),n.memoizedState.cache!==a&&(n.flags|=2048),ji(ln),$e(n),null;case 25:return null;case 30:return null}throw Error(s(156,n.tag))}function Sx(e,n){switch(Gu(n),n.tag){case 1:return e=n.flags,e&65536?(n.flags=e&-65537|128,n):null;case 3:return ji(ln),Lt(),e=n.flags,(e&65536)!==0&&(e&128)===0?(n.flags=e&-65537|128,n):null;case 26:case 27:case 5:return le(n),null;case 31:if(n.memoizedState!==null){if(Jn(n),n.alternate===null)throw Error(s(340));as()}return e=n.flags,e&65536?(n.flags=e&-65537|128,n):null;case 13:if(Jn(n),e=n.memoizedState,e!==null&&e.dehydrated!==null){if(n.alternate===null)throw Error(s(340));as()}return e=n.flags,e&65536?(n.flags=e&-65537|128,n):null;case 19:return at(sn),null;case 4:return Lt(),null;case 10:return ji(n.type),null;case 22:case 23:return Jn(n),nf(),e!==null&&at(os),e=n.flags,e&65536?(n.flags=e&-65537|128,n):null;case 24:return ji(ln),null;case 25:return null;default:return null}}function Tg(e,n){switch(Gu(n),n.tag){case 3:ji(ln),Lt();break;case 26:case 27:case 5:le(n);break;case 4:Lt();break;case 31:n.memoizedState!==null&&Jn(n);break;case 13:Jn(n);break;case 19:at(sn);break;case 10:ji(n.type);break;case 22:case 23:Jn(n),nf(),e!==null&&at(os);break;case 24:ji(ln)}}function Mo(e,n){try{var a=n.updateQueue,o=a!==null?a.lastEffect:null;if(o!==null){var u=o.next;a=u;do{if((a.tag&e)===e){o=void 0;var f=a.create,y=a.inst;o=f(),y.destroy=o}a=a.next}while(a!==u)}}catch(A){He(n,n.return,A)}}function ba(e,n,a){try{var o=n.updateQueue,u=o!==null?o.lastEffect:null;if(u!==null){var f=u.next;o=f;do{if((o.tag&e)===e){var y=o.inst,A=y.destroy;if(A!==void 0){y.destroy=void 0,u=n;var H=a,nt=A;try{nt()}catch(vt){He(u,H,vt)}}}o=o.next}while(o!==f)}}catch(vt){He(n,n.return,vt)}}function bg(e){var n=e.updateQueue;if(n!==null){var a=e.stateNode;try{mm(n,a)}catch(o){He(e,e.return,o)}}}function Ag(e,n,a){a.props=hs(e.type,e.memoizedProps),a.state=e.memoizedState;try{a.componentWillUnmount()}catch(o){He(e,n,o)}}function Eo(e,n){try{var a=e.ref;if(a!==null){switch(e.tag){case 26:case 27:case 5:var o=e.stateNode;break;case 30:o=e.stateNode;break;default:o=e.stateNode}typeof a=="function"?e.refCleanup=a(o):a.current=o}}catch(u){He(e,n,u)}}function Li(e,n){var a=e.ref,o=e.refCleanup;if(a!==null)if(typeof o=="function")try{o()}catch(u){He(e,n,u)}finally{e.refCleanup=null,e=e.alternate,e!=null&&(e.refCleanup=null)}else if(typeof a=="function")try{a(null)}catch(u){He(e,n,u)}else a.current=null}function Rg(e){var n=e.type,a=e.memoizedProps,o=e.stateNode;try{t:switch(n){case"button":case"input":case"select":case"textarea":a.autoFocus&&o.focus();break t;case"img":a.src?o.src=a.src:a.srcSet&&(o.srcset=a.srcSet)}}catch(u){He(e,e.return,u)}}function Pf(e,n,a){try{var o=e.stateNode;kx(o,e.type,a,n),o[Tn]=n}catch(u){He(e,e.return,u)}}function Cg(e){return e.tag===5||e.tag===3||e.tag===26||e.tag===27&&La(e.type)||e.tag===4}function zf(e){t:for(;;){for(;e.sibling===null;){if(e.return===null||Cg(e.return))return null;e=e.return}for(e.sibling.return=e.return,e=e.sibling;e.tag!==5&&e.tag!==6&&e.tag!==18;){if(e.tag===27&&La(e.type)||e.flags&2||e.child===null||e.tag===4)continue t;e.child.return=e,e=e.child}if(!(e.flags&2))return e.stateNode}}function Bf(e,n,a){var o=e.tag;if(o===5||o===6)e=e.stateNode,n?(a.nodeType===9?a.body:a.nodeName==="HTML"?a.ownerDocument.body:a).insertBefore(e,n):(n=a.nodeType===9?a.body:a.nodeName==="HTML"?a.ownerDocument.body:a,n.appendChild(e),a=a._reactRootContainer,a!=null||n.onclick!==null||(n.onclick=Gi));else if(o!==4&&(o===27&&La(e.type)&&(a=e.stateNode,n=null),e=e.child,e!==null))for(Bf(e,n,a),e=e.sibling;e!==null;)Bf(e,n,a),e=e.sibling}function jl(e,n,a){var o=e.tag;if(o===5||o===6)e=e.stateNode,n?a.insertBefore(e,n):a.appendChild(e);else if(o!==4&&(o===27&&La(e.type)&&(a=e.stateNode),e=e.child,e!==null))for(jl(e,n,a),e=e.sibling;e!==null;)jl(e,n,a),e=e.sibling}function wg(e){var n=e.stateNode,a=e.memoizedProps;try{for(var o=e.type,u=n.attributes;u.length;)n.removeAttributeNode(u[0]);wn(n,o,a),n[on]=e,n[Tn]=a}catch(f){He(e,e.return,f)}}var Qi=!1,fn=!1,If=!1,Dg=typeof WeakSet=="function"?WeakSet:Set,xn=null;function Mx(e,n){if(e=e.containerInfo,sh=dc,e=Vp(e),Du(e)){if("selectionStart"in e)var a={start:e.selectionStart,end:e.selectionEnd};else t:{a=(a=e.ownerDocument)&&a.defaultView||window;var o=a.getSelection&&a.getSelection();if(o&&o.rangeCount!==0){a=o.anchorNode;var u=o.anchorOffset,f=o.focusNode;o=o.focusOffset;try{a.nodeType,f.nodeType}catch{a=null;break t}var y=0,A=-1,H=-1,nt=0,vt=0,St=e,st=null;e:for(;;){for(var ut;St!==a||u!==0&&St.nodeType!==3||(A=y+u),St!==f||o!==0&&St.nodeType!==3||(H=y+o),St.nodeType===3&&(y+=St.nodeValue.length),(ut=St.firstChild)!==null;)st=St,St=ut;for(;;){if(St===e)break e;if(st===a&&++nt===u&&(A=y),st===f&&++vt===o&&(H=y),(ut=St.nextSibling)!==null)break;St=st,st=St.parentNode}St=ut}a=A===-1||H===-1?null:{start:A,end:H}}else a=null}a=a||{start:0,end:0}}else a=null;for(rh={focusedElem:e,selectionRange:a},dc=!1,xn=n;xn!==null;)if(n=xn,e=n.child,(n.subtreeFlags&1028)!==0&&e!==null)e.return=n,xn=e;else for(;xn!==null;){switch(n=xn,f=n.alternate,e=n.flags,n.tag){case 0:if((e&4)!==0&&(e=n.updateQueue,e=e!==null?e.events:null,e!==null))for(a=0;a<e.length;a++)u=e[a],u.ref.impl=u.nextImpl;break;case 11:case 15:break;case 1:if((e&1024)!==0&&f!==null){e=void 0,a=n,u=f.memoizedProps,f=f.memoizedState,o=a.stateNode;try{var Yt=hs(a.type,u);e=o.getSnapshotBeforeUpdate(Yt,f),o.__reactInternalSnapshotBeforeUpdate=e}catch(ce){He(a,a.return,ce)}}break;case 3:if((e&1024)!==0){if(e=n.stateNode.containerInfo,a=e.nodeType,a===9)ch(e);else if(a===1)switch(e.nodeName){case"HEAD":case"HTML":case"BODY":ch(e);break;default:e.textContent=""}}break;case 5:case 26:case 27:case 6:case 4:case 17:break;default:if((e&1024)!==0)throw Error(s(163))}if(e=n.sibling,e!==null){e.return=n.return,xn=e;break}xn=n.return}}function Ug(e,n,a){var o=a.flags;switch(a.tag){case 0:case 11:case 15:$i(e,a),o&4&&Mo(5,a);break;case 1:if($i(e,a),o&4)if(e=a.stateNode,n===null)try{e.componentDidMount()}catch(y){He(a,a.return,y)}else{var u=hs(a.type,n.memoizedProps);n=n.memoizedState;try{e.componentDidUpdate(u,n,e.__reactInternalSnapshotBeforeUpdate)}catch(y){He(a,a.return,y)}}o&64&&bg(a),o&512&&Eo(a,a.return);break;case 3:if($i(e,a),o&64&&(e=a.updateQueue,e!==null)){if(n=null,a.child!==null)switch(a.child.tag){case 27:case 5:n=a.child.stateNode;break;case 1:n=a.child.stateNode}try{mm(e,n)}catch(y){He(a,a.return,y)}}break;case 27:n===null&&o&4&&wg(a);case 26:case 5:$i(e,a),n===null&&o&4&&Rg(a),o&512&&Eo(a,a.return);break;case 12:$i(e,a);break;case 31:$i(e,a),o&4&&Og(e,a);break;case 13:$i(e,a),o&4&&Pg(e,a),o&64&&(e=a.memoizedState,e!==null&&(e=e.dehydrated,e!==null&&(a=Ux.bind(null,a),Qx(e,a))));break;case 22:if(o=a.memoizedState!==null||Qi,!o){n=n!==null&&n.memoizedState!==null||fn,u=Qi;var f=fn;Qi=o,(fn=n)&&!f?ta(e,a,(a.subtreeFlags&8772)!==0):$i(e,a),Qi=u,fn=f}break;case 30:break;default:$i(e,a)}}function Lg(e){var n=e.alternate;n!==null&&(e.alternate=null,Lg(n)),e.child=null,e.deletions=null,e.sibling=null,e.tag===5&&(n=e.stateNode,n!==null&&C(n)),e.stateNode=null,e.return=null,e.dependencies=null,e.memoizedProps=null,e.memoizedState=null,e.pendingProps=null,e.stateNode=null,e.updateQueue=null}var tn=null,Hn=!1;function Ji(e,n,a){for(a=a.child;a!==null;)Ng(e,n,a),a=a.sibling}function Ng(e,n,a){if(Ot&&typeof Ot.onCommitFiberUnmount=="function")try{Ot.onCommitFiberUnmount(Ut,a)}catch{}switch(a.tag){case 26:fn||Li(a,n),Ji(e,n,a),a.memoizedState?a.memoizedState.count--:a.stateNode&&(a=a.stateNode,a.parentNode.removeChild(a));break;case 27:fn||Li(a,n);var o=tn,u=Hn;La(a.type)&&(tn=a.stateNode,Hn=!1),Ji(e,n,a),Lo(a.stateNode),tn=o,Hn=u;break;case 5:fn||Li(a,n);case 6:if(o=tn,u=Hn,tn=null,Ji(e,n,a),tn=o,Hn=u,tn!==null)if(Hn)try{(tn.nodeType===9?tn.body:tn.nodeName==="HTML"?tn.ownerDocument.body:tn).removeChild(a.stateNode)}catch(f){He(a,n,f)}else try{tn.removeChild(a.stateNode)}catch(f){He(a,n,f)}break;case 18:tn!==null&&(Hn?(e=tn,b_(e.nodeType===9?e.body:e.nodeName==="HTML"?e.ownerDocument.body:e,a.stateNode),pr(e)):b_(tn,a.stateNode));break;case 4:o=tn,u=Hn,tn=a.stateNode.containerInfo,Hn=!0,Ji(e,n,a),tn=o,Hn=u;break;case 0:case 11:case 14:case 15:ba(2,a,n),fn||ba(4,a,n),Ji(e,n,a);break;case 1:fn||(Li(a,n),o=a.stateNode,typeof o.componentWillUnmount=="function"&&Ag(a,n,o)),Ji(e,n,a);break;case 21:Ji(e,n,a);break;case 22:fn=(o=fn)||a.memoizedState!==null,Ji(e,n,a),fn=o;break;default:Ji(e,n,a)}}function Og(e,n){if(n.memoizedState===null&&(e=n.alternate,e!==null&&(e=e.memoizedState,e!==null))){e=e.dehydrated;try{pr(e)}catch(a){He(n,n.return,a)}}}function Pg(e,n){if(n.memoizedState===null&&(e=n.alternate,e!==null&&(e=e.memoizedState,e!==null&&(e=e.dehydrated,e!==null))))try{pr(e)}catch(a){He(n,n.return,a)}}function Ex(e){switch(e.tag){case 31:case 13:case 19:var n=e.stateNode;return n===null&&(n=e.stateNode=new Dg),n;case 22:return e=e.stateNode,n=e._retryCache,n===null&&(n=e._retryCache=new Dg),n;default:throw Error(s(435,e.tag))}}function Yl(e,n){var a=Ex(e);n.forEach(function(o){if(!a.has(o)){a.add(o);var u=Lx.bind(null,e,o);o.then(u,u)}})}function Gn(e,n){var a=n.deletions;if(a!==null)for(var o=0;o<a.length;o++){var u=a[o],f=e,y=n,A=y;t:for(;A!==null;){switch(A.tag){case 27:if(La(A.type)){tn=A.stateNode,Hn=!1;break t}break;case 5:tn=A.stateNode,Hn=!1;break t;case 3:case 4:tn=A.stateNode.containerInfo,Hn=!0;break t}A=A.return}if(tn===null)throw Error(s(160));Ng(f,y,u),tn=null,Hn=!1,f=u.alternate,f!==null&&(f.return=null),u.return=null}if(n.subtreeFlags&13886)for(n=n.child;n!==null;)zg(n,e),n=n.sibling}var Si=null;function zg(e,n){var a=e.alternate,o=e.flags;switch(e.tag){case 0:case 11:case 14:case 15:Gn(n,e),Vn(e),o&4&&(ba(3,e,e.return),Mo(3,e),ba(5,e,e.return));break;case 1:Gn(n,e),Vn(e),o&512&&(fn||a===null||Li(a,a.return)),o&64&&Qi&&(e=e.updateQueue,e!==null&&(o=e.callbacks,o!==null&&(a=e.shared.hiddenCallbacks,e.shared.hiddenCallbacks=a===null?o:a.concat(o))));break;case 26:var u=Si;if(Gn(n,e),Vn(e),o&512&&(fn||a===null||Li(a,a.return)),o&4){var f=a!==null?a.memoizedState:null;if(o=e.memoizedState,a===null)if(o===null)if(e.stateNode===null){t:{o=e.type,a=e.memoizedProps,u=u.ownerDocument||u;e:switch(o){case"title":f=u.getElementsByTagName("title")[0],(!f||f[Qa]||f[on]||f.namespaceURI==="http://www.w3.org/2000/svg"||f.hasAttribute("itemprop"))&&(f=u.createElement(o),u.head.insertBefore(f,u.querySelector("head > title"))),wn(f,o,a),f[on]=e,bt(f),o=f;break t;case"link":var y=z_("link","href",u).get(o+(a.href||""));if(y){for(var A=0;A<y.length;A++)if(f=y[A],f.getAttribute("href")===(a.href==null||a.href===""?null:a.href)&&f.getAttribute("rel")===(a.rel==null?null:a.rel)&&f.getAttribute("title")===(a.title==null?null:a.title)&&f.getAttribute("crossorigin")===(a.crossOrigin==null?null:a.crossOrigin)){y.splice(A,1);break e}}f=u.createElement(o),wn(f,o,a),u.head.appendChild(f);break;case"meta":if(y=z_("meta","content",u).get(o+(a.content||""))){for(A=0;A<y.length;A++)if(f=y[A],f.getAttribute("content")===(a.content==null?null:""+a.content)&&f.getAttribute("name")===(a.name==null?null:a.name)&&f.getAttribute("property")===(a.property==null?null:a.property)&&f.getAttribute("http-equiv")===(a.httpEquiv==null?null:a.httpEquiv)&&f.getAttribute("charset")===(a.charSet==null?null:a.charSet)){y.splice(A,1);break e}}f=u.createElement(o),wn(f,o,a),u.head.appendChild(f);break;default:throw Error(s(468,o))}f[on]=e,bt(f),o=f}e.stateNode=o}else B_(u,e.type,e.stateNode);else e.stateNode=P_(u,o,e.memoizedProps);else f!==o?(f===null?a.stateNode!==null&&(a=a.stateNode,a.parentNode.removeChild(a)):f.count--,o===null?B_(u,e.type,e.stateNode):P_(u,o,e.memoizedProps)):o===null&&e.stateNode!==null&&Pf(e,e.memoizedProps,a.memoizedProps)}break;case 27:Gn(n,e),Vn(e),o&512&&(fn||a===null||Li(a,a.return)),a!==null&&o&4&&Pf(e,e.memoizedProps,a.memoizedProps);break;case 5:if(Gn(n,e),Vn(e),o&512&&(fn||a===null||Li(a,a.return)),e.flags&32){u=e.stateNode;try{Bs(u,"")}catch(Yt){He(e,e.return,Yt)}}o&4&&e.stateNode!=null&&(u=e.memoizedProps,Pf(e,u,a!==null?a.memoizedProps:u)),o&1024&&(If=!0);break;case 6:if(Gn(n,e),Vn(e),o&4){if(e.stateNode===null)throw Error(s(162));o=e.memoizedProps,a=e.stateNode;try{a.nodeValue=o}catch(Yt){He(e,e.return,Yt)}}break;case 3:if(cc=null,u=Si,Si=oc(n.containerInfo),Gn(n,e),Si=u,Vn(e),o&4&&a!==null&&a.memoizedState.isDehydrated)try{pr(n.containerInfo)}catch(Yt){He(e,e.return,Yt)}If&&(If=!1,Bg(e));break;case 4:o=Si,Si=oc(e.stateNode.containerInfo),Gn(n,e),Vn(e),Si=o;break;case 12:Gn(n,e),Vn(e);break;case 31:Gn(n,e),Vn(e),o&4&&(o=e.updateQueue,o!==null&&(e.updateQueue=null,Yl(e,o)));break;case 13:Gn(n,e),Vn(e),e.child.flags&8192&&e.memoizedState!==null!=(a!==null&&a.memoizedState!==null)&&(Zl=E()),o&4&&(o=e.updateQueue,o!==null&&(e.updateQueue=null,Yl(e,o)));break;case 22:u=e.memoizedState!==null;var H=a!==null&&a.memoizedState!==null,nt=Qi,vt=fn;if(Qi=nt||u,fn=vt||H,Gn(n,e),fn=vt,Qi=nt,Vn(e),o&8192)t:for(n=e.stateNode,n._visibility=u?n._visibility&-2:n._visibility|1,u&&(a===null||H||Qi||fn||ds(e)),a=null,n=e;;){if(n.tag===5||n.tag===26){if(a===null){H=a=n;try{if(f=H.stateNode,u)y=f.style,typeof y.setProperty=="function"?y.setProperty("display","none","important"):y.display="none";else{A=H.stateNode;var St=H.memoizedProps.style,st=St!=null&&St.hasOwnProperty("display")?St.display:null;A.style.display=st==null||typeof st=="boolean"?"":(""+st).trim()}}catch(Yt){He(H,H.return,Yt)}}}else if(n.tag===6){if(a===null){H=n;try{H.stateNode.nodeValue=u?"":H.memoizedProps}catch(Yt){He(H,H.return,Yt)}}}else if(n.tag===18){if(a===null){H=n;try{var ut=H.stateNode;u?A_(ut,!0):A_(H.stateNode,!1)}catch(Yt){He(H,H.return,Yt)}}}else if((n.tag!==22&&n.tag!==23||n.memoizedState===null||n===e)&&n.child!==null){n.child.return=n,n=n.child;continue}if(n===e)break t;for(;n.sibling===null;){if(n.return===null||n.return===e)break t;a===n&&(a=null),n=n.return}a===n&&(a=null),n.sibling.return=n.return,n=n.sibling}o&4&&(o=e.updateQueue,o!==null&&(a=o.retryQueue,a!==null&&(o.retryQueue=null,Yl(e,a))));break;case 19:Gn(n,e),Vn(e),o&4&&(o=e.updateQueue,o!==null&&(e.updateQueue=null,Yl(e,o)));break;case 30:break;case 21:break;default:Gn(n,e),Vn(e)}}function Vn(e){var n=e.flags;if(n&2){try{for(var a,o=e.return;o!==null;){if(Cg(o)){a=o;break}o=o.return}if(a==null)throw Error(s(160));switch(a.tag){case 27:var u=a.stateNode,f=zf(e);jl(e,f,u);break;case 5:var y=a.stateNode;a.flags&32&&(Bs(y,""),a.flags&=-33);var A=zf(e);jl(e,A,y);break;case 3:case 4:var H=a.stateNode.containerInfo,nt=zf(e);Bf(e,nt,H);break;default:throw Error(s(161))}}catch(vt){He(e,e.return,vt)}e.flags&=-3}n&4096&&(e.flags&=-4097)}function Bg(e){if(e.subtreeFlags&1024)for(e=e.child;e!==null;){var n=e;Bg(n),n.tag===5&&n.flags&1024&&n.stateNode.reset(),e=e.sibling}}function $i(e,n){if(n.subtreeFlags&8772)for(n=n.child;n!==null;)Ug(e,n.alternate,n),n=n.sibling}function ds(e){for(e=e.child;e!==null;){var n=e;switch(n.tag){case 0:case 11:case 14:case 15:ba(4,n,n.return),ds(n);break;case 1:Li(n,n.return);var a=n.stateNode;typeof a.componentWillUnmount=="function"&&Ag(n,n.return,a),ds(n);break;case 27:Lo(n.stateNode);case 26:case 5:Li(n,n.return),ds(n);break;case 22:n.memoizedState===null&&ds(n);break;case 30:ds(n);break;default:ds(n)}e=e.sibling}}function ta(e,n,a){for(a=a&&(n.subtreeFlags&8772)!==0,n=n.child;n!==null;){var o=n.alternate,u=e,f=n,y=f.flags;switch(f.tag){case 0:case 11:case 15:ta(u,f,a),Mo(4,f);break;case 1:if(ta(u,f,a),o=f,u=o.stateNode,typeof u.componentDidMount=="function")try{u.componentDidMount()}catch(nt){He(o,o.return,nt)}if(o=f,u=o.updateQueue,u!==null){var A=o.stateNode;try{var H=u.shared.hiddenCallbacks;if(H!==null)for(u.shared.hiddenCallbacks=null,u=0;u<H.length;u++)pm(H[u],A)}catch(nt){He(o,o.return,nt)}}a&&y&64&&bg(f),Eo(f,f.return);break;case 27:wg(f);case 26:case 5:ta(u,f,a),a&&o===null&&y&4&&Rg(f),Eo(f,f.return);break;case 12:ta(u,f,a);break;case 31:ta(u,f,a),a&&y&4&&Og(u,f);break;case 13:ta(u,f,a),a&&y&4&&Pg(u,f);break;case 22:f.memoizedState===null&&ta(u,f,a),Eo(f,f.return);break;case 30:break;default:ta(u,f,a)}n=n.sibling}}function Ff(e,n){var a=null;e!==null&&e.memoizedState!==null&&e.memoizedState.cachePool!==null&&(a=e.memoizedState.cachePool.pool),e=null,n.memoizedState!==null&&n.memoizedState.cachePool!==null&&(e=n.memoizedState.cachePool.pool),e!==a&&(e!=null&&e.refCount++,a!=null&&lo(a))}function Hf(e,n){e=null,n.alternate!==null&&(e=n.alternate.memoizedState.cache),n=n.memoizedState.cache,n!==e&&(n.refCount++,e!=null&&lo(e))}function Mi(e,n,a,o){if(n.subtreeFlags&10256)for(n=n.child;n!==null;)Ig(e,n,a,o),n=n.sibling}function Ig(e,n,a,o){var u=n.flags;switch(n.tag){case 0:case 11:case 15:Mi(e,n,a,o),u&2048&&Mo(9,n);break;case 1:Mi(e,n,a,o);break;case 3:Mi(e,n,a,o),u&2048&&(e=null,n.alternate!==null&&(e=n.alternate.memoizedState.cache),n=n.memoizedState.cache,n!==e&&(n.refCount++,e!=null&&lo(e)));break;case 12:if(u&2048){Mi(e,n,a,o),e=n.stateNode;try{var f=n.memoizedProps,y=f.id,A=f.onPostCommit;typeof A=="function"&&A(y,n.alternate===null?"mount":"update",e.passiveEffectDuration,-0)}catch(H){He(n,n.return,H)}}else Mi(e,n,a,o);break;case 31:Mi(e,n,a,o);break;case 13:Mi(e,n,a,o);break;case 23:break;case 22:f=n.stateNode,y=n.alternate,n.memoizedState!==null?f._visibility&2?Mi(e,n,a,o):To(e,n):f._visibility&2?Mi(e,n,a,o):(f._visibility|=2,ir(e,n,a,o,(n.subtreeFlags&10256)!==0||!1)),u&2048&&Ff(y,n);break;case 24:Mi(e,n,a,o),u&2048&&Hf(n.alternate,n);break;default:Mi(e,n,a,o)}}function ir(e,n,a,o,u){for(u=u&&((n.subtreeFlags&10256)!==0||!1),n=n.child;n!==null;){var f=e,y=n,A=a,H=o,nt=y.flags;switch(y.tag){case 0:case 11:case 15:ir(f,y,A,H,u),Mo(8,y);break;case 23:break;case 22:var vt=y.stateNode;y.memoizedState!==null?vt._visibility&2?ir(f,y,A,H,u):To(f,y):(vt._visibility|=2,ir(f,y,A,H,u)),u&&nt&2048&&Ff(y.alternate,y);break;case 24:ir(f,y,A,H,u),u&&nt&2048&&Hf(y.alternate,y);break;default:ir(f,y,A,H,u)}n=n.sibling}}function To(e,n){if(n.subtreeFlags&10256)for(n=n.child;n!==null;){var a=e,o=n,u=o.flags;switch(o.tag){case 22:To(a,o),u&2048&&Ff(o.alternate,o);break;case 24:To(a,o),u&2048&&Hf(o.alternate,o);break;default:To(a,o)}n=n.sibling}}var bo=8192;function ar(e,n,a){if(e.subtreeFlags&bo)for(e=e.child;e!==null;)Fg(e,n,a),e=e.sibling}function Fg(e,n,a){switch(e.tag){case 26:ar(e,n,a),e.flags&bo&&e.memoizedState!==null&&cS(a,Si,e.memoizedState,e.memoizedProps);break;case 5:ar(e,n,a);break;case 3:case 4:var o=Si;Si=oc(e.stateNode.containerInfo),ar(e,n,a),Si=o;break;case 22:e.memoizedState===null&&(o=e.alternate,o!==null&&o.memoizedState!==null?(o=bo,bo=16777216,ar(e,n,a),bo=o):ar(e,n,a));break;default:ar(e,n,a)}}function Hg(e){var n=e.alternate;if(n!==null&&(e=n.child,e!==null)){n.child=null;do n=e.sibling,e.sibling=null,e=n;while(e!==null)}}function Ao(e){var n=e.deletions;if((e.flags&16)!==0){if(n!==null)for(var a=0;a<n.length;a++){var o=n[a];xn=o,Vg(o,e)}Hg(e)}if(e.subtreeFlags&10256)for(e=e.child;e!==null;)Gg(e),e=e.sibling}function Gg(e){switch(e.tag){case 0:case 11:case 15:Ao(e),e.flags&2048&&ba(9,e,e.return);break;case 3:Ao(e);break;case 12:Ao(e);break;case 22:var n=e.stateNode;e.memoizedState!==null&&n._visibility&2&&(e.return===null||e.return.tag!==13)?(n._visibility&=-3,ql(e)):Ao(e);break;default:Ao(e)}}function ql(e){var n=e.deletions;if((e.flags&16)!==0){if(n!==null)for(var a=0;a<n.length;a++){var o=n[a];xn=o,Vg(o,e)}Hg(e)}for(e=e.child;e!==null;){switch(n=e,n.tag){case 0:case 11:case 15:ba(8,n,n.return),ql(n);break;case 22:a=n.stateNode,a._visibility&2&&(a._visibility&=-3,ql(n));break;default:ql(n)}e=e.sibling}}function Vg(e,n){for(;xn!==null;){var a=xn;switch(a.tag){case 0:case 11:case 15:ba(8,a,n);break;case 23:case 22:if(a.memoizedState!==null&&a.memoizedState.cachePool!==null){var o=a.memoizedState.cachePool.pool;o!=null&&o.refCount++}break;case 24:lo(a.memoizedState.cache)}if(o=a.child,o!==null)o.return=a,xn=o;else t:for(a=e;xn!==null;){o=xn;var u=o.sibling,f=o.return;if(Lg(o),o===a){xn=null;break t}if(u!==null){u.return=f,xn=u;break t}xn=f}}}var Tx={getCacheForType:function(e){var n=Rn(ln),a=n.data.get(e);return a===void 0&&(a=e(),n.data.set(e,a)),a},cacheSignal:function(){return Rn(ln).controller.signal}},bx=typeof WeakMap=="function"?WeakMap:Map,ze=0,Ke=null,be=null,we=0,Fe=0,$n=null,Aa=!1,sr=!1,Gf=!1,ea=0,nn=0,Ra=0,ps=0,Vf=0,ti=0,rr=0,Ro=null,kn=null,kf=!1,Zl=0,kg=0,Kl=1/0,Ql=null,Ca=null,mn=0,wa=null,or=null,na=0,Xf=0,Wf=null,Xg=null,Co=0,jf=null;function ei(){return(ze&2)!==0&&we!==0?we&-we:N.T!==null?Jf():Ka()}function Wg(){if(ti===0)if((we&536870912)===0||Ue){var e=Rt;Rt<<=1,(Rt&3932160)===0&&(Rt=262144),ti=e}else ti=536870912;return e=Qn.current,e!==null&&(e.flags|=32),ti}function Xn(e,n,a){(e===Ke&&(Fe===2||Fe===9)||e.cancelPendingCommit!==null)&&(lr(e,0),Da(e,we,ti,!1)),En(e,a),((ze&2)===0||e!==Ke)&&(e===Ke&&((ze&2)===0&&(ps|=a),nn===4&&Da(e,we,ti,!1)),Ni(e))}function jg(e,n,a){if((ze&6)!==0)throw Error(s(327));var o=!a&&(n&127)===0&&(n&e.expiredLanes)===0||zt(e,n),u=o?Cx(e,n):qf(e,n,!0),f=o;do{if(u===0){sr&&!o&&Da(e,n,0,!1);break}else{if(a=e.current.alternate,f&&!Ax(a)){u=qf(e,n,!1),f=!1;continue}if(u===2){if(f=n,e.errorRecoveryDisabledLanes&f)var y=0;else y=e.pendingLanes&-536870913,y=y!==0?y:y&536870912?536870912:0;if(y!==0){n=y;t:{var A=e;u=Ro;var H=A.current.memoizedState.isDehydrated;if(H&&(lr(A,y).flags|=256),y=qf(A,y,!1),y!==2){if(Gf&&!H){A.errorRecoveryDisabledLanes|=f,ps|=f,u=4;break t}f=kn,kn=u,f!==null&&(kn===null?kn=f:kn.push.apply(kn,f))}u=y}if(f=!1,u!==2)continue}}if(u===1){lr(e,0),Da(e,n,0,!0);break}t:{switch(o=e,f=u,f){case 0:case 1:throw Error(s(345));case 4:if((n&4194048)!==n)break;case 6:Da(o,n,ti,!Aa);break t;case 2:kn=null;break;case 3:case 5:break;default:throw Error(s(329))}if((n&62914560)===n&&(u=Zl+300-E(),10<u)){if(Da(o,n,ti,!Aa),pt(o,0,!0)!==0)break t;na=n,o.timeoutHandle=E_(Yg.bind(null,o,a,kn,Ql,kf,n,ti,ps,rr,Aa,f,"Throttled",-0,0),u);break t}Yg(o,a,kn,Ql,kf,n,ti,ps,rr,Aa,f,null,-0,0)}}break}while(!0);Ni(e)}function Yg(e,n,a,o,u,f,y,A,H,nt,vt,St,st,ut){if(e.timeoutHandle=-1,St=n.subtreeFlags,St&8192||(St&16785408)===16785408){St={stylesheets:null,count:0,imgCount:0,imgBytes:0,suspenseyImages:[],waitingForImages:!0,waitingForViewTransition:!1,unsuspend:Gi},Fg(n,f,St);var Yt=(f&62914560)===f?Zl-E():(f&4194048)===f?kg-E():0;if(Yt=uS(St,Yt),Yt!==null){na=f,e.cancelPendingCommit=Yt(e_.bind(null,e,n,f,a,o,u,y,A,H,vt,St,null,st,ut)),Da(e,f,y,!nt);return}}e_(e,n,f,a,o,u,y,A,H)}function Ax(e){for(var n=e;;){var a=n.tag;if((a===0||a===11||a===15)&&n.flags&16384&&(a=n.updateQueue,a!==null&&(a=a.stores,a!==null)))for(var o=0;o<a.length;o++){var u=a[o],f=u.getSnapshot;u=u.value;try{if(!Zn(f(),u))return!1}catch{return!1}}if(a=n.child,n.subtreeFlags&16384&&a!==null)a.return=n,n=a;else{if(n===e)break;for(;n.sibling===null;){if(n.return===null||n.return===e)return!0;n=n.return}n.sibling.return=n.return,n=n.sibling}}return!0}function Da(e,n,a,o){n&=~Vf,n&=~ps,e.suspendedLanes|=n,e.pingedLanes&=~n,o&&(e.warmLanes|=n),o=e.expirationTimes;for(var u=n;0<u;){var f=31-It(u),y=1<<f;o[f]=-1,u&=~y}a!==0&&qr(e,a,n)}function Jl(){return(ze&6)===0?(wo(0),!1):!0}function Yf(){if(be!==null){if(Fe===0)var e=be.return;else e=be,Wi=ss=null,cf(e),Js=null,uo=0,e=be;for(;e!==null;)Tg(e.alternate,e),e=e.return;be=null}}function lr(e,n){var a=e.timeoutHandle;a!==-1&&(e.timeoutHandle=-1,jx(a)),a=e.cancelPendingCommit,a!==null&&(e.cancelPendingCommit=null,a()),na=0,Yf(),Ke=e,be=a=ki(e.current,null),we=n,Fe=0,$n=null,Aa=!1,sr=zt(e,n),Gf=!1,rr=ti=Vf=ps=Ra=nn=0,kn=Ro=null,kf=!1,(n&8)!==0&&(n|=n&32);var o=e.entangledLanes;if(o!==0)for(e=e.entanglements,o&=n;0<o;){var u=31-It(o),f=1<<u;n|=e[u],o&=~f}return ea=n,yl(),a}function qg(e,n){de=null,N.H=yo,n===Qs||n===Rl?(n=um(),Fe=3):n===Ku?(n=um(),Fe=4):Fe=n===bf?8:n!==null&&typeof n=="object"&&typeof n.then=="function"?6:1,$n=n,be===null&&(nn=1,Gl(e,ci(n,e.current)))}function Zg(){var e=Qn.current;return e===null?!0:(we&4194048)===we?di===null:(we&62914560)===we||(we&536870912)!==0?e===di:!1}function Kg(){var e=N.H;return N.H=yo,e===null?yo:e}function Qg(){var e=N.A;return N.A=Tx,e}function $l(){nn=4,Aa||(we&4194048)!==we&&Qn.current!==null||(sr=!0),(Ra&134217727)===0&&(ps&134217727)===0||Ke===null||Da(Ke,we,ti,!1)}function qf(e,n,a){var o=ze;ze|=2;var u=Kg(),f=Qg();(Ke!==e||we!==n)&&(Ql=null,lr(e,n)),n=!1;var y=nn;t:do try{if(Fe!==0&&be!==null){var A=be,H=$n;switch(Fe){case 8:Yf(),y=6;break t;case 3:case 2:case 9:case 6:Qn.current===null&&(n=!0);var nt=Fe;if(Fe=0,$n=null,cr(e,A,H,nt),a&&sr){y=0;break t}break;default:nt=Fe,Fe=0,$n=null,cr(e,A,H,nt)}}Rx(),y=nn;break}catch(vt){qg(e,vt)}while(!0);return n&&e.shellSuspendCounter++,Wi=ss=null,ze=o,N.H=u,N.A=f,be===null&&(Ke=null,we=0,yl()),y}function Rx(){for(;be!==null;)Jg(be)}function Cx(e,n){var a=ze;ze|=2;var o=Kg(),u=Qg();Ke!==e||we!==n?(Ql=null,Kl=E()+500,lr(e,n)):sr=zt(e,n);t:do try{if(Fe!==0&&be!==null){n=be;var f=$n;e:switch(Fe){case 1:Fe=0,$n=null,cr(e,n,f,1);break;case 2:case 9:if(lm(f)){Fe=0,$n=null,$g(n);break}n=function(){Fe!==2&&Fe!==9||Ke!==e||(Fe=7),Ni(e)},f.then(n,n);break t;case 3:Fe=7;break t;case 4:Fe=5;break t;case 7:lm(f)?(Fe=0,$n=null,$g(n)):(Fe=0,$n=null,cr(e,n,f,7));break;case 5:var y=null;switch(be.tag){case 26:y=be.memoizedState;case 5:case 27:var A=be;if(y?I_(y):A.stateNode.complete){Fe=0,$n=null;var H=A.sibling;if(H!==null)be=H;else{var nt=A.return;nt!==null?(be=nt,tc(nt)):be=null}break e}}Fe=0,$n=null,cr(e,n,f,5);break;case 6:Fe=0,$n=null,cr(e,n,f,6);break;case 8:Yf(),nn=6;break t;default:throw Error(s(462))}}wx();break}catch(vt){qg(e,vt)}while(!0);return Wi=ss=null,N.H=o,N.A=u,ze=a,be!==null?0:(Ke=null,we=0,yl(),nn)}function wx(){for(;be!==null&&!Se();)Jg(be)}function Jg(e){var n=Mg(e.alternate,e,ea);e.memoizedProps=e.pendingProps,n===null?tc(e):be=n}function $g(e){var n=e,a=n.alternate;switch(n.tag){case 15:case 0:n=gg(a,n,n.pendingProps,n.type,void 0,we);break;case 11:n=gg(a,n,n.pendingProps,n.type.render,n.ref,we);break;case 5:cf(n);default:Tg(a,n),n=be=Qp(n,ea),n=Mg(a,n,ea)}e.memoizedProps=e.pendingProps,n===null?tc(e):be=n}function cr(e,n,a,o){Wi=ss=null,cf(n),Js=null,uo=0;var u=n.return;try{if(_x(e,u,n,a,we)){nn=1,Gl(e,ci(a,e.current)),be=null;return}}catch(f){if(u!==null)throw be=u,f;nn=1,Gl(e,ci(a,e.current)),be=null;return}n.flags&32768?(Ue||o===1?e=!0:sr||(we&536870912)!==0?e=!1:(Aa=e=!0,(o===2||o===9||o===3||o===6)&&(o=Qn.current,o!==null&&o.tag===13&&(o.flags|=16384))),t_(n,e)):tc(n)}function tc(e){var n=e;do{if((n.flags&32768)!==0){t_(n,Aa);return}e=n.return;var a=xx(n.alternate,n,ea);if(a!==null){be=a;return}if(n=n.sibling,n!==null){be=n;return}be=n=e}while(n!==null);nn===0&&(nn=5)}function t_(e,n){do{var a=Sx(e.alternate,e);if(a!==null){a.flags&=32767,be=a;return}if(a=e.return,a!==null&&(a.flags|=32768,a.subtreeFlags=0,a.deletions=null),!n&&(e=e.sibling,e!==null)){be=e;return}be=e=a}while(e!==null);nn=6,be=null}function e_(e,n,a,o,u,f,y,A,H){e.cancelPendingCommit=null;do ec();while(mn!==0);if((ze&6)!==0)throw Error(s(327));if(n!==null){if(n===e.current)throw Error(s(177));if(f=n.lanes|n.childLanes,f|=Pu,oi(e,a,f,y,A,H),e===Ke&&(be=Ke=null,we=0),or=n,wa=e,na=a,Xf=f,Wf=u,Xg=o,(n.subtreeFlags&10256)!==0||(n.flags&10256)!==0?(e.callbackNode=null,e.callbackPriority=0,Nx(dt,function(){return r_(),null})):(e.callbackNode=null,e.callbackPriority=0),o=(n.flags&13878)!==0,(n.subtreeFlags&13878)!==0||o){o=N.T,N.T=null,u=J.p,J.p=2,y=ze,ze|=4;try{Mx(e,n,a)}finally{ze=y,J.p=u,N.T=o}}mn=1,n_(),i_(),a_()}}function n_(){if(mn===1){mn=0;var e=wa,n=or,a=(n.flags&13878)!==0;if((n.subtreeFlags&13878)!==0||a){a=N.T,N.T=null;var o=J.p;J.p=2;var u=ze;ze|=4;try{zg(n,e);var f=rh,y=Vp(e.containerInfo),A=f.focusedElem,H=f.selectionRange;if(y!==A&&A&&A.ownerDocument&&Gp(A.ownerDocument.documentElement,A)){if(H!==null&&Du(A)){var nt=H.start,vt=H.end;if(vt===void 0&&(vt=nt),"selectionStart"in A)A.selectionStart=nt,A.selectionEnd=Math.min(vt,A.value.length);else{var St=A.ownerDocument||document,st=St&&St.defaultView||window;if(st.getSelection){var ut=st.getSelection(),Yt=A.textContent.length,ce=Math.min(H.start,Yt),Xe=H.end===void 0?ce:Math.min(H.end,Yt);!ut.extend&&ce>Xe&&(y=Xe,Xe=ce,ce=y);var Q=Hp(A,ce),k=Hp(A,Xe);if(Q&&k&&(ut.rangeCount!==1||ut.anchorNode!==Q.node||ut.anchorOffset!==Q.offset||ut.focusNode!==k.node||ut.focusOffset!==k.offset)){var et=St.createRange();et.setStart(Q.node,Q.offset),ut.removeAllRanges(),ce>Xe?(ut.addRange(et),ut.extend(k.node,k.offset)):(et.setEnd(k.node,k.offset),ut.addRange(et))}}}}for(St=[],ut=A;ut=ut.parentNode;)ut.nodeType===1&&St.push({element:ut,left:ut.scrollLeft,top:ut.scrollTop});for(typeof A.focus=="function"&&A.focus(),A=0;A<St.length;A++){var yt=St[A];yt.element.scrollLeft=yt.left,yt.element.scrollTop=yt.top}}dc=!!sh,rh=sh=null}finally{ze=u,J.p=o,N.T=a}}e.current=n,mn=2}}function i_(){if(mn===2){mn=0;var e=wa,n=or,a=(n.flags&8772)!==0;if((n.subtreeFlags&8772)!==0||a){a=N.T,N.T=null;var o=J.p;J.p=2;var u=ze;ze|=4;try{Ug(e,n.alternate,n)}finally{ze=u,J.p=o,N.T=a}}mn=3}}function a_(){if(mn===4||mn===3){mn=0,O();var e=wa,n=or,a=na,o=Xg;(n.subtreeFlags&10256)!==0||(n.flags&10256)!==0?mn=5:(mn=0,or=wa=null,s_(e,e.pendingLanes));var u=e.pendingLanes;if(u===0&&(Ca=null),Ps(a),n=n.stateNode,Ot&&typeof Ot.onCommitFiberRoot=="function")try{Ot.onCommitFiberRoot(Ut,n,void 0,(n.current.flags&128)===128)}catch{}if(o!==null){n=N.T,u=J.p,J.p=2,N.T=null;try{for(var f=e.onRecoverableError,y=0;y<o.length;y++){var A=o[y];f(A.value,{componentStack:A.stack})}}finally{N.T=n,J.p=u}}(na&3)!==0&&ec(),Ni(e),u=e.pendingLanes,(a&261930)!==0&&(u&42)!==0?e===jf?Co++:(Co=0,jf=e):Co=0,wo(0)}}function s_(e,n){(e.pooledCacheLanes&=n)===0&&(n=e.pooledCache,n!=null&&(e.pooledCache=null,lo(n)))}function ec(){return n_(),i_(),a_(),r_()}function r_(){if(mn!==5)return!1;var e=wa,n=Xf;Xf=0;var a=Ps(na),o=N.T,u=J.p;try{J.p=32>a?32:a,N.T=null,a=Wf,Wf=null;var f=wa,y=na;if(mn=0,or=wa=null,na=0,(ze&6)!==0)throw Error(s(331));var A=ze;if(ze|=4,Gg(f.current),Ig(f,f.current,y,a),ze=A,wo(0,!1),Ot&&typeof Ot.onPostCommitFiberRoot=="function")try{Ot.onPostCommitFiberRoot(Ut,f)}catch{}return!0}finally{J.p=u,N.T=o,s_(e,n)}}function o_(e,n,a){n=ci(a,n),n=Tf(e.stateNode,n,2),e=Ma(e,n,2),e!==null&&(En(e,2),Ni(e))}function He(e,n,a){if(e.tag===3)o_(e,e,a);else for(;n!==null;){if(n.tag===3){o_(n,e,a);break}else if(n.tag===1){var o=n.stateNode;if(typeof n.type.getDerivedStateFromError=="function"||typeof o.componentDidCatch=="function"&&(Ca===null||!Ca.has(o))){e=ci(a,e),a=lg(2),o=Ma(n,a,2),o!==null&&(cg(a,o,n,e),En(o,2),Ni(o));break}}n=n.return}}function Zf(e,n,a){var o=e.pingCache;if(o===null){o=e.pingCache=new bx;var u=new Set;o.set(n,u)}else u=o.get(n),u===void 0&&(u=new Set,o.set(n,u));u.has(a)||(Gf=!0,u.add(a),e=Dx.bind(null,e,n,a),n.then(e,e))}function Dx(e,n,a){var o=e.pingCache;o!==null&&o.delete(n),e.pingedLanes|=e.suspendedLanes&a,e.warmLanes&=~a,Ke===e&&(we&a)===a&&(nn===4||nn===3&&(we&62914560)===we&&300>E()-Zl?(ze&2)===0&&lr(e,0):Vf|=a,rr===we&&(rr=0)),Ni(e)}function l_(e,n){n===0&&(n=_e()),e=ns(e,n),e!==null&&(En(e,n),Ni(e))}function Ux(e){var n=e.memoizedState,a=0;n!==null&&(a=n.retryLane),l_(e,a)}function Lx(e,n){var a=0;switch(e.tag){case 31:case 13:var o=e.stateNode,u=e.memoizedState;u!==null&&(a=u.retryLane);break;case 19:o=e.stateNode;break;case 22:o=e.stateNode._retryCache;break;default:throw Error(s(314))}o!==null&&o.delete(n),l_(e,a)}function Nx(e,n){return Ht(e,n)}var nc=null,ur=null,Kf=!1,ic=!1,Qf=!1,Ua=0;function Ni(e){e!==ur&&e.next===null&&(ur===null?nc=ur=e:ur=ur.next=e),ic=!0,Kf||(Kf=!0,Px())}function wo(e,n){if(!Qf&&ic){Qf=!0;do for(var a=!1,o=nc;o!==null;){if(e!==0){var u=o.pendingLanes;if(u===0)var f=0;else{var y=o.suspendedLanes,A=o.pingedLanes;f=(1<<31-It(42|e)+1)-1,f&=u&~(y&~A),f=f&201326741?f&201326741|1:f?f|2:0}f!==0&&(a=!0,h_(o,f))}else f=we,f=pt(o,o===Ke?f:0,o.cancelPendingCommit!==null||o.timeoutHandle!==-1),(f&3)===0||zt(o,f)||(a=!0,h_(o,f));o=o.next}while(a);Qf=!1}}function Ox(){c_()}function c_(){ic=Kf=!1;var e=0;Ua!==0&&Wx()&&(e=Ua);for(var n=E(),a=null,o=nc;o!==null;){var u=o.next,f=u_(o,n);f===0?(o.next=null,a===null?nc=u:a.next=u,u===null&&(ur=a)):(a=o,(e!==0||(f&3)!==0)&&(ic=!0)),o=u}mn!==0&&mn!==5||wo(e),Ua!==0&&(Ua=0)}function u_(e,n){for(var a=e.suspendedLanes,o=e.pingedLanes,u=e.expirationTimes,f=e.pendingLanes&-62914561;0<f;){var y=31-It(f),A=1<<y,H=u[y];H===-1?((A&a)===0||(A&o)!==0)&&(u[y]=ne(A,n)):H<=n&&(e.expiredLanes|=A),f&=~A}if(n=Ke,a=we,a=pt(e,e===n?a:0,e.cancelPendingCommit!==null||e.timeoutHandle!==-1),o=e.callbackNode,a===0||e===n&&(Fe===2||Fe===9)||e.cancelPendingCommit!==null)return o!==null&&o!==null&&xe(o),e.callbackNode=null,e.callbackPriority=0;if((a&3)===0||zt(e,a)){if(n=a&-a,n===e.callbackPriority)return n;switch(o!==null&&xe(o),Ps(a)){case 2:case 8:a=At;break;case 32:a=dt;break;case 268435456:a=Nt;break;default:a=dt}return o=f_.bind(null,e),a=Ht(a,o),e.callbackPriority=n,e.callbackNode=a,n}return o!==null&&o!==null&&xe(o),e.callbackPriority=2,e.callbackNode=null,2}function f_(e,n){if(mn!==0&&mn!==5)return e.callbackNode=null,e.callbackPriority=0,null;var a=e.callbackNode;if(ec()&&e.callbackNode!==a)return null;var o=we;return o=pt(e,e===Ke?o:0,e.cancelPendingCommit!==null||e.timeoutHandle!==-1),o===0?null:(jg(e,o,n),u_(e,E()),e.callbackNode!=null&&e.callbackNode===a?f_.bind(null,e):null)}function h_(e,n){if(ec())return null;jg(e,n,!0)}function Px(){Yx(function(){(ze&6)!==0?Ht(gt,Ox):c_()})}function Jf(){if(Ua===0){var e=Zs;e===0&&(e=_t,_t<<=1,(_t&261888)===0&&(_t=256)),Ua=e}return Ua}function d_(e){return e==null||typeof e=="symbol"||typeof e=="boolean"?null:typeof e=="function"?e:fl(""+e)}function p_(e,n){var a=n.ownerDocument.createElement("input");return a.name=n.name,a.value=n.value,e.id&&a.setAttribute("form",e.id),n.parentNode.insertBefore(a,n),e=new FormData(e),a.parentNode.removeChild(a),e}function zx(e,n,a,o,u){if(n==="submit"&&a&&a.stateNode===u){var f=d_((u[Tn]||null).action),y=o.submitter;y&&(n=(n=y[Tn]||null)?d_(n.formAction):y.getAttribute("formAction"),n!==null&&(f=n,y=null));var A=new ml("action","action",null,o,u);e.push({event:A,listeners:[{instance:null,listener:function(){if(o.defaultPrevented){if(Ua!==0){var H=y?p_(u,y):new FormData(u);vf(a,{pending:!0,data:H,method:u.method,action:f},null,H)}}else typeof f=="function"&&(A.preventDefault(),H=y?p_(u,y):new FormData(u),vf(a,{pending:!0,data:H,method:u.method,action:f},f,H))},currentTarget:u}]})}}for(var $f=0;$f<Ou.length;$f++){var th=Ou[$f],Bx=th.toLowerCase(),Ix=th[0].toUpperCase()+th.slice(1);xi(Bx,"on"+Ix)}xi(Wp,"onAnimationEnd"),xi(jp,"onAnimationIteration"),xi(Yp,"onAnimationStart"),xi("dblclick","onDoubleClick"),xi("focusin","onFocus"),xi("focusout","onBlur"),xi(tx,"onTransitionRun"),xi(ex,"onTransitionStart"),xi(nx,"onTransitionCancel"),xi(qp,"onTransitionEnd"),ie("onMouseEnter",["mouseout","mouseover"]),ie("onMouseLeave",["mouseout","mouseover"]),ie("onPointerEnter",["pointerout","pointerover"]),ie("onPointerLeave",["pointerout","pointerover"]),kt("onChange","change click focusin focusout input keydown keyup selectionchange".split(" ")),kt("onSelect","focusout contextmenu dragend focusin keydown keyup mousedown mouseup selectionchange".split(" ")),kt("onBeforeInput",["compositionend","keypress","textInput","paste"]),kt("onCompositionEnd","compositionend focusout keydown keypress keyup mousedown".split(" ")),kt("onCompositionStart","compositionstart focusout keydown keypress keyup mousedown".split(" ")),kt("onCompositionUpdate","compositionupdate focusout keydown keypress keyup mousedown".split(" "));var Do="abort canplay canplaythrough durationchange emptied encrypted ended error loadeddata loadedmetadata loadstart pause play playing progress ratechange resize seeked seeking stalled suspend timeupdate volumechange waiting".split(" "),Fx=new Set("beforetoggle cancel close invalid load scroll scrollend toggle".split(" ").concat(Do));function m_(e,n){n=(n&4)!==0;for(var a=0;a<e.length;a++){var o=e[a],u=o.event;o=o.listeners;t:{var f=void 0;if(n)for(var y=o.length-1;0<=y;y--){var A=o[y],H=A.instance,nt=A.currentTarget;if(A=A.listener,H!==f&&u.isPropagationStopped())break t;f=A,u.currentTarget=nt;try{f(u)}catch(vt){vl(vt)}u.currentTarget=null,f=H}else for(y=0;y<o.length;y++){if(A=o[y],H=A.instance,nt=A.currentTarget,A=A.listener,H!==f&&u.isPropagationStopped())break t;f=A,u.currentTarget=nt;try{f(u)}catch(vt){vl(vt)}u.currentTarget=null,f=H}}}}function Ae(e,n){var a=n[Kr];a===void 0&&(a=n[Kr]=new Set);var o=e+"__bubble";a.has(o)||(g_(n,e,2,!1),a.add(o))}function eh(e,n,a){var o=0;n&&(o|=4),g_(a,e,o,n)}var ac="_reactListening"+Math.random().toString(36).slice(2);function nh(e){if(!e[ac]){e[ac]=!0,Gt.forEach(function(a){a!=="selectionchange"&&(Fx.has(a)||eh(a,!1,e),eh(a,!0,e))});var n=e.nodeType===9?e:e.ownerDocument;n===null||n[ac]||(n[ac]=!0,eh("selectionchange",!1,n))}}function g_(e,n,a,o){switch(W_(n)){case 2:var u=dS;break;case 8:u=pS;break;default:u=_h}a=u.bind(null,n,a,e),u=void 0,!Su||n!=="touchstart"&&n!=="touchmove"&&n!=="wheel"||(u=!0),o?u!==void 0?e.addEventListener(n,a,{capture:!0,passive:u}):e.addEventListener(n,a,!0):u!==void 0?e.addEventListener(n,a,{passive:u}):e.addEventListener(n,a,!1)}function ih(e,n,a,o,u){var f=o;if((n&1)===0&&(n&2)===0&&o!==null)t:for(;;){if(o===null)return;var y=o.tag;if(y===3||y===4){var A=o.stateNode.containerInfo;if(A===u)break;if(y===4)for(y=o.return;y!==null;){var H=y.tag;if((H===3||H===4)&&y.stateNode.containerInfo===u)return;y=y.return}for(;A!==null;){if(y=Z(A),y===null)return;if(H=y.tag,H===5||H===6||H===26||H===27){o=f=y;continue t}A=A.parentNode}}o=o.return}Sp(function(){var nt=f,vt=yu(a),St=[];t:{var st=Zp.get(e);if(st!==void 0){var ut=ml,Yt=e;switch(e){case"keypress":if(dl(a)===0)break t;case"keydown":case"keyup":ut=Ly;break;case"focusin":Yt="focus",ut=bu;break;case"focusout":Yt="blur",ut=bu;break;case"beforeblur":case"afterblur":ut=bu;break;case"click":if(a.button===2)break t;case"auxclick":case"dblclick":case"mousedown":case"mousemove":case"mouseup":case"mouseout":case"mouseover":case"contextmenu":ut=Tp;break;case"drag":case"dragend":case"dragenter":case"dragexit":case"dragleave":case"dragover":case"dragstart":case"drop":ut=xy;break;case"touchcancel":case"touchend":case"touchmove":case"touchstart":ut=Py;break;case Wp:case jp:case Yp:ut=Ey;break;case qp:ut=By;break;case"scroll":case"scrollend":ut=vy;break;case"wheel":ut=Fy;break;case"copy":case"cut":case"paste":ut=by;break;case"gotpointercapture":case"lostpointercapture":case"pointercancel":case"pointerdown":case"pointermove":case"pointerout":case"pointerover":case"pointerup":ut=Ap;break;case"toggle":case"beforetoggle":ut=Gy}var ce=(n&4)!==0,Xe=!ce&&(e==="scroll"||e==="scrollend"),Q=ce?st!==null?st+"Capture":null:st;ce=[];for(var k=nt,et;k!==null;){var yt=k;if(et=yt.stateNode,yt=yt.tag,yt!==5&&yt!==26&&yt!==27||et===null||Q===null||(yt=Qr(k,Q),yt!=null&&ce.push(Uo(k,yt,et))),Xe)break;k=k.return}0<ce.length&&(st=new ut(st,Yt,null,a,vt),St.push({event:st,listeners:ce}))}}if((n&7)===0){t:{if(st=e==="mouseover"||e==="pointerover",ut=e==="mouseout"||e==="pointerout",st&&a!==vu&&(Yt=a.relatedTarget||a.fromElement)&&(Z(Yt)||Yt[pa]))break t;if((ut||st)&&(st=vt.window===vt?vt:(st=vt.ownerDocument)?st.defaultView||st.parentWindow:window,ut?(Yt=a.relatedTarget||a.toElement,ut=nt,Yt=Yt?Z(Yt):null,Yt!==null&&(Xe=c(Yt),ce=Yt.tag,Yt!==Xe||ce!==5&&ce!==27&&ce!==6)&&(Yt=null)):(ut=null,Yt=nt),ut!==Yt)){if(ce=Tp,yt="onMouseLeave",Q="onMouseEnter",k="mouse",(e==="pointerout"||e==="pointerover")&&(ce=Ap,yt="onPointerLeave",Q="onPointerEnter",k="pointer"),Xe=ut==null?st:lt(ut),et=Yt==null?st:lt(Yt),st=new ce(yt,k+"leave",ut,a,vt),st.target=Xe,st.relatedTarget=et,yt=null,Z(vt)===nt&&(ce=new ce(Q,k+"enter",Yt,a,vt),ce.target=et,ce.relatedTarget=Xe,yt=ce),Xe=yt,ut&&Yt)e:{for(ce=Hx,Q=ut,k=Yt,et=0,yt=Q;yt;yt=ce(yt))et++;yt=0;for(var se=k;se;se=ce(se))yt++;for(;0<et-yt;)Q=ce(Q),et--;for(;0<yt-et;)k=ce(k),yt--;for(;et--;){if(Q===k||k!==null&&Q===k.alternate){ce=Q;break e}Q=ce(Q),k=ce(k)}ce=null}else ce=null;ut!==null&&__(St,st,ut,ce,!1),Yt!==null&&Xe!==null&&__(St,Xe,Yt,ce,!0)}}t:{if(st=nt?lt(nt):window,ut=st.nodeName&&st.nodeName.toLowerCase(),ut==="select"||ut==="input"&&st.type==="file")var Ne=Op;else if(Lp(st))if(Pp)Ne=Qy;else{Ne=Zy;var te=qy}else ut=st.nodeName,!ut||ut.toLowerCase()!=="input"||st.type!=="checkbox"&&st.type!=="radio"?nt&&_u(nt.elementType)&&(Ne=Op):Ne=Ky;if(Ne&&(Ne=Ne(e,nt))){Np(St,Ne,a,vt);break t}te&&te(e,st,nt),e==="focusout"&&nt&&st.type==="number"&&nt.memoizedProps.value!=null&&pn(st,"number",st.value)}switch(te=nt?lt(nt):window,e){case"focusin":(Lp(te)||te.contentEditable==="true")&&(Gs=te,Uu=nt,so=null);break;case"focusout":so=Uu=Gs=null;break;case"mousedown":Lu=!0;break;case"contextmenu":case"mouseup":case"dragend":Lu=!1,kp(St,a,vt);break;case"selectionchange":if($y)break;case"keydown":case"keyup":kp(St,a,vt)}var pe;if(Ru)t:{switch(e){case"compositionstart":var De="onCompositionStart";break t;case"compositionend":De="onCompositionEnd";break t;case"compositionupdate":De="onCompositionUpdate";break t}De=void 0}else Hs?Dp(e,a)&&(De="onCompositionEnd"):e==="keydown"&&a.keyCode===229&&(De="onCompositionStart");De&&(Rp&&a.locale!=="ko"&&(Hs||De!=="onCompositionStart"?De==="onCompositionEnd"&&Hs&&(pe=Mp()):(ma=vt,Mu="value"in ma?ma.value:ma.textContent,Hs=!0)),te=sc(nt,De),0<te.length&&(De=new bp(De,e,null,a,vt),St.push({event:De,listeners:te}),pe?De.data=pe:(pe=Up(a),pe!==null&&(De.data=pe)))),(pe=ky?Xy(e,a):Wy(e,a))&&(De=sc(nt,"onBeforeInput"),0<De.length&&(te=new bp("onBeforeInput","beforeinput",null,a,vt),St.push({event:te,listeners:De}),te.data=pe)),zx(St,e,nt,a,vt)}m_(St,n)})}function Uo(e,n,a){return{instance:e,listener:n,currentTarget:a}}function sc(e,n){for(var a=n+"Capture",o=[];e!==null;){var u=e,f=u.stateNode;if(u=u.tag,u!==5&&u!==26&&u!==27||f===null||(u=Qr(e,a),u!=null&&o.unshift(Uo(e,u,f)),u=Qr(e,n),u!=null&&o.push(Uo(e,u,f))),e.tag===3)return o;e=e.return}return[]}function Hx(e){if(e===null)return null;do e=e.return;while(e&&e.tag!==5&&e.tag!==27);return e||null}function __(e,n,a,o,u){for(var f=n._reactName,y=[];a!==null&&a!==o;){var A=a,H=A.alternate,nt=A.stateNode;if(A=A.tag,H!==null&&H===o)break;A!==5&&A!==26&&A!==27||nt===null||(H=nt,u?(nt=Qr(a,f),nt!=null&&y.unshift(Uo(a,nt,H))):u||(nt=Qr(a,f),nt!=null&&y.push(Uo(a,nt,H)))),a=a.return}y.length!==0&&e.push({event:n,listeners:y})}var Gx=/\r\n?/g,Vx=/\u0000|\uFFFD/g;function v_(e){return(typeof e=="string"?e:""+e).replace(Gx,`
`).replace(Vx,"")}function y_(e,n){return n=v_(n),v_(e)===n}function ke(e,n,a,o,u,f){switch(a){case"children":typeof o=="string"?n==="body"||n==="textarea"&&o===""||Bs(e,o):(typeof o=="number"||typeof o=="bigint")&&n!=="body"&&Bs(e,""+o);break;case"className":Ie(e,"class",o);break;case"tabIndex":Ie(e,"tabindex",o);break;case"dir":case"role":case"viewBox":case"width":case"height":Ie(e,a,o);break;case"style":yp(e,o,f);break;case"data":if(n!=="object"){Ie(e,"data",o);break}case"src":case"href":if(o===""&&(n!=="a"||a!=="href")){e.removeAttribute(a);break}if(o==null||typeof o=="function"||typeof o=="symbol"||typeof o=="boolean"){e.removeAttribute(a);break}o=fl(""+o),e.setAttribute(a,o);break;case"action":case"formAction":if(typeof o=="function"){e.setAttribute(a,"javascript:throw new Error('A React form was unexpectedly submitted. If you called form.submit() manually, consider using form.requestSubmit() instead. If you\\'re trying to use event.stopPropagation() in a submit event handler, consider also calling event.preventDefault().')");break}else typeof f=="function"&&(a==="formAction"?(n!=="input"&&ke(e,n,"name",u.name,u,null),ke(e,n,"formEncType",u.formEncType,u,null),ke(e,n,"formMethod",u.formMethod,u,null),ke(e,n,"formTarget",u.formTarget,u,null)):(ke(e,n,"encType",u.encType,u,null),ke(e,n,"method",u.method,u,null),ke(e,n,"target",u.target,u,null)));if(o==null||typeof o=="symbol"||typeof o=="boolean"){e.removeAttribute(a);break}o=fl(""+o),e.setAttribute(a,o);break;case"onClick":o!=null&&(e.onclick=Gi);break;case"onScroll":o!=null&&Ae("scroll",e);break;case"onScrollEnd":o!=null&&Ae("scrollend",e);break;case"dangerouslySetInnerHTML":if(o!=null){if(typeof o!="object"||!("__html"in o))throw Error(s(61));if(a=o.__html,a!=null){if(u.children!=null)throw Error(s(60));e.innerHTML=a}}break;case"multiple":e.multiple=o&&typeof o!="function"&&typeof o!="symbol";break;case"muted":e.muted=o&&typeof o!="function"&&typeof o!="symbol";break;case"suppressContentEditableWarning":case"suppressHydrationWarning":case"defaultValue":case"defaultChecked":case"innerHTML":case"ref":break;case"autoFocus":break;case"xlinkHref":if(o==null||typeof o=="function"||typeof o=="boolean"||typeof o=="symbol"){e.removeAttribute("xlink:href");break}a=fl(""+o),e.setAttributeNS("http://www.w3.org/1999/xlink","xlink:href",a);break;case"contentEditable":case"spellCheck":case"draggable":case"value":case"autoReverse":case"externalResourcesRequired":case"focusable":case"preserveAlpha":o!=null&&typeof o!="function"&&typeof o!="symbol"?e.setAttribute(a,""+o):e.removeAttribute(a);break;case"inert":case"allowFullScreen":case"async":case"autoPlay":case"controls":case"default":case"defer":case"disabled":case"disablePictureInPicture":case"disableRemotePlayback":case"formNoValidate":case"hidden":case"loop":case"noModule":case"noValidate":case"open":case"playsInline":case"readOnly":case"required":case"reversed":case"scoped":case"seamless":case"itemScope":o&&typeof o!="function"&&typeof o!="symbol"?e.setAttribute(a,""):e.removeAttribute(a);break;case"capture":case"download":o===!0?e.setAttribute(a,""):o!==!1&&o!=null&&typeof o!="function"&&typeof o!="symbol"?e.setAttribute(a,o):e.removeAttribute(a);break;case"cols":case"rows":case"size":case"span":o!=null&&typeof o!="function"&&typeof o!="symbol"&&!isNaN(o)&&1<=o?e.setAttribute(a,o):e.removeAttribute(a);break;case"rowSpan":case"start":o==null||typeof o=="function"||typeof o=="symbol"||isNaN(o)?e.removeAttribute(a):e.setAttribute(a,o);break;case"popover":Ae("beforetoggle",e),Ae("toggle",e),Ze(e,"popover",o);break;case"xlinkActuate":Te(e,"http://www.w3.org/1999/xlink","xlink:actuate",o);break;case"xlinkArcrole":Te(e,"http://www.w3.org/1999/xlink","xlink:arcrole",o);break;case"xlinkRole":Te(e,"http://www.w3.org/1999/xlink","xlink:role",o);break;case"xlinkShow":Te(e,"http://www.w3.org/1999/xlink","xlink:show",o);break;case"xlinkTitle":Te(e,"http://www.w3.org/1999/xlink","xlink:title",o);break;case"xlinkType":Te(e,"http://www.w3.org/1999/xlink","xlink:type",o);break;case"xmlBase":Te(e,"http://www.w3.org/XML/1998/namespace","xml:base",o);break;case"xmlLang":Te(e,"http://www.w3.org/XML/1998/namespace","xml:lang",o);break;case"xmlSpace":Te(e,"http://www.w3.org/XML/1998/namespace","xml:space",o);break;case"is":Ze(e,"is",o);break;case"innerText":case"textContent":break;default:(!(2<a.length)||a[0]!=="o"&&a[0]!=="O"||a[1]!=="n"&&a[1]!=="N")&&(a=gy.get(a)||a,Ze(e,a,o))}}function ah(e,n,a,o,u,f){switch(a){case"style":yp(e,o,f);break;case"dangerouslySetInnerHTML":if(o!=null){if(typeof o!="object"||!("__html"in o))throw Error(s(61));if(a=o.__html,a!=null){if(u.children!=null)throw Error(s(60));e.innerHTML=a}}break;case"children":typeof o=="string"?Bs(e,o):(typeof o=="number"||typeof o=="bigint")&&Bs(e,""+o);break;case"onScroll":o!=null&&Ae("scroll",e);break;case"onScrollEnd":o!=null&&Ae("scrollend",e);break;case"onClick":o!=null&&(e.onclick=Gi);break;case"suppressContentEditableWarning":case"suppressHydrationWarning":case"innerHTML":case"ref":break;case"innerText":case"textContent":break;default:if(!qt.hasOwnProperty(a))t:{if(a[0]==="o"&&a[1]==="n"&&(u=a.endsWith("Capture"),n=a.slice(2,u?a.length-7:void 0),f=e[Tn]||null,f=f!=null?f[a]:null,typeof f=="function"&&e.removeEventListener(n,f,u),typeof o=="function")){typeof f!="function"&&f!==null&&(a in e?e[a]=null:e.hasAttribute(a)&&e.removeAttribute(a)),e.addEventListener(n,o,u);break t}a in e?e[a]=o:o===!0?e.setAttribute(a,""):Ze(e,a,o)}}}function wn(e,n,a){switch(n){case"div":case"span":case"svg":case"path":case"a":case"g":case"p":case"li":break;case"img":Ae("error",e),Ae("load",e);var o=!1,u=!1,f;for(f in a)if(a.hasOwnProperty(f)){var y=a[f];if(y!=null)switch(f){case"src":o=!0;break;case"srcSet":u=!0;break;case"children":case"dangerouslySetInnerHTML":throw Error(s(137,n));default:ke(e,n,f,y,a,null)}}u&&ke(e,n,"srcSet",a.srcSet,a,null),o&&ke(e,n,"src",a.src,a,null);return;case"input":Ae("invalid",e);var A=f=y=u=null,H=null,nt=null;for(o in a)if(a.hasOwnProperty(o)){var vt=a[o];if(vt!=null)switch(o){case"name":u=vt;break;case"type":y=vt;break;case"checked":H=vt;break;case"defaultChecked":nt=vt;break;case"value":f=vt;break;case"defaultValue":A=vt;break;case"children":case"dangerouslySetInnerHTML":if(vt!=null)throw Error(s(137,n));break;default:ke(e,n,o,vt,a,null)}}Pn(e,f,A,H,nt,y,u,!1);return;case"select":Ae("invalid",e),o=y=f=null;for(u in a)if(a.hasOwnProperty(u)&&(A=a[u],A!=null))switch(u){case"value":f=A;break;case"defaultValue":y=A;break;case"multiple":o=A;default:ke(e,n,u,A,a,null)}n=f,a=y,e.multiple=!!o,n!=null?an(e,!!o,n,!1):a!=null&&an(e,!!o,a,!0);return;case"textarea":Ae("invalid",e),f=u=o=null;for(y in a)if(a.hasOwnProperty(y)&&(A=a[y],A!=null))switch(y){case"value":o=A;break;case"defaultValue":u=A;break;case"children":f=A;break;case"dangerouslySetInnerHTML":if(A!=null)throw Error(s(91));break;default:ke(e,n,y,A,a,null)}wi(e,o,u,f);return;case"option":for(H in a)if(a.hasOwnProperty(H)&&(o=a[H],o!=null))switch(H){case"selected":e.selected=o&&typeof o!="function"&&typeof o!="symbol";break;default:ke(e,n,H,o,a,null)}return;case"dialog":Ae("beforetoggle",e),Ae("toggle",e),Ae("cancel",e),Ae("close",e);break;case"iframe":case"object":Ae("load",e);break;case"video":case"audio":for(o=0;o<Do.length;o++)Ae(Do[o],e);break;case"image":Ae("error",e),Ae("load",e);break;case"details":Ae("toggle",e);break;case"embed":case"source":case"link":Ae("error",e),Ae("load",e);case"area":case"base":case"br":case"col":case"hr":case"keygen":case"meta":case"param":case"track":case"wbr":case"menuitem":for(nt in a)if(a.hasOwnProperty(nt)&&(o=a[nt],o!=null))switch(nt){case"children":case"dangerouslySetInnerHTML":throw Error(s(137,n));default:ke(e,n,nt,o,a,null)}return;default:if(_u(n)){for(vt in a)a.hasOwnProperty(vt)&&(o=a[vt],o!==void 0&&ah(e,n,vt,o,a,void 0));return}}for(A in a)a.hasOwnProperty(A)&&(o=a[A],o!=null&&ke(e,n,A,o,a,null))}function kx(e,n,a,o){switch(n){case"div":case"span":case"svg":case"path":case"a":case"g":case"p":case"li":break;case"input":var u=null,f=null,y=null,A=null,H=null,nt=null,vt=null;for(ut in a){var St=a[ut];if(a.hasOwnProperty(ut)&&St!=null)switch(ut){case"checked":break;case"value":break;case"defaultValue":H=St;default:o.hasOwnProperty(ut)||ke(e,n,ut,null,o,St)}}for(var st in o){var ut=o[st];if(St=a[st],o.hasOwnProperty(st)&&(ut!=null||St!=null))switch(st){case"type":f=ut;break;case"name":u=ut;break;case"checked":nt=ut;break;case"defaultChecked":vt=ut;break;case"value":y=ut;break;case"defaultValue":A=ut;break;case"children":case"dangerouslySetInnerHTML":if(ut!=null)throw Error(s(137,n));break;default:ut!==St&&ke(e,n,st,ut,o,St)}}Dn(e,y,A,H,nt,vt,f,u);return;case"select":ut=y=A=st=null;for(f in a)if(H=a[f],a.hasOwnProperty(f)&&H!=null)switch(f){case"value":break;case"multiple":ut=H;default:o.hasOwnProperty(f)||ke(e,n,f,null,o,H)}for(u in o)if(f=o[u],H=a[u],o.hasOwnProperty(u)&&(f!=null||H!=null))switch(u){case"value":st=f;break;case"defaultValue":A=f;break;case"multiple":y=f;default:f!==H&&ke(e,n,u,f,o,H)}n=A,a=y,o=ut,st!=null?an(e,!!a,st,!1):!!o!=!!a&&(n!=null?an(e,!!a,n,!0):an(e,!!a,a?[]:"",!1));return;case"textarea":ut=st=null;for(A in a)if(u=a[A],a.hasOwnProperty(A)&&u!=null&&!o.hasOwnProperty(A))switch(A){case"value":break;case"children":break;default:ke(e,n,A,null,o,u)}for(y in o)if(u=o[y],f=a[y],o.hasOwnProperty(y)&&(u!=null||f!=null))switch(y){case"value":st=u;break;case"defaultValue":ut=u;break;case"children":break;case"dangerouslySetInnerHTML":if(u!=null)throw Error(s(91));break;default:u!==f&&ke(e,n,y,u,o,f)}zs(e,st,ut);return;case"option":for(var Yt in a)if(st=a[Yt],a.hasOwnProperty(Yt)&&st!=null&&!o.hasOwnProperty(Yt))switch(Yt){case"selected":e.selected=!1;break;default:ke(e,n,Yt,null,o,st)}for(H in o)if(st=o[H],ut=a[H],o.hasOwnProperty(H)&&st!==ut&&(st!=null||ut!=null))switch(H){case"selected":e.selected=st&&typeof st!="function"&&typeof st!="symbol";break;default:ke(e,n,H,st,o,ut)}return;case"img":case"link":case"area":case"base":case"br":case"col":case"embed":case"hr":case"keygen":case"meta":case"param":case"source":case"track":case"wbr":case"menuitem":for(var ce in a)st=a[ce],a.hasOwnProperty(ce)&&st!=null&&!o.hasOwnProperty(ce)&&ke(e,n,ce,null,o,st);for(nt in o)if(st=o[nt],ut=a[nt],o.hasOwnProperty(nt)&&st!==ut&&(st!=null||ut!=null))switch(nt){case"children":case"dangerouslySetInnerHTML":if(st!=null)throw Error(s(137,n));break;default:ke(e,n,nt,st,o,ut)}return;default:if(_u(n)){for(var Xe in a)st=a[Xe],a.hasOwnProperty(Xe)&&st!==void 0&&!o.hasOwnProperty(Xe)&&ah(e,n,Xe,void 0,o,st);for(vt in o)st=o[vt],ut=a[vt],!o.hasOwnProperty(vt)||st===ut||st===void 0&&ut===void 0||ah(e,n,vt,st,o,ut);return}}for(var Q in a)st=a[Q],a.hasOwnProperty(Q)&&st!=null&&!o.hasOwnProperty(Q)&&ke(e,n,Q,null,o,st);for(St in o)st=o[St],ut=a[St],!o.hasOwnProperty(St)||st===ut||st==null&&ut==null||ke(e,n,St,st,o,ut)}function x_(e){switch(e){case"css":case"script":case"font":case"img":case"image":case"input":case"link":return!0;default:return!1}}function Xx(){if(typeof performance.getEntriesByType=="function"){for(var e=0,n=0,a=performance.getEntriesByType("resource"),o=0;o<a.length;o++){var u=a[o],f=u.transferSize,y=u.initiatorType,A=u.duration;if(f&&A&&x_(y)){for(y=0,A=u.responseEnd,o+=1;o<a.length;o++){var H=a[o],nt=H.startTime;if(nt>A)break;var vt=H.transferSize,St=H.initiatorType;vt&&x_(St)&&(H=H.responseEnd,y+=vt*(H<A?1:(A-nt)/(H-nt)))}if(--o,n+=8*(f+y)/(u.duration/1e3),e++,10<e)break}}if(0<e)return n/e/1e6}return navigator.connection&&(e=navigator.connection.downlink,typeof e=="number")?e:5}var sh=null,rh=null;function rc(e){return e.nodeType===9?e:e.ownerDocument}function S_(e){switch(e){case"http://www.w3.org/2000/svg":return 1;case"http://www.w3.org/1998/Math/MathML":return 2;default:return 0}}function M_(e,n){if(e===0)switch(n){case"svg":return 1;case"math":return 2;default:return 0}return e===1&&n==="foreignObject"?0:e}function oh(e,n){return e==="textarea"||e==="noscript"||typeof n.children=="string"||typeof n.children=="number"||typeof n.children=="bigint"||typeof n.dangerouslySetInnerHTML=="object"&&n.dangerouslySetInnerHTML!==null&&n.dangerouslySetInnerHTML.__html!=null}var lh=null;function Wx(){var e=window.event;return e&&e.type==="popstate"?e===lh?!1:(lh=e,!0):(lh=null,!1)}var E_=typeof setTimeout=="function"?setTimeout:void 0,jx=typeof clearTimeout=="function"?clearTimeout:void 0,T_=typeof Promise=="function"?Promise:void 0,Yx=typeof queueMicrotask=="function"?queueMicrotask:typeof T_<"u"?function(e){return T_.resolve(null).then(e).catch(qx)}:E_;function qx(e){setTimeout(function(){throw e})}function La(e){return e==="head"}function b_(e,n){var a=n,o=0;do{var u=a.nextSibling;if(e.removeChild(a),u&&u.nodeType===8)if(a=u.data,a==="/$"||a==="/&"){if(o===0){e.removeChild(u),pr(n);return}o--}else if(a==="$"||a==="$?"||a==="$~"||a==="$!"||a==="&")o++;else if(a==="html")Lo(e.ownerDocument.documentElement);else if(a==="head"){a=e.ownerDocument.head,Lo(a);for(var f=a.firstChild;f;){var y=f.nextSibling,A=f.nodeName;f[Qa]||A==="SCRIPT"||A==="STYLE"||A==="LINK"&&f.rel.toLowerCase()==="stylesheet"||a.removeChild(f),f=y}}else a==="body"&&Lo(e.ownerDocument.body);a=u}while(a);pr(n)}function A_(e,n){var a=e;e=0;do{var o=a.nextSibling;if(a.nodeType===1?n?(a._stashedDisplay=a.style.display,a.style.display="none"):(a.style.display=a._stashedDisplay||"",a.getAttribute("style")===""&&a.removeAttribute("style")):a.nodeType===3&&(n?(a._stashedText=a.nodeValue,a.nodeValue=""):a.nodeValue=a._stashedText||""),o&&o.nodeType===8)if(a=o.data,a==="/$"){if(e===0)break;e--}else a!=="$"&&a!=="$?"&&a!=="$~"&&a!=="$!"||e++;a=o}while(a)}function ch(e){var n=e.firstChild;for(n&&n.nodeType===10&&(n=n.nextSibling);n;){var a=n;switch(n=n.nextSibling,a.nodeName){case"HTML":case"HEAD":case"BODY":ch(a),C(a);continue;case"SCRIPT":case"STYLE":continue;case"LINK":if(a.rel.toLowerCase()==="stylesheet")continue}e.removeChild(a)}}function Zx(e,n,a,o){for(;e.nodeType===1;){var u=a;if(e.nodeName.toLowerCase()!==n.toLowerCase()){if(!o&&(e.nodeName!=="INPUT"||e.type!=="hidden"))break}else if(o){if(!e[Qa])switch(n){case"meta":if(!e.hasAttribute("itemprop"))break;return e;case"link":if(f=e.getAttribute("rel"),f==="stylesheet"&&e.hasAttribute("data-precedence"))break;if(f!==u.rel||e.getAttribute("href")!==(u.href==null||u.href===""?null:u.href)||e.getAttribute("crossorigin")!==(u.crossOrigin==null?null:u.crossOrigin)||e.getAttribute("title")!==(u.title==null?null:u.title))break;return e;case"style":if(e.hasAttribute("data-precedence"))break;return e;case"script":if(f=e.getAttribute("src"),(f!==(u.src==null?null:u.src)||e.getAttribute("type")!==(u.type==null?null:u.type)||e.getAttribute("crossorigin")!==(u.crossOrigin==null?null:u.crossOrigin))&&f&&e.hasAttribute("async")&&!e.hasAttribute("itemprop"))break;return e;default:return e}}else if(n==="input"&&e.type==="hidden"){var f=u.name==null?null:""+u.name;if(u.type==="hidden"&&e.getAttribute("name")===f)return e}else return e;if(e=pi(e.nextSibling),e===null)break}return null}function Kx(e,n,a){if(n==="")return null;for(;e.nodeType!==3;)if((e.nodeType!==1||e.nodeName!=="INPUT"||e.type!=="hidden")&&!a||(e=pi(e.nextSibling),e===null))return null;return e}function R_(e,n){for(;e.nodeType!==8;)if((e.nodeType!==1||e.nodeName!=="INPUT"||e.type!=="hidden")&&!n||(e=pi(e.nextSibling),e===null))return null;return e}function uh(e){return e.data==="$?"||e.data==="$~"}function fh(e){return e.data==="$!"||e.data==="$?"&&e.ownerDocument.readyState!=="loading"}function Qx(e,n){var a=e.ownerDocument;if(e.data==="$~")e._reactRetry=n;else if(e.data!=="$?"||a.readyState!=="loading")n();else{var o=function(){n(),a.removeEventListener("DOMContentLoaded",o)};a.addEventListener("DOMContentLoaded",o),e._reactRetry=o}}function pi(e){for(;e!=null;e=e.nextSibling){var n=e.nodeType;if(n===1||n===3)break;if(n===8){if(n=e.data,n==="$"||n==="$!"||n==="$?"||n==="$~"||n==="&"||n==="F!"||n==="F")break;if(n==="/$"||n==="/&")return null}}return e}var hh=null;function C_(e){e=e.nextSibling;for(var n=0;e;){if(e.nodeType===8){var a=e.data;if(a==="/$"||a==="/&"){if(n===0)return pi(e.nextSibling);n--}else a!=="$"&&a!=="$!"&&a!=="$?"&&a!=="$~"&&a!=="&"||n++}e=e.nextSibling}return null}function w_(e){e=e.previousSibling;for(var n=0;e;){if(e.nodeType===8){var a=e.data;if(a==="$"||a==="$!"||a==="$?"||a==="$~"||a==="&"){if(n===0)return e;n--}else a!=="/$"&&a!=="/&"||n++}e=e.previousSibling}return null}function D_(e,n,a){switch(n=rc(a),e){case"html":if(e=n.documentElement,!e)throw Error(s(452));return e;case"head":if(e=n.head,!e)throw Error(s(453));return e;case"body":if(e=n.body,!e)throw Error(s(454));return e;default:throw Error(s(451))}}function Lo(e){for(var n=e.attributes;n.length;)e.removeAttributeNode(n[0]);C(e)}var mi=new Map,U_=new Set;function oc(e){return typeof e.getRootNode=="function"?e.getRootNode():e.nodeType===9?e:e.ownerDocument}var ia=J.d;J.d={f:Jx,r:$x,D:tS,C:eS,L:nS,m:iS,X:sS,S:aS,M:rS};function Jx(){var e=ia.f(),n=Jl();return e||n}function $x(e){var n=rt(e);n!==null&&n.tag===5&&n.type==="form"?qm(n):ia.r(e)}var fr=typeof document>"u"?null:document;function L_(e,n,a){var o=fr;if(o&&typeof n=="string"&&n){var u=Ce(n);u='link[rel="'+e+'"][href="'+u+'"]',typeof a=="string"&&(u+='[crossorigin="'+a+'"]'),U_.has(u)||(U_.add(u),e={rel:e,crossOrigin:a,href:n},o.querySelector(u)===null&&(n=o.createElement("link"),wn(n,"link",e),bt(n),o.head.appendChild(n)))}}function tS(e){ia.D(e),L_("dns-prefetch",e,null)}function eS(e,n){ia.C(e,n),L_("preconnect",e,n)}function nS(e,n,a){ia.L(e,n,a);var o=fr;if(o&&e&&n){var u='link[rel="preload"][as="'+Ce(n)+'"]';n==="image"&&a&&a.imageSrcSet?(u+='[imagesrcset="'+Ce(a.imageSrcSet)+'"]',typeof a.imageSizes=="string"&&(u+='[imagesizes="'+Ce(a.imageSizes)+'"]')):u+='[href="'+Ce(e)+'"]';var f=u;switch(n){case"style":f=hr(e);break;case"script":f=dr(e)}mi.has(f)||(e=g({rel:"preload",href:n==="image"&&a&&a.imageSrcSet?void 0:e,as:n},a),mi.set(f,e),o.querySelector(u)!==null||n==="style"&&o.querySelector(No(f))||n==="script"&&o.querySelector(Oo(f))||(n=o.createElement("link"),wn(n,"link",e),bt(n),o.head.appendChild(n)))}}function iS(e,n){ia.m(e,n);var a=fr;if(a&&e){var o=n&&typeof n.as=="string"?n.as:"script",u='link[rel="modulepreload"][as="'+Ce(o)+'"][href="'+Ce(e)+'"]',f=u;switch(o){case"audioworklet":case"paintworklet":case"serviceworker":case"sharedworker":case"worker":case"script":f=dr(e)}if(!mi.has(f)&&(e=g({rel:"modulepreload",href:e},n),mi.set(f,e),a.querySelector(u)===null)){switch(o){case"audioworklet":case"paintworklet":case"serviceworker":case"sharedworker":case"worker":case"script":if(a.querySelector(Oo(f)))return}o=a.createElement("link"),wn(o,"link",e),bt(o),a.head.appendChild(o)}}}function aS(e,n,a){ia.S(e,n,a);var o=fr;if(o&&e){var u=$(o).hoistableStyles,f=hr(e);n=n||"default";var y=u.get(f);if(!y){var A={loading:0,preload:null};if(y=o.querySelector(No(f)))A.loading=5;else{e=g({rel:"stylesheet",href:e,"data-precedence":n},a),(a=mi.get(f))&&dh(e,a);var H=y=o.createElement("link");bt(H),wn(H,"link",e),H._p=new Promise(function(nt,vt){H.onload=nt,H.onerror=vt}),H.addEventListener("load",function(){A.loading|=1}),H.addEventListener("error",function(){A.loading|=2}),A.loading|=4,lc(y,n,o)}y={type:"stylesheet",instance:y,count:1,state:A},u.set(f,y)}}}function sS(e,n){ia.X(e,n);var a=fr;if(a&&e){var o=$(a).hoistableScripts,u=dr(e),f=o.get(u);f||(f=a.querySelector(Oo(u)),f||(e=g({src:e,async:!0},n),(n=mi.get(u))&&ph(e,n),f=a.createElement("script"),bt(f),wn(f,"link",e),a.head.appendChild(f)),f={type:"script",instance:f,count:1,state:null},o.set(u,f))}}function rS(e,n){ia.M(e,n);var a=fr;if(a&&e){var o=$(a).hoistableScripts,u=dr(e),f=o.get(u);f||(f=a.querySelector(Oo(u)),f||(e=g({src:e,async:!0,type:"module"},n),(n=mi.get(u))&&ph(e,n),f=a.createElement("script"),bt(f),wn(f,"link",e),a.head.appendChild(f)),f={type:"script",instance:f,count:1,state:null},o.set(u,f))}}function N_(e,n,a,o){var u=(u=q.current)?oc(u):null;if(!u)throw Error(s(446));switch(e){case"meta":case"title":return null;case"style":return typeof a.precedence=="string"&&typeof a.href=="string"?(n=hr(a.href),a=$(u).hoistableStyles,o=a.get(n),o||(o={type:"style",instance:null,count:0,state:null},a.set(n,o)),o):{type:"void",instance:null,count:0,state:null};case"link":if(a.rel==="stylesheet"&&typeof a.href=="string"&&typeof a.precedence=="string"){e=hr(a.href);var f=$(u).hoistableStyles,y=f.get(e);if(y||(u=u.ownerDocument||u,y={type:"stylesheet",instance:null,count:0,state:{loading:0,preload:null}},f.set(e,y),(f=u.querySelector(No(e)))&&!f._p&&(y.instance=f,y.state.loading=5),mi.has(e)||(a={rel:"preload",as:"style",href:a.href,crossOrigin:a.crossOrigin,integrity:a.integrity,media:a.media,hrefLang:a.hrefLang,referrerPolicy:a.referrerPolicy},mi.set(e,a),f||oS(u,e,a,y.state))),n&&o===null)throw Error(s(528,""));return y}if(n&&o!==null)throw Error(s(529,""));return null;case"script":return n=a.async,a=a.src,typeof a=="string"&&n&&typeof n!="function"&&typeof n!="symbol"?(n=dr(a),a=$(u).hoistableScripts,o=a.get(n),o||(o={type:"script",instance:null,count:0,state:null},a.set(n,o)),o):{type:"void",instance:null,count:0,state:null};default:throw Error(s(444,e))}}function hr(e){return'href="'+Ce(e)+'"'}function No(e){return'link[rel="stylesheet"]['+e+"]"}function O_(e){return g({},e,{"data-precedence":e.precedence,precedence:null})}function oS(e,n,a,o){e.querySelector('link[rel="preload"][as="style"]['+n+"]")?o.loading=1:(n=e.createElement("link"),o.preload=n,n.addEventListener("load",function(){return o.loading|=1}),n.addEventListener("error",function(){return o.loading|=2}),wn(n,"link",a),bt(n),e.head.appendChild(n))}function dr(e){return'[src="'+Ce(e)+'"]'}function Oo(e){return"script[async]"+e}function P_(e,n,a){if(n.count++,n.instance===null)switch(n.type){case"style":var o=e.querySelector('style[data-href~="'+Ce(a.href)+'"]');if(o)return n.instance=o,bt(o),o;var u=g({},a,{"data-href":a.href,"data-precedence":a.precedence,href:null,precedence:null});return o=(e.ownerDocument||e).createElement("style"),bt(o),wn(o,"style",u),lc(o,a.precedence,e),n.instance=o;case"stylesheet":u=hr(a.href);var f=e.querySelector(No(u));if(f)return n.state.loading|=4,n.instance=f,bt(f),f;o=O_(a),(u=mi.get(u))&&dh(o,u),f=(e.ownerDocument||e).createElement("link"),bt(f);var y=f;return y._p=new Promise(function(A,H){y.onload=A,y.onerror=H}),wn(f,"link",o),n.state.loading|=4,lc(f,a.precedence,e),n.instance=f;case"script":return f=dr(a.src),(u=e.querySelector(Oo(f)))?(n.instance=u,bt(u),u):(o=a,(u=mi.get(f))&&(o=g({},a),ph(o,u)),e=e.ownerDocument||e,u=e.createElement("script"),bt(u),wn(u,"link",o),e.head.appendChild(u),n.instance=u);case"void":return null;default:throw Error(s(443,n.type))}else n.type==="stylesheet"&&(n.state.loading&4)===0&&(o=n.instance,n.state.loading|=4,lc(o,a.precedence,e));return n.instance}function lc(e,n,a){for(var o=a.querySelectorAll('link[rel="stylesheet"][data-precedence],style[data-precedence]'),u=o.length?o[o.length-1]:null,f=u,y=0;y<o.length;y++){var A=o[y];if(A.dataset.precedence===n)f=A;else if(f!==u)break}f?f.parentNode.insertBefore(e,f.nextSibling):(n=a.nodeType===9?a.head:a,n.insertBefore(e,n.firstChild))}function dh(e,n){e.crossOrigin==null&&(e.crossOrigin=n.crossOrigin),e.referrerPolicy==null&&(e.referrerPolicy=n.referrerPolicy),e.title==null&&(e.title=n.title)}function ph(e,n){e.crossOrigin==null&&(e.crossOrigin=n.crossOrigin),e.referrerPolicy==null&&(e.referrerPolicy=n.referrerPolicy),e.integrity==null&&(e.integrity=n.integrity)}var cc=null;function z_(e,n,a){if(cc===null){var o=new Map,u=cc=new Map;u.set(a,o)}else u=cc,o=u.get(a),o||(o=new Map,u.set(a,o));if(o.has(e))return o;for(o.set(e,null),a=a.getElementsByTagName(e),u=0;u<a.length;u++){var f=a[u];if(!(f[Qa]||f[on]||e==="link"&&f.getAttribute("rel")==="stylesheet")&&f.namespaceURI!=="http://www.w3.org/2000/svg"){var y=f.getAttribute(n)||"";y=e+y;var A=o.get(y);A?A.push(f):o.set(y,[f])}}return o}function B_(e,n,a){e=e.ownerDocument||e,e.head.insertBefore(a,n==="title"?e.querySelector("head > title"):null)}function lS(e,n,a){if(a===1||n.itemProp!=null)return!1;switch(e){case"meta":case"title":return!0;case"style":if(typeof n.precedence!="string"||typeof n.href!="string"||n.href==="")break;return!0;case"link":if(typeof n.rel!="string"||typeof n.href!="string"||n.href===""||n.onLoad||n.onError)break;switch(n.rel){case"stylesheet":return e=n.disabled,typeof n.precedence=="string"&&e==null;default:return!0}case"script":if(n.async&&typeof n.async!="function"&&typeof n.async!="symbol"&&!n.onLoad&&!n.onError&&n.src&&typeof n.src=="string")return!0}return!1}function I_(e){return!(e.type==="stylesheet"&&(e.state.loading&3)===0)}function cS(e,n,a,o){if(a.type==="stylesheet"&&(typeof o.media!="string"||matchMedia(o.media).matches!==!1)&&(a.state.loading&4)===0){if(a.instance===null){var u=hr(o.href),f=n.querySelector(No(u));if(f){n=f._p,n!==null&&typeof n=="object"&&typeof n.then=="function"&&(e.count++,e=uc.bind(e),n.then(e,e)),a.state.loading|=4,a.instance=f,bt(f);return}f=n.ownerDocument||n,o=O_(o),(u=mi.get(u))&&dh(o,u),f=f.createElement("link"),bt(f);var y=f;y._p=new Promise(function(A,H){y.onload=A,y.onerror=H}),wn(f,"link",o),a.instance=f}e.stylesheets===null&&(e.stylesheets=new Map),e.stylesheets.set(a,n),(n=a.state.preload)&&(a.state.loading&3)===0&&(e.count++,a=uc.bind(e),n.addEventListener("load",a),n.addEventListener("error",a))}}var mh=0;function uS(e,n){return e.stylesheets&&e.count===0&&hc(e,e.stylesheets),0<e.count||0<e.imgCount?function(a){var o=setTimeout(function(){if(e.stylesheets&&hc(e,e.stylesheets),e.unsuspend){var f=e.unsuspend;e.unsuspend=null,f()}},6e4+n);0<e.imgBytes&&mh===0&&(mh=62500*Xx());var u=setTimeout(function(){if(e.waitingForImages=!1,e.count===0&&(e.stylesheets&&hc(e,e.stylesheets),e.unsuspend)){var f=e.unsuspend;e.unsuspend=null,f()}},(e.imgBytes>mh?50:800)+n);return e.unsuspend=a,function(){e.unsuspend=null,clearTimeout(o),clearTimeout(u)}}:null}function uc(){if(this.count--,this.count===0&&(this.imgCount===0||!this.waitingForImages)){if(this.stylesheets)hc(this,this.stylesheets);else if(this.unsuspend){var e=this.unsuspend;this.unsuspend=null,e()}}}var fc=null;function hc(e,n){e.stylesheets=null,e.unsuspend!==null&&(e.count++,fc=new Map,n.forEach(fS,e),fc=null,uc.call(e))}function fS(e,n){if(!(n.state.loading&4)){var a=fc.get(e);if(a)var o=a.get(null);else{a=new Map,fc.set(e,a);for(var u=e.querySelectorAll("link[data-precedence],style[data-precedence]"),f=0;f<u.length;f++){var y=u[f];(y.nodeName==="LINK"||y.getAttribute("media")!=="not all")&&(a.set(y.dataset.precedence,y),o=y)}o&&a.set(null,o)}u=n.instance,y=u.getAttribute("data-precedence"),f=a.get(y)||o,f===o&&a.set(null,u),a.set(y,u),this.count++,o=uc.bind(this),u.addEventListener("load",o),u.addEventListener("error",o),f?f.parentNode.insertBefore(u,f.nextSibling):(e=e.nodeType===9?e.head:e,e.insertBefore(u,e.firstChild)),n.state.loading|=4}}var Po={$$typeof:U,Provider:null,Consumer:null,_currentValue:Y,_currentValue2:Y,_threadCount:0};function hS(e,n,a,o,u,f,y,A,H){this.tag=1,this.containerInfo=e,this.pingCache=this.current=this.pendingChildren=null,this.timeoutHandle=-1,this.callbackNode=this.next=this.pendingContext=this.context=this.cancelPendingCommit=null,this.callbackPriority=0,this.expirationTimes=Re(-1),this.entangledLanes=this.shellSuspendCounter=this.errorRecoveryDisabledLanes=this.expiredLanes=this.warmLanes=this.pingedLanes=this.suspendedLanes=this.pendingLanes=0,this.entanglements=Re(0),this.hiddenUpdates=Re(null),this.identifierPrefix=o,this.onUncaughtError=u,this.onCaughtError=f,this.onRecoverableError=y,this.pooledCache=null,this.pooledCacheLanes=0,this.formState=H,this.incompleteTransitions=new Map}function F_(e,n,a,o,u,f,y,A,H,nt,vt,St){return e=new hS(e,n,a,y,H,nt,vt,St,A),n=1,f===!0&&(n|=24),f=Kn(3,null,null,n),e.current=f,f.stateNode=e,n=Yu(),n.refCount++,e.pooledCache=n,n.refCount++,f.memoizedState={element:o,isDehydrated:a,cache:n},Qu(f),e}function H_(e){return e?(e=Xs,e):Xs}function G_(e,n,a,o,u,f){u=H_(u),o.context===null?o.context=u:o.pendingContext=u,o=Sa(n),o.payload={element:a},f=f===void 0?null:f,f!==null&&(o.callback=f),a=Ma(e,o,n),a!==null&&(Xn(a,e,n),ho(a,e,n))}function V_(e,n){if(e=e.memoizedState,e!==null&&e.dehydrated!==null){var a=e.retryLane;e.retryLane=a!==0&&a<n?a:n}}function gh(e,n){V_(e,n),(e=e.alternate)&&V_(e,n)}function k_(e){if(e.tag===13||e.tag===31){var n=ns(e,67108864);n!==null&&Xn(n,e,67108864),gh(e,67108864)}}function X_(e){if(e.tag===13||e.tag===31){var n=ei();n=Os(n);var a=ns(e,n);a!==null&&Xn(a,e,n),gh(e,n)}}var dc=!0;function dS(e,n,a,o){var u=N.T;N.T=null;var f=J.p;try{J.p=2,_h(e,n,a,o)}finally{J.p=f,N.T=u}}function pS(e,n,a,o){var u=N.T;N.T=null;var f=J.p;try{J.p=8,_h(e,n,a,o)}finally{J.p=f,N.T=u}}function _h(e,n,a,o){if(dc){var u=vh(o);if(u===null)ih(e,n,o,pc,a),j_(e,o);else if(gS(u,e,n,a,o))o.stopPropagation();else if(j_(e,o),n&4&&-1<mS.indexOf(e)){for(;u!==null;){var f=rt(u);if(f!==null)switch(f.tag){case 3:if(f=f.stateNode,f.current.memoizedState.isDehydrated){var y=Et(f.pendingLanes);if(y!==0){var A=f;for(A.pendingLanes|=2,A.entangledLanes|=2;y;){var H=1<<31-It(y);A.entanglements[1]|=H,y&=~H}Ni(f),(ze&6)===0&&(Kl=E()+500,wo(0))}}break;case 31:case 13:A=ns(f,2),A!==null&&Xn(A,f,2),Jl(),gh(f,2)}if(f=vh(o),f===null&&ih(e,n,o,pc,a),f===u)break;u=f}u!==null&&o.stopPropagation()}else ih(e,n,o,null,a)}}function vh(e){return e=yu(e),yh(e)}var pc=null;function yh(e){if(pc=null,e=Z(e),e!==null){var n=c(e);if(n===null)e=null;else{var a=n.tag;if(a===13){if(e=h(n),e!==null)return e;e=null}else if(a===31){if(e=d(n),e!==null)return e;e=null}else if(a===3){if(n.stateNode.current.memoizedState.isDehydrated)return n.tag===3?n.stateNode.containerInfo:null;e=null}else n!==e&&(e=null)}}return pc=e,null}function W_(e){switch(e){case"beforetoggle":case"cancel":case"click":case"close":case"contextmenu":case"copy":case"cut":case"auxclick":case"dblclick":case"dragend":case"dragstart":case"drop":case"focusin":case"focusout":case"input":case"invalid":case"keydown":case"keypress":case"keyup":case"mousedown":case"mouseup":case"paste":case"pause":case"play":case"pointercancel":case"pointerdown":case"pointerup":case"ratechange":case"reset":case"resize":case"seeked":case"submit":case"toggle":case"touchcancel":case"touchend":case"touchstart":case"volumechange":case"change":case"selectionchange":case"textInput":case"compositionstart":case"compositionend":case"compositionupdate":case"beforeblur":case"afterblur":case"beforeinput":case"blur":case"fullscreenchange":case"focus":case"hashchange":case"popstate":case"select":case"selectstart":return 2;case"drag":case"dragenter":case"dragexit":case"dragleave":case"dragover":case"mousemove":case"mouseout":case"mouseover":case"pointermove":case"pointerout":case"pointerover":case"scroll":case"touchmove":case"wheel":case"mouseenter":case"mouseleave":case"pointerenter":case"pointerleave":return 8;case"message":switch(tt()){case gt:return 2;case At:return 8;case dt:case Jt:return 32;case Nt:return 268435456;default:return 32}default:return 32}}var xh=!1,Na=null,Oa=null,Pa=null,zo=new Map,Bo=new Map,za=[],mS="mousedown mouseup touchcancel touchend touchstart auxclick dblclick pointercancel pointerdown pointerup dragend dragstart drop compositionend compositionstart keydown keypress keyup input textInput copy cut paste click change contextmenu reset".split(" ");function j_(e,n){switch(e){case"focusin":case"focusout":Na=null;break;case"dragenter":case"dragleave":Oa=null;break;case"mouseover":case"mouseout":Pa=null;break;case"pointerover":case"pointerout":zo.delete(n.pointerId);break;case"gotpointercapture":case"lostpointercapture":Bo.delete(n.pointerId)}}function Io(e,n,a,o,u,f){return e===null||e.nativeEvent!==f?(e={blockedOn:n,domEventName:a,eventSystemFlags:o,nativeEvent:f,targetContainers:[u]},n!==null&&(n=rt(n),n!==null&&k_(n)),e):(e.eventSystemFlags|=o,n=e.targetContainers,u!==null&&n.indexOf(u)===-1&&n.push(u),e)}function gS(e,n,a,o,u){switch(n){case"focusin":return Na=Io(Na,e,n,a,o,u),!0;case"dragenter":return Oa=Io(Oa,e,n,a,o,u),!0;case"mouseover":return Pa=Io(Pa,e,n,a,o,u),!0;case"pointerover":var f=u.pointerId;return zo.set(f,Io(zo.get(f)||null,e,n,a,o,u)),!0;case"gotpointercapture":return f=u.pointerId,Bo.set(f,Io(Bo.get(f)||null,e,n,a,o,u)),!0}return!1}function Y_(e){var n=Z(e.target);if(n!==null){var a=c(n);if(a!==null){if(n=a.tag,n===13){if(n=h(a),n!==null){e.blockedOn=n,Zr(e.priority,function(){X_(a)});return}}else if(n===31){if(n=d(a),n!==null){e.blockedOn=n,Zr(e.priority,function(){X_(a)});return}}else if(n===3&&a.stateNode.current.memoizedState.isDehydrated){e.blockedOn=a.tag===3?a.stateNode.containerInfo:null;return}}}e.blockedOn=null}function mc(e){if(e.blockedOn!==null)return!1;for(var n=e.targetContainers;0<n.length;){var a=vh(e.nativeEvent);if(a===null){a=e.nativeEvent;var o=new a.constructor(a.type,a);vu=o,a.target.dispatchEvent(o),vu=null}else return n=rt(a),n!==null&&k_(n),e.blockedOn=a,!1;n.shift()}return!0}function q_(e,n,a){mc(e)&&a.delete(n)}function _S(){xh=!1,Na!==null&&mc(Na)&&(Na=null),Oa!==null&&mc(Oa)&&(Oa=null),Pa!==null&&mc(Pa)&&(Pa=null),zo.forEach(q_),Bo.forEach(q_)}function gc(e,n){e.blockedOn===n&&(e.blockedOn=null,xh||(xh=!0,r.unstable_scheduleCallback(r.unstable_NormalPriority,_S)))}var _c=null;function Z_(e){_c!==e&&(_c=e,r.unstable_scheduleCallback(r.unstable_NormalPriority,function(){_c===e&&(_c=null);for(var n=0;n<e.length;n+=3){var a=e[n],o=e[n+1],u=e[n+2];if(typeof o!="function"){if(yh(o||a)===null)continue;break}var f=rt(a);f!==null&&(e.splice(n,3),n-=3,vf(f,{pending:!0,data:u,method:a.method,action:o},o,u))}}))}function pr(e){function n(H){return gc(H,e)}Na!==null&&gc(Na,e),Oa!==null&&gc(Oa,e),Pa!==null&&gc(Pa,e),zo.forEach(n),Bo.forEach(n);for(var a=0;a<za.length;a++){var o=za[a];o.blockedOn===e&&(o.blockedOn=null)}for(;0<za.length&&(a=za[0],a.blockedOn===null);)Y_(a),a.blockedOn===null&&za.shift();if(a=(e.ownerDocument||e).$$reactFormReplay,a!=null)for(o=0;o<a.length;o+=3){var u=a[o],f=a[o+1],y=u[Tn]||null;if(typeof f=="function")y||Z_(a);else if(y){var A=null;if(f&&f.hasAttribute("formAction")){if(u=f,y=f[Tn]||null)A=y.formAction;else if(yh(u)!==null)continue}else A=y.action;typeof A=="function"?a[o+1]=A:(a.splice(o,3),o-=3),Z_(a)}}}function K_(){function e(f){f.canIntercept&&f.info==="react-transition"&&f.intercept({handler:function(){return new Promise(function(y){return u=y})},focusReset:"manual",scroll:"manual"})}function n(){u!==null&&(u(),u=null),o||setTimeout(a,20)}function a(){if(!o&&!navigation.transition){var f=navigation.currentEntry;f&&f.url!=null&&navigation.navigate(f.url,{state:f.getState(),info:"react-transition",history:"replace"})}}if(typeof navigation=="object"){var o=!1,u=null;return navigation.addEventListener("navigate",e),navigation.addEventListener("navigatesuccess",n),navigation.addEventListener("navigateerror",n),setTimeout(a,100),function(){o=!0,navigation.removeEventListener("navigate",e),navigation.removeEventListener("navigatesuccess",n),navigation.removeEventListener("navigateerror",n),u!==null&&(u(),u=null)}}}function Sh(e){this._internalRoot=e}vc.prototype.render=Sh.prototype.render=function(e){var n=this._internalRoot;if(n===null)throw Error(s(409));var a=n.current,o=ei();G_(a,o,e,n,null,null)},vc.prototype.unmount=Sh.prototype.unmount=function(){var e=this._internalRoot;if(e!==null){this._internalRoot=null;var n=e.containerInfo;G_(e.current,2,null,e,null,null),Jl(),n[pa]=null}};function vc(e){this._internalRoot=e}vc.prototype.unstable_scheduleHydration=function(e){if(e){var n=Ka();e={blockedOn:null,target:e,priority:n};for(var a=0;a<za.length&&n!==0&&n<za[a].priority;a++);za.splice(a,0,e),a===0&&Y_(e)}};var Q_=t.version;if(Q_!=="19.2.0")throw Error(s(527,Q_,"19.2.0"));J.findDOMNode=function(e){var n=e._reactInternals;if(n===void 0)throw typeof e.render=="function"?Error(s(188)):(e=Object.keys(e).join(","),Error(s(268,e)));return e=p(n),e=e!==null?v(e):null,e=e===null?null:e.stateNode,e};var vS={bundleType:0,version:"19.2.0",rendererPackageName:"react-dom",currentDispatcherRef:N,reconcilerVersion:"19.2.0"};if(typeof __REACT_DEVTOOLS_GLOBAL_HOOK__<"u"){var yc=__REACT_DEVTOOLS_GLOBAL_HOOK__;if(!yc.isDisabled&&yc.supportsFiber)try{Ut=yc.inject(vS),Ot=yc}catch{}}return Ho.createRoot=function(e,n){if(!l(e))throw Error(s(299));var a=!1,o="",u=ag,f=sg,y=rg;return n!=null&&(n.unstable_strictMode===!0&&(a=!0),n.identifierPrefix!==void 0&&(o=n.identifierPrefix),n.onUncaughtError!==void 0&&(u=n.onUncaughtError),n.onCaughtError!==void 0&&(f=n.onCaughtError),n.onRecoverableError!==void 0&&(y=n.onRecoverableError)),n=F_(e,1,!1,null,null,a,o,null,u,f,y,K_),e[pa]=n.current,nh(e),new Sh(n)},Ho.hydrateRoot=function(e,n,a){if(!l(e))throw Error(s(299));var o=!1,u="",f=ag,y=sg,A=rg,H=null;return a!=null&&(a.unstable_strictMode===!0&&(o=!0),a.identifierPrefix!==void 0&&(u=a.identifierPrefix),a.onUncaughtError!==void 0&&(f=a.onUncaughtError),a.onCaughtError!==void 0&&(y=a.onCaughtError),a.onRecoverableError!==void 0&&(A=a.onRecoverableError),a.formState!==void 0&&(H=a.formState)),n=F_(e,1,!0,n,a??null,o,u,H,f,y,A,K_),n.context=H_(null),a=n.current,o=ei(),o=Os(o),u=Sa(o),u.callback=null,Ma(a,u,o),a=o,n.current.lanes=a,En(n,a),Ni(n),e[pa]=n.current,nh(e),new vc(n)},Ho.version="19.2.0",Ho}var l0;function US(){if(l0)return Th.exports;l0=1;function r(){if(!(typeof __REACT_DEVTOOLS_GLOBAL_HOOK__>"u"||typeof __REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE!="function"))try{__REACT_DEVTOOLS_GLOBAL_HOOK__.checkDCE(r)}catch(t){console.error(t)}}return r(),Th.exports=DS(),Th.exports}var LS=US();function Lv(r){return(r==null?void 0:r.type)==="entity"&&(r==null?void 0:r.dateKind)==="earliest-linked-memory"?r.createdAt||null:(r==null?void 0:r.capturedAt)||null}function kR(r){const t=Lv(r);if(typeof t!="string")return"unknown";const i=t.match(/^(\d{4})(?:-|$)/);return i?i[1]:"unknown"}function NS(r){const t=(r==null?void 0:r.years)??(r==null?void 0:r.yearCounts)??r??{},i=Array.isArray(t)?t.map(s=>typeof s=="string"?s:s==null?void 0:s.year).filter(Boolean):Object.keys(t||{});return[...new Set(i.map(String).filter(s=>s.toLowerCase()!=="unknown"))].sort((s,l)=>s.localeCompare(l))}function Ch(r,t){const i=(r==null?void 0:r.years)??(r==null?void 0:r.yearCounts)??r??{};if(String(t).toLowerCase()==="unknown"){if(Array.isArray(i)){const s=i.find(l=>String(typeof l=="string"?l:l==null?void 0:l.year).toLowerCase()==="unknown");return typeof s=="object"&&Number(s.count)||0}return Number((r==null?void 0:r.unknown)??(r==null?void 0:r.unknownCount)??(i==null?void 0:i.Unknown)??(i==null?void 0:i.unknown))||0}if(Array.isArray(i)){const s=i.find(l=>String(typeof l=="string"?l:l==null?void 0:l.year)===String(t));return typeof s=="object"&&Number(s.count)||0}return Number(i==null?void 0:i[t])||0}function OS(r,t,i){if(!r.length||String(t).toLowerCase()==="unknown")return null;const s=r.indexOf(t);return s===-1?i>0?r.at(-1):r[0]:r[s+i]??null}function PS(r,t=[],i=500){const s=new Set(t),l=new Map,c=r||[];for(const h of[...c.filter(d=>s.has(d==null?void 0:d.id)),...c.filter(d=>!s.has(d==null?void 0:d.id))])if(!(!(h!=null&&h.id)||l.has(h.id))&&(l.set(h.id,{...h,isContext:!s.has(h.id)}),l.size>=i))break;return[...l.values()]}const zS=Math.PI*(3-Math.sqrt(5));function Nv(r){return{...r,from:(r==null?void 0:r.from)??(r==null?void 0:r.source),to:(r==null?void 0:r.to)??(r==null?void 0:r.target),kind:(r==null?void 0:r.kind)??(r==null?void 0:r.type)}}function c0(r){let t=2166136261;for(const i of r)t=Math.imul(t^i.charCodeAt(0),16777619);return t>>>0}function u0(r,t,i=0){if(t<=1)return[Math.cos(i),.24,Math.sin(i)];const s=1-2*((r+.5)/t),l=Math.sqrt(Math.max(0,1-s*s)),c=r*zS+i;return[l*Math.cos(c),s,l*Math.sin(c)]}function BS(r,t=[]){const i=new Map,s=r.filter(m=>m.type==="entity").sort((m,p)=>m.id.localeCompare(p.id)),l=new Map;s.forEach((m,p)=>{const v=p*Math.PI*2/Math.max(1,s.length)-Math.PI/2,g=[Math.cos(v)*.36,Math.sin(v)*.27,Math.sin(v*2+.45)*.13];l.set(m.id,g),i.set(m.id,g)});const c=new Map(s.map(m=>[m.id,[]]));for(const m of r)c.has(m.entityId)&&c.get(m.entityId).push(m);for(const[m,p]of c){const v=l.get(m);p.sort((g,x)=>f0(g).localeCompare(f0(x))||g.id.localeCompare(x.id)),p.forEach((g,x)=>{const S=u0(x,p.length,c0(g.id)%6283/1e3),b=.12+Math.sqrt(x+1)*.043;i.set(g.id,v.map((R,M)=>R+S[M]*b))})}const h=r.filter(m=>!i.has(m.id)).sort((m,p)=>m.id.localeCompare(p.id));h.forEach((m,p)=>{const v=u0(p,h.length,c0(m.id)%6283/1e3),g=.2+Math.sqrt(p+1)*.045;i.set(m.id,v.map(x=>x*g))});const d=new Set(i.keys());return{positions:i,edges:t.map(Nv).filter(m=>d.has(m.from)&&d.has(m.to))}}function f0(r){return r.createdAt||r.capturedAt||"9999"}/**
 * @license
 * Copyright 2010-2025 Three.js Authors
 * SPDX-License-Identifier: MIT
 */const np="180",Br={ROTATE:0,DOLLY:1,PAN:2},Pr={ROTATE:0,PAN:1,DOLLY_PAN:2,DOLLY_ROTATE:3},IS=0,h0=1,FS=2,Ov=1,HS=2,ca=3,qa=0,Yn=1,ua=2,ja=0,Ir=1,d0=2,p0=3,m0=4,GS=5,Ts=100,VS=101,kS=102,XS=103,WS=104,jS=200,YS=201,qS=202,ZS=203,fd=204,hd=205,KS=206,QS=207,JS=208,$S=209,tM=210,eM=211,nM=212,iM=213,aM=214,dd=0,pd=1,md=2,Hr=3,gd=4,_d=5,vd=6,yd=7,Pv=0,sM=1,rM=2,Ya=0,oM=1,lM=2,cM=3,zv=4,uM=5,fM=6,hM=7,Bv=300,Gr=301,Vr=302,xd=303,Sd=304,fu=306,Md=1e3,As=1001,Ed=1002,si=1003,dM=1004,xc=1005,Pi=1006,wh=1007,Rs=1008,Ii=1009,Iv=1010,Fv=1011,il=1012,ip=1013,Cs=1014,zi=1015,cl=1016,ap=1017,sp=1018,al=1020,Hv=35902,Gv=35899,Vv=1021,kv=1022,Ri=1023,sl=1026,rl=1027,rp=1028,op=1029,Xv=1030,lp=1031,cp=1033,Qc=33776,Jc=33777,$c=33778,tu=33779,Td=35840,bd=35841,Ad=35842,Rd=35843,Cd=36196,wd=37492,Dd=37496,Ud=37808,Ld=37809,Nd=37810,Od=37811,Pd=37812,zd=37813,Bd=37814,Id=37815,Fd=37816,Hd=37817,Gd=37818,Vd=37819,kd=37820,Xd=37821,Wd=36492,jd=36494,Yd=36495,qd=36283,Zd=36284,Kd=36285,Qd=36286,pM=3200,mM=3201,Wv=0,gM=1,Wa="",jn="srgb",kr="srgb-linear",nu="linear",We="srgb",mr=7680,g0=519,_M=512,vM=513,yM=514,jv=515,xM=516,SM=517,MM=518,EM=519,Jd=35044,_0="300 es",Bi=2e3,iu=2001;class Us{addEventListener(t,i){this._listeners===void 0&&(this._listeners={});const s=this._listeners;s[t]===void 0&&(s[t]=[]),s[t].indexOf(i)===-1&&s[t].push(i)}hasEventListener(t,i){const s=this._listeners;return s===void 0?!1:s[t]!==void 0&&s[t].indexOf(i)!==-1}removeEventListener(t,i){const s=this._listeners;if(s===void 0)return;const l=s[t];if(l!==void 0){const c=l.indexOf(i);c!==-1&&l.splice(c,1)}}dispatchEvent(t){const i=this._listeners;if(i===void 0)return;const s=i[t.type];if(s!==void 0){t.target=this;const l=s.slice(0);for(let c=0,h=l.length;c<h;c++)l[c].call(this,t);t.target=null}}}const Ln=["00","01","02","03","04","05","06","07","08","09","0a","0b","0c","0d","0e","0f","10","11","12","13","14","15","16","17","18","19","1a","1b","1c","1d","1e","1f","20","21","22","23","24","25","26","27","28","29","2a","2b","2c","2d","2e","2f","30","31","32","33","34","35","36","37","38","39","3a","3b","3c","3d","3e","3f","40","41","42","43","44","45","46","47","48","49","4a","4b","4c","4d","4e","4f","50","51","52","53","54","55","56","57","58","59","5a","5b","5c","5d","5e","5f","60","61","62","63","64","65","66","67","68","69","6a","6b","6c","6d","6e","6f","70","71","72","73","74","75","76","77","78","79","7a","7b","7c","7d","7e","7f","80","81","82","83","84","85","86","87","88","89","8a","8b","8c","8d","8e","8f","90","91","92","93","94","95","96","97","98","99","9a","9b","9c","9d","9e","9f","a0","a1","a2","a3","a4","a5","a6","a7","a8","a9","aa","ab","ac","ad","ae","af","b0","b1","b2","b3","b4","b5","b6","b7","b8","b9","ba","bb","bc","bd","be","bf","c0","c1","c2","c3","c4","c5","c6","c7","c8","c9","ca","cb","cc","cd","ce","cf","d0","d1","d2","d3","d4","d5","d6","d7","d8","d9","da","db","dc","dd","de","df","e0","e1","e2","e3","e4","e5","e6","e7","e8","e9","ea","eb","ec","ed","ee","ef","f0","f1","f2","f3","f4","f5","f6","f7","f8","f9","fa","fb","fc","fd","fe","ff"];let v0=1234567;const el=Math.PI/180,ol=180/Math.PI;function fa(){const r=Math.random()*4294967295|0,t=Math.random()*4294967295|0,i=Math.random()*4294967295|0,s=Math.random()*4294967295|0;return(Ln[r&255]+Ln[r>>8&255]+Ln[r>>16&255]+Ln[r>>24&255]+"-"+Ln[t&255]+Ln[t>>8&255]+"-"+Ln[t>>16&15|64]+Ln[t>>24&255]+"-"+Ln[i&63|128]+Ln[i>>8&255]+"-"+Ln[i>>16&255]+Ln[i>>24&255]+Ln[s&255]+Ln[s>>8&255]+Ln[s>>16&255]+Ln[s>>24&255]).toLowerCase()}function ye(r,t,i){return Math.max(t,Math.min(i,r))}function up(r,t){return(r%t+t)%t}function TM(r,t,i,s,l){return s+(r-t)*(l-s)/(i-t)}function bM(r,t,i){return r!==t?(i-r)/(t-r):0}function nl(r,t,i){return(1-i)*r+i*t}function AM(r,t,i,s){return nl(r,t,1-Math.exp(-i*s))}function RM(r,t=1){return t-Math.abs(up(r,t*2)-t)}function CM(r,t,i){return r<=t?0:r>=i?1:(r=(r-t)/(i-t),r*r*(3-2*r))}function wM(r,t,i){return r<=t?0:r>=i?1:(r=(r-t)/(i-t),r*r*r*(r*(r*6-15)+10))}function DM(r,t){return r+Math.floor(Math.random()*(t-r+1))}function UM(r,t){return r+Math.random()*(t-r)}function LM(r){return r*(.5-Math.random())}function NM(r){r!==void 0&&(v0=r);let t=v0+=1831565813;return t=Math.imul(t^t>>>15,t|1),t^=t+Math.imul(t^t>>>7,t|61),((t^t>>>14)>>>0)/4294967296}function OM(r){return r*el}function PM(r){return r*ol}function zM(r){return(r&r-1)===0&&r!==0}function BM(r){return Math.pow(2,Math.ceil(Math.log(r)/Math.LN2))}function IM(r){return Math.pow(2,Math.floor(Math.log(r)/Math.LN2))}function FM(r,t,i,s,l){const c=Math.cos,h=Math.sin,d=c(i/2),m=h(i/2),p=c((t+s)/2),v=h((t+s)/2),g=c((t-s)/2),x=h((t-s)/2),S=c((s-t)/2),b=h((s-t)/2);switch(l){case"XYX":r.set(d*v,m*g,m*x,d*p);break;case"YZY":r.set(m*x,d*v,m*g,d*p);break;case"ZXZ":r.set(m*g,m*x,d*v,d*p);break;case"XZX":r.set(d*v,m*b,m*S,d*p);break;case"YXY":r.set(m*S,d*v,m*b,d*p);break;case"ZYZ":r.set(m*b,m*S,d*v,d*p);break;default:console.warn("THREE.MathUtils: .setQuaternionFromProperEuler() encountered an unknown order: "+l)}}function Ai(r,t){switch(t.constructor){case Float32Array:return r;case Uint32Array:return r/4294967295;case Uint16Array:return r/65535;case Uint8Array:return r/255;case Int32Array:return Math.max(r/2147483647,-1);case Int16Array:return Math.max(r/32767,-1);case Int8Array:return Math.max(r/127,-1);default:throw new Error("Invalid component type.")}}function Ge(r,t){switch(t.constructor){case Float32Array:return r;case Uint32Array:return Math.round(r*4294967295);case Uint16Array:return Math.round(r*65535);case Uint8Array:return Math.round(r*255);case Int32Array:return Math.round(r*2147483647);case Int16Array:return Math.round(r*32767);case Int8Array:return Math.round(r*127);default:throw new Error("Invalid component type.")}}const Yv={DEG2RAD:el,RAD2DEG:ol,generateUUID:fa,clamp:ye,euclideanModulo:up,mapLinear:TM,inverseLerp:bM,lerp:nl,damp:AM,pingpong:RM,smoothstep:CM,smootherstep:wM,randInt:DM,randFloat:UM,randFloatSpread:LM,seededRandom:NM,degToRad:OM,radToDeg:PM,isPowerOfTwo:zM,ceilPowerOfTwo:BM,floorPowerOfTwo:IM,setQuaternionFromProperEuler:FM,normalize:Ge,denormalize:Ai};class re{constructor(t=0,i=0){re.prototype.isVector2=!0,this.x=t,this.y=i}get width(){return this.x}set width(t){this.x=t}get height(){return this.y}set height(t){this.y=t}set(t,i){return this.x=t,this.y=i,this}setScalar(t){return this.x=t,this.y=t,this}setX(t){return this.x=t,this}setY(t){return this.y=t,this}setComponent(t,i){switch(t){case 0:this.x=i;break;case 1:this.y=i;break;default:throw new Error("index is out of range: "+t)}return this}getComponent(t){switch(t){case 0:return this.x;case 1:return this.y;default:throw new Error("index is out of range: "+t)}}clone(){return new this.constructor(this.x,this.y)}copy(t){return this.x=t.x,this.y=t.y,this}add(t){return this.x+=t.x,this.y+=t.y,this}addScalar(t){return this.x+=t,this.y+=t,this}addVectors(t,i){return this.x=t.x+i.x,this.y=t.y+i.y,this}addScaledVector(t,i){return this.x+=t.x*i,this.y+=t.y*i,this}sub(t){return this.x-=t.x,this.y-=t.y,this}subScalar(t){return this.x-=t,this.y-=t,this}subVectors(t,i){return this.x=t.x-i.x,this.y=t.y-i.y,this}multiply(t){return this.x*=t.x,this.y*=t.y,this}multiplyScalar(t){return this.x*=t,this.y*=t,this}divide(t){return this.x/=t.x,this.y/=t.y,this}divideScalar(t){return this.multiplyScalar(1/t)}applyMatrix3(t){const i=this.x,s=this.y,l=t.elements;return this.x=l[0]*i+l[3]*s+l[6],this.y=l[1]*i+l[4]*s+l[7],this}min(t){return this.x=Math.min(this.x,t.x),this.y=Math.min(this.y,t.y),this}max(t){return this.x=Math.max(this.x,t.x),this.y=Math.max(this.y,t.y),this}clamp(t,i){return this.x=ye(this.x,t.x,i.x),this.y=ye(this.y,t.y,i.y),this}clampScalar(t,i){return this.x=ye(this.x,t,i),this.y=ye(this.y,t,i),this}clampLength(t,i){const s=this.length();return this.divideScalar(s||1).multiplyScalar(ye(s,t,i))}floor(){return this.x=Math.floor(this.x),this.y=Math.floor(this.y),this}ceil(){return this.x=Math.ceil(this.x),this.y=Math.ceil(this.y),this}round(){return this.x=Math.round(this.x),this.y=Math.round(this.y),this}roundToZero(){return this.x=Math.trunc(this.x),this.y=Math.trunc(this.y),this}negate(){return this.x=-this.x,this.y=-this.y,this}dot(t){return this.x*t.x+this.y*t.y}cross(t){return this.x*t.y-this.y*t.x}lengthSq(){return this.x*this.x+this.y*this.y}length(){return Math.sqrt(this.x*this.x+this.y*this.y)}manhattanLength(){return Math.abs(this.x)+Math.abs(this.y)}normalize(){return this.divideScalar(this.length()||1)}angle(){return Math.atan2(-this.y,-this.x)+Math.PI}angleTo(t){const i=Math.sqrt(this.lengthSq()*t.lengthSq());if(i===0)return Math.PI/2;const s=this.dot(t)/i;return Math.acos(ye(s,-1,1))}distanceTo(t){return Math.sqrt(this.distanceToSquared(t))}distanceToSquared(t){const i=this.x-t.x,s=this.y-t.y;return i*i+s*s}manhattanDistanceTo(t){return Math.abs(this.x-t.x)+Math.abs(this.y-t.y)}setLength(t){return this.normalize().multiplyScalar(t)}lerp(t,i){return this.x+=(t.x-this.x)*i,this.y+=(t.y-this.y)*i,this}lerpVectors(t,i,s){return this.x=t.x+(i.x-t.x)*s,this.y=t.y+(i.y-t.y)*s,this}equals(t){return t.x===this.x&&t.y===this.y}fromArray(t,i=0){return this.x=t[i],this.y=t[i+1],this}toArray(t=[],i=0){return t[i]=this.x,t[i+1]=this.y,t}fromBufferAttribute(t,i){return this.x=t.getX(i),this.y=t.getY(i),this}rotateAround(t,i){const s=Math.cos(i),l=Math.sin(i),c=this.x-t.x,h=this.y-t.y;return this.x=c*s-h*l+t.x,this.y=c*l+h*s+t.y,this}random(){return this.x=Math.random(),this.y=Math.random(),this}*[Symbol.iterator](){yield this.x,yield this.y}}class ws{constructor(t=0,i=0,s=0,l=1){this.isQuaternion=!0,this._x=t,this._y=i,this._z=s,this._w=l}static slerpFlat(t,i,s,l,c,h,d){let m=s[l+0],p=s[l+1],v=s[l+2],g=s[l+3];const x=c[h+0],S=c[h+1],b=c[h+2],R=c[h+3];if(d===0){t[i+0]=m,t[i+1]=p,t[i+2]=v,t[i+3]=g;return}if(d===1){t[i+0]=x,t[i+1]=S,t[i+2]=b,t[i+3]=R;return}if(g!==R||m!==x||p!==S||v!==b){let M=1-d;const _=m*x+p*S+v*b+g*R,L=_>=0?1:-1,U=1-_*_;if(U>Number.EPSILON){const P=Math.sqrt(U),B=Math.atan2(P,_*L);M=Math.sin(M*B)/P,d=Math.sin(d*B)/P}const T=d*L;if(m=m*M+x*T,p=p*M+S*T,v=v*M+b*T,g=g*M+R*T,M===1-d){const P=1/Math.sqrt(m*m+p*p+v*v+g*g);m*=P,p*=P,v*=P,g*=P}}t[i]=m,t[i+1]=p,t[i+2]=v,t[i+3]=g}static multiplyQuaternionsFlat(t,i,s,l,c,h){const d=s[l],m=s[l+1],p=s[l+2],v=s[l+3],g=c[h],x=c[h+1],S=c[h+2],b=c[h+3];return t[i]=d*b+v*g+m*S-p*x,t[i+1]=m*b+v*x+p*g-d*S,t[i+2]=p*b+v*S+d*x-m*g,t[i+3]=v*b-d*g-m*x-p*S,t}get x(){return this._x}set x(t){this._x=t,this._onChangeCallback()}get y(){return this._y}set y(t){this._y=t,this._onChangeCallback()}get z(){return this._z}set z(t){this._z=t,this._onChangeCallback()}get w(){return this._w}set w(t){this._w=t,this._onChangeCallback()}set(t,i,s,l){return this._x=t,this._y=i,this._z=s,this._w=l,this._onChangeCallback(),this}clone(){return new this.constructor(this._x,this._y,this._z,this._w)}copy(t){return this._x=t.x,this._y=t.y,this._z=t.z,this._w=t.w,this._onChangeCallback(),this}setFromEuler(t,i=!0){const s=t._x,l=t._y,c=t._z,h=t._order,d=Math.cos,m=Math.sin,p=d(s/2),v=d(l/2),g=d(c/2),x=m(s/2),S=m(l/2),b=m(c/2);switch(h){case"XYZ":this._x=x*v*g+p*S*b,this._y=p*S*g-x*v*b,this._z=p*v*b+x*S*g,this._w=p*v*g-x*S*b;break;case"YXZ":this._x=x*v*g+p*S*b,this._y=p*S*g-x*v*b,this._z=p*v*b-x*S*g,this._w=p*v*g+x*S*b;break;case"ZXY":this._x=x*v*g-p*S*b,this._y=p*S*g+x*v*b,this._z=p*v*b+x*S*g,this._w=p*v*g-x*S*b;break;case"ZYX":this._x=x*v*g-p*S*b,this._y=p*S*g+x*v*b,this._z=p*v*b-x*S*g,this._w=p*v*g+x*S*b;break;case"YZX":this._x=x*v*g+p*S*b,this._y=p*S*g+x*v*b,this._z=p*v*b-x*S*g,this._w=p*v*g-x*S*b;break;case"XZY":this._x=x*v*g-p*S*b,this._y=p*S*g-x*v*b,this._z=p*v*b+x*S*g,this._w=p*v*g+x*S*b;break;default:console.warn("THREE.Quaternion: .setFromEuler() encountered an unknown order: "+h)}return i===!0&&this._onChangeCallback(),this}setFromAxisAngle(t,i){const s=i/2,l=Math.sin(s);return this._x=t.x*l,this._y=t.y*l,this._z=t.z*l,this._w=Math.cos(s),this._onChangeCallback(),this}setFromRotationMatrix(t){const i=t.elements,s=i[0],l=i[4],c=i[8],h=i[1],d=i[5],m=i[9],p=i[2],v=i[6],g=i[10],x=s+d+g;if(x>0){const S=.5/Math.sqrt(x+1);this._w=.25/S,this._x=(v-m)*S,this._y=(c-p)*S,this._z=(h-l)*S}else if(s>d&&s>g){const S=2*Math.sqrt(1+s-d-g);this._w=(v-m)/S,this._x=.25*S,this._y=(l+h)/S,this._z=(c+p)/S}else if(d>g){const S=2*Math.sqrt(1+d-s-g);this._w=(c-p)/S,this._x=(l+h)/S,this._y=.25*S,this._z=(m+v)/S}else{const S=2*Math.sqrt(1+g-s-d);this._w=(h-l)/S,this._x=(c+p)/S,this._y=(m+v)/S,this._z=.25*S}return this._onChangeCallback(),this}setFromUnitVectors(t,i){let s=t.dot(i)+1;return s<1e-8?(s=0,Math.abs(t.x)>Math.abs(t.z)?(this._x=-t.y,this._y=t.x,this._z=0,this._w=s):(this._x=0,this._y=-t.z,this._z=t.y,this._w=s)):(this._x=t.y*i.z-t.z*i.y,this._y=t.z*i.x-t.x*i.z,this._z=t.x*i.y-t.y*i.x,this._w=s),this.normalize()}angleTo(t){return 2*Math.acos(Math.abs(ye(this.dot(t),-1,1)))}rotateTowards(t,i){const s=this.angleTo(t);if(s===0)return this;const l=Math.min(1,i/s);return this.slerp(t,l),this}identity(){return this.set(0,0,0,1)}invert(){return this.conjugate()}conjugate(){return this._x*=-1,this._y*=-1,this._z*=-1,this._onChangeCallback(),this}dot(t){return this._x*t._x+this._y*t._y+this._z*t._z+this._w*t._w}lengthSq(){return this._x*this._x+this._y*this._y+this._z*this._z+this._w*this._w}length(){return Math.sqrt(this._x*this._x+this._y*this._y+this._z*this._z+this._w*this._w)}normalize(){let t=this.length();return t===0?(this._x=0,this._y=0,this._z=0,this._w=1):(t=1/t,this._x=this._x*t,this._y=this._y*t,this._z=this._z*t,this._w=this._w*t),this._onChangeCallback(),this}multiply(t){return this.multiplyQuaternions(this,t)}premultiply(t){return this.multiplyQuaternions(t,this)}multiplyQuaternions(t,i){const s=t._x,l=t._y,c=t._z,h=t._w,d=i._x,m=i._y,p=i._z,v=i._w;return this._x=s*v+h*d+l*p-c*m,this._y=l*v+h*m+c*d-s*p,this._z=c*v+h*p+s*m-l*d,this._w=h*v-s*d-l*m-c*p,this._onChangeCallback(),this}slerp(t,i){if(i===0)return this;if(i===1)return this.copy(t);const s=this._x,l=this._y,c=this._z,h=this._w;let d=h*t._w+s*t._x+l*t._y+c*t._z;if(d<0?(this._w=-t._w,this._x=-t._x,this._y=-t._y,this._z=-t._z,d=-d):this.copy(t),d>=1)return this._w=h,this._x=s,this._y=l,this._z=c,this;const m=1-d*d;if(m<=Number.EPSILON){const S=1-i;return this._w=S*h+i*this._w,this._x=S*s+i*this._x,this._y=S*l+i*this._y,this._z=S*c+i*this._z,this.normalize(),this}const p=Math.sqrt(m),v=Math.atan2(p,d),g=Math.sin((1-i)*v)/p,x=Math.sin(i*v)/p;return this._w=h*g+this._w*x,this._x=s*g+this._x*x,this._y=l*g+this._y*x,this._z=c*g+this._z*x,this._onChangeCallback(),this}slerpQuaternions(t,i,s){return this.copy(t).slerp(i,s)}random(){const t=2*Math.PI*Math.random(),i=2*Math.PI*Math.random(),s=Math.random(),l=Math.sqrt(1-s),c=Math.sqrt(s);return this.set(l*Math.sin(t),l*Math.cos(t),c*Math.sin(i),c*Math.cos(i))}equals(t){return t._x===this._x&&t._y===this._y&&t._z===this._z&&t._w===this._w}fromArray(t,i=0){return this._x=t[i],this._y=t[i+1],this._z=t[i+2],this._w=t[i+3],this._onChangeCallback(),this}toArray(t=[],i=0){return t[i]=this._x,t[i+1]=this._y,t[i+2]=this._z,t[i+3]=this._w,t}fromBufferAttribute(t,i){return this._x=t.getX(i),this._y=t.getY(i),this._z=t.getZ(i),this._w=t.getW(i),this._onChangeCallback(),this}toJSON(){return this.toArray()}_onChange(t){return this._onChangeCallback=t,this}_onChangeCallback(){}*[Symbol.iterator](){yield this._x,yield this._y,yield this._z,yield this._w}}class X{constructor(t=0,i=0,s=0){X.prototype.isVector3=!0,this.x=t,this.y=i,this.z=s}set(t,i,s){return s===void 0&&(s=this.z),this.x=t,this.y=i,this.z=s,this}setScalar(t){return this.x=t,this.y=t,this.z=t,this}setX(t){return this.x=t,this}setY(t){return this.y=t,this}setZ(t){return this.z=t,this}setComponent(t,i){switch(t){case 0:this.x=i;break;case 1:this.y=i;break;case 2:this.z=i;break;default:throw new Error("index is out of range: "+t)}return this}getComponent(t){switch(t){case 0:return this.x;case 1:return this.y;case 2:return this.z;default:throw new Error("index is out of range: "+t)}}clone(){return new this.constructor(this.x,this.y,this.z)}copy(t){return this.x=t.x,this.y=t.y,this.z=t.z,this}add(t){return this.x+=t.x,this.y+=t.y,this.z+=t.z,this}addScalar(t){return this.x+=t,this.y+=t,this.z+=t,this}addVectors(t,i){return this.x=t.x+i.x,this.y=t.y+i.y,this.z=t.z+i.z,this}addScaledVector(t,i){return this.x+=t.x*i,this.y+=t.y*i,this.z+=t.z*i,this}sub(t){return this.x-=t.x,this.y-=t.y,this.z-=t.z,this}subScalar(t){return this.x-=t,this.y-=t,this.z-=t,this}subVectors(t,i){return this.x=t.x-i.x,this.y=t.y-i.y,this.z=t.z-i.z,this}multiply(t){return this.x*=t.x,this.y*=t.y,this.z*=t.z,this}multiplyScalar(t){return this.x*=t,this.y*=t,this.z*=t,this}multiplyVectors(t,i){return this.x=t.x*i.x,this.y=t.y*i.y,this.z=t.z*i.z,this}applyEuler(t){return this.applyQuaternion(y0.setFromEuler(t))}applyAxisAngle(t,i){return this.applyQuaternion(y0.setFromAxisAngle(t,i))}applyMatrix3(t){const i=this.x,s=this.y,l=this.z,c=t.elements;return this.x=c[0]*i+c[3]*s+c[6]*l,this.y=c[1]*i+c[4]*s+c[7]*l,this.z=c[2]*i+c[5]*s+c[8]*l,this}applyNormalMatrix(t){return this.applyMatrix3(t).normalize()}applyMatrix4(t){const i=this.x,s=this.y,l=this.z,c=t.elements,h=1/(c[3]*i+c[7]*s+c[11]*l+c[15]);return this.x=(c[0]*i+c[4]*s+c[8]*l+c[12])*h,this.y=(c[1]*i+c[5]*s+c[9]*l+c[13])*h,this.z=(c[2]*i+c[6]*s+c[10]*l+c[14])*h,this}applyQuaternion(t){const i=this.x,s=this.y,l=this.z,c=t.x,h=t.y,d=t.z,m=t.w,p=2*(h*l-d*s),v=2*(d*i-c*l),g=2*(c*s-h*i);return this.x=i+m*p+h*g-d*v,this.y=s+m*v+d*p-c*g,this.z=l+m*g+c*v-h*p,this}project(t){return this.applyMatrix4(t.matrixWorldInverse).applyMatrix4(t.projectionMatrix)}unproject(t){return this.applyMatrix4(t.projectionMatrixInverse).applyMatrix4(t.matrixWorld)}transformDirection(t){const i=this.x,s=this.y,l=this.z,c=t.elements;return this.x=c[0]*i+c[4]*s+c[8]*l,this.y=c[1]*i+c[5]*s+c[9]*l,this.z=c[2]*i+c[6]*s+c[10]*l,this.normalize()}divide(t){return this.x/=t.x,this.y/=t.y,this.z/=t.z,this}divideScalar(t){return this.multiplyScalar(1/t)}min(t){return this.x=Math.min(this.x,t.x),this.y=Math.min(this.y,t.y),this.z=Math.min(this.z,t.z),this}max(t){return this.x=Math.max(this.x,t.x),this.y=Math.max(this.y,t.y),this.z=Math.max(this.z,t.z),this}clamp(t,i){return this.x=ye(this.x,t.x,i.x),this.y=ye(this.y,t.y,i.y),this.z=ye(this.z,t.z,i.z),this}clampScalar(t,i){return this.x=ye(this.x,t,i),this.y=ye(this.y,t,i),this.z=ye(this.z,t,i),this}clampLength(t,i){const s=this.length();return this.divideScalar(s||1).multiplyScalar(ye(s,t,i))}floor(){return this.x=Math.floor(this.x),this.y=Math.floor(this.y),this.z=Math.floor(this.z),this}ceil(){return this.x=Math.ceil(this.x),this.y=Math.ceil(this.y),this.z=Math.ceil(this.z),this}round(){return this.x=Math.round(this.x),this.y=Math.round(this.y),this.z=Math.round(this.z),this}roundToZero(){return this.x=Math.trunc(this.x),this.y=Math.trunc(this.y),this.z=Math.trunc(this.z),this}negate(){return this.x=-this.x,this.y=-this.y,this.z=-this.z,this}dot(t){return this.x*t.x+this.y*t.y+this.z*t.z}lengthSq(){return this.x*this.x+this.y*this.y+this.z*this.z}length(){return Math.sqrt(this.x*this.x+this.y*this.y+this.z*this.z)}manhattanLength(){return Math.abs(this.x)+Math.abs(this.y)+Math.abs(this.z)}normalize(){return this.divideScalar(this.length()||1)}setLength(t){return this.normalize().multiplyScalar(t)}lerp(t,i){return this.x+=(t.x-this.x)*i,this.y+=(t.y-this.y)*i,this.z+=(t.z-this.z)*i,this}lerpVectors(t,i,s){return this.x=t.x+(i.x-t.x)*s,this.y=t.y+(i.y-t.y)*s,this.z=t.z+(i.z-t.z)*s,this}cross(t){return this.crossVectors(this,t)}crossVectors(t,i){const s=t.x,l=t.y,c=t.z,h=i.x,d=i.y,m=i.z;return this.x=l*m-c*d,this.y=c*h-s*m,this.z=s*d-l*h,this}projectOnVector(t){const i=t.lengthSq();if(i===0)return this.set(0,0,0);const s=t.dot(this)/i;return this.copy(t).multiplyScalar(s)}projectOnPlane(t){return Dh.copy(this).projectOnVector(t),this.sub(Dh)}reflect(t){return this.sub(Dh.copy(t).multiplyScalar(2*this.dot(t)))}angleTo(t){const i=Math.sqrt(this.lengthSq()*t.lengthSq());if(i===0)return Math.PI/2;const s=this.dot(t)/i;return Math.acos(ye(s,-1,1))}distanceTo(t){return Math.sqrt(this.distanceToSquared(t))}distanceToSquared(t){const i=this.x-t.x,s=this.y-t.y,l=this.z-t.z;return i*i+s*s+l*l}manhattanDistanceTo(t){return Math.abs(this.x-t.x)+Math.abs(this.y-t.y)+Math.abs(this.z-t.z)}setFromSpherical(t){return this.setFromSphericalCoords(t.radius,t.phi,t.theta)}setFromSphericalCoords(t,i,s){const l=Math.sin(i)*t;return this.x=l*Math.sin(s),this.y=Math.cos(i)*t,this.z=l*Math.cos(s),this}setFromCylindrical(t){return this.setFromCylindricalCoords(t.radius,t.theta,t.y)}setFromCylindricalCoords(t,i,s){return this.x=t*Math.sin(i),this.y=s,this.z=t*Math.cos(i),this}setFromMatrixPosition(t){const i=t.elements;return this.x=i[12],this.y=i[13],this.z=i[14],this}setFromMatrixScale(t){const i=this.setFromMatrixColumn(t,0).length(),s=this.setFromMatrixColumn(t,1).length(),l=this.setFromMatrixColumn(t,2).length();return this.x=i,this.y=s,this.z=l,this}setFromMatrixColumn(t,i){return this.fromArray(t.elements,i*4)}setFromMatrix3Column(t,i){return this.fromArray(t.elements,i*3)}setFromEuler(t){return this.x=t._x,this.y=t._y,this.z=t._z,this}setFromColor(t){return this.x=t.r,this.y=t.g,this.z=t.b,this}equals(t){return t.x===this.x&&t.y===this.y&&t.z===this.z}fromArray(t,i=0){return this.x=t[i],this.y=t[i+1],this.z=t[i+2],this}toArray(t=[],i=0){return t[i]=this.x,t[i+1]=this.y,t[i+2]=this.z,t}fromBufferAttribute(t,i){return this.x=t.getX(i),this.y=t.getY(i),this.z=t.getZ(i),this}random(){return this.x=Math.random(),this.y=Math.random(),this.z=Math.random(),this}randomDirection(){const t=Math.random()*Math.PI*2,i=Math.random()*2-1,s=Math.sqrt(1-i*i);return this.x=s*Math.cos(t),this.y=i,this.z=s*Math.sin(t),this}*[Symbol.iterator](){yield this.x,yield this.y,yield this.z}}const Dh=new X,y0=new ws;class me{constructor(t,i,s,l,c,h,d,m,p){me.prototype.isMatrix3=!0,this.elements=[1,0,0,0,1,0,0,0,1],t!==void 0&&this.set(t,i,s,l,c,h,d,m,p)}set(t,i,s,l,c,h,d,m,p){const v=this.elements;return v[0]=t,v[1]=l,v[2]=d,v[3]=i,v[4]=c,v[5]=m,v[6]=s,v[7]=h,v[8]=p,this}identity(){return this.set(1,0,0,0,1,0,0,0,1),this}copy(t){const i=this.elements,s=t.elements;return i[0]=s[0],i[1]=s[1],i[2]=s[2],i[3]=s[3],i[4]=s[4],i[5]=s[5],i[6]=s[6],i[7]=s[7],i[8]=s[8],this}extractBasis(t,i,s){return t.setFromMatrix3Column(this,0),i.setFromMatrix3Column(this,1),s.setFromMatrix3Column(this,2),this}setFromMatrix4(t){const i=t.elements;return this.set(i[0],i[4],i[8],i[1],i[5],i[9],i[2],i[6],i[10]),this}multiply(t){return this.multiplyMatrices(this,t)}premultiply(t){return this.multiplyMatrices(t,this)}multiplyMatrices(t,i){const s=t.elements,l=i.elements,c=this.elements,h=s[0],d=s[3],m=s[6],p=s[1],v=s[4],g=s[7],x=s[2],S=s[5],b=s[8],R=l[0],M=l[3],_=l[6],L=l[1],U=l[4],T=l[7],P=l[2],B=l[5],F=l[8];return c[0]=h*R+d*L+m*P,c[3]=h*M+d*U+m*B,c[6]=h*_+d*T+m*F,c[1]=p*R+v*L+g*P,c[4]=p*M+v*U+g*B,c[7]=p*_+v*T+g*F,c[2]=x*R+S*L+b*P,c[5]=x*M+S*U+b*B,c[8]=x*_+S*T+b*F,this}multiplyScalar(t){const i=this.elements;return i[0]*=t,i[3]*=t,i[6]*=t,i[1]*=t,i[4]*=t,i[7]*=t,i[2]*=t,i[5]*=t,i[8]*=t,this}determinant(){const t=this.elements,i=t[0],s=t[1],l=t[2],c=t[3],h=t[4],d=t[5],m=t[6],p=t[7],v=t[8];return i*h*v-i*d*p-s*c*v+s*d*m+l*c*p-l*h*m}invert(){const t=this.elements,i=t[0],s=t[1],l=t[2],c=t[3],h=t[4],d=t[5],m=t[6],p=t[7],v=t[8],g=v*h-d*p,x=d*m-v*c,S=p*c-h*m,b=i*g+s*x+l*S;if(b===0)return this.set(0,0,0,0,0,0,0,0,0);const R=1/b;return t[0]=g*R,t[1]=(l*p-v*s)*R,t[2]=(d*s-l*h)*R,t[3]=x*R,t[4]=(v*i-l*m)*R,t[5]=(l*c-d*i)*R,t[6]=S*R,t[7]=(s*m-p*i)*R,t[8]=(h*i-s*c)*R,this}transpose(){let t;const i=this.elements;return t=i[1],i[1]=i[3],i[3]=t,t=i[2],i[2]=i[6],i[6]=t,t=i[5],i[5]=i[7],i[7]=t,this}getNormalMatrix(t){return this.setFromMatrix4(t).invert().transpose()}transposeIntoArray(t){const i=this.elements;return t[0]=i[0],t[1]=i[3],t[2]=i[6],t[3]=i[1],t[4]=i[4],t[5]=i[7],t[6]=i[2],t[7]=i[5],t[8]=i[8],this}setUvTransform(t,i,s,l,c,h,d){const m=Math.cos(c),p=Math.sin(c);return this.set(s*m,s*p,-s*(m*h+p*d)+h+t,-l*p,l*m,-l*(-p*h+m*d)+d+i,0,0,1),this}scale(t,i){return this.premultiply(Uh.makeScale(t,i)),this}rotate(t){return this.premultiply(Uh.makeRotation(-t)),this}translate(t,i){return this.premultiply(Uh.makeTranslation(t,i)),this}makeTranslation(t,i){return t.isVector2?this.set(1,0,t.x,0,1,t.y,0,0,1):this.set(1,0,t,0,1,i,0,0,1),this}makeRotation(t){const i=Math.cos(t),s=Math.sin(t);return this.set(i,-s,0,s,i,0,0,0,1),this}makeScale(t,i){return this.set(t,0,0,0,i,0,0,0,1),this}equals(t){const i=this.elements,s=t.elements;for(let l=0;l<9;l++)if(i[l]!==s[l])return!1;return!0}fromArray(t,i=0){for(let s=0;s<9;s++)this.elements[s]=t[s+i];return this}toArray(t=[],i=0){const s=this.elements;return t[i]=s[0],t[i+1]=s[1],t[i+2]=s[2],t[i+3]=s[3],t[i+4]=s[4],t[i+5]=s[5],t[i+6]=s[6],t[i+7]=s[7],t[i+8]=s[8],t}clone(){return new this.constructor().fromArray(this.elements)}}const Uh=new me;function qv(r){for(let t=r.length-1;t>=0;--t)if(r[t]>=65535)return!0;return!1}function au(r){return document.createElementNS("http://www.w3.org/1999/xhtml",r)}function HM(){const r=au("canvas");return r.style.display="block",r}const x0={};function ll(r){r in x0||(x0[r]=!0,console.warn(r))}function GM(r,t,i){return new Promise(function(s,l){function c(){switch(r.clientWaitSync(t,r.SYNC_FLUSH_COMMANDS_BIT,0)){case r.WAIT_FAILED:l();break;case r.TIMEOUT_EXPIRED:setTimeout(c,i);break;default:s()}}setTimeout(c,i)})}const S0=new me().set(.4123908,.3575843,.1804808,.212639,.7151687,.0721923,.0193308,.1191948,.9505322),M0=new me().set(3.2409699,-1.5373832,-.4986108,-.9692436,1.8759675,.0415551,.0556301,-.203977,1.0569715);function VM(){const r={enabled:!0,workingColorSpace:kr,spaces:{},convert:function(l,c,h){return this.enabled===!1||c===h||!c||!h||(this.spaces[c].transfer===We&&(l.r=ha(l.r),l.g=ha(l.g),l.b=ha(l.b)),this.spaces[c].primaries!==this.spaces[h].primaries&&(l.applyMatrix3(this.spaces[c].toXYZ),l.applyMatrix3(this.spaces[h].fromXYZ)),this.spaces[h].transfer===We&&(l.r=Fr(l.r),l.g=Fr(l.g),l.b=Fr(l.b))),l},workingToColorSpace:function(l,c){return this.convert(l,this.workingColorSpace,c)},colorSpaceToWorking:function(l,c){return this.convert(l,c,this.workingColorSpace)},getPrimaries:function(l){return this.spaces[l].primaries},getTransfer:function(l){return l===Wa?nu:this.spaces[l].transfer},getToneMappingMode:function(l){return this.spaces[l].outputColorSpaceConfig.toneMappingMode||"standard"},getLuminanceCoefficients:function(l,c=this.workingColorSpace){return l.fromArray(this.spaces[c].luminanceCoefficients)},define:function(l){Object.assign(this.spaces,l)},_getMatrix:function(l,c,h){return l.copy(this.spaces[c].toXYZ).multiply(this.spaces[h].fromXYZ)},_getDrawingBufferColorSpace:function(l){return this.spaces[l].outputColorSpaceConfig.drawingBufferColorSpace},_getUnpackColorSpace:function(l=this.workingColorSpace){return this.spaces[l].workingColorSpaceConfig.unpackColorSpace},fromWorkingColorSpace:function(l,c){return ll("THREE.ColorManagement: .fromWorkingColorSpace() has been renamed to .workingToColorSpace()."),r.workingToColorSpace(l,c)},toWorkingColorSpace:function(l,c){return ll("THREE.ColorManagement: .toWorkingColorSpace() has been renamed to .colorSpaceToWorking()."),r.colorSpaceToWorking(l,c)}},t=[.64,.33,.3,.6,.15,.06],i=[.2126,.7152,.0722],s=[.3127,.329];return r.define({[kr]:{primaries:t,whitePoint:s,transfer:nu,toXYZ:S0,fromXYZ:M0,luminanceCoefficients:i,workingColorSpaceConfig:{unpackColorSpace:jn},outputColorSpaceConfig:{drawingBufferColorSpace:jn}},[jn]:{primaries:t,whitePoint:s,transfer:We,toXYZ:S0,fromXYZ:M0,luminanceCoefficients:i,outputColorSpaceConfig:{drawingBufferColorSpace:jn}}}),r}const Pe=VM();function ha(r){return r<.04045?r*.0773993808:Math.pow(r*.9478672986+.0521327014,2.4)}function Fr(r){return r<.0031308?r*12.92:1.055*Math.pow(r,.41666)-.055}let gr;class kM{static getDataURL(t,i="image/png"){if(/^data:/i.test(t.src)||typeof HTMLCanvasElement>"u")return t.src;let s;if(t instanceof HTMLCanvasElement)s=t;else{gr===void 0&&(gr=au("canvas")),gr.width=t.width,gr.height=t.height;const l=gr.getContext("2d");t instanceof ImageData?l.putImageData(t,0,0):l.drawImage(t,0,0,t.width,t.height),s=gr}return s.toDataURL(i)}static sRGBToLinear(t){if(typeof HTMLImageElement<"u"&&t instanceof HTMLImageElement||typeof HTMLCanvasElement<"u"&&t instanceof HTMLCanvasElement||typeof ImageBitmap<"u"&&t instanceof ImageBitmap){const i=au("canvas");i.width=t.width,i.height=t.height;const s=i.getContext("2d");s.drawImage(t,0,0,t.width,t.height);const l=s.getImageData(0,0,t.width,t.height),c=l.data;for(let h=0;h<c.length;h++)c[h]=ha(c[h]/255)*255;return s.putImageData(l,0,0),i}else if(t.data){const i=t.data.slice(0);for(let s=0;s<i.length;s++)i instanceof Uint8Array||i instanceof Uint8ClampedArray?i[s]=Math.floor(ha(i[s]/255)*255):i[s]=ha(i[s]);return{data:i,width:t.width,height:t.height}}else return console.warn("THREE.ImageUtils.sRGBToLinear(): Unsupported image type. No color space conversion applied."),t}}let XM=0;class fp{constructor(t=null){this.isSource=!0,Object.defineProperty(this,"id",{value:XM++}),this.uuid=fa(),this.data=t,this.dataReady=!0,this.version=0}getSize(t){const i=this.data;return typeof HTMLVideoElement<"u"&&i instanceof HTMLVideoElement?t.set(i.videoWidth,i.videoHeight,0):i instanceof VideoFrame?t.set(i.displayHeight,i.displayWidth,0):i!==null?t.set(i.width,i.height,i.depth||0):t.set(0,0,0),t}set needsUpdate(t){t===!0&&this.version++}toJSON(t){const i=t===void 0||typeof t=="string";if(!i&&t.images[this.uuid]!==void 0)return t.images[this.uuid];const s={uuid:this.uuid,url:""},l=this.data;if(l!==null){let c;if(Array.isArray(l)){c=[];for(let h=0,d=l.length;h<d;h++)l[h].isDataTexture?c.push(Lh(l[h].image)):c.push(Lh(l[h]))}else c=Lh(l);s.url=c}return i||(t.images[this.uuid]=s),s}}function Lh(r){return typeof HTMLImageElement<"u"&&r instanceof HTMLImageElement||typeof HTMLCanvasElement<"u"&&r instanceof HTMLCanvasElement||typeof ImageBitmap<"u"&&r instanceof ImageBitmap?kM.getDataURL(r):r.data?{data:Array.from(r.data),width:r.width,height:r.height,type:r.data.constructor.name}:(console.warn("THREE.Texture: Unable to serialize Texture."),{})}let WM=0;const Nh=new X;class On extends Us{constructor(t=On.DEFAULT_IMAGE,i=On.DEFAULT_MAPPING,s=As,l=As,c=Pi,h=Rs,d=Ri,m=Ii,p=On.DEFAULT_ANISOTROPY,v=Wa){super(),this.isTexture=!0,Object.defineProperty(this,"id",{value:WM++}),this.uuid=fa(),this.name="",this.source=new fp(t),this.mipmaps=[],this.mapping=i,this.channel=0,this.wrapS=s,this.wrapT=l,this.magFilter=c,this.minFilter=h,this.anisotropy=p,this.format=d,this.internalFormat=null,this.type=m,this.offset=new re(0,0),this.repeat=new re(1,1),this.center=new re(0,0),this.rotation=0,this.matrixAutoUpdate=!0,this.matrix=new me,this.generateMipmaps=!0,this.premultiplyAlpha=!1,this.flipY=!0,this.unpackAlignment=4,this.colorSpace=v,this.userData={},this.updateRanges=[],this.version=0,this.onUpdate=null,this.renderTarget=null,this.isRenderTargetTexture=!1,this.isArrayTexture=!!(t&&t.depth&&t.depth>1),this.pmremVersion=0}get width(){return this.source.getSize(Nh).x}get height(){return this.source.getSize(Nh).y}get depth(){return this.source.getSize(Nh).z}get image(){return this.source.data}set image(t=null){this.source.data=t}updateMatrix(){this.matrix.setUvTransform(this.offset.x,this.offset.y,this.repeat.x,this.repeat.y,this.rotation,this.center.x,this.center.y)}addUpdateRange(t,i){this.updateRanges.push({start:t,count:i})}clearUpdateRanges(){this.updateRanges.length=0}clone(){return new this.constructor().copy(this)}copy(t){return this.name=t.name,this.source=t.source,this.mipmaps=t.mipmaps.slice(0),this.mapping=t.mapping,this.channel=t.channel,this.wrapS=t.wrapS,this.wrapT=t.wrapT,this.magFilter=t.magFilter,this.minFilter=t.minFilter,this.anisotropy=t.anisotropy,this.format=t.format,this.internalFormat=t.internalFormat,this.type=t.type,this.offset.copy(t.offset),this.repeat.copy(t.repeat),this.center.copy(t.center),this.rotation=t.rotation,this.matrixAutoUpdate=t.matrixAutoUpdate,this.matrix.copy(t.matrix),this.generateMipmaps=t.generateMipmaps,this.premultiplyAlpha=t.premultiplyAlpha,this.flipY=t.flipY,this.unpackAlignment=t.unpackAlignment,this.colorSpace=t.colorSpace,this.renderTarget=t.renderTarget,this.isRenderTargetTexture=t.isRenderTargetTexture,this.isArrayTexture=t.isArrayTexture,this.userData=JSON.parse(JSON.stringify(t.userData)),this.needsUpdate=!0,this}setValues(t){for(const i in t){const s=t[i];if(s===void 0){console.warn(`THREE.Texture.setValues(): parameter '${i}' has value of undefined.`);continue}const l=this[i];if(l===void 0){console.warn(`THREE.Texture.setValues(): property '${i}' does not exist.`);continue}l&&s&&l.isVector2&&s.isVector2||l&&s&&l.isVector3&&s.isVector3||l&&s&&l.isMatrix3&&s.isMatrix3?l.copy(s):this[i]=s}}toJSON(t){const i=t===void 0||typeof t=="string";if(!i&&t.textures[this.uuid]!==void 0)return t.textures[this.uuid];const s={metadata:{version:4.7,type:"Texture",generator:"Texture.toJSON"},uuid:this.uuid,name:this.name,image:this.source.toJSON(t).uuid,mapping:this.mapping,channel:this.channel,repeat:[this.repeat.x,this.repeat.y],offset:[this.offset.x,this.offset.y],center:[this.center.x,this.center.y],rotation:this.rotation,wrap:[this.wrapS,this.wrapT],format:this.format,internalFormat:this.internalFormat,type:this.type,colorSpace:this.colorSpace,minFilter:this.minFilter,magFilter:this.magFilter,anisotropy:this.anisotropy,flipY:this.flipY,generateMipmaps:this.generateMipmaps,premultiplyAlpha:this.premultiplyAlpha,unpackAlignment:this.unpackAlignment};return Object.keys(this.userData).length>0&&(s.userData=this.userData),i||(t.textures[this.uuid]=s),s}dispose(){this.dispatchEvent({type:"dispose"})}transformUv(t){if(this.mapping!==Bv)return t;if(t.applyMatrix3(this.matrix),t.x<0||t.x>1)switch(this.wrapS){case Md:t.x=t.x-Math.floor(t.x);break;case As:t.x=t.x<0?0:1;break;case Ed:Math.abs(Math.floor(t.x)%2)===1?t.x=Math.ceil(t.x)-t.x:t.x=t.x-Math.floor(t.x);break}if(t.y<0||t.y>1)switch(this.wrapT){case Md:t.y=t.y-Math.floor(t.y);break;case As:t.y=t.y<0?0:1;break;case Ed:Math.abs(Math.floor(t.y)%2)===1?t.y=Math.ceil(t.y)-t.y:t.y=t.y-Math.floor(t.y);break}return this.flipY&&(t.y=1-t.y),t}set needsUpdate(t){t===!0&&(this.version++,this.source.needsUpdate=!0)}set needsPMREMUpdate(t){t===!0&&this.pmremVersion++}}On.DEFAULT_IMAGE=null;On.DEFAULT_MAPPING=Bv;On.DEFAULT_ANISOTROPY=1;class Ye{constructor(t=0,i=0,s=0,l=1){Ye.prototype.isVector4=!0,this.x=t,this.y=i,this.z=s,this.w=l}get width(){return this.z}set width(t){this.z=t}get height(){return this.w}set height(t){this.w=t}set(t,i,s,l){return this.x=t,this.y=i,this.z=s,this.w=l,this}setScalar(t){return this.x=t,this.y=t,this.z=t,this.w=t,this}setX(t){return this.x=t,this}setY(t){return this.y=t,this}setZ(t){return this.z=t,this}setW(t){return this.w=t,this}setComponent(t,i){switch(t){case 0:this.x=i;break;case 1:this.y=i;break;case 2:this.z=i;break;case 3:this.w=i;break;default:throw new Error("index is out of range: "+t)}return this}getComponent(t){switch(t){case 0:return this.x;case 1:return this.y;case 2:return this.z;case 3:return this.w;default:throw new Error("index is out of range: "+t)}}clone(){return new this.constructor(this.x,this.y,this.z,this.w)}copy(t){return this.x=t.x,this.y=t.y,this.z=t.z,this.w=t.w!==void 0?t.w:1,this}add(t){return this.x+=t.x,this.y+=t.y,this.z+=t.z,this.w+=t.w,this}addScalar(t){return this.x+=t,this.y+=t,this.z+=t,this.w+=t,this}addVectors(t,i){return this.x=t.x+i.x,this.y=t.y+i.y,this.z=t.z+i.z,this.w=t.w+i.w,this}addScaledVector(t,i){return this.x+=t.x*i,this.y+=t.y*i,this.z+=t.z*i,this.w+=t.w*i,this}sub(t){return this.x-=t.x,this.y-=t.y,this.z-=t.z,this.w-=t.w,this}subScalar(t){return this.x-=t,this.y-=t,this.z-=t,this.w-=t,this}subVectors(t,i){return this.x=t.x-i.x,this.y=t.y-i.y,this.z=t.z-i.z,this.w=t.w-i.w,this}multiply(t){return this.x*=t.x,this.y*=t.y,this.z*=t.z,this.w*=t.w,this}multiplyScalar(t){return this.x*=t,this.y*=t,this.z*=t,this.w*=t,this}applyMatrix4(t){const i=this.x,s=this.y,l=this.z,c=this.w,h=t.elements;return this.x=h[0]*i+h[4]*s+h[8]*l+h[12]*c,this.y=h[1]*i+h[5]*s+h[9]*l+h[13]*c,this.z=h[2]*i+h[6]*s+h[10]*l+h[14]*c,this.w=h[3]*i+h[7]*s+h[11]*l+h[15]*c,this}divide(t){return this.x/=t.x,this.y/=t.y,this.z/=t.z,this.w/=t.w,this}divideScalar(t){return this.multiplyScalar(1/t)}setAxisAngleFromQuaternion(t){this.w=2*Math.acos(t.w);const i=Math.sqrt(1-t.w*t.w);return i<1e-4?(this.x=1,this.y=0,this.z=0):(this.x=t.x/i,this.y=t.y/i,this.z=t.z/i),this}setAxisAngleFromRotationMatrix(t){let i,s,l,c;const m=t.elements,p=m[0],v=m[4],g=m[8],x=m[1],S=m[5],b=m[9],R=m[2],M=m[6],_=m[10];if(Math.abs(v-x)<.01&&Math.abs(g-R)<.01&&Math.abs(b-M)<.01){if(Math.abs(v+x)<.1&&Math.abs(g+R)<.1&&Math.abs(b+M)<.1&&Math.abs(p+S+_-3)<.1)return this.set(1,0,0,0),this;i=Math.PI;const U=(p+1)/2,T=(S+1)/2,P=(_+1)/2,B=(v+x)/4,F=(g+R)/4,G=(b+M)/4;return U>T&&U>P?U<.01?(s=0,l=.707106781,c=.707106781):(s=Math.sqrt(U),l=B/s,c=F/s):T>P?T<.01?(s=.707106781,l=0,c=.707106781):(l=Math.sqrt(T),s=B/l,c=G/l):P<.01?(s=.707106781,l=.707106781,c=0):(c=Math.sqrt(P),s=F/c,l=G/c),this.set(s,l,c,i),this}let L=Math.sqrt((M-b)*(M-b)+(g-R)*(g-R)+(x-v)*(x-v));return Math.abs(L)<.001&&(L=1),this.x=(M-b)/L,this.y=(g-R)/L,this.z=(x-v)/L,this.w=Math.acos((p+S+_-1)/2),this}setFromMatrixPosition(t){const i=t.elements;return this.x=i[12],this.y=i[13],this.z=i[14],this.w=i[15],this}min(t){return this.x=Math.min(this.x,t.x),this.y=Math.min(this.y,t.y),this.z=Math.min(this.z,t.z),this.w=Math.min(this.w,t.w),this}max(t){return this.x=Math.max(this.x,t.x),this.y=Math.max(this.y,t.y),this.z=Math.max(this.z,t.z),this.w=Math.max(this.w,t.w),this}clamp(t,i){return this.x=ye(this.x,t.x,i.x),this.y=ye(this.y,t.y,i.y),this.z=ye(this.z,t.z,i.z),this.w=ye(this.w,t.w,i.w),this}clampScalar(t,i){return this.x=ye(this.x,t,i),this.y=ye(this.y,t,i),this.z=ye(this.z,t,i),this.w=ye(this.w,t,i),this}clampLength(t,i){const s=this.length();return this.divideScalar(s||1).multiplyScalar(ye(s,t,i))}floor(){return this.x=Math.floor(this.x),this.y=Math.floor(this.y),this.z=Math.floor(this.z),this.w=Math.floor(this.w),this}ceil(){return this.x=Math.ceil(this.x),this.y=Math.ceil(this.y),this.z=Math.ceil(this.z),this.w=Math.ceil(this.w),this}round(){return this.x=Math.round(this.x),this.y=Math.round(this.y),this.z=Math.round(this.z),this.w=Math.round(this.w),this}roundToZero(){return this.x=Math.trunc(this.x),this.y=Math.trunc(this.y),this.z=Math.trunc(this.z),this.w=Math.trunc(this.w),this}negate(){return this.x=-this.x,this.y=-this.y,this.z=-this.z,this.w=-this.w,this}dot(t){return this.x*t.x+this.y*t.y+this.z*t.z+this.w*t.w}lengthSq(){return this.x*this.x+this.y*this.y+this.z*this.z+this.w*this.w}length(){return Math.sqrt(this.x*this.x+this.y*this.y+this.z*this.z+this.w*this.w)}manhattanLength(){return Math.abs(this.x)+Math.abs(this.y)+Math.abs(this.z)+Math.abs(this.w)}normalize(){return this.divideScalar(this.length()||1)}setLength(t){return this.normalize().multiplyScalar(t)}lerp(t,i){return this.x+=(t.x-this.x)*i,this.y+=(t.y-this.y)*i,this.z+=(t.z-this.z)*i,this.w+=(t.w-this.w)*i,this}lerpVectors(t,i,s){return this.x=t.x+(i.x-t.x)*s,this.y=t.y+(i.y-t.y)*s,this.z=t.z+(i.z-t.z)*s,this.w=t.w+(i.w-t.w)*s,this}equals(t){return t.x===this.x&&t.y===this.y&&t.z===this.z&&t.w===this.w}fromArray(t,i=0){return this.x=t[i],this.y=t[i+1],this.z=t[i+2],this.w=t[i+3],this}toArray(t=[],i=0){return t[i]=this.x,t[i+1]=this.y,t[i+2]=this.z,t[i+3]=this.w,t}fromBufferAttribute(t,i){return this.x=t.getX(i),this.y=t.getY(i),this.z=t.getZ(i),this.w=t.getW(i),this}random(){return this.x=Math.random(),this.y=Math.random(),this.z=Math.random(),this.w=Math.random(),this}*[Symbol.iterator](){yield this.x,yield this.y,yield this.z,yield this.w}}class jM extends Us{constructor(t=1,i=1,s={}){super(),s=Object.assign({generateMipmaps:!1,internalFormat:null,minFilter:Pi,depthBuffer:!0,stencilBuffer:!1,resolveDepthBuffer:!0,resolveStencilBuffer:!0,depthTexture:null,samples:0,count:1,depth:1,multiview:!1},s),this.isRenderTarget=!0,this.width=t,this.height=i,this.depth=s.depth,this.scissor=new Ye(0,0,t,i),this.scissorTest=!1,this.viewport=new Ye(0,0,t,i);const l={width:t,height:i,depth:s.depth},c=new On(l);this.textures=[];const h=s.count;for(let d=0;d<h;d++)this.textures[d]=c.clone(),this.textures[d].isRenderTargetTexture=!0,this.textures[d].renderTarget=this;this._setTextureOptions(s),this.depthBuffer=s.depthBuffer,this.stencilBuffer=s.stencilBuffer,this.resolveDepthBuffer=s.resolveDepthBuffer,this.resolveStencilBuffer=s.resolveStencilBuffer,this._depthTexture=null,this.depthTexture=s.depthTexture,this.samples=s.samples,this.multiview=s.multiview}_setTextureOptions(t={}){const i={minFilter:Pi,generateMipmaps:!1,flipY:!1,internalFormat:null};t.mapping!==void 0&&(i.mapping=t.mapping),t.wrapS!==void 0&&(i.wrapS=t.wrapS),t.wrapT!==void 0&&(i.wrapT=t.wrapT),t.wrapR!==void 0&&(i.wrapR=t.wrapR),t.magFilter!==void 0&&(i.magFilter=t.magFilter),t.minFilter!==void 0&&(i.minFilter=t.minFilter),t.format!==void 0&&(i.format=t.format),t.type!==void 0&&(i.type=t.type),t.anisotropy!==void 0&&(i.anisotropy=t.anisotropy),t.colorSpace!==void 0&&(i.colorSpace=t.colorSpace),t.flipY!==void 0&&(i.flipY=t.flipY),t.generateMipmaps!==void 0&&(i.generateMipmaps=t.generateMipmaps),t.internalFormat!==void 0&&(i.internalFormat=t.internalFormat);for(let s=0;s<this.textures.length;s++)this.textures[s].setValues(i)}get texture(){return this.textures[0]}set texture(t){this.textures[0]=t}set depthTexture(t){this._depthTexture!==null&&(this._depthTexture.renderTarget=null),t!==null&&(t.renderTarget=this),this._depthTexture=t}get depthTexture(){return this._depthTexture}setSize(t,i,s=1){if(this.width!==t||this.height!==i||this.depth!==s){this.width=t,this.height=i,this.depth=s;for(let l=0,c=this.textures.length;l<c;l++)this.textures[l].image.width=t,this.textures[l].image.height=i,this.textures[l].image.depth=s,this.textures[l].isArrayTexture=this.textures[l].image.depth>1;this.dispose()}this.viewport.set(0,0,t,i),this.scissor.set(0,0,t,i)}clone(){return new this.constructor().copy(this)}copy(t){this.width=t.width,this.height=t.height,this.depth=t.depth,this.scissor.copy(t.scissor),this.scissorTest=t.scissorTest,this.viewport.copy(t.viewport),this.textures.length=0;for(let i=0,s=t.textures.length;i<s;i++){this.textures[i]=t.textures[i].clone(),this.textures[i].isRenderTargetTexture=!0,this.textures[i].renderTarget=this;const l=Object.assign({},t.textures[i].image);this.textures[i].source=new fp(l)}return this.depthBuffer=t.depthBuffer,this.stencilBuffer=t.stencilBuffer,this.resolveDepthBuffer=t.resolveDepthBuffer,this.resolveStencilBuffer=t.resolveStencilBuffer,t.depthTexture!==null&&(this.depthTexture=t.depthTexture.clone()),this.samples=t.samples,this}dispose(){this.dispatchEvent({type:"dispose"})}}class Ds extends jM{constructor(t=1,i=1,s={}){super(t,i,s),this.isWebGLRenderTarget=!0}}class Zv extends On{constructor(t=null,i=1,s=1,l=1){super(null),this.isDataArrayTexture=!0,this.image={data:t,width:i,height:s,depth:l},this.magFilter=si,this.minFilter=si,this.wrapR=As,this.generateMipmaps=!1,this.flipY=!1,this.unpackAlignment=1,this.layerUpdates=new Set}addLayerUpdate(t){this.layerUpdates.add(t)}clearLayerUpdates(){this.layerUpdates.clear()}}class YM extends On{constructor(t=null,i=1,s=1,l=1){super(null),this.isData3DTexture=!0,this.image={data:t,width:i,height:s,depth:l},this.magFilter=si,this.minFilter=si,this.wrapR=As,this.generateMipmaps=!1,this.flipY=!1,this.unpackAlignment=1}}class da{constructor(t=new X(1/0,1/0,1/0),i=new X(-1/0,-1/0,-1/0)){this.isBox3=!0,this.min=t,this.max=i}set(t,i){return this.min.copy(t),this.max.copy(i),this}setFromArray(t){this.makeEmpty();for(let i=0,s=t.length;i<s;i+=3)this.expandByPoint(Ei.fromArray(t,i));return this}setFromBufferAttribute(t){this.makeEmpty();for(let i=0,s=t.count;i<s;i++)this.expandByPoint(Ei.fromBufferAttribute(t,i));return this}setFromPoints(t){this.makeEmpty();for(let i=0,s=t.length;i<s;i++)this.expandByPoint(t[i]);return this}setFromCenterAndSize(t,i){const s=Ei.copy(i).multiplyScalar(.5);return this.min.copy(t).sub(s),this.max.copy(t).add(s),this}setFromObject(t,i=!1){return this.makeEmpty(),this.expandByObject(t,i)}clone(){return new this.constructor().copy(this)}copy(t){return this.min.copy(t.min),this.max.copy(t.max),this}makeEmpty(){return this.min.x=this.min.y=this.min.z=1/0,this.max.x=this.max.y=this.max.z=-1/0,this}isEmpty(){return this.max.x<this.min.x||this.max.y<this.min.y||this.max.z<this.min.z}getCenter(t){return this.isEmpty()?t.set(0,0,0):t.addVectors(this.min,this.max).multiplyScalar(.5)}getSize(t){return this.isEmpty()?t.set(0,0,0):t.subVectors(this.max,this.min)}expandByPoint(t){return this.min.min(t),this.max.max(t),this}expandByVector(t){return this.min.sub(t),this.max.add(t),this}expandByScalar(t){return this.min.addScalar(-t),this.max.addScalar(t),this}expandByObject(t,i=!1){t.updateWorldMatrix(!1,!1);const s=t.geometry;if(s!==void 0){const c=s.getAttribute("position");if(i===!0&&c!==void 0&&t.isInstancedMesh!==!0)for(let h=0,d=c.count;h<d;h++)t.isMesh===!0?t.getVertexPosition(h,Ei):Ei.fromBufferAttribute(c,h),Ei.applyMatrix4(t.matrixWorld),this.expandByPoint(Ei);else t.boundingBox!==void 0?(t.boundingBox===null&&t.computeBoundingBox(),Sc.copy(t.boundingBox)):(s.boundingBox===null&&s.computeBoundingBox(),Sc.copy(s.boundingBox)),Sc.applyMatrix4(t.matrixWorld),this.union(Sc)}const l=t.children;for(let c=0,h=l.length;c<h;c++)this.expandByObject(l[c],i);return this}containsPoint(t){return t.x>=this.min.x&&t.x<=this.max.x&&t.y>=this.min.y&&t.y<=this.max.y&&t.z>=this.min.z&&t.z<=this.max.z}containsBox(t){return this.min.x<=t.min.x&&t.max.x<=this.max.x&&this.min.y<=t.min.y&&t.max.y<=this.max.y&&this.min.z<=t.min.z&&t.max.z<=this.max.z}getParameter(t,i){return i.set((t.x-this.min.x)/(this.max.x-this.min.x),(t.y-this.min.y)/(this.max.y-this.min.y),(t.z-this.min.z)/(this.max.z-this.min.z))}intersectsBox(t){return t.max.x>=this.min.x&&t.min.x<=this.max.x&&t.max.y>=this.min.y&&t.min.y<=this.max.y&&t.max.z>=this.min.z&&t.min.z<=this.max.z}intersectsSphere(t){return this.clampPoint(t.center,Ei),Ei.distanceToSquared(t.center)<=t.radius*t.radius}intersectsPlane(t){let i,s;return t.normal.x>0?(i=t.normal.x*this.min.x,s=t.normal.x*this.max.x):(i=t.normal.x*this.max.x,s=t.normal.x*this.min.x),t.normal.y>0?(i+=t.normal.y*this.min.y,s+=t.normal.y*this.max.y):(i+=t.normal.y*this.max.y,s+=t.normal.y*this.min.y),t.normal.z>0?(i+=t.normal.z*this.min.z,s+=t.normal.z*this.max.z):(i+=t.normal.z*this.max.z,s+=t.normal.z*this.min.z),i<=-t.constant&&s>=-t.constant}intersectsTriangle(t){if(this.isEmpty())return!1;this.getCenter(Go),Mc.subVectors(this.max,Go),_r.subVectors(t.a,Go),vr.subVectors(t.b,Go),yr.subVectors(t.c,Go),Ia.subVectors(vr,_r),Fa.subVectors(yr,vr),ms.subVectors(_r,yr);let i=[0,-Ia.z,Ia.y,0,-Fa.z,Fa.y,0,-ms.z,ms.y,Ia.z,0,-Ia.x,Fa.z,0,-Fa.x,ms.z,0,-ms.x,-Ia.y,Ia.x,0,-Fa.y,Fa.x,0,-ms.y,ms.x,0];return!Oh(i,_r,vr,yr,Mc)||(i=[1,0,0,0,1,0,0,0,1],!Oh(i,_r,vr,yr,Mc))?!1:(Ec.crossVectors(Ia,Fa),i=[Ec.x,Ec.y,Ec.z],Oh(i,_r,vr,yr,Mc))}clampPoint(t,i){return i.copy(t).clamp(this.min,this.max)}distanceToPoint(t){return this.clampPoint(t,Ei).distanceTo(t)}getBoundingSphere(t){return this.isEmpty()?t.makeEmpty():(this.getCenter(t.center),t.radius=this.getSize(Ei).length()*.5),t}intersect(t){return this.min.max(t.min),this.max.min(t.max),this.isEmpty()&&this.makeEmpty(),this}union(t){return this.min.min(t.min),this.max.max(t.max),this}applyMatrix4(t){return this.isEmpty()?this:(aa[0].set(this.min.x,this.min.y,this.min.z).applyMatrix4(t),aa[1].set(this.min.x,this.min.y,this.max.z).applyMatrix4(t),aa[2].set(this.min.x,this.max.y,this.min.z).applyMatrix4(t),aa[3].set(this.min.x,this.max.y,this.max.z).applyMatrix4(t),aa[4].set(this.max.x,this.min.y,this.min.z).applyMatrix4(t),aa[5].set(this.max.x,this.min.y,this.max.z).applyMatrix4(t),aa[6].set(this.max.x,this.max.y,this.min.z).applyMatrix4(t),aa[7].set(this.max.x,this.max.y,this.max.z).applyMatrix4(t),this.setFromPoints(aa),this)}translate(t){return this.min.add(t),this.max.add(t),this}equals(t){return t.min.equals(this.min)&&t.max.equals(this.max)}toJSON(){return{min:this.min.toArray(),max:this.max.toArray()}}fromJSON(t){return this.min.fromArray(t.min),this.max.fromArray(t.max),this}}const aa=[new X,new X,new X,new X,new X,new X,new X,new X],Ei=new X,Sc=new da,_r=new X,vr=new X,yr=new X,Ia=new X,Fa=new X,ms=new X,Go=new X,Mc=new X,Ec=new X,gs=new X;function Oh(r,t,i,s,l){for(let c=0,h=r.length-3;c<=h;c+=3){gs.fromArray(r,c);const d=l.x*Math.abs(gs.x)+l.y*Math.abs(gs.y)+l.z*Math.abs(gs.z),m=t.dot(gs),p=i.dot(gs),v=s.dot(gs);if(Math.max(-Math.max(m,p,v),Math.min(m,p,v))>d)return!1}return!0}const qM=new da,Vo=new X,Ph=new X;class Wr{constructor(t=new X,i=-1){this.isSphere=!0,this.center=t,this.radius=i}set(t,i){return this.center.copy(t),this.radius=i,this}setFromPoints(t,i){const s=this.center;i!==void 0?s.copy(i):qM.setFromPoints(t).getCenter(s);let l=0;for(let c=0,h=t.length;c<h;c++)l=Math.max(l,s.distanceToSquared(t[c]));return this.radius=Math.sqrt(l),this}copy(t){return this.center.copy(t.center),this.radius=t.radius,this}isEmpty(){return this.radius<0}makeEmpty(){return this.center.set(0,0,0),this.radius=-1,this}containsPoint(t){return t.distanceToSquared(this.center)<=this.radius*this.radius}distanceToPoint(t){return t.distanceTo(this.center)-this.radius}intersectsSphere(t){const i=this.radius+t.radius;return t.center.distanceToSquared(this.center)<=i*i}intersectsBox(t){return t.intersectsSphere(this)}intersectsPlane(t){return Math.abs(t.distanceToPoint(this.center))<=this.radius}clampPoint(t,i){const s=this.center.distanceToSquared(t);return i.copy(t),s>this.radius*this.radius&&(i.sub(this.center).normalize(),i.multiplyScalar(this.radius).add(this.center)),i}getBoundingBox(t){return this.isEmpty()?(t.makeEmpty(),t):(t.set(this.center,this.center),t.expandByScalar(this.radius),t)}applyMatrix4(t){return this.center.applyMatrix4(t),this.radius=this.radius*t.getMaxScaleOnAxis(),this}translate(t){return this.center.add(t),this}expandByPoint(t){if(this.isEmpty())return this.center.copy(t),this.radius=0,this;Vo.subVectors(t,this.center);const i=Vo.lengthSq();if(i>this.radius*this.radius){const s=Math.sqrt(i),l=(s-this.radius)*.5;this.center.addScaledVector(Vo,l/s),this.radius+=l}return this}union(t){return t.isEmpty()?this:this.isEmpty()?(this.copy(t),this):(this.center.equals(t.center)===!0?this.radius=Math.max(this.radius,t.radius):(Ph.subVectors(t.center,this.center).setLength(t.radius),this.expandByPoint(Vo.copy(t.center).add(Ph)),this.expandByPoint(Vo.copy(t.center).sub(Ph))),this)}equals(t){return t.center.equals(this.center)&&t.radius===this.radius}clone(){return new this.constructor().copy(this)}toJSON(){return{radius:this.radius,center:this.center.toArray()}}fromJSON(t){return this.radius=t.radius,this.center.fromArray(t.center),this}}const sa=new X,zh=new X,Tc=new X,Ha=new X,Bh=new X,bc=new X,Ih=new X;class hu{constructor(t=new X,i=new X(0,0,-1)){this.origin=t,this.direction=i}set(t,i){return this.origin.copy(t),this.direction.copy(i),this}copy(t){return this.origin.copy(t.origin),this.direction.copy(t.direction),this}at(t,i){return i.copy(this.origin).addScaledVector(this.direction,t)}lookAt(t){return this.direction.copy(t).sub(this.origin).normalize(),this}recast(t){return this.origin.copy(this.at(t,sa)),this}closestPointToPoint(t,i){i.subVectors(t,this.origin);const s=i.dot(this.direction);return s<0?i.copy(this.origin):i.copy(this.origin).addScaledVector(this.direction,s)}distanceToPoint(t){return Math.sqrt(this.distanceSqToPoint(t))}distanceSqToPoint(t){const i=sa.subVectors(t,this.origin).dot(this.direction);return i<0?this.origin.distanceToSquared(t):(sa.copy(this.origin).addScaledVector(this.direction,i),sa.distanceToSquared(t))}distanceSqToSegment(t,i,s,l){zh.copy(t).add(i).multiplyScalar(.5),Tc.copy(i).sub(t).normalize(),Ha.copy(this.origin).sub(zh);const c=t.distanceTo(i)*.5,h=-this.direction.dot(Tc),d=Ha.dot(this.direction),m=-Ha.dot(Tc),p=Ha.lengthSq(),v=Math.abs(1-h*h);let g,x,S,b;if(v>0)if(g=h*m-d,x=h*d-m,b=c*v,g>=0)if(x>=-b)if(x<=b){const R=1/v;g*=R,x*=R,S=g*(g+h*x+2*d)+x*(h*g+x+2*m)+p}else x=c,g=Math.max(0,-(h*x+d)),S=-g*g+x*(x+2*m)+p;else x=-c,g=Math.max(0,-(h*x+d)),S=-g*g+x*(x+2*m)+p;else x<=-b?(g=Math.max(0,-(-h*c+d)),x=g>0?-c:Math.min(Math.max(-c,-m),c),S=-g*g+x*(x+2*m)+p):x<=b?(g=0,x=Math.min(Math.max(-c,-m),c),S=x*(x+2*m)+p):(g=Math.max(0,-(h*c+d)),x=g>0?c:Math.min(Math.max(-c,-m),c),S=-g*g+x*(x+2*m)+p);else x=h>0?-c:c,g=Math.max(0,-(h*x+d)),S=-g*g+x*(x+2*m)+p;return s&&s.copy(this.origin).addScaledVector(this.direction,g),l&&l.copy(zh).addScaledVector(Tc,x),S}intersectSphere(t,i){sa.subVectors(t.center,this.origin);const s=sa.dot(this.direction),l=sa.dot(sa)-s*s,c=t.radius*t.radius;if(l>c)return null;const h=Math.sqrt(c-l),d=s-h,m=s+h;return m<0?null:d<0?this.at(m,i):this.at(d,i)}intersectsSphere(t){return t.radius<0?!1:this.distanceSqToPoint(t.center)<=t.radius*t.radius}distanceToPlane(t){const i=t.normal.dot(this.direction);if(i===0)return t.distanceToPoint(this.origin)===0?0:null;const s=-(this.origin.dot(t.normal)+t.constant)/i;return s>=0?s:null}intersectPlane(t,i){const s=this.distanceToPlane(t);return s===null?null:this.at(s,i)}intersectsPlane(t){const i=t.distanceToPoint(this.origin);return i===0||t.normal.dot(this.direction)*i<0}intersectBox(t,i){let s,l,c,h,d,m;const p=1/this.direction.x,v=1/this.direction.y,g=1/this.direction.z,x=this.origin;return p>=0?(s=(t.min.x-x.x)*p,l=(t.max.x-x.x)*p):(s=(t.max.x-x.x)*p,l=(t.min.x-x.x)*p),v>=0?(c=(t.min.y-x.y)*v,h=(t.max.y-x.y)*v):(c=(t.max.y-x.y)*v,h=(t.min.y-x.y)*v),s>h||c>l||((c>s||isNaN(s))&&(s=c),(h<l||isNaN(l))&&(l=h),g>=0?(d=(t.min.z-x.z)*g,m=(t.max.z-x.z)*g):(d=(t.max.z-x.z)*g,m=(t.min.z-x.z)*g),s>m||d>l)||((d>s||s!==s)&&(s=d),(m<l||l!==l)&&(l=m),l<0)?null:this.at(s>=0?s:l,i)}intersectsBox(t){return this.intersectBox(t,sa)!==null}intersectTriangle(t,i,s,l,c){Bh.subVectors(i,t),bc.subVectors(s,t),Ih.crossVectors(Bh,bc);let h=this.direction.dot(Ih),d;if(h>0){if(l)return null;d=1}else if(h<0)d=-1,h=-h;else return null;Ha.subVectors(this.origin,t);const m=d*this.direction.dot(bc.crossVectors(Ha,bc));if(m<0)return null;const p=d*this.direction.dot(Bh.cross(Ha));if(p<0||m+p>h)return null;const v=-d*Ha.dot(Ih);return v<0?null:this.at(v/h,c)}applyMatrix4(t){return this.origin.applyMatrix4(t),this.direction.transformDirection(t),this}equals(t){return t.origin.equals(this.origin)&&t.direction.equals(this.direction)}clone(){return new this.constructor().copy(this)}}class qe{constructor(t,i,s,l,c,h,d,m,p,v,g,x,S,b,R,M){qe.prototype.isMatrix4=!0,this.elements=[1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1],t!==void 0&&this.set(t,i,s,l,c,h,d,m,p,v,g,x,S,b,R,M)}set(t,i,s,l,c,h,d,m,p,v,g,x,S,b,R,M){const _=this.elements;return _[0]=t,_[4]=i,_[8]=s,_[12]=l,_[1]=c,_[5]=h,_[9]=d,_[13]=m,_[2]=p,_[6]=v,_[10]=g,_[14]=x,_[3]=S,_[7]=b,_[11]=R,_[15]=M,this}identity(){return this.set(1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1),this}clone(){return new qe().fromArray(this.elements)}copy(t){const i=this.elements,s=t.elements;return i[0]=s[0],i[1]=s[1],i[2]=s[2],i[3]=s[3],i[4]=s[4],i[5]=s[5],i[6]=s[6],i[7]=s[7],i[8]=s[8],i[9]=s[9],i[10]=s[10],i[11]=s[11],i[12]=s[12],i[13]=s[13],i[14]=s[14],i[15]=s[15],this}copyPosition(t){const i=this.elements,s=t.elements;return i[12]=s[12],i[13]=s[13],i[14]=s[14],this}setFromMatrix3(t){const i=t.elements;return this.set(i[0],i[3],i[6],0,i[1],i[4],i[7],0,i[2],i[5],i[8],0,0,0,0,1),this}extractBasis(t,i,s){return t.setFromMatrixColumn(this,0),i.setFromMatrixColumn(this,1),s.setFromMatrixColumn(this,2),this}makeBasis(t,i,s){return this.set(t.x,i.x,s.x,0,t.y,i.y,s.y,0,t.z,i.z,s.z,0,0,0,0,1),this}extractRotation(t){const i=this.elements,s=t.elements,l=1/xr.setFromMatrixColumn(t,0).length(),c=1/xr.setFromMatrixColumn(t,1).length(),h=1/xr.setFromMatrixColumn(t,2).length();return i[0]=s[0]*l,i[1]=s[1]*l,i[2]=s[2]*l,i[3]=0,i[4]=s[4]*c,i[5]=s[5]*c,i[6]=s[6]*c,i[7]=0,i[8]=s[8]*h,i[9]=s[9]*h,i[10]=s[10]*h,i[11]=0,i[12]=0,i[13]=0,i[14]=0,i[15]=1,this}makeRotationFromEuler(t){const i=this.elements,s=t.x,l=t.y,c=t.z,h=Math.cos(s),d=Math.sin(s),m=Math.cos(l),p=Math.sin(l),v=Math.cos(c),g=Math.sin(c);if(t.order==="XYZ"){const x=h*v,S=h*g,b=d*v,R=d*g;i[0]=m*v,i[4]=-m*g,i[8]=p,i[1]=S+b*p,i[5]=x-R*p,i[9]=-d*m,i[2]=R-x*p,i[6]=b+S*p,i[10]=h*m}else if(t.order==="YXZ"){const x=m*v,S=m*g,b=p*v,R=p*g;i[0]=x+R*d,i[4]=b*d-S,i[8]=h*p,i[1]=h*g,i[5]=h*v,i[9]=-d,i[2]=S*d-b,i[6]=R+x*d,i[10]=h*m}else if(t.order==="ZXY"){const x=m*v,S=m*g,b=p*v,R=p*g;i[0]=x-R*d,i[4]=-h*g,i[8]=b+S*d,i[1]=S+b*d,i[5]=h*v,i[9]=R-x*d,i[2]=-h*p,i[6]=d,i[10]=h*m}else if(t.order==="ZYX"){const x=h*v,S=h*g,b=d*v,R=d*g;i[0]=m*v,i[4]=b*p-S,i[8]=x*p+R,i[1]=m*g,i[5]=R*p+x,i[9]=S*p-b,i[2]=-p,i[6]=d*m,i[10]=h*m}else if(t.order==="YZX"){const x=h*m,S=h*p,b=d*m,R=d*p;i[0]=m*v,i[4]=R-x*g,i[8]=b*g+S,i[1]=g,i[5]=h*v,i[9]=-d*v,i[2]=-p*v,i[6]=S*g+b,i[10]=x-R*g}else if(t.order==="XZY"){const x=h*m,S=h*p,b=d*m,R=d*p;i[0]=m*v,i[4]=-g,i[8]=p*v,i[1]=x*g+R,i[5]=h*v,i[9]=S*g-b,i[2]=b*g-S,i[6]=d*v,i[10]=R*g+x}return i[3]=0,i[7]=0,i[11]=0,i[12]=0,i[13]=0,i[14]=0,i[15]=1,this}makeRotationFromQuaternion(t){return this.compose(ZM,t,KM)}lookAt(t,i,s){const l=this.elements;return ni.subVectors(t,i),ni.lengthSq()===0&&(ni.z=1),ni.normalize(),Ga.crossVectors(s,ni),Ga.lengthSq()===0&&(Math.abs(s.z)===1?ni.x+=1e-4:ni.z+=1e-4,ni.normalize(),Ga.crossVectors(s,ni)),Ga.normalize(),Ac.crossVectors(ni,Ga),l[0]=Ga.x,l[4]=Ac.x,l[8]=ni.x,l[1]=Ga.y,l[5]=Ac.y,l[9]=ni.y,l[2]=Ga.z,l[6]=Ac.z,l[10]=ni.z,this}multiply(t){return this.multiplyMatrices(this,t)}premultiply(t){return this.multiplyMatrices(t,this)}multiplyMatrices(t,i){const s=t.elements,l=i.elements,c=this.elements,h=s[0],d=s[4],m=s[8],p=s[12],v=s[1],g=s[5],x=s[9],S=s[13],b=s[2],R=s[6],M=s[10],_=s[14],L=s[3],U=s[7],T=s[11],P=s[15],B=l[0],F=l[4],G=l[8],D=l[12],w=l[1],V=l[5],K=l[9],ot=l[13],ct=l[2],ht=l[6],N=l[10],J=l[14],Y=l[3],Tt=l[7],Ct=l[11],z=l[15];return c[0]=h*B+d*w+m*ct+p*Y,c[4]=h*F+d*V+m*ht+p*Tt,c[8]=h*G+d*K+m*N+p*Ct,c[12]=h*D+d*ot+m*J+p*z,c[1]=v*B+g*w+x*ct+S*Y,c[5]=v*F+g*V+x*ht+S*Tt,c[9]=v*G+g*K+x*N+S*Ct,c[13]=v*D+g*ot+x*J+S*z,c[2]=b*B+R*w+M*ct+_*Y,c[6]=b*F+R*V+M*ht+_*Tt,c[10]=b*G+R*K+M*N+_*Ct,c[14]=b*D+R*ot+M*J+_*z,c[3]=L*B+U*w+T*ct+P*Y,c[7]=L*F+U*V+T*ht+P*Tt,c[11]=L*G+U*K+T*N+P*Ct,c[15]=L*D+U*ot+T*J+P*z,this}multiplyScalar(t){const i=this.elements;return i[0]*=t,i[4]*=t,i[8]*=t,i[12]*=t,i[1]*=t,i[5]*=t,i[9]*=t,i[13]*=t,i[2]*=t,i[6]*=t,i[10]*=t,i[14]*=t,i[3]*=t,i[7]*=t,i[11]*=t,i[15]*=t,this}determinant(){const t=this.elements,i=t[0],s=t[4],l=t[8],c=t[12],h=t[1],d=t[5],m=t[9],p=t[13],v=t[2],g=t[6],x=t[10],S=t[14],b=t[3],R=t[7],M=t[11],_=t[15];return b*(+c*m*g-l*p*g-c*d*x+s*p*x+l*d*S-s*m*S)+R*(+i*m*S-i*p*x+c*h*x-l*h*S+l*p*v-c*m*v)+M*(+i*p*g-i*d*S-c*h*g+s*h*S+c*d*v-s*p*v)+_*(-l*d*v-i*m*g+i*d*x+l*h*g-s*h*x+s*m*v)}transpose(){const t=this.elements;let i;return i=t[1],t[1]=t[4],t[4]=i,i=t[2],t[2]=t[8],t[8]=i,i=t[6],t[6]=t[9],t[9]=i,i=t[3],t[3]=t[12],t[12]=i,i=t[7],t[7]=t[13],t[13]=i,i=t[11],t[11]=t[14],t[14]=i,this}setPosition(t,i,s){const l=this.elements;return t.isVector3?(l[12]=t.x,l[13]=t.y,l[14]=t.z):(l[12]=t,l[13]=i,l[14]=s),this}invert(){const t=this.elements,i=t[0],s=t[1],l=t[2],c=t[3],h=t[4],d=t[5],m=t[6],p=t[7],v=t[8],g=t[9],x=t[10],S=t[11],b=t[12],R=t[13],M=t[14],_=t[15],L=g*M*p-R*x*p+R*m*S-d*M*S-g*m*_+d*x*_,U=b*x*p-v*M*p-b*m*S+h*M*S+v*m*_-h*x*_,T=v*R*p-b*g*p+b*d*S-h*R*S-v*d*_+h*g*_,P=b*g*m-v*R*m-b*d*x+h*R*x+v*d*M-h*g*M,B=i*L+s*U+l*T+c*P;if(B===0)return this.set(0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0);const F=1/B;return t[0]=L*F,t[1]=(R*x*c-g*M*c-R*l*S+s*M*S+g*l*_-s*x*_)*F,t[2]=(d*M*c-R*m*c+R*l*p-s*M*p-d*l*_+s*m*_)*F,t[3]=(g*m*c-d*x*c-g*l*p+s*x*p+d*l*S-s*m*S)*F,t[4]=U*F,t[5]=(v*M*c-b*x*c+b*l*S-i*M*S-v*l*_+i*x*_)*F,t[6]=(b*m*c-h*M*c-b*l*p+i*M*p+h*l*_-i*m*_)*F,t[7]=(h*x*c-v*m*c+v*l*p-i*x*p-h*l*S+i*m*S)*F,t[8]=T*F,t[9]=(b*g*c-v*R*c-b*s*S+i*R*S+v*s*_-i*g*_)*F,t[10]=(h*R*c-b*d*c+b*s*p-i*R*p-h*s*_+i*d*_)*F,t[11]=(v*d*c-h*g*c-v*s*p+i*g*p+h*s*S-i*d*S)*F,t[12]=P*F,t[13]=(v*R*l-b*g*l+b*s*x-i*R*x-v*s*M+i*g*M)*F,t[14]=(b*d*l-h*R*l-b*s*m+i*R*m+h*s*M-i*d*M)*F,t[15]=(h*g*l-v*d*l+v*s*m-i*g*m-h*s*x+i*d*x)*F,this}scale(t){const i=this.elements,s=t.x,l=t.y,c=t.z;return i[0]*=s,i[4]*=l,i[8]*=c,i[1]*=s,i[5]*=l,i[9]*=c,i[2]*=s,i[6]*=l,i[10]*=c,i[3]*=s,i[7]*=l,i[11]*=c,this}getMaxScaleOnAxis(){const t=this.elements,i=t[0]*t[0]+t[1]*t[1]+t[2]*t[2],s=t[4]*t[4]+t[5]*t[5]+t[6]*t[6],l=t[8]*t[8]+t[9]*t[9]+t[10]*t[10];return Math.sqrt(Math.max(i,s,l))}makeTranslation(t,i,s){return t.isVector3?this.set(1,0,0,t.x,0,1,0,t.y,0,0,1,t.z,0,0,0,1):this.set(1,0,0,t,0,1,0,i,0,0,1,s,0,0,0,1),this}makeRotationX(t){const i=Math.cos(t),s=Math.sin(t);return this.set(1,0,0,0,0,i,-s,0,0,s,i,0,0,0,0,1),this}makeRotationY(t){const i=Math.cos(t),s=Math.sin(t);return this.set(i,0,s,0,0,1,0,0,-s,0,i,0,0,0,0,1),this}makeRotationZ(t){const i=Math.cos(t),s=Math.sin(t);return this.set(i,-s,0,0,s,i,0,0,0,0,1,0,0,0,0,1),this}makeRotationAxis(t,i){const s=Math.cos(i),l=Math.sin(i),c=1-s,h=t.x,d=t.y,m=t.z,p=c*h,v=c*d;return this.set(p*h+s,p*d-l*m,p*m+l*d,0,p*d+l*m,v*d+s,v*m-l*h,0,p*m-l*d,v*m+l*h,c*m*m+s,0,0,0,0,1),this}makeScale(t,i,s){return this.set(t,0,0,0,0,i,0,0,0,0,s,0,0,0,0,1),this}makeShear(t,i,s,l,c,h){return this.set(1,s,c,0,t,1,h,0,i,l,1,0,0,0,0,1),this}compose(t,i,s){const l=this.elements,c=i._x,h=i._y,d=i._z,m=i._w,p=c+c,v=h+h,g=d+d,x=c*p,S=c*v,b=c*g,R=h*v,M=h*g,_=d*g,L=m*p,U=m*v,T=m*g,P=s.x,B=s.y,F=s.z;return l[0]=(1-(R+_))*P,l[1]=(S+T)*P,l[2]=(b-U)*P,l[3]=0,l[4]=(S-T)*B,l[5]=(1-(x+_))*B,l[6]=(M+L)*B,l[7]=0,l[8]=(b+U)*F,l[9]=(M-L)*F,l[10]=(1-(x+R))*F,l[11]=0,l[12]=t.x,l[13]=t.y,l[14]=t.z,l[15]=1,this}decompose(t,i,s){const l=this.elements;let c=xr.set(l[0],l[1],l[2]).length();const h=xr.set(l[4],l[5],l[6]).length(),d=xr.set(l[8],l[9],l[10]).length();this.determinant()<0&&(c=-c),t.x=l[12],t.y=l[13],t.z=l[14],Ti.copy(this);const p=1/c,v=1/h,g=1/d;return Ti.elements[0]*=p,Ti.elements[1]*=p,Ti.elements[2]*=p,Ti.elements[4]*=v,Ti.elements[5]*=v,Ti.elements[6]*=v,Ti.elements[8]*=g,Ti.elements[9]*=g,Ti.elements[10]*=g,i.setFromRotationMatrix(Ti),s.x=c,s.y=h,s.z=d,this}makePerspective(t,i,s,l,c,h,d=Bi,m=!1){const p=this.elements,v=2*c/(i-t),g=2*c/(s-l),x=(i+t)/(i-t),S=(s+l)/(s-l);let b,R;if(m)b=c/(h-c),R=h*c/(h-c);else if(d===Bi)b=-(h+c)/(h-c),R=-2*h*c/(h-c);else if(d===iu)b=-h/(h-c),R=-h*c/(h-c);else throw new Error("THREE.Matrix4.makePerspective(): Invalid coordinate system: "+d);return p[0]=v,p[4]=0,p[8]=x,p[12]=0,p[1]=0,p[5]=g,p[9]=S,p[13]=0,p[2]=0,p[6]=0,p[10]=b,p[14]=R,p[3]=0,p[7]=0,p[11]=-1,p[15]=0,this}makeOrthographic(t,i,s,l,c,h,d=Bi,m=!1){const p=this.elements,v=2/(i-t),g=2/(s-l),x=-(i+t)/(i-t),S=-(s+l)/(s-l);let b,R;if(m)b=1/(h-c),R=h/(h-c);else if(d===Bi)b=-2/(h-c),R=-(h+c)/(h-c);else if(d===iu)b=-1/(h-c),R=-c/(h-c);else throw new Error("THREE.Matrix4.makeOrthographic(): Invalid coordinate system: "+d);return p[0]=v,p[4]=0,p[8]=0,p[12]=x,p[1]=0,p[5]=g,p[9]=0,p[13]=S,p[2]=0,p[6]=0,p[10]=b,p[14]=R,p[3]=0,p[7]=0,p[11]=0,p[15]=1,this}equals(t){const i=this.elements,s=t.elements;for(let l=0;l<16;l++)if(i[l]!==s[l])return!1;return!0}fromArray(t,i=0){for(let s=0;s<16;s++)this.elements[s]=t[s+i];return this}toArray(t=[],i=0){const s=this.elements;return t[i]=s[0],t[i+1]=s[1],t[i+2]=s[2],t[i+3]=s[3],t[i+4]=s[4],t[i+5]=s[5],t[i+6]=s[6],t[i+7]=s[7],t[i+8]=s[8],t[i+9]=s[9],t[i+10]=s[10],t[i+11]=s[11],t[i+12]=s[12],t[i+13]=s[13],t[i+14]=s[14],t[i+15]=s[15],t}}const xr=new X,Ti=new qe,ZM=new X(0,0,0),KM=new X(1,1,1),Ga=new X,Ac=new X,ni=new X,E0=new qe,T0=new ws;class Fi{constructor(t=0,i=0,s=0,l=Fi.DEFAULT_ORDER){this.isEuler=!0,this._x=t,this._y=i,this._z=s,this._order=l}get x(){return this._x}set x(t){this._x=t,this._onChangeCallback()}get y(){return this._y}set y(t){this._y=t,this._onChangeCallback()}get z(){return this._z}set z(t){this._z=t,this._onChangeCallback()}get order(){return this._order}set order(t){this._order=t,this._onChangeCallback()}set(t,i,s,l=this._order){return this._x=t,this._y=i,this._z=s,this._order=l,this._onChangeCallback(),this}clone(){return new this.constructor(this._x,this._y,this._z,this._order)}copy(t){return this._x=t._x,this._y=t._y,this._z=t._z,this._order=t._order,this._onChangeCallback(),this}setFromRotationMatrix(t,i=this._order,s=!0){const l=t.elements,c=l[0],h=l[4],d=l[8],m=l[1],p=l[5],v=l[9],g=l[2],x=l[6],S=l[10];switch(i){case"XYZ":this._y=Math.asin(ye(d,-1,1)),Math.abs(d)<.9999999?(this._x=Math.atan2(-v,S),this._z=Math.atan2(-h,c)):(this._x=Math.atan2(x,p),this._z=0);break;case"YXZ":this._x=Math.asin(-ye(v,-1,1)),Math.abs(v)<.9999999?(this._y=Math.atan2(d,S),this._z=Math.atan2(m,p)):(this._y=Math.atan2(-g,c),this._z=0);break;case"ZXY":this._x=Math.asin(ye(x,-1,1)),Math.abs(x)<.9999999?(this._y=Math.atan2(-g,S),this._z=Math.atan2(-h,p)):(this._y=0,this._z=Math.atan2(m,c));break;case"ZYX":this._y=Math.asin(-ye(g,-1,1)),Math.abs(g)<.9999999?(this._x=Math.atan2(x,S),this._z=Math.atan2(m,c)):(this._x=0,this._z=Math.atan2(-h,p));break;case"YZX":this._z=Math.asin(ye(m,-1,1)),Math.abs(m)<.9999999?(this._x=Math.atan2(-v,p),this._y=Math.atan2(-g,c)):(this._x=0,this._y=Math.atan2(d,S));break;case"XZY":this._z=Math.asin(-ye(h,-1,1)),Math.abs(h)<.9999999?(this._x=Math.atan2(x,p),this._y=Math.atan2(d,c)):(this._x=Math.atan2(-v,S),this._y=0);break;default:console.warn("THREE.Euler: .setFromRotationMatrix() encountered an unknown order: "+i)}return this._order=i,s===!0&&this._onChangeCallback(),this}setFromQuaternion(t,i,s){return E0.makeRotationFromQuaternion(t),this.setFromRotationMatrix(E0,i,s)}setFromVector3(t,i=this._order){return this.set(t.x,t.y,t.z,i)}reorder(t){return T0.setFromEuler(this),this.setFromQuaternion(T0,t)}equals(t){return t._x===this._x&&t._y===this._y&&t._z===this._z&&t._order===this._order}fromArray(t){return this._x=t[0],this._y=t[1],this._z=t[2],t[3]!==void 0&&(this._order=t[3]),this._onChangeCallback(),this}toArray(t=[],i=0){return t[i]=this._x,t[i+1]=this._y,t[i+2]=this._z,t[i+3]=this._order,t}_onChange(t){return this._onChangeCallback=t,this}_onChangeCallback(){}*[Symbol.iterator](){yield this._x,yield this._y,yield this._z,yield this._order}}Fi.DEFAULT_ORDER="XYZ";class hp{constructor(){this.mask=1}set(t){this.mask=(1<<t|0)>>>0}enable(t){this.mask|=1<<t|0}enableAll(){this.mask=-1}toggle(t){this.mask^=1<<t|0}disable(t){this.mask&=~(1<<t|0)}disableAll(){this.mask=0}test(t){return(this.mask&t.mask)!==0}isEnabled(t){return(this.mask&(1<<t|0))!==0}}let QM=0;const b0=new X,Sr=new ws,ra=new qe,Rc=new X,ko=new X,JM=new X,$M=new ws,A0=new X(1,0,0),R0=new X(0,1,0),C0=new X(0,0,1),w0={type:"added"},tE={type:"removed"},Mr={type:"childadded",child:null},Fh={type:"childremoved",child:null};class Mn extends Us{constructor(){super(),this.isObject3D=!0,Object.defineProperty(this,"id",{value:QM++}),this.uuid=fa(),this.name="",this.type="Object3D",this.parent=null,this.children=[],this.up=Mn.DEFAULT_UP.clone();const t=new X,i=new Fi,s=new ws,l=new X(1,1,1);function c(){s.setFromEuler(i,!1)}function h(){i.setFromQuaternion(s,void 0,!1)}i._onChange(c),s._onChange(h),Object.defineProperties(this,{position:{configurable:!0,enumerable:!0,value:t},rotation:{configurable:!0,enumerable:!0,value:i},quaternion:{configurable:!0,enumerable:!0,value:s},scale:{configurable:!0,enumerable:!0,value:l},modelViewMatrix:{value:new qe},normalMatrix:{value:new me}}),this.matrix=new qe,this.matrixWorld=new qe,this.matrixAutoUpdate=Mn.DEFAULT_MATRIX_AUTO_UPDATE,this.matrixWorldAutoUpdate=Mn.DEFAULT_MATRIX_WORLD_AUTO_UPDATE,this.matrixWorldNeedsUpdate=!1,this.layers=new hp,this.visible=!0,this.castShadow=!1,this.receiveShadow=!1,this.frustumCulled=!0,this.renderOrder=0,this.animations=[],this.customDepthMaterial=void 0,this.customDistanceMaterial=void 0,this.userData={}}onBeforeShadow(){}onAfterShadow(){}onBeforeRender(){}onAfterRender(){}applyMatrix4(t){this.matrixAutoUpdate&&this.updateMatrix(),this.matrix.premultiply(t),this.matrix.decompose(this.position,this.quaternion,this.scale)}applyQuaternion(t){return this.quaternion.premultiply(t),this}setRotationFromAxisAngle(t,i){this.quaternion.setFromAxisAngle(t,i)}setRotationFromEuler(t){this.quaternion.setFromEuler(t,!0)}setRotationFromMatrix(t){this.quaternion.setFromRotationMatrix(t)}setRotationFromQuaternion(t){this.quaternion.copy(t)}rotateOnAxis(t,i){return Sr.setFromAxisAngle(t,i),this.quaternion.multiply(Sr),this}rotateOnWorldAxis(t,i){return Sr.setFromAxisAngle(t,i),this.quaternion.premultiply(Sr),this}rotateX(t){return this.rotateOnAxis(A0,t)}rotateY(t){return this.rotateOnAxis(R0,t)}rotateZ(t){return this.rotateOnAxis(C0,t)}translateOnAxis(t,i){return b0.copy(t).applyQuaternion(this.quaternion),this.position.add(b0.multiplyScalar(i)),this}translateX(t){return this.translateOnAxis(A0,t)}translateY(t){return this.translateOnAxis(R0,t)}translateZ(t){return this.translateOnAxis(C0,t)}localToWorld(t){return this.updateWorldMatrix(!0,!1),t.applyMatrix4(this.matrixWorld)}worldToLocal(t){return this.updateWorldMatrix(!0,!1),t.applyMatrix4(ra.copy(this.matrixWorld).invert())}lookAt(t,i,s){t.isVector3?Rc.copy(t):Rc.set(t,i,s);const l=this.parent;this.updateWorldMatrix(!0,!1),ko.setFromMatrixPosition(this.matrixWorld),this.isCamera||this.isLight?ra.lookAt(ko,Rc,this.up):ra.lookAt(Rc,ko,this.up),this.quaternion.setFromRotationMatrix(ra),l&&(ra.extractRotation(l.matrixWorld),Sr.setFromRotationMatrix(ra),this.quaternion.premultiply(Sr.invert()))}add(t){if(arguments.length>1){for(let i=0;i<arguments.length;i++)this.add(arguments[i]);return this}return t===this?(console.error("THREE.Object3D.add: object can't be added as a child of itself.",t),this):(t&&t.isObject3D?(t.removeFromParent(),t.parent=this,this.children.push(t),t.dispatchEvent(w0),Mr.child=t,this.dispatchEvent(Mr),Mr.child=null):console.error("THREE.Object3D.add: object not an instance of THREE.Object3D.",t),this)}remove(t){if(arguments.length>1){for(let s=0;s<arguments.length;s++)this.remove(arguments[s]);return this}const i=this.children.indexOf(t);return i!==-1&&(t.parent=null,this.children.splice(i,1),t.dispatchEvent(tE),Fh.child=t,this.dispatchEvent(Fh),Fh.child=null),this}removeFromParent(){const t=this.parent;return t!==null&&t.remove(this),this}clear(){return this.remove(...this.children)}attach(t){return this.updateWorldMatrix(!0,!1),ra.copy(this.matrixWorld).invert(),t.parent!==null&&(t.parent.updateWorldMatrix(!0,!1),ra.multiply(t.parent.matrixWorld)),t.applyMatrix4(ra),t.removeFromParent(),t.parent=this,this.children.push(t),t.updateWorldMatrix(!1,!0),t.dispatchEvent(w0),Mr.child=t,this.dispatchEvent(Mr),Mr.child=null,this}getObjectById(t){return this.getObjectByProperty("id",t)}getObjectByName(t){return this.getObjectByProperty("name",t)}getObjectByProperty(t,i){if(this[t]===i)return this;for(let s=0,l=this.children.length;s<l;s++){const h=this.children[s].getObjectByProperty(t,i);if(h!==void 0)return h}}getObjectsByProperty(t,i,s=[]){this[t]===i&&s.push(this);const l=this.children;for(let c=0,h=l.length;c<h;c++)l[c].getObjectsByProperty(t,i,s);return s}getWorldPosition(t){return this.updateWorldMatrix(!0,!1),t.setFromMatrixPosition(this.matrixWorld)}getWorldQuaternion(t){return this.updateWorldMatrix(!0,!1),this.matrixWorld.decompose(ko,t,JM),t}getWorldScale(t){return this.updateWorldMatrix(!0,!1),this.matrixWorld.decompose(ko,$M,t),t}getWorldDirection(t){this.updateWorldMatrix(!0,!1);const i=this.matrixWorld.elements;return t.set(i[8],i[9],i[10]).normalize()}raycast(){}traverse(t){t(this);const i=this.children;for(let s=0,l=i.length;s<l;s++)i[s].traverse(t)}traverseVisible(t){if(this.visible===!1)return;t(this);const i=this.children;for(let s=0,l=i.length;s<l;s++)i[s].traverseVisible(t)}traverseAncestors(t){const i=this.parent;i!==null&&(t(i),i.traverseAncestors(t))}updateMatrix(){this.matrix.compose(this.position,this.quaternion,this.scale),this.matrixWorldNeedsUpdate=!0}updateMatrixWorld(t){this.matrixAutoUpdate&&this.updateMatrix(),(this.matrixWorldNeedsUpdate||t)&&(this.matrixWorldAutoUpdate===!0&&(this.parent===null?this.matrixWorld.copy(this.matrix):this.matrixWorld.multiplyMatrices(this.parent.matrixWorld,this.matrix)),this.matrixWorldNeedsUpdate=!1,t=!0);const i=this.children;for(let s=0,l=i.length;s<l;s++)i[s].updateMatrixWorld(t)}updateWorldMatrix(t,i){const s=this.parent;if(t===!0&&s!==null&&s.updateWorldMatrix(!0,!1),this.matrixAutoUpdate&&this.updateMatrix(),this.matrixWorldAutoUpdate===!0&&(this.parent===null?this.matrixWorld.copy(this.matrix):this.matrixWorld.multiplyMatrices(this.parent.matrixWorld,this.matrix)),i===!0){const l=this.children;for(let c=0,h=l.length;c<h;c++)l[c].updateWorldMatrix(!1,!0)}}toJSON(t){const i=t===void 0||typeof t=="string",s={};i&&(t={geometries:{},materials:{},textures:{},images:{},shapes:{},skeletons:{},animations:{},nodes:{}},s.metadata={version:4.7,type:"Object",generator:"Object3D.toJSON"});const l={};l.uuid=this.uuid,l.type=this.type,this.name!==""&&(l.name=this.name),this.castShadow===!0&&(l.castShadow=!0),this.receiveShadow===!0&&(l.receiveShadow=!0),this.visible===!1&&(l.visible=!1),this.frustumCulled===!1&&(l.frustumCulled=!1),this.renderOrder!==0&&(l.renderOrder=this.renderOrder),Object.keys(this.userData).length>0&&(l.userData=this.userData),l.layers=this.layers.mask,l.matrix=this.matrix.toArray(),l.up=this.up.toArray(),this.matrixAutoUpdate===!1&&(l.matrixAutoUpdate=!1),this.isInstancedMesh&&(l.type="InstancedMesh",l.count=this.count,l.instanceMatrix=this.instanceMatrix.toJSON(),this.instanceColor!==null&&(l.instanceColor=this.instanceColor.toJSON())),this.isBatchedMesh&&(l.type="BatchedMesh",l.perObjectFrustumCulled=this.perObjectFrustumCulled,l.sortObjects=this.sortObjects,l.drawRanges=this._drawRanges,l.reservedRanges=this._reservedRanges,l.geometryInfo=this._geometryInfo.map(d=>({...d,boundingBox:d.boundingBox?d.boundingBox.toJSON():void 0,boundingSphere:d.boundingSphere?d.boundingSphere.toJSON():void 0})),l.instanceInfo=this._instanceInfo.map(d=>({...d})),l.availableInstanceIds=this._availableInstanceIds.slice(),l.availableGeometryIds=this._availableGeometryIds.slice(),l.nextIndexStart=this._nextIndexStart,l.nextVertexStart=this._nextVertexStart,l.geometryCount=this._geometryCount,l.maxInstanceCount=this._maxInstanceCount,l.maxVertexCount=this._maxVertexCount,l.maxIndexCount=this._maxIndexCount,l.geometryInitialized=this._geometryInitialized,l.matricesTexture=this._matricesTexture.toJSON(t),l.indirectTexture=this._indirectTexture.toJSON(t),this._colorsTexture!==null&&(l.colorsTexture=this._colorsTexture.toJSON(t)),this.boundingSphere!==null&&(l.boundingSphere=this.boundingSphere.toJSON()),this.boundingBox!==null&&(l.boundingBox=this.boundingBox.toJSON()));function c(d,m){return d[m.uuid]===void 0&&(d[m.uuid]=m.toJSON(t)),m.uuid}if(this.isScene)this.background&&(this.background.isColor?l.background=this.background.toJSON():this.background.isTexture&&(l.background=this.background.toJSON(t).uuid)),this.environment&&this.environment.isTexture&&this.environment.isRenderTargetTexture!==!0&&(l.environment=this.environment.toJSON(t).uuid);else if(this.isMesh||this.isLine||this.isPoints){l.geometry=c(t.geometries,this.geometry);const d=this.geometry.parameters;if(d!==void 0&&d.shapes!==void 0){const m=d.shapes;if(Array.isArray(m))for(let p=0,v=m.length;p<v;p++){const g=m[p];c(t.shapes,g)}else c(t.shapes,m)}}if(this.isSkinnedMesh&&(l.bindMode=this.bindMode,l.bindMatrix=this.bindMatrix.toArray(),this.skeleton!==void 0&&(c(t.skeletons,this.skeleton),l.skeleton=this.skeleton.uuid)),this.material!==void 0)if(Array.isArray(this.material)){const d=[];for(let m=0,p=this.material.length;m<p;m++)d.push(c(t.materials,this.material[m]));l.material=d}else l.material=c(t.materials,this.material);if(this.children.length>0){l.children=[];for(let d=0;d<this.children.length;d++)l.children.push(this.children[d].toJSON(t).object)}if(this.animations.length>0){l.animations=[];for(let d=0;d<this.animations.length;d++){const m=this.animations[d];l.animations.push(c(t.animations,m))}}if(i){const d=h(t.geometries),m=h(t.materials),p=h(t.textures),v=h(t.images),g=h(t.shapes),x=h(t.skeletons),S=h(t.animations),b=h(t.nodes);d.length>0&&(s.geometries=d),m.length>0&&(s.materials=m),p.length>0&&(s.textures=p),v.length>0&&(s.images=v),g.length>0&&(s.shapes=g),x.length>0&&(s.skeletons=x),S.length>0&&(s.animations=S),b.length>0&&(s.nodes=b)}return s.object=l,s;function h(d){const m=[];for(const p in d){const v=d[p];delete v.metadata,m.push(v)}return m}}clone(t){return new this.constructor().copy(this,t)}copy(t,i=!0){if(this.name=t.name,this.up.copy(t.up),this.position.copy(t.position),this.rotation.order=t.rotation.order,this.quaternion.copy(t.quaternion),this.scale.copy(t.scale),this.matrix.copy(t.matrix),this.matrixWorld.copy(t.matrixWorld),this.matrixAutoUpdate=t.matrixAutoUpdate,this.matrixWorldAutoUpdate=t.matrixWorldAutoUpdate,this.matrixWorldNeedsUpdate=t.matrixWorldNeedsUpdate,this.layers.mask=t.layers.mask,this.visible=t.visible,this.castShadow=t.castShadow,this.receiveShadow=t.receiveShadow,this.frustumCulled=t.frustumCulled,this.renderOrder=t.renderOrder,this.animations=t.animations.slice(),this.userData=JSON.parse(JSON.stringify(t.userData)),i===!0)for(let s=0;s<t.children.length;s++){const l=t.children[s];this.add(l.clone())}return this}}Mn.DEFAULT_UP=new X(0,1,0);Mn.DEFAULT_MATRIX_AUTO_UPDATE=!0;Mn.DEFAULT_MATRIX_WORLD_AUTO_UPDATE=!0;const bi=new X,oa=new X,Hh=new X,la=new X,Er=new X,Tr=new X,D0=new X,Gh=new X,Vh=new X,kh=new X,Xh=new Ye,Wh=new Ye,jh=new Ye;class _i{constructor(t=new X,i=new X,s=new X){this.a=t,this.b=i,this.c=s}static getNormal(t,i,s,l){l.subVectors(s,i),bi.subVectors(t,i),l.cross(bi);const c=l.lengthSq();return c>0?l.multiplyScalar(1/Math.sqrt(c)):l.set(0,0,0)}static getBarycoord(t,i,s,l,c){bi.subVectors(l,i),oa.subVectors(s,i),Hh.subVectors(t,i);const h=bi.dot(bi),d=bi.dot(oa),m=bi.dot(Hh),p=oa.dot(oa),v=oa.dot(Hh),g=h*p-d*d;if(g===0)return c.set(0,0,0),null;const x=1/g,S=(p*m-d*v)*x,b=(h*v-d*m)*x;return c.set(1-S-b,b,S)}static containsPoint(t,i,s,l){return this.getBarycoord(t,i,s,l,la)===null?!1:la.x>=0&&la.y>=0&&la.x+la.y<=1}static getInterpolation(t,i,s,l,c,h,d,m){return this.getBarycoord(t,i,s,l,la)===null?(m.x=0,m.y=0,"z"in m&&(m.z=0),"w"in m&&(m.w=0),null):(m.setScalar(0),m.addScaledVector(c,la.x),m.addScaledVector(h,la.y),m.addScaledVector(d,la.z),m)}static getInterpolatedAttribute(t,i,s,l,c,h){return Xh.setScalar(0),Wh.setScalar(0),jh.setScalar(0),Xh.fromBufferAttribute(t,i),Wh.fromBufferAttribute(t,s),jh.fromBufferAttribute(t,l),h.setScalar(0),h.addScaledVector(Xh,c.x),h.addScaledVector(Wh,c.y),h.addScaledVector(jh,c.z),h}static isFrontFacing(t,i,s,l){return bi.subVectors(s,i),oa.subVectors(t,i),bi.cross(oa).dot(l)<0}set(t,i,s){return this.a.copy(t),this.b.copy(i),this.c.copy(s),this}setFromPointsAndIndices(t,i,s,l){return this.a.copy(t[i]),this.b.copy(t[s]),this.c.copy(t[l]),this}setFromAttributeAndIndices(t,i,s,l){return this.a.fromBufferAttribute(t,i),this.b.fromBufferAttribute(t,s),this.c.fromBufferAttribute(t,l),this}clone(){return new this.constructor().copy(this)}copy(t){return this.a.copy(t.a),this.b.copy(t.b),this.c.copy(t.c),this}getArea(){return bi.subVectors(this.c,this.b),oa.subVectors(this.a,this.b),bi.cross(oa).length()*.5}getMidpoint(t){return t.addVectors(this.a,this.b).add(this.c).multiplyScalar(1/3)}getNormal(t){return _i.getNormal(this.a,this.b,this.c,t)}getPlane(t){return t.setFromCoplanarPoints(this.a,this.b,this.c)}getBarycoord(t,i){return _i.getBarycoord(t,this.a,this.b,this.c,i)}getInterpolation(t,i,s,l,c){return _i.getInterpolation(t,this.a,this.b,this.c,i,s,l,c)}containsPoint(t){return _i.containsPoint(t,this.a,this.b,this.c)}isFrontFacing(t){return _i.isFrontFacing(this.a,this.b,this.c,t)}intersectsBox(t){return t.intersectsTriangle(this)}closestPointToPoint(t,i){const s=this.a,l=this.b,c=this.c;let h,d;Er.subVectors(l,s),Tr.subVectors(c,s),Gh.subVectors(t,s);const m=Er.dot(Gh),p=Tr.dot(Gh);if(m<=0&&p<=0)return i.copy(s);Vh.subVectors(t,l);const v=Er.dot(Vh),g=Tr.dot(Vh);if(v>=0&&g<=v)return i.copy(l);const x=m*g-v*p;if(x<=0&&m>=0&&v<=0)return h=m/(m-v),i.copy(s).addScaledVector(Er,h);kh.subVectors(t,c);const S=Er.dot(kh),b=Tr.dot(kh);if(b>=0&&S<=b)return i.copy(c);const R=S*p-m*b;if(R<=0&&p>=0&&b<=0)return d=p/(p-b),i.copy(s).addScaledVector(Tr,d);const M=v*b-S*g;if(M<=0&&g-v>=0&&S-b>=0)return D0.subVectors(c,l),d=(g-v)/(g-v+(S-b)),i.copy(l).addScaledVector(D0,d);const _=1/(M+R+x);return h=R*_,d=x*_,i.copy(s).addScaledVector(Er,h).addScaledVector(Tr,d)}equals(t){return t.a.equals(this.a)&&t.b.equals(this.b)&&t.c.equals(this.c)}}const Kv={aliceblue:15792383,antiquewhite:16444375,aqua:65535,aquamarine:8388564,azure:15794175,beige:16119260,bisque:16770244,black:0,blanchedalmond:16772045,blue:255,blueviolet:9055202,brown:10824234,burlywood:14596231,cadetblue:6266528,chartreuse:8388352,chocolate:13789470,coral:16744272,cornflowerblue:6591981,cornsilk:16775388,crimson:14423100,cyan:65535,darkblue:139,darkcyan:35723,darkgoldenrod:12092939,darkgray:11119017,darkgreen:25600,darkgrey:11119017,darkkhaki:12433259,darkmagenta:9109643,darkolivegreen:5597999,darkorange:16747520,darkorchid:10040012,darkred:9109504,darksalmon:15308410,darkseagreen:9419919,darkslateblue:4734347,darkslategray:3100495,darkslategrey:3100495,darkturquoise:52945,darkviolet:9699539,deeppink:16716947,deepskyblue:49151,dimgray:6908265,dimgrey:6908265,dodgerblue:2003199,firebrick:11674146,floralwhite:16775920,forestgreen:2263842,fuchsia:16711935,gainsboro:14474460,ghostwhite:16316671,gold:16766720,goldenrod:14329120,gray:8421504,green:32768,greenyellow:11403055,grey:8421504,honeydew:15794160,hotpink:16738740,indianred:13458524,indigo:4915330,ivory:16777200,khaki:15787660,lavender:15132410,lavenderblush:16773365,lawngreen:8190976,lemonchiffon:16775885,lightblue:11393254,lightcoral:15761536,lightcyan:14745599,lightgoldenrodyellow:16448210,lightgray:13882323,lightgreen:9498256,lightgrey:13882323,lightpink:16758465,lightsalmon:16752762,lightseagreen:2142890,lightskyblue:8900346,lightslategray:7833753,lightslategrey:7833753,lightsteelblue:11584734,lightyellow:16777184,lime:65280,limegreen:3329330,linen:16445670,magenta:16711935,maroon:8388608,mediumaquamarine:6737322,mediumblue:205,mediumorchid:12211667,mediumpurple:9662683,mediumseagreen:3978097,mediumslateblue:8087790,mediumspringgreen:64154,mediumturquoise:4772300,mediumvioletred:13047173,midnightblue:1644912,mintcream:16121850,mistyrose:16770273,moccasin:16770229,navajowhite:16768685,navy:128,oldlace:16643558,olive:8421376,olivedrab:7048739,orange:16753920,orangered:16729344,orchid:14315734,palegoldenrod:15657130,palegreen:10025880,paleturquoise:11529966,palevioletred:14381203,papayawhip:16773077,peachpuff:16767673,peru:13468991,pink:16761035,plum:14524637,powderblue:11591910,purple:8388736,rebeccapurple:6697881,red:16711680,rosybrown:12357519,royalblue:4286945,saddlebrown:9127187,salmon:16416882,sandybrown:16032864,seagreen:3050327,seashell:16774638,sienna:10506797,silver:12632256,skyblue:8900331,slateblue:6970061,slategray:7372944,slategrey:7372944,snow:16775930,springgreen:65407,steelblue:4620980,tan:13808780,teal:32896,thistle:14204888,tomato:16737095,turquoise:4251856,violet:15631086,wheat:16113331,white:16777215,whitesmoke:16119285,yellow:16776960,yellowgreen:10145074},Va={h:0,s:0,l:0},Cc={h:0,s:0,l:0};function Yh(r,t,i){return i<0&&(i+=1),i>1&&(i-=1),i<1/6?r+(t-r)*6*i:i<1/2?t:i<2/3?r+(t-r)*6*(2/3-i):r}class Me{constructor(t,i,s){return this.isColor=!0,this.r=1,this.g=1,this.b=1,this.set(t,i,s)}set(t,i,s){if(i===void 0&&s===void 0){const l=t;l&&l.isColor?this.copy(l):typeof l=="number"?this.setHex(l):typeof l=="string"&&this.setStyle(l)}else this.setRGB(t,i,s);return this}setScalar(t){return this.r=t,this.g=t,this.b=t,this}setHex(t,i=jn){return t=Math.floor(t),this.r=(t>>16&255)/255,this.g=(t>>8&255)/255,this.b=(t&255)/255,Pe.colorSpaceToWorking(this,i),this}setRGB(t,i,s,l=Pe.workingColorSpace){return this.r=t,this.g=i,this.b=s,Pe.colorSpaceToWorking(this,l),this}setHSL(t,i,s,l=Pe.workingColorSpace){if(t=up(t,1),i=ye(i,0,1),s=ye(s,0,1),i===0)this.r=this.g=this.b=s;else{const c=s<=.5?s*(1+i):s+i-s*i,h=2*s-c;this.r=Yh(h,c,t+1/3),this.g=Yh(h,c,t),this.b=Yh(h,c,t-1/3)}return Pe.colorSpaceToWorking(this,l),this}setStyle(t,i=jn){function s(c){c!==void 0&&parseFloat(c)<1&&console.warn("THREE.Color: Alpha component of "+t+" will be ignored.")}let l;if(l=/^(\w+)\(([^\)]*)\)/.exec(t)){let c;const h=l[1],d=l[2];switch(h){case"rgb":case"rgba":if(c=/^\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(?:,\s*(\d*\.?\d+)\s*)?$/.exec(d))return s(c[4]),this.setRGB(Math.min(255,parseInt(c[1],10))/255,Math.min(255,parseInt(c[2],10))/255,Math.min(255,parseInt(c[3],10))/255,i);if(c=/^\s*(\d+)\%\s*,\s*(\d+)\%\s*,\s*(\d+)\%\s*(?:,\s*(\d*\.?\d+)\s*)?$/.exec(d))return s(c[4]),this.setRGB(Math.min(100,parseInt(c[1],10))/100,Math.min(100,parseInt(c[2],10))/100,Math.min(100,parseInt(c[3],10))/100,i);break;case"hsl":case"hsla":if(c=/^\s*(\d*\.?\d+)\s*,\s*(\d*\.?\d+)\%\s*,\s*(\d*\.?\d+)\%\s*(?:,\s*(\d*\.?\d+)\s*)?$/.exec(d))return s(c[4]),this.setHSL(parseFloat(c[1])/360,parseFloat(c[2])/100,parseFloat(c[3])/100,i);break;default:console.warn("THREE.Color: Unknown color model "+t)}}else if(l=/^\#([A-Fa-f\d]+)$/.exec(t)){const c=l[1],h=c.length;if(h===3)return this.setRGB(parseInt(c.charAt(0),16)/15,parseInt(c.charAt(1),16)/15,parseInt(c.charAt(2),16)/15,i);if(h===6)return this.setHex(parseInt(c,16),i);console.warn("THREE.Color: Invalid hex color "+t)}else if(t&&t.length>0)return this.setColorName(t,i);return this}setColorName(t,i=jn){const s=Kv[t.toLowerCase()];return s!==void 0?this.setHex(s,i):console.warn("THREE.Color: Unknown color "+t),this}clone(){return new this.constructor(this.r,this.g,this.b)}copy(t){return this.r=t.r,this.g=t.g,this.b=t.b,this}copySRGBToLinear(t){return this.r=ha(t.r),this.g=ha(t.g),this.b=ha(t.b),this}copyLinearToSRGB(t){return this.r=Fr(t.r),this.g=Fr(t.g),this.b=Fr(t.b),this}convertSRGBToLinear(){return this.copySRGBToLinear(this),this}convertLinearToSRGB(){return this.copyLinearToSRGB(this),this}getHex(t=jn){return Pe.workingToColorSpace(Nn.copy(this),t),Math.round(ye(Nn.r*255,0,255))*65536+Math.round(ye(Nn.g*255,0,255))*256+Math.round(ye(Nn.b*255,0,255))}getHexString(t=jn){return("000000"+this.getHex(t).toString(16)).slice(-6)}getHSL(t,i=Pe.workingColorSpace){Pe.workingToColorSpace(Nn.copy(this),i);const s=Nn.r,l=Nn.g,c=Nn.b,h=Math.max(s,l,c),d=Math.min(s,l,c);let m,p;const v=(d+h)/2;if(d===h)m=0,p=0;else{const g=h-d;switch(p=v<=.5?g/(h+d):g/(2-h-d),h){case s:m=(l-c)/g+(l<c?6:0);break;case l:m=(c-s)/g+2;break;case c:m=(s-l)/g+4;break}m/=6}return t.h=m,t.s=p,t.l=v,t}getRGB(t,i=Pe.workingColorSpace){return Pe.workingToColorSpace(Nn.copy(this),i),t.r=Nn.r,t.g=Nn.g,t.b=Nn.b,t}getStyle(t=jn){Pe.workingToColorSpace(Nn.copy(this),t);const i=Nn.r,s=Nn.g,l=Nn.b;return t!==jn?`color(${t} ${i.toFixed(3)} ${s.toFixed(3)} ${l.toFixed(3)})`:`rgb(${Math.round(i*255)},${Math.round(s*255)},${Math.round(l*255)})`}offsetHSL(t,i,s){return this.getHSL(Va),this.setHSL(Va.h+t,Va.s+i,Va.l+s)}add(t){return this.r+=t.r,this.g+=t.g,this.b+=t.b,this}addColors(t,i){return this.r=t.r+i.r,this.g=t.g+i.g,this.b=t.b+i.b,this}addScalar(t){return this.r+=t,this.g+=t,this.b+=t,this}sub(t){return this.r=Math.max(0,this.r-t.r),this.g=Math.max(0,this.g-t.g),this.b=Math.max(0,this.b-t.b),this}multiply(t){return this.r*=t.r,this.g*=t.g,this.b*=t.b,this}multiplyScalar(t){return this.r*=t,this.g*=t,this.b*=t,this}lerp(t,i){return this.r+=(t.r-this.r)*i,this.g+=(t.g-this.g)*i,this.b+=(t.b-this.b)*i,this}lerpColors(t,i,s){return this.r=t.r+(i.r-t.r)*s,this.g=t.g+(i.g-t.g)*s,this.b=t.b+(i.b-t.b)*s,this}lerpHSL(t,i){this.getHSL(Va),t.getHSL(Cc);const s=nl(Va.h,Cc.h,i),l=nl(Va.s,Cc.s,i),c=nl(Va.l,Cc.l,i);return this.setHSL(s,l,c),this}setFromVector3(t){return this.r=t.x,this.g=t.y,this.b=t.z,this}applyMatrix3(t){const i=this.r,s=this.g,l=this.b,c=t.elements;return this.r=c[0]*i+c[3]*s+c[6]*l,this.g=c[1]*i+c[4]*s+c[7]*l,this.b=c[2]*i+c[5]*s+c[8]*l,this}equals(t){return t.r===this.r&&t.g===this.g&&t.b===this.b}fromArray(t,i=0){return this.r=t[i],this.g=t[i+1],this.b=t[i+2],this}toArray(t=[],i=0){return t[i]=this.r,t[i+1]=this.g,t[i+2]=this.b,t}fromBufferAttribute(t,i){return this.r=t.getX(i),this.g=t.getY(i),this.b=t.getZ(i),this}toJSON(){return this.getHex()}*[Symbol.iterator](){yield this.r,yield this.g,yield this.b}}const Nn=new Me;Me.NAMES=Kv;let eE=0;class Ls extends Us{constructor(){super(),this.isMaterial=!0,Object.defineProperty(this,"id",{value:eE++}),this.uuid=fa(),this.name="",this.type="Material",this.blending=Ir,this.side=qa,this.vertexColors=!1,this.opacity=1,this.transparent=!1,this.alphaHash=!1,this.blendSrc=fd,this.blendDst=hd,this.blendEquation=Ts,this.blendSrcAlpha=null,this.blendDstAlpha=null,this.blendEquationAlpha=null,this.blendColor=new Me(0,0,0),this.blendAlpha=0,this.depthFunc=Hr,this.depthTest=!0,this.depthWrite=!0,this.stencilWriteMask=255,this.stencilFunc=g0,this.stencilRef=0,this.stencilFuncMask=255,this.stencilFail=mr,this.stencilZFail=mr,this.stencilZPass=mr,this.stencilWrite=!1,this.clippingPlanes=null,this.clipIntersection=!1,this.clipShadows=!1,this.shadowSide=null,this.colorWrite=!0,this.precision=null,this.polygonOffset=!1,this.polygonOffsetFactor=0,this.polygonOffsetUnits=0,this.dithering=!1,this.alphaToCoverage=!1,this.premultipliedAlpha=!1,this.forceSinglePass=!1,this.allowOverride=!0,this.visible=!0,this.toneMapped=!0,this.userData={},this.version=0,this._alphaTest=0}get alphaTest(){return this._alphaTest}set alphaTest(t){this._alphaTest>0!=t>0&&this.version++,this._alphaTest=t}onBeforeRender(){}onBeforeCompile(){}customProgramCacheKey(){return this.onBeforeCompile.toString()}setValues(t){if(t!==void 0)for(const i in t){const s=t[i];if(s===void 0){console.warn(`THREE.Material: parameter '${i}' has value of undefined.`);continue}const l=this[i];if(l===void 0){console.warn(`THREE.Material: '${i}' is not a property of THREE.${this.type}.`);continue}l&&l.isColor?l.set(s):l&&l.isVector3&&s&&s.isVector3?l.copy(s):this[i]=s}}toJSON(t){const i=t===void 0||typeof t=="string";i&&(t={textures:{},images:{}});const s={metadata:{version:4.7,type:"Material",generator:"Material.toJSON"}};s.uuid=this.uuid,s.type=this.type,this.name!==""&&(s.name=this.name),this.color&&this.color.isColor&&(s.color=this.color.getHex()),this.roughness!==void 0&&(s.roughness=this.roughness),this.metalness!==void 0&&(s.metalness=this.metalness),this.sheen!==void 0&&(s.sheen=this.sheen),this.sheenColor&&this.sheenColor.isColor&&(s.sheenColor=this.sheenColor.getHex()),this.sheenRoughness!==void 0&&(s.sheenRoughness=this.sheenRoughness),this.emissive&&this.emissive.isColor&&(s.emissive=this.emissive.getHex()),this.emissiveIntensity!==void 0&&this.emissiveIntensity!==1&&(s.emissiveIntensity=this.emissiveIntensity),this.specular&&this.specular.isColor&&(s.specular=this.specular.getHex()),this.specularIntensity!==void 0&&(s.specularIntensity=this.specularIntensity),this.specularColor&&this.specularColor.isColor&&(s.specularColor=this.specularColor.getHex()),this.shininess!==void 0&&(s.shininess=this.shininess),this.clearcoat!==void 0&&(s.clearcoat=this.clearcoat),this.clearcoatRoughness!==void 0&&(s.clearcoatRoughness=this.clearcoatRoughness),this.clearcoatMap&&this.clearcoatMap.isTexture&&(s.clearcoatMap=this.clearcoatMap.toJSON(t).uuid),this.clearcoatRoughnessMap&&this.clearcoatRoughnessMap.isTexture&&(s.clearcoatRoughnessMap=this.clearcoatRoughnessMap.toJSON(t).uuid),this.clearcoatNormalMap&&this.clearcoatNormalMap.isTexture&&(s.clearcoatNormalMap=this.clearcoatNormalMap.toJSON(t).uuid,s.clearcoatNormalScale=this.clearcoatNormalScale.toArray()),this.sheenColorMap&&this.sheenColorMap.isTexture&&(s.sheenColorMap=this.sheenColorMap.toJSON(t).uuid),this.sheenRoughnessMap&&this.sheenRoughnessMap.isTexture&&(s.sheenRoughnessMap=this.sheenRoughnessMap.toJSON(t).uuid),this.dispersion!==void 0&&(s.dispersion=this.dispersion),this.iridescence!==void 0&&(s.iridescence=this.iridescence),this.iridescenceIOR!==void 0&&(s.iridescenceIOR=this.iridescenceIOR),this.iridescenceThicknessRange!==void 0&&(s.iridescenceThicknessRange=this.iridescenceThicknessRange),this.iridescenceMap&&this.iridescenceMap.isTexture&&(s.iridescenceMap=this.iridescenceMap.toJSON(t).uuid),this.iridescenceThicknessMap&&this.iridescenceThicknessMap.isTexture&&(s.iridescenceThicknessMap=this.iridescenceThicknessMap.toJSON(t).uuid),this.anisotropy!==void 0&&(s.anisotropy=this.anisotropy),this.anisotropyRotation!==void 0&&(s.anisotropyRotation=this.anisotropyRotation),this.anisotropyMap&&this.anisotropyMap.isTexture&&(s.anisotropyMap=this.anisotropyMap.toJSON(t).uuid),this.map&&this.map.isTexture&&(s.map=this.map.toJSON(t).uuid),this.matcap&&this.matcap.isTexture&&(s.matcap=this.matcap.toJSON(t).uuid),this.alphaMap&&this.alphaMap.isTexture&&(s.alphaMap=this.alphaMap.toJSON(t).uuid),this.lightMap&&this.lightMap.isTexture&&(s.lightMap=this.lightMap.toJSON(t).uuid,s.lightMapIntensity=this.lightMapIntensity),this.aoMap&&this.aoMap.isTexture&&(s.aoMap=this.aoMap.toJSON(t).uuid,s.aoMapIntensity=this.aoMapIntensity),this.bumpMap&&this.bumpMap.isTexture&&(s.bumpMap=this.bumpMap.toJSON(t).uuid,s.bumpScale=this.bumpScale),this.normalMap&&this.normalMap.isTexture&&(s.normalMap=this.normalMap.toJSON(t).uuid,s.normalMapType=this.normalMapType,s.normalScale=this.normalScale.toArray()),this.displacementMap&&this.displacementMap.isTexture&&(s.displacementMap=this.displacementMap.toJSON(t).uuid,s.displacementScale=this.displacementScale,s.displacementBias=this.displacementBias),this.roughnessMap&&this.roughnessMap.isTexture&&(s.roughnessMap=this.roughnessMap.toJSON(t).uuid),this.metalnessMap&&this.metalnessMap.isTexture&&(s.metalnessMap=this.metalnessMap.toJSON(t).uuid),this.emissiveMap&&this.emissiveMap.isTexture&&(s.emissiveMap=this.emissiveMap.toJSON(t).uuid),this.specularMap&&this.specularMap.isTexture&&(s.specularMap=this.specularMap.toJSON(t).uuid),this.specularIntensityMap&&this.specularIntensityMap.isTexture&&(s.specularIntensityMap=this.specularIntensityMap.toJSON(t).uuid),this.specularColorMap&&this.specularColorMap.isTexture&&(s.specularColorMap=this.specularColorMap.toJSON(t).uuid),this.envMap&&this.envMap.isTexture&&(s.envMap=this.envMap.toJSON(t).uuid,this.combine!==void 0&&(s.combine=this.combine)),this.envMapRotation!==void 0&&(s.envMapRotation=this.envMapRotation.toArray()),this.envMapIntensity!==void 0&&(s.envMapIntensity=this.envMapIntensity),this.reflectivity!==void 0&&(s.reflectivity=this.reflectivity),this.refractionRatio!==void 0&&(s.refractionRatio=this.refractionRatio),this.gradientMap&&this.gradientMap.isTexture&&(s.gradientMap=this.gradientMap.toJSON(t).uuid),this.transmission!==void 0&&(s.transmission=this.transmission),this.transmissionMap&&this.transmissionMap.isTexture&&(s.transmissionMap=this.transmissionMap.toJSON(t).uuid),this.thickness!==void 0&&(s.thickness=this.thickness),this.thicknessMap&&this.thicknessMap.isTexture&&(s.thicknessMap=this.thicknessMap.toJSON(t).uuid),this.attenuationDistance!==void 0&&this.attenuationDistance!==1/0&&(s.attenuationDistance=this.attenuationDistance),this.attenuationColor!==void 0&&(s.attenuationColor=this.attenuationColor.getHex()),this.size!==void 0&&(s.size=this.size),this.shadowSide!==null&&(s.shadowSide=this.shadowSide),this.sizeAttenuation!==void 0&&(s.sizeAttenuation=this.sizeAttenuation),this.blending!==Ir&&(s.blending=this.blending),this.side!==qa&&(s.side=this.side),this.vertexColors===!0&&(s.vertexColors=!0),this.opacity<1&&(s.opacity=this.opacity),this.transparent===!0&&(s.transparent=!0),this.blendSrc!==fd&&(s.blendSrc=this.blendSrc),this.blendDst!==hd&&(s.blendDst=this.blendDst),this.blendEquation!==Ts&&(s.blendEquation=this.blendEquation),this.blendSrcAlpha!==null&&(s.blendSrcAlpha=this.blendSrcAlpha),this.blendDstAlpha!==null&&(s.blendDstAlpha=this.blendDstAlpha),this.blendEquationAlpha!==null&&(s.blendEquationAlpha=this.blendEquationAlpha),this.blendColor&&this.blendColor.isColor&&(s.blendColor=this.blendColor.getHex()),this.blendAlpha!==0&&(s.blendAlpha=this.blendAlpha),this.depthFunc!==Hr&&(s.depthFunc=this.depthFunc),this.depthTest===!1&&(s.depthTest=this.depthTest),this.depthWrite===!1&&(s.depthWrite=this.depthWrite),this.colorWrite===!1&&(s.colorWrite=this.colorWrite),this.stencilWriteMask!==255&&(s.stencilWriteMask=this.stencilWriteMask),this.stencilFunc!==g0&&(s.stencilFunc=this.stencilFunc),this.stencilRef!==0&&(s.stencilRef=this.stencilRef),this.stencilFuncMask!==255&&(s.stencilFuncMask=this.stencilFuncMask),this.stencilFail!==mr&&(s.stencilFail=this.stencilFail),this.stencilZFail!==mr&&(s.stencilZFail=this.stencilZFail),this.stencilZPass!==mr&&(s.stencilZPass=this.stencilZPass),this.stencilWrite===!0&&(s.stencilWrite=this.stencilWrite),this.rotation!==void 0&&this.rotation!==0&&(s.rotation=this.rotation),this.polygonOffset===!0&&(s.polygonOffset=!0),this.polygonOffsetFactor!==0&&(s.polygonOffsetFactor=this.polygonOffsetFactor),this.polygonOffsetUnits!==0&&(s.polygonOffsetUnits=this.polygonOffsetUnits),this.linewidth!==void 0&&this.linewidth!==1&&(s.linewidth=this.linewidth),this.dashSize!==void 0&&(s.dashSize=this.dashSize),this.gapSize!==void 0&&(s.gapSize=this.gapSize),this.scale!==void 0&&(s.scale=this.scale),this.dithering===!0&&(s.dithering=!0),this.alphaTest>0&&(s.alphaTest=this.alphaTest),this.alphaHash===!0&&(s.alphaHash=!0),this.alphaToCoverage===!0&&(s.alphaToCoverage=!0),this.premultipliedAlpha===!0&&(s.premultipliedAlpha=!0),this.forceSinglePass===!0&&(s.forceSinglePass=!0),this.wireframe===!0&&(s.wireframe=!0),this.wireframeLinewidth>1&&(s.wireframeLinewidth=this.wireframeLinewidth),this.wireframeLinecap!=="round"&&(s.wireframeLinecap=this.wireframeLinecap),this.wireframeLinejoin!=="round"&&(s.wireframeLinejoin=this.wireframeLinejoin),this.flatShading===!0&&(s.flatShading=!0),this.visible===!1&&(s.visible=!1),this.toneMapped===!1&&(s.toneMapped=!1),this.fog===!1&&(s.fog=!1),Object.keys(this.userData).length>0&&(s.userData=this.userData);function l(c){const h=[];for(const d in c){const m=c[d];delete m.metadata,h.push(m)}return h}if(i){const c=l(t.textures),h=l(t.images);c.length>0&&(s.textures=c),h.length>0&&(s.images=h)}return s}clone(){return new this.constructor().copy(this)}copy(t){this.name=t.name,this.blending=t.blending,this.side=t.side,this.vertexColors=t.vertexColors,this.opacity=t.opacity,this.transparent=t.transparent,this.blendSrc=t.blendSrc,this.blendDst=t.blendDst,this.blendEquation=t.blendEquation,this.blendSrcAlpha=t.blendSrcAlpha,this.blendDstAlpha=t.blendDstAlpha,this.blendEquationAlpha=t.blendEquationAlpha,this.blendColor.copy(t.blendColor),this.blendAlpha=t.blendAlpha,this.depthFunc=t.depthFunc,this.depthTest=t.depthTest,this.depthWrite=t.depthWrite,this.stencilWriteMask=t.stencilWriteMask,this.stencilFunc=t.stencilFunc,this.stencilRef=t.stencilRef,this.stencilFuncMask=t.stencilFuncMask,this.stencilFail=t.stencilFail,this.stencilZFail=t.stencilZFail,this.stencilZPass=t.stencilZPass,this.stencilWrite=t.stencilWrite;const i=t.clippingPlanes;let s=null;if(i!==null){const l=i.length;s=new Array(l);for(let c=0;c!==l;++c)s[c]=i[c].clone()}return this.clippingPlanes=s,this.clipIntersection=t.clipIntersection,this.clipShadows=t.clipShadows,this.shadowSide=t.shadowSide,this.colorWrite=t.colorWrite,this.precision=t.precision,this.polygonOffset=t.polygonOffset,this.polygonOffsetFactor=t.polygonOffsetFactor,this.polygonOffsetUnits=t.polygonOffsetUnits,this.dithering=t.dithering,this.alphaTest=t.alphaTest,this.alphaHash=t.alphaHash,this.alphaToCoverage=t.alphaToCoverage,this.premultipliedAlpha=t.premultipliedAlpha,this.forceSinglePass=t.forceSinglePass,this.visible=t.visible,this.toneMapped=t.toneMapped,this.userData=JSON.parse(JSON.stringify(t.userData)),this}dispose(){this.dispatchEvent({type:"dispose"})}set needsUpdate(t){t===!0&&this.version++}}class su extends Ls{constructor(t){super(),this.isMeshBasicMaterial=!0,this.type="MeshBasicMaterial",this.color=new Me(16777215),this.map=null,this.lightMap=null,this.lightMapIntensity=1,this.aoMap=null,this.aoMapIntensity=1,this.specularMap=null,this.alphaMap=null,this.envMap=null,this.envMapRotation=new Fi,this.combine=Pv,this.reflectivity=1,this.refractionRatio=.98,this.wireframe=!1,this.wireframeLinewidth=1,this.wireframeLinecap="round",this.wireframeLinejoin="round",this.fog=!0,this.setValues(t)}copy(t){return super.copy(t),this.color.copy(t.color),this.map=t.map,this.lightMap=t.lightMap,this.lightMapIntensity=t.lightMapIntensity,this.aoMap=t.aoMap,this.aoMapIntensity=t.aoMapIntensity,this.specularMap=t.specularMap,this.alphaMap=t.alphaMap,this.envMap=t.envMap,this.envMapRotation.copy(t.envMapRotation),this.combine=t.combine,this.reflectivity=t.reflectivity,this.refractionRatio=t.refractionRatio,this.wireframe=t.wireframe,this.wireframeLinewidth=t.wireframeLinewidth,this.wireframeLinecap=t.wireframeLinecap,this.wireframeLinejoin=t.wireframeLinejoin,this.fog=t.fog,this}}const hn=new X,wc=new re;let nE=0;class yi{constructor(t,i,s=!1){if(Array.isArray(t))throw new TypeError("THREE.BufferAttribute: array should be a Typed Array.");this.isBufferAttribute=!0,Object.defineProperty(this,"id",{value:nE++}),this.name="",this.array=t,this.itemSize=i,this.count=t!==void 0?t.length/i:0,this.normalized=s,this.usage=Jd,this.updateRanges=[],this.gpuType=zi,this.version=0}onUploadCallback(){}set needsUpdate(t){t===!0&&this.version++}setUsage(t){return this.usage=t,this}addUpdateRange(t,i){this.updateRanges.push({start:t,count:i})}clearUpdateRanges(){this.updateRanges.length=0}copy(t){return this.name=t.name,this.array=new t.array.constructor(t.array),this.itemSize=t.itemSize,this.count=t.count,this.normalized=t.normalized,this.usage=t.usage,this.gpuType=t.gpuType,this}copyAt(t,i,s){t*=this.itemSize,s*=i.itemSize;for(let l=0,c=this.itemSize;l<c;l++)this.array[t+l]=i.array[s+l];return this}copyArray(t){return this.array.set(t),this}applyMatrix3(t){if(this.itemSize===2)for(let i=0,s=this.count;i<s;i++)wc.fromBufferAttribute(this,i),wc.applyMatrix3(t),this.setXY(i,wc.x,wc.y);else if(this.itemSize===3)for(let i=0,s=this.count;i<s;i++)hn.fromBufferAttribute(this,i),hn.applyMatrix3(t),this.setXYZ(i,hn.x,hn.y,hn.z);return this}applyMatrix4(t){for(let i=0,s=this.count;i<s;i++)hn.fromBufferAttribute(this,i),hn.applyMatrix4(t),this.setXYZ(i,hn.x,hn.y,hn.z);return this}applyNormalMatrix(t){for(let i=0,s=this.count;i<s;i++)hn.fromBufferAttribute(this,i),hn.applyNormalMatrix(t),this.setXYZ(i,hn.x,hn.y,hn.z);return this}transformDirection(t){for(let i=0,s=this.count;i<s;i++)hn.fromBufferAttribute(this,i),hn.transformDirection(t),this.setXYZ(i,hn.x,hn.y,hn.z);return this}set(t,i=0){return this.array.set(t,i),this}getComponent(t,i){let s=this.array[t*this.itemSize+i];return this.normalized&&(s=Ai(s,this.array)),s}setComponent(t,i,s){return this.normalized&&(s=Ge(s,this.array)),this.array[t*this.itemSize+i]=s,this}getX(t){let i=this.array[t*this.itemSize];return this.normalized&&(i=Ai(i,this.array)),i}setX(t,i){return this.normalized&&(i=Ge(i,this.array)),this.array[t*this.itemSize]=i,this}getY(t){let i=this.array[t*this.itemSize+1];return this.normalized&&(i=Ai(i,this.array)),i}setY(t,i){return this.normalized&&(i=Ge(i,this.array)),this.array[t*this.itemSize+1]=i,this}getZ(t){let i=this.array[t*this.itemSize+2];return this.normalized&&(i=Ai(i,this.array)),i}setZ(t,i){return this.normalized&&(i=Ge(i,this.array)),this.array[t*this.itemSize+2]=i,this}getW(t){let i=this.array[t*this.itemSize+3];return this.normalized&&(i=Ai(i,this.array)),i}setW(t,i){return this.normalized&&(i=Ge(i,this.array)),this.array[t*this.itemSize+3]=i,this}setXY(t,i,s){return t*=this.itemSize,this.normalized&&(i=Ge(i,this.array),s=Ge(s,this.array)),this.array[t+0]=i,this.array[t+1]=s,this}setXYZ(t,i,s,l){return t*=this.itemSize,this.normalized&&(i=Ge(i,this.array),s=Ge(s,this.array),l=Ge(l,this.array)),this.array[t+0]=i,this.array[t+1]=s,this.array[t+2]=l,this}setXYZW(t,i,s,l,c){return t*=this.itemSize,this.normalized&&(i=Ge(i,this.array),s=Ge(s,this.array),l=Ge(l,this.array),c=Ge(c,this.array)),this.array[t+0]=i,this.array[t+1]=s,this.array[t+2]=l,this.array[t+3]=c,this}onUpload(t){return this.onUploadCallback=t,this}clone(){return new this.constructor(this.array,this.itemSize).copy(this)}toJSON(){const t={itemSize:this.itemSize,type:this.array.constructor.name,array:Array.from(this.array),normalized:this.normalized};return this.name!==""&&(t.name=this.name),this.usage!==Jd&&(t.usage=this.usage),t}}class Qv extends yi{constructor(t,i,s){super(new Uint16Array(t),i,s)}}class Jv extends yi{constructor(t,i,s){super(new Uint32Array(t),i,s)}}class dn extends yi{constructor(t,i,s){super(new Float32Array(t),i,s)}}let iE=0;const gi=new qe,qh=new Mn,br=new X,ii=new da,Xo=new da,Sn=new X;class ri extends Us{constructor(){super(),this.isBufferGeometry=!0,Object.defineProperty(this,"id",{value:iE++}),this.uuid=fa(),this.name="",this.type="BufferGeometry",this.index=null,this.indirect=null,this.attributes={},this.morphAttributes={},this.morphTargetsRelative=!1,this.groups=[],this.boundingBox=null,this.boundingSphere=null,this.drawRange={start:0,count:1/0},this.userData={}}getIndex(){return this.index}setIndex(t){return Array.isArray(t)?this.index=new(qv(t)?Jv:Qv)(t,1):this.index=t,this}setIndirect(t){return this.indirect=t,this}getIndirect(){return this.indirect}getAttribute(t){return this.attributes[t]}setAttribute(t,i){return this.attributes[t]=i,this}deleteAttribute(t){return delete this.attributes[t],this}hasAttribute(t){return this.attributes[t]!==void 0}addGroup(t,i,s=0){this.groups.push({start:t,count:i,materialIndex:s})}clearGroups(){this.groups=[]}setDrawRange(t,i){this.drawRange.start=t,this.drawRange.count=i}applyMatrix4(t){const i=this.attributes.position;i!==void 0&&(i.applyMatrix4(t),i.needsUpdate=!0);const s=this.attributes.normal;if(s!==void 0){const c=new me().getNormalMatrix(t);s.applyNormalMatrix(c),s.needsUpdate=!0}const l=this.attributes.tangent;return l!==void 0&&(l.transformDirection(t),l.needsUpdate=!0),this.boundingBox!==null&&this.computeBoundingBox(),this.boundingSphere!==null&&this.computeBoundingSphere(),this}applyQuaternion(t){return gi.makeRotationFromQuaternion(t),this.applyMatrix4(gi),this}rotateX(t){return gi.makeRotationX(t),this.applyMatrix4(gi),this}rotateY(t){return gi.makeRotationY(t),this.applyMatrix4(gi),this}rotateZ(t){return gi.makeRotationZ(t),this.applyMatrix4(gi),this}translate(t,i,s){return gi.makeTranslation(t,i,s),this.applyMatrix4(gi),this}scale(t,i,s){return gi.makeScale(t,i,s),this.applyMatrix4(gi),this}lookAt(t){return qh.lookAt(t),qh.updateMatrix(),this.applyMatrix4(qh.matrix),this}center(){return this.computeBoundingBox(),this.boundingBox.getCenter(br).negate(),this.translate(br.x,br.y,br.z),this}setFromPoints(t){const i=this.getAttribute("position");if(i===void 0){const s=[];for(let l=0,c=t.length;l<c;l++){const h=t[l];s.push(h.x,h.y,h.z||0)}this.setAttribute("position",new dn(s,3))}else{const s=Math.min(t.length,i.count);for(let l=0;l<s;l++){const c=t[l];i.setXYZ(l,c.x,c.y,c.z||0)}t.length>i.count&&console.warn("THREE.BufferGeometry: Buffer size too small for points data. Use .dispose() and create a new geometry."),i.needsUpdate=!0}return this}computeBoundingBox(){this.boundingBox===null&&(this.boundingBox=new da);const t=this.attributes.position,i=this.morphAttributes.position;if(t&&t.isGLBufferAttribute){console.error("THREE.BufferGeometry.computeBoundingBox(): GLBufferAttribute requires a manual bounding box.",this),this.boundingBox.set(new X(-1/0,-1/0,-1/0),new X(1/0,1/0,1/0));return}if(t!==void 0){if(this.boundingBox.setFromBufferAttribute(t),i)for(let s=0,l=i.length;s<l;s++){const c=i[s];ii.setFromBufferAttribute(c),this.morphTargetsRelative?(Sn.addVectors(this.boundingBox.min,ii.min),this.boundingBox.expandByPoint(Sn),Sn.addVectors(this.boundingBox.max,ii.max),this.boundingBox.expandByPoint(Sn)):(this.boundingBox.expandByPoint(ii.min),this.boundingBox.expandByPoint(ii.max))}}else this.boundingBox.makeEmpty();(isNaN(this.boundingBox.min.x)||isNaN(this.boundingBox.min.y)||isNaN(this.boundingBox.min.z))&&console.error('THREE.BufferGeometry.computeBoundingBox(): Computed min/max have NaN values. The "position" attribute is likely to have NaN values.',this)}computeBoundingSphere(){this.boundingSphere===null&&(this.boundingSphere=new Wr);const t=this.attributes.position,i=this.morphAttributes.position;if(t&&t.isGLBufferAttribute){console.error("THREE.BufferGeometry.computeBoundingSphere(): GLBufferAttribute requires a manual bounding sphere.",this),this.boundingSphere.set(new X,1/0);return}if(t){const s=this.boundingSphere.center;if(ii.setFromBufferAttribute(t),i)for(let c=0,h=i.length;c<h;c++){const d=i[c];Xo.setFromBufferAttribute(d),this.morphTargetsRelative?(Sn.addVectors(ii.min,Xo.min),ii.expandByPoint(Sn),Sn.addVectors(ii.max,Xo.max),ii.expandByPoint(Sn)):(ii.expandByPoint(Xo.min),ii.expandByPoint(Xo.max))}ii.getCenter(s);let l=0;for(let c=0,h=t.count;c<h;c++)Sn.fromBufferAttribute(t,c),l=Math.max(l,s.distanceToSquared(Sn));if(i)for(let c=0,h=i.length;c<h;c++){const d=i[c],m=this.morphTargetsRelative;for(let p=0,v=d.count;p<v;p++)Sn.fromBufferAttribute(d,p),m&&(br.fromBufferAttribute(t,p),Sn.add(br)),l=Math.max(l,s.distanceToSquared(Sn))}this.boundingSphere.radius=Math.sqrt(l),isNaN(this.boundingSphere.radius)&&console.error('THREE.BufferGeometry.computeBoundingSphere(): Computed radius is NaN. The "position" attribute is likely to have NaN values.',this)}}computeTangents(){const t=this.index,i=this.attributes;if(t===null||i.position===void 0||i.normal===void 0||i.uv===void 0){console.error("THREE.BufferGeometry: .computeTangents() failed. Missing required attributes (index, position, normal or uv)");return}const s=i.position,l=i.normal,c=i.uv;this.hasAttribute("tangent")===!1&&this.setAttribute("tangent",new yi(new Float32Array(4*s.count),4));const h=this.getAttribute("tangent"),d=[],m=[];for(let G=0;G<s.count;G++)d[G]=new X,m[G]=new X;const p=new X,v=new X,g=new X,x=new re,S=new re,b=new re,R=new X,M=new X;function _(G,D,w){p.fromBufferAttribute(s,G),v.fromBufferAttribute(s,D),g.fromBufferAttribute(s,w),x.fromBufferAttribute(c,G),S.fromBufferAttribute(c,D),b.fromBufferAttribute(c,w),v.sub(p),g.sub(p),S.sub(x),b.sub(x);const V=1/(S.x*b.y-b.x*S.y);isFinite(V)&&(R.copy(v).multiplyScalar(b.y).addScaledVector(g,-S.y).multiplyScalar(V),M.copy(g).multiplyScalar(S.x).addScaledVector(v,-b.x).multiplyScalar(V),d[G].add(R),d[D].add(R),d[w].add(R),m[G].add(M),m[D].add(M),m[w].add(M))}let L=this.groups;L.length===0&&(L=[{start:0,count:t.count}]);for(let G=0,D=L.length;G<D;++G){const w=L[G],V=w.start,K=w.count;for(let ot=V,ct=V+K;ot<ct;ot+=3)_(t.getX(ot+0),t.getX(ot+1),t.getX(ot+2))}const U=new X,T=new X,P=new X,B=new X;function F(G){P.fromBufferAttribute(l,G),B.copy(P);const D=d[G];U.copy(D),U.sub(P.multiplyScalar(P.dot(D))).normalize(),T.crossVectors(B,D);const V=T.dot(m[G])<0?-1:1;h.setXYZW(G,U.x,U.y,U.z,V)}for(let G=0,D=L.length;G<D;++G){const w=L[G],V=w.start,K=w.count;for(let ot=V,ct=V+K;ot<ct;ot+=3)F(t.getX(ot+0)),F(t.getX(ot+1)),F(t.getX(ot+2))}}computeVertexNormals(){const t=this.index,i=this.getAttribute("position");if(i!==void 0){let s=this.getAttribute("normal");if(s===void 0)s=new yi(new Float32Array(i.count*3),3),this.setAttribute("normal",s);else for(let x=0,S=s.count;x<S;x++)s.setXYZ(x,0,0,0);const l=new X,c=new X,h=new X,d=new X,m=new X,p=new X,v=new X,g=new X;if(t)for(let x=0,S=t.count;x<S;x+=3){const b=t.getX(x+0),R=t.getX(x+1),M=t.getX(x+2);l.fromBufferAttribute(i,b),c.fromBufferAttribute(i,R),h.fromBufferAttribute(i,M),v.subVectors(h,c),g.subVectors(l,c),v.cross(g),d.fromBufferAttribute(s,b),m.fromBufferAttribute(s,R),p.fromBufferAttribute(s,M),d.add(v),m.add(v),p.add(v),s.setXYZ(b,d.x,d.y,d.z),s.setXYZ(R,m.x,m.y,m.z),s.setXYZ(M,p.x,p.y,p.z)}else for(let x=0,S=i.count;x<S;x+=3)l.fromBufferAttribute(i,x+0),c.fromBufferAttribute(i,x+1),h.fromBufferAttribute(i,x+2),v.subVectors(h,c),g.subVectors(l,c),v.cross(g),s.setXYZ(x+0,v.x,v.y,v.z),s.setXYZ(x+1,v.x,v.y,v.z),s.setXYZ(x+2,v.x,v.y,v.z);this.normalizeNormals(),s.needsUpdate=!0}}normalizeNormals(){const t=this.attributes.normal;for(let i=0,s=t.count;i<s;i++)Sn.fromBufferAttribute(t,i),Sn.normalize(),t.setXYZ(i,Sn.x,Sn.y,Sn.z)}toNonIndexed(){function t(d,m){const p=d.array,v=d.itemSize,g=d.normalized,x=new p.constructor(m.length*v);let S=0,b=0;for(let R=0,M=m.length;R<M;R++){d.isInterleavedBufferAttribute?S=m[R]*d.data.stride+d.offset:S=m[R]*v;for(let _=0;_<v;_++)x[b++]=p[S++]}return new yi(x,v,g)}if(this.index===null)return console.warn("THREE.BufferGeometry.toNonIndexed(): BufferGeometry is already non-indexed."),this;const i=new ri,s=this.index.array,l=this.attributes;for(const d in l){const m=l[d],p=t(m,s);i.setAttribute(d,p)}const c=this.morphAttributes;for(const d in c){const m=[],p=c[d];for(let v=0,g=p.length;v<g;v++){const x=p[v],S=t(x,s);m.push(S)}i.morphAttributes[d]=m}i.morphTargetsRelative=this.morphTargetsRelative;const h=this.groups;for(let d=0,m=h.length;d<m;d++){const p=h[d];i.addGroup(p.start,p.count,p.materialIndex)}return i}toJSON(){const t={metadata:{version:4.7,type:"BufferGeometry",generator:"BufferGeometry.toJSON"}};if(t.uuid=this.uuid,t.type=this.type,this.name!==""&&(t.name=this.name),Object.keys(this.userData).length>0&&(t.userData=this.userData),this.parameters!==void 0){const m=this.parameters;for(const p in m)m[p]!==void 0&&(t[p]=m[p]);return t}t.data={attributes:{}};const i=this.index;i!==null&&(t.data.index={type:i.array.constructor.name,array:Array.prototype.slice.call(i.array)});const s=this.attributes;for(const m in s){const p=s[m];t.data.attributes[m]=p.toJSON(t.data)}const l={};let c=!1;for(const m in this.morphAttributes){const p=this.morphAttributes[m],v=[];for(let g=0,x=p.length;g<x;g++){const S=p[g];v.push(S.toJSON(t.data))}v.length>0&&(l[m]=v,c=!0)}c&&(t.data.morphAttributes=l,t.data.morphTargetsRelative=this.morphTargetsRelative);const h=this.groups;h.length>0&&(t.data.groups=JSON.parse(JSON.stringify(h)));const d=this.boundingSphere;return d!==null&&(t.data.boundingSphere=d.toJSON()),t}clone(){return new this.constructor().copy(this)}copy(t){this.index=null,this.attributes={},this.morphAttributes={},this.groups=[],this.boundingBox=null,this.boundingSphere=null;const i={};this.name=t.name;const s=t.index;s!==null&&this.setIndex(s.clone());const l=t.attributes;for(const p in l){const v=l[p];this.setAttribute(p,v.clone(i))}const c=t.morphAttributes;for(const p in c){const v=[],g=c[p];for(let x=0,S=g.length;x<S;x++)v.push(g[x].clone(i));this.morphAttributes[p]=v}this.morphTargetsRelative=t.morphTargetsRelative;const h=t.groups;for(let p=0,v=h.length;p<v;p++){const g=h[p];this.addGroup(g.start,g.count,g.materialIndex)}const d=t.boundingBox;d!==null&&(this.boundingBox=d.clone());const m=t.boundingSphere;return m!==null&&(this.boundingSphere=m.clone()),this.drawRange.start=t.drawRange.start,this.drawRange.count=t.drawRange.count,this.userData=t.userData,this}dispose(){this.dispatchEvent({type:"dispose"})}}const U0=new qe,_s=new hu,Dc=new Wr,L0=new X,Uc=new X,Lc=new X,Nc=new X,Zh=new X,Oc=new X,N0=new X,Pc=new X;class vi extends Mn{constructor(t=new ri,i=new su){super(),this.isMesh=!0,this.type="Mesh",this.geometry=t,this.material=i,this.morphTargetDictionary=void 0,this.morphTargetInfluences=void 0,this.count=1,this.updateMorphTargets()}copy(t,i){return super.copy(t,i),t.morphTargetInfluences!==void 0&&(this.morphTargetInfluences=t.morphTargetInfluences.slice()),t.morphTargetDictionary!==void 0&&(this.morphTargetDictionary=Object.assign({},t.morphTargetDictionary)),this.material=Array.isArray(t.material)?t.material.slice():t.material,this.geometry=t.geometry,this}updateMorphTargets(){const i=this.geometry.morphAttributes,s=Object.keys(i);if(s.length>0){const l=i[s[0]];if(l!==void 0){this.morphTargetInfluences=[],this.morphTargetDictionary={};for(let c=0,h=l.length;c<h;c++){const d=l[c].name||String(c);this.morphTargetInfluences.push(0),this.morphTargetDictionary[d]=c}}}}getVertexPosition(t,i){const s=this.geometry,l=s.attributes.position,c=s.morphAttributes.position,h=s.morphTargetsRelative;i.fromBufferAttribute(l,t);const d=this.morphTargetInfluences;if(c&&d){Oc.set(0,0,0);for(let m=0,p=c.length;m<p;m++){const v=d[m],g=c[m];v!==0&&(Zh.fromBufferAttribute(g,t),h?Oc.addScaledVector(Zh,v):Oc.addScaledVector(Zh.sub(i),v))}i.add(Oc)}return i}raycast(t,i){const s=this.geometry,l=this.material,c=this.matrixWorld;l!==void 0&&(s.boundingSphere===null&&s.computeBoundingSphere(),Dc.copy(s.boundingSphere),Dc.applyMatrix4(c),_s.copy(t.ray).recast(t.near),!(Dc.containsPoint(_s.origin)===!1&&(_s.intersectSphere(Dc,L0)===null||_s.origin.distanceToSquared(L0)>(t.far-t.near)**2))&&(U0.copy(c).invert(),_s.copy(t.ray).applyMatrix4(U0),!(s.boundingBox!==null&&_s.intersectsBox(s.boundingBox)===!1)&&this._computeIntersections(t,i,_s)))}_computeIntersections(t,i,s){let l;const c=this.geometry,h=this.material,d=c.index,m=c.attributes.position,p=c.attributes.uv,v=c.attributes.uv1,g=c.attributes.normal,x=c.groups,S=c.drawRange;if(d!==null)if(Array.isArray(h))for(let b=0,R=x.length;b<R;b++){const M=x[b],_=h[M.materialIndex],L=Math.max(M.start,S.start),U=Math.min(d.count,Math.min(M.start+M.count,S.start+S.count));for(let T=L,P=U;T<P;T+=3){const B=d.getX(T),F=d.getX(T+1),G=d.getX(T+2);l=zc(this,_,t,s,p,v,g,B,F,G),l&&(l.faceIndex=Math.floor(T/3),l.face.materialIndex=M.materialIndex,i.push(l))}}else{const b=Math.max(0,S.start),R=Math.min(d.count,S.start+S.count);for(let M=b,_=R;M<_;M+=3){const L=d.getX(M),U=d.getX(M+1),T=d.getX(M+2);l=zc(this,h,t,s,p,v,g,L,U,T),l&&(l.faceIndex=Math.floor(M/3),i.push(l))}}else if(m!==void 0)if(Array.isArray(h))for(let b=0,R=x.length;b<R;b++){const M=x[b],_=h[M.materialIndex],L=Math.max(M.start,S.start),U=Math.min(m.count,Math.min(M.start+M.count,S.start+S.count));for(let T=L,P=U;T<P;T+=3){const B=T,F=T+1,G=T+2;l=zc(this,_,t,s,p,v,g,B,F,G),l&&(l.faceIndex=Math.floor(T/3),l.face.materialIndex=M.materialIndex,i.push(l))}}else{const b=Math.max(0,S.start),R=Math.min(m.count,S.start+S.count);for(let M=b,_=R;M<_;M+=3){const L=M,U=M+1,T=M+2;l=zc(this,h,t,s,p,v,g,L,U,T),l&&(l.faceIndex=Math.floor(M/3),i.push(l))}}}}function aE(r,t,i,s,l,c,h,d){let m;if(t.side===Yn?m=s.intersectTriangle(h,c,l,!0,d):m=s.intersectTriangle(l,c,h,t.side===qa,d),m===null)return null;Pc.copy(d),Pc.applyMatrix4(r.matrixWorld);const p=i.ray.origin.distanceTo(Pc);return p<i.near||p>i.far?null:{distance:p,point:Pc.clone(),object:r}}function zc(r,t,i,s,l,c,h,d,m,p){r.getVertexPosition(d,Uc),r.getVertexPosition(m,Lc),r.getVertexPosition(p,Nc);const v=aE(r,t,i,s,Uc,Lc,Nc,N0);if(v){const g=new X;_i.getBarycoord(N0,Uc,Lc,Nc,g),l&&(v.uv=_i.getInterpolatedAttribute(l,d,m,p,g,new re)),c&&(v.uv1=_i.getInterpolatedAttribute(c,d,m,p,g,new re)),h&&(v.normal=_i.getInterpolatedAttribute(h,d,m,p,g,new X),v.normal.dot(s.direction)>0&&v.normal.multiplyScalar(-1));const x={a:d,b:m,c:p,normal:new X,materialIndex:0};_i.getNormal(Uc,Lc,Nc,x.normal),v.face=x,v.barycoord=g}return v}class jr extends ri{constructor(t=1,i=1,s=1,l=1,c=1,h=1){super(),this.type="BoxGeometry",this.parameters={width:t,height:i,depth:s,widthSegments:l,heightSegments:c,depthSegments:h};const d=this;l=Math.floor(l),c=Math.floor(c),h=Math.floor(h);const m=[],p=[],v=[],g=[];let x=0,S=0;b("z","y","x",-1,-1,s,i,t,h,c,0),b("z","y","x",1,-1,s,i,-t,h,c,1),b("x","z","y",1,1,t,s,i,l,h,2),b("x","z","y",1,-1,t,s,-i,l,h,3),b("x","y","z",1,-1,t,i,s,l,c,4),b("x","y","z",-1,-1,t,i,-s,l,c,5),this.setIndex(m),this.setAttribute("position",new dn(p,3)),this.setAttribute("normal",new dn(v,3)),this.setAttribute("uv",new dn(g,2));function b(R,M,_,L,U,T,P,B,F,G,D){const w=T/F,V=P/G,K=T/2,ot=P/2,ct=B/2,ht=F+1,N=G+1;let J=0,Y=0;const Tt=new X;for(let Ct=0;Ct<N;Ct++){const z=Ct*V-ot;for(let at=0;at<ht;at++){const Mt=at*w-K;Tt[R]=Mt*L,Tt[M]=z*U,Tt[_]=ct,p.push(Tt.x,Tt.y,Tt.z),Tt[R]=0,Tt[M]=0,Tt[_]=B>0?1:-1,v.push(Tt.x,Tt.y,Tt.z),g.push(at/F),g.push(1-Ct/G),J+=1}}for(let Ct=0;Ct<G;Ct++)for(let z=0;z<F;z++){const at=x+z+ht*Ct,Mt=x+z+ht*(Ct+1),wt=x+(z+1)+ht*(Ct+1),mt=x+(z+1)+ht*Ct;m.push(at,Mt,mt),m.push(Mt,wt,mt),Y+=6}d.addGroup(S,Y,D),S+=Y,x+=J}}copy(t){return super.copy(t),this.parameters=Object.assign({},t.parameters),this}static fromJSON(t){return new jr(t.width,t.height,t.depth,t.widthSegments,t.heightSegments,t.depthSegments)}}function Xr(r){const t={};for(const i in r){t[i]={};for(const s in r[i]){const l=r[i][s];l&&(l.isColor||l.isMatrix3||l.isMatrix4||l.isVector2||l.isVector3||l.isVector4||l.isTexture||l.isQuaternion)?l.isRenderTargetTexture?(console.warn("UniformsUtils: Textures of render targets cannot be cloned via cloneUniforms() or mergeUniforms()."),t[i][s]=null):t[i][s]=l.clone():Array.isArray(l)?t[i][s]=l.slice():t[i][s]=l}}return t}function In(r){const t={};for(let i=0;i<r.length;i++){const s=Xr(r[i]);for(const l in s)t[l]=s[l]}return t}function sE(r){const t=[];for(let i=0;i<r.length;i++)t.push(r[i].clone());return t}function $v(r){const t=r.getRenderTarget();return t===null?r.outputColorSpace:t.isXRRenderTarget===!0?t.texture.colorSpace:Pe.workingColorSpace}const rE={clone:Xr,merge:In};var oE=`void main() {
	gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 );
}`,lE=`void main() {
	gl_FragColor = vec4( 1.0, 0.0, 0.0, 1.0 );
}`;class Za extends Ls{constructor(t){super(),this.isShaderMaterial=!0,this.type="ShaderMaterial",this.defines={},this.uniforms={},this.uniformsGroups=[],this.vertexShader=oE,this.fragmentShader=lE,this.linewidth=1,this.wireframe=!1,this.wireframeLinewidth=1,this.fog=!1,this.lights=!1,this.clipping=!1,this.forceSinglePass=!0,this.extensions={clipCullDistance:!1,multiDraw:!1},this.defaultAttributeValues={color:[1,1,1],uv:[0,0],uv1:[0,0]},this.index0AttributeName=void 0,this.uniformsNeedUpdate=!1,this.glslVersion=null,t!==void 0&&this.setValues(t)}copy(t){return super.copy(t),this.fragmentShader=t.fragmentShader,this.vertexShader=t.vertexShader,this.uniforms=Xr(t.uniforms),this.uniformsGroups=sE(t.uniformsGroups),this.defines=Object.assign({},t.defines),this.wireframe=t.wireframe,this.wireframeLinewidth=t.wireframeLinewidth,this.fog=t.fog,this.lights=t.lights,this.clipping=t.clipping,this.extensions=Object.assign({},t.extensions),this.glslVersion=t.glslVersion,this}toJSON(t){const i=super.toJSON(t);i.glslVersion=this.glslVersion,i.uniforms={};for(const l in this.uniforms){const h=this.uniforms[l].value;h&&h.isTexture?i.uniforms[l]={type:"t",value:h.toJSON(t).uuid}:h&&h.isColor?i.uniforms[l]={type:"c",value:h.getHex()}:h&&h.isVector2?i.uniforms[l]={type:"v2",value:h.toArray()}:h&&h.isVector3?i.uniforms[l]={type:"v3",value:h.toArray()}:h&&h.isVector4?i.uniforms[l]={type:"v4",value:h.toArray()}:h&&h.isMatrix3?i.uniforms[l]={type:"m3",value:h.toArray()}:h&&h.isMatrix4?i.uniforms[l]={type:"m4",value:h.toArray()}:i.uniforms[l]={value:h}}Object.keys(this.defines).length>0&&(i.defines=this.defines),i.vertexShader=this.vertexShader,i.fragmentShader=this.fragmentShader,i.lights=this.lights,i.clipping=this.clipping;const s={};for(const l in this.extensions)this.extensions[l]===!0&&(s[l]=!0);return Object.keys(s).length>0&&(i.extensions=s),i}}class ty extends Mn{constructor(){super(),this.isCamera=!0,this.type="Camera",this.matrixWorldInverse=new qe,this.projectionMatrix=new qe,this.projectionMatrixInverse=new qe,this.coordinateSystem=Bi,this._reversedDepth=!1}get reversedDepth(){return this._reversedDepth}copy(t,i){return super.copy(t,i),this.matrixWorldInverse.copy(t.matrixWorldInverse),this.projectionMatrix.copy(t.projectionMatrix),this.projectionMatrixInverse.copy(t.projectionMatrixInverse),this.coordinateSystem=t.coordinateSystem,this}getWorldDirection(t){return super.getWorldDirection(t).negate()}updateMatrixWorld(t){super.updateMatrixWorld(t),this.matrixWorldInverse.copy(this.matrixWorld).invert()}updateWorldMatrix(t,i){super.updateWorldMatrix(t,i),this.matrixWorldInverse.copy(this.matrixWorld).invert()}clone(){return new this.constructor().copy(this)}}const ka=new X,O0=new re,P0=new re;class ai extends ty{constructor(t=50,i=1,s=.1,l=2e3){super(),this.isPerspectiveCamera=!0,this.type="PerspectiveCamera",this.fov=t,this.zoom=1,this.near=s,this.far=l,this.focus=10,this.aspect=i,this.view=null,this.filmGauge=35,this.filmOffset=0,this.updateProjectionMatrix()}copy(t,i){return super.copy(t,i),this.fov=t.fov,this.zoom=t.zoom,this.near=t.near,this.far=t.far,this.focus=t.focus,this.aspect=t.aspect,this.view=t.view===null?null:Object.assign({},t.view),this.filmGauge=t.filmGauge,this.filmOffset=t.filmOffset,this}setFocalLength(t){const i=.5*this.getFilmHeight()/t;this.fov=ol*2*Math.atan(i),this.updateProjectionMatrix()}getFocalLength(){const t=Math.tan(el*.5*this.fov);return .5*this.getFilmHeight()/t}getEffectiveFOV(){return ol*2*Math.atan(Math.tan(el*.5*this.fov)/this.zoom)}getFilmWidth(){return this.filmGauge*Math.min(this.aspect,1)}getFilmHeight(){return this.filmGauge/Math.max(this.aspect,1)}getViewBounds(t,i,s){ka.set(-1,-1,.5).applyMatrix4(this.projectionMatrixInverse),i.set(ka.x,ka.y).multiplyScalar(-t/ka.z),ka.set(1,1,.5).applyMatrix4(this.projectionMatrixInverse),s.set(ka.x,ka.y).multiplyScalar(-t/ka.z)}getViewSize(t,i){return this.getViewBounds(t,O0,P0),i.subVectors(P0,O0)}setViewOffset(t,i,s,l,c,h){this.aspect=t/i,this.view===null&&(this.view={enabled:!0,fullWidth:1,fullHeight:1,offsetX:0,offsetY:0,width:1,height:1}),this.view.enabled=!0,this.view.fullWidth=t,this.view.fullHeight=i,this.view.offsetX=s,this.view.offsetY=l,this.view.width=c,this.view.height=h,this.updateProjectionMatrix()}clearViewOffset(){this.view!==null&&(this.view.enabled=!1),this.updateProjectionMatrix()}updateProjectionMatrix(){const t=this.near;let i=t*Math.tan(el*.5*this.fov)/this.zoom,s=2*i,l=this.aspect*s,c=-.5*l;const h=this.view;if(this.view!==null&&this.view.enabled){const m=h.fullWidth,p=h.fullHeight;c+=h.offsetX*l/m,i-=h.offsetY*s/p,l*=h.width/m,s*=h.height/p}const d=this.filmOffset;d!==0&&(c+=t*d/this.getFilmWidth()),this.projectionMatrix.makePerspective(c,c+l,i,i-s,t,this.far,this.coordinateSystem,this.reversedDepth),this.projectionMatrixInverse.copy(this.projectionMatrix).invert()}toJSON(t){const i=super.toJSON(t);return i.object.fov=this.fov,i.object.zoom=this.zoom,i.object.near=this.near,i.object.far=this.far,i.object.focus=this.focus,i.object.aspect=this.aspect,this.view!==null&&(i.object.view=Object.assign({},this.view)),i.object.filmGauge=this.filmGauge,i.object.filmOffset=this.filmOffset,i}}const Ar=-90,Rr=1;class cE extends Mn{constructor(t,i,s){super(),this.type="CubeCamera",this.renderTarget=s,this.coordinateSystem=null,this.activeMipmapLevel=0;const l=new ai(Ar,Rr,t,i);l.layers=this.layers,this.add(l);const c=new ai(Ar,Rr,t,i);c.layers=this.layers,this.add(c);const h=new ai(Ar,Rr,t,i);h.layers=this.layers,this.add(h);const d=new ai(Ar,Rr,t,i);d.layers=this.layers,this.add(d);const m=new ai(Ar,Rr,t,i);m.layers=this.layers,this.add(m);const p=new ai(Ar,Rr,t,i);p.layers=this.layers,this.add(p)}updateCoordinateSystem(){const t=this.coordinateSystem,i=this.children.concat(),[s,l,c,h,d,m]=i;for(const p of i)this.remove(p);if(t===Bi)s.up.set(0,1,0),s.lookAt(1,0,0),l.up.set(0,1,0),l.lookAt(-1,0,0),c.up.set(0,0,-1),c.lookAt(0,1,0),h.up.set(0,0,1),h.lookAt(0,-1,0),d.up.set(0,1,0),d.lookAt(0,0,1),m.up.set(0,1,0),m.lookAt(0,0,-1);else if(t===iu)s.up.set(0,-1,0),s.lookAt(-1,0,0),l.up.set(0,-1,0),l.lookAt(1,0,0),c.up.set(0,0,1),c.lookAt(0,1,0),h.up.set(0,0,-1),h.lookAt(0,-1,0),d.up.set(0,-1,0),d.lookAt(0,0,1),m.up.set(0,-1,0),m.lookAt(0,0,-1);else throw new Error("THREE.CubeCamera.updateCoordinateSystem(): Invalid coordinate system: "+t);for(const p of i)this.add(p),p.updateMatrixWorld()}update(t,i){this.parent===null&&this.updateMatrixWorld();const{renderTarget:s,activeMipmapLevel:l}=this;this.coordinateSystem!==t.coordinateSystem&&(this.coordinateSystem=t.coordinateSystem,this.updateCoordinateSystem());const[c,h,d,m,p,v]=this.children,g=t.getRenderTarget(),x=t.getActiveCubeFace(),S=t.getActiveMipmapLevel(),b=t.xr.enabled;t.xr.enabled=!1;const R=s.texture.generateMipmaps;s.texture.generateMipmaps=!1,t.setRenderTarget(s,0,l),t.render(i,c),t.setRenderTarget(s,1,l),t.render(i,h),t.setRenderTarget(s,2,l),t.render(i,d),t.setRenderTarget(s,3,l),t.render(i,m),t.setRenderTarget(s,4,l),t.render(i,p),s.texture.generateMipmaps=R,t.setRenderTarget(s,5,l),t.render(i,v),t.setRenderTarget(g,x,S),t.xr.enabled=b,s.texture.needsPMREMUpdate=!0}}class ey extends On{constructor(t=[],i=Gr,s,l,c,h,d,m,p,v){super(t,i,s,l,c,h,d,m,p,v),this.isCubeTexture=!0,this.flipY=!1}get images(){return this.image}set images(t){this.image=t}}class uE extends Ds{constructor(t=1,i={}){super(t,t,i),this.isWebGLCubeRenderTarget=!0;const s={width:t,height:t,depth:1},l=[s,s,s,s,s,s];this.texture=new ey(l),this._setTextureOptions(i),this.texture.isRenderTargetTexture=!0}fromEquirectangularTexture(t,i){this.texture.type=i.type,this.texture.colorSpace=i.colorSpace,this.texture.generateMipmaps=i.generateMipmaps,this.texture.minFilter=i.minFilter,this.texture.magFilter=i.magFilter;const s={uniforms:{tEquirect:{value:null}},vertexShader:`

				varying vec3 vWorldDirection;

				vec3 transformDirection( in vec3 dir, in mat4 matrix ) {

					return normalize( ( matrix * vec4( dir, 0.0 ) ).xyz );

				}

				void main() {

					vWorldDirection = transformDirection( position, modelMatrix );

					#include <begin_vertex>
					#include <project_vertex>

				}
			`,fragmentShader:`

				uniform sampler2D tEquirect;

				varying vec3 vWorldDirection;

				#include <common>

				void main() {

					vec3 direction = normalize( vWorldDirection );

					vec2 sampleUV = equirectUv( direction );

					gl_FragColor = texture2D( tEquirect, sampleUV );

				}
			`},l=new jr(5,5,5),c=new Za({name:"CubemapFromEquirect",uniforms:Xr(s.uniforms),vertexShader:s.vertexShader,fragmentShader:s.fragmentShader,side:Yn,blending:ja});c.uniforms.tEquirect.value=i;const h=new vi(l,c),d=i.minFilter;return i.minFilter===Rs&&(i.minFilter=Pi),new cE(1,10,this).update(t,h),i.minFilter=d,h.geometry.dispose(),h.material.dispose(),this}clear(t,i=!0,s=!0,l=!0){const c=t.getRenderTarget();for(let h=0;h<6;h++)t.setRenderTarget(this,h),t.clear(i,s,l);t.setRenderTarget(c)}}class Bc extends Mn{constructor(){super(),this.isGroup=!0,this.type="Group"}}const fE={type:"move"};class Kh{constructor(){this._targetRay=null,this._grip=null,this._hand=null}getHandSpace(){return this._hand===null&&(this._hand=new Bc,this._hand.matrixAutoUpdate=!1,this._hand.visible=!1,this._hand.joints={},this._hand.inputState={pinching:!1}),this._hand}getTargetRaySpace(){return this._targetRay===null&&(this._targetRay=new Bc,this._targetRay.matrixAutoUpdate=!1,this._targetRay.visible=!1,this._targetRay.hasLinearVelocity=!1,this._targetRay.linearVelocity=new X,this._targetRay.hasAngularVelocity=!1,this._targetRay.angularVelocity=new X),this._targetRay}getGripSpace(){return this._grip===null&&(this._grip=new Bc,this._grip.matrixAutoUpdate=!1,this._grip.visible=!1,this._grip.hasLinearVelocity=!1,this._grip.linearVelocity=new X,this._grip.hasAngularVelocity=!1,this._grip.angularVelocity=new X),this._grip}dispatchEvent(t){return this._targetRay!==null&&this._targetRay.dispatchEvent(t),this._grip!==null&&this._grip.dispatchEvent(t),this._hand!==null&&this._hand.dispatchEvent(t),this}connect(t){if(t&&t.hand){const i=this._hand;if(i)for(const s of t.hand.values())this._getHandJoint(i,s)}return this.dispatchEvent({type:"connected",data:t}),this}disconnect(t){return this.dispatchEvent({type:"disconnected",data:t}),this._targetRay!==null&&(this._targetRay.visible=!1),this._grip!==null&&(this._grip.visible=!1),this._hand!==null&&(this._hand.visible=!1),this}update(t,i,s){let l=null,c=null,h=null;const d=this._targetRay,m=this._grip,p=this._hand;if(t&&i.session.visibilityState!=="visible-blurred"){if(p&&t.hand){h=!0;for(const R of t.hand.values()){const M=i.getJointPose(R,s),_=this._getHandJoint(p,R);M!==null&&(_.matrix.fromArray(M.transform.matrix),_.matrix.decompose(_.position,_.rotation,_.scale),_.matrixWorldNeedsUpdate=!0,_.jointRadius=M.radius),_.visible=M!==null}const v=p.joints["index-finger-tip"],g=p.joints["thumb-tip"],x=v.position.distanceTo(g.position),S=.02,b=.005;p.inputState.pinching&&x>S+b?(p.inputState.pinching=!1,this.dispatchEvent({type:"pinchend",handedness:t.handedness,target:this})):!p.inputState.pinching&&x<=S-b&&(p.inputState.pinching=!0,this.dispatchEvent({type:"pinchstart",handedness:t.handedness,target:this}))}else m!==null&&t.gripSpace&&(c=i.getPose(t.gripSpace,s),c!==null&&(m.matrix.fromArray(c.transform.matrix),m.matrix.decompose(m.position,m.rotation,m.scale),m.matrixWorldNeedsUpdate=!0,c.linearVelocity?(m.hasLinearVelocity=!0,m.linearVelocity.copy(c.linearVelocity)):m.hasLinearVelocity=!1,c.angularVelocity?(m.hasAngularVelocity=!0,m.angularVelocity.copy(c.angularVelocity)):m.hasAngularVelocity=!1));d!==null&&(l=i.getPose(t.targetRaySpace,s),l===null&&c!==null&&(l=c),l!==null&&(d.matrix.fromArray(l.transform.matrix),d.matrix.decompose(d.position,d.rotation,d.scale),d.matrixWorldNeedsUpdate=!0,l.linearVelocity?(d.hasLinearVelocity=!0,d.linearVelocity.copy(l.linearVelocity)):d.hasLinearVelocity=!1,l.angularVelocity?(d.hasAngularVelocity=!0,d.angularVelocity.copy(l.angularVelocity)):d.hasAngularVelocity=!1,this.dispatchEvent(fE)))}return d!==null&&(d.visible=l!==null),m!==null&&(m.visible=c!==null),p!==null&&(p.visible=h!==null),this}_getHandJoint(t,i){if(t.joints[i.jointName]===void 0){const s=new Bc;s.matrixAutoUpdate=!1,s.visible=!1,t.joints[i.jointName]=s,t.add(s)}return t.joints[i.jointName]}}class hE extends Mn{constructor(){super(),this.isScene=!0,this.type="Scene",this.background=null,this.environment=null,this.fog=null,this.backgroundBlurriness=0,this.backgroundIntensity=1,this.backgroundRotation=new Fi,this.environmentIntensity=1,this.environmentRotation=new Fi,this.overrideMaterial=null,typeof __THREE_DEVTOOLS__<"u"&&__THREE_DEVTOOLS__.dispatchEvent(new CustomEvent("observe",{detail:this}))}copy(t,i){return super.copy(t,i),t.background!==null&&(this.background=t.background.clone()),t.environment!==null&&(this.environment=t.environment.clone()),t.fog!==null&&(this.fog=t.fog.clone()),this.backgroundBlurriness=t.backgroundBlurriness,this.backgroundIntensity=t.backgroundIntensity,this.backgroundRotation.copy(t.backgroundRotation),this.environmentIntensity=t.environmentIntensity,this.environmentRotation.copy(t.environmentRotation),t.overrideMaterial!==null&&(this.overrideMaterial=t.overrideMaterial.clone()),this.matrixAutoUpdate=t.matrixAutoUpdate,this}toJSON(t){const i=super.toJSON(t);return this.fog!==null&&(i.object.fog=this.fog.toJSON()),this.backgroundBlurriness>0&&(i.object.backgroundBlurriness=this.backgroundBlurriness),this.backgroundIntensity!==1&&(i.object.backgroundIntensity=this.backgroundIntensity),i.object.backgroundRotation=this.backgroundRotation.toArray(),this.environmentIntensity!==1&&(i.object.environmentIntensity=this.environmentIntensity),i.object.environmentRotation=this.environmentRotation.toArray(),i}}class dE{constructor(t,i){this.isInterleavedBuffer=!0,this.array=t,this.stride=i,this.count=t!==void 0?t.length/i:0,this.usage=Jd,this.updateRanges=[],this.version=0,this.uuid=fa()}onUploadCallback(){}set needsUpdate(t){t===!0&&this.version++}setUsage(t){return this.usage=t,this}addUpdateRange(t,i){this.updateRanges.push({start:t,count:i})}clearUpdateRanges(){this.updateRanges.length=0}copy(t){return this.array=new t.array.constructor(t.array),this.count=t.count,this.stride=t.stride,this.usage=t.usage,this}copyAt(t,i,s){t*=this.stride,s*=i.stride;for(let l=0,c=this.stride;l<c;l++)this.array[t+l]=i.array[s+l];return this}set(t,i=0){return this.array.set(t,i),this}clone(t){t.arrayBuffers===void 0&&(t.arrayBuffers={}),this.array.buffer._uuid===void 0&&(this.array.buffer._uuid=fa()),t.arrayBuffers[this.array.buffer._uuid]===void 0&&(t.arrayBuffers[this.array.buffer._uuid]=this.array.slice(0).buffer);const i=new this.array.constructor(t.arrayBuffers[this.array.buffer._uuid]),s=new this.constructor(i,this.stride);return s.setUsage(this.usage),s}onUpload(t){return this.onUploadCallback=t,this}toJSON(t){return t.arrayBuffers===void 0&&(t.arrayBuffers={}),this.array.buffer._uuid===void 0&&(this.array.buffer._uuid=fa()),t.arrayBuffers[this.array.buffer._uuid]===void 0&&(t.arrayBuffers[this.array.buffer._uuid]=Array.from(new Uint32Array(this.array.buffer))),{uuid:this.uuid,buffer:this.array.buffer._uuid,type:this.array.constructor.name,stride:this.stride}}}const Bn=new X;class ru{constructor(t,i,s,l=!1){this.isInterleavedBufferAttribute=!0,this.name="",this.data=t,this.itemSize=i,this.offset=s,this.normalized=l}get count(){return this.data.count}get array(){return this.data.array}set needsUpdate(t){this.data.needsUpdate=t}applyMatrix4(t){for(let i=0,s=this.data.count;i<s;i++)Bn.fromBufferAttribute(this,i),Bn.applyMatrix4(t),this.setXYZ(i,Bn.x,Bn.y,Bn.z);return this}applyNormalMatrix(t){for(let i=0,s=this.count;i<s;i++)Bn.fromBufferAttribute(this,i),Bn.applyNormalMatrix(t),this.setXYZ(i,Bn.x,Bn.y,Bn.z);return this}transformDirection(t){for(let i=0,s=this.count;i<s;i++)Bn.fromBufferAttribute(this,i),Bn.transformDirection(t),this.setXYZ(i,Bn.x,Bn.y,Bn.z);return this}getComponent(t,i){let s=this.array[t*this.data.stride+this.offset+i];return this.normalized&&(s=Ai(s,this.array)),s}setComponent(t,i,s){return this.normalized&&(s=Ge(s,this.array)),this.data.array[t*this.data.stride+this.offset+i]=s,this}setX(t,i){return this.normalized&&(i=Ge(i,this.array)),this.data.array[t*this.data.stride+this.offset]=i,this}setY(t,i){return this.normalized&&(i=Ge(i,this.array)),this.data.array[t*this.data.stride+this.offset+1]=i,this}setZ(t,i){return this.normalized&&(i=Ge(i,this.array)),this.data.array[t*this.data.stride+this.offset+2]=i,this}setW(t,i){return this.normalized&&(i=Ge(i,this.array)),this.data.array[t*this.data.stride+this.offset+3]=i,this}getX(t){let i=this.data.array[t*this.data.stride+this.offset];return this.normalized&&(i=Ai(i,this.array)),i}getY(t){let i=this.data.array[t*this.data.stride+this.offset+1];return this.normalized&&(i=Ai(i,this.array)),i}getZ(t){let i=this.data.array[t*this.data.stride+this.offset+2];return this.normalized&&(i=Ai(i,this.array)),i}getW(t){let i=this.data.array[t*this.data.stride+this.offset+3];return this.normalized&&(i=Ai(i,this.array)),i}setXY(t,i,s){return t=t*this.data.stride+this.offset,this.normalized&&(i=Ge(i,this.array),s=Ge(s,this.array)),this.data.array[t+0]=i,this.data.array[t+1]=s,this}setXYZ(t,i,s,l){return t=t*this.data.stride+this.offset,this.normalized&&(i=Ge(i,this.array),s=Ge(s,this.array),l=Ge(l,this.array)),this.data.array[t+0]=i,this.data.array[t+1]=s,this.data.array[t+2]=l,this}setXYZW(t,i,s,l,c){return t=t*this.data.stride+this.offset,this.normalized&&(i=Ge(i,this.array),s=Ge(s,this.array),l=Ge(l,this.array),c=Ge(c,this.array)),this.data.array[t+0]=i,this.data.array[t+1]=s,this.data.array[t+2]=l,this.data.array[t+3]=c,this}clone(t){if(t===void 0){console.log("THREE.InterleavedBufferAttribute.clone(): Cloning an interleaved buffer attribute will de-interleave buffer data.");const i=[];for(let s=0;s<this.count;s++){const l=s*this.data.stride+this.offset;for(let c=0;c<this.itemSize;c++)i.push(this.data.array[l+c])}return new yi(new this.array.constructor(i),this.itemSize,this.normalized)}else return t.interleavedBuffers===void 0&&(t.interleavedBuffers={}),t.interleavedBuffers[this.data.uuid]===void 0&&(t.interleavedBuffers[this.data.uuid]=this.data.clone(t)),new ru(t.interleavedBuffers[this.data.uuid],this.itemSize,this.offset,this.normalized)}toJSON(t){if(t===void 0){console.log("THREE.InterleavedBufferAttribute.toJSON(): Serializing an interleaved buffer attribute will de-interleave buffer data.");const i=[];for(let s=0;s<this.count;s++){const l=s*this.data.stride+this.offset;for(let c=0;c<this.itemSize;c++)i.push(this.data.array[l+c])}return{itemSize:this.itemSize,type:this.array.constructor.name,array:i,normalized:this.normalized}}else return t.interleavedBuffers===void 0&&(t.interleavedBuffers={}),t.interleavedBuffers[this.data.uuid]===void 0&&(t.interleavedBuffers[this.data.uuid]=this.data.toJSON(t)),{isInterleavedBufferAttribute:!0,itemSize:this.itemSize,data:this.data.uuid,offset:this.offset,normalized:this.normalized}}}class ny extends Ls{constructor(t){super(),this.isSpriteMaterial=!0,this.type="SpriteMaterial",this.color=new Me(16777215),this.map=null,this.alphaMap=null,this.rotation=0,this.sizeAttenuation=!0,this.transparent=!0,this.fog=!0,this.setValues(t)}copy(t){return super.copy(t),this.color.copy(t.color),this.map=t.map,this.alphaMap=t.alphaMap,this.rotation=t.rotation,this.sizeAttenuation=t.sizeAttenuation,this.fog=t.fog,this}}let Cr;const Wo=new X,wr=new X,Dr=new X,Ur=new re,jo=new re,iy=new qe,Ic=new X,Yo=new X,Fc=new X,z0=new re,Qh=new re,B0=new re;class pE extends Mn{constructor(t=new ny){if(super(),this.isSprite=!0,this.type="Sprite",Cr===void 0){Cr=new ri;const i=new Float32Array([-.5,-.5,0,0,0,.5,-.5,0,1,0,.5,.5,0,1,1,-.5,.5,0,0,1]),s=new dE(i,5);Cr.setIndex([0,1,2,0,2,3]),Cr.setAttribute("position",new ru(s,3,0,!1)),Cr.setAttribute("uv",new ru(s,2,3,!1))}this.geometry=Cr,this.material=t,this.center=new re(.5,.5),this.count=1}raycast(t,i){t.camera===null&&console.error('THREE.Sprite: "Raycaster.camera" needs to be set in order to raycast against sprites.'),wr.setFromMatrixScale(this.matrixWorld),iy.copy(t.camera.matrixWorld),this.modelViewMatrix.multiplyMatrices(t.camera.matrixWorldInverse,this.matrixWorld),Dr.setFromMatrixPosition(this.modelViewMatrix),t.camera.isPerspectiveCamera&&this.material.sizeAttenuation===!1&&wr.multiplyScalar(-Dr.z);const s=this.material.rotation;let l,c;s!==0&&(c=Math.cos(s),l=Math.sin(s));const h=this.center;Hc(Ic.set(-.5,-.5,0),Dr,h,wr,l,c),Hc(Yo.set(.5,-.5,0),Dr,h,wr,l,c),Hc(Fc.set(.5,.5,0),Dr,h,wr,l,c),z0.set(0,0),Qh.set(1,0),B0.set(1,1);let d=t.ray.intersectTriangle(Ic,Yo,Fc,!1,Wo);if(d===null&&(Hc(Yo.set(-.5,.5,0),Dr,h,wr,l,c),Qh.set(0,1),d=t.ray.intersectTriangle(Ic,Fc,Yo,!1,Wo),d===null))return;const m=t.ray.origin.distanceTo(Wo);m<t.near||m>t.far||i.push({distance:m,point:Wo.clone(),uv:_i.getInterpolation(Wo,Ic,Yo,Fc,z0,Qh,B0,new re),face:null,object:this})}copy(t,i){return super.copy(t,i),t.center!==void 0&&this.center.copy(t.center),this.material=t.material,this}}function Hc(r,t,i,s,l,c){Ur.subVectors(r,i).addScalar(.5).multiply(s),l!==void 0?(jo.x=c*Ur.x-l*Ur.y,jo.y=l*Ur.x+c*Ur.y):jo.copy(Ur),r.copy(t),r.x+=jo.x,r.y+=jo.y,r.applyMatrix4(iy)}class mE extends On{constructor(t=null,i=1,s=1,l,c,h,d,m,p=si,v=si,g,x){super(null,h,d,m,p,v,l,c,g,x),this.isDataTexture=!0,this.image={data:t,width:i,height:s},this.generateMipmaps=!1,this.flipY=!1,this.unpackAlignment=1}}class I0 extends yi{constructor(t,i,s,l=1){super(t,i,s),this.isInstancedBufferAttribute=!0,this.meshPerAttribute=l}copy(t){return super.copy(t),this.meshPerAttribute=t.meshPerAttribute,this}toJSON(){const t=super.toJSON();return t.meshPerAttribute=this.meshPerAttribute,t.isInstancedBufferAttribute=!0,t}}const Lr=new qe,F0=new qe,Gc=[],H0=new da,gE=new qe,qo=new vi,Zo=new Wr;class G0 extends vi{constructor(t,i,s){super(t,i),this.isInstancedMesh=!0,this.instanceMatrix=new I0(new Float32Array(s*16),16),this.instanceColor=null,this.morphTexture=null,this.count=s,this.boundingBox=null,this.boundingSphere=null;for(let l=0;l<s;l++)this.setMatrixAt(l,gE)}computeBoundingBox(){const t=this.geometry,i=this.count;this.boundingBox===null&&(this.boundingBox=new da),t.boundingBox===null&&t.computeBoundingBox(),this.boundingBox.makeEmpty();for(let s=0;s<i;s++)this.getMatrixAt(s,Lr),H0.copy(t.boundingBox).applyMatrix4(Lr),this.boundingBox.union(H0)}computeBoundingSphere(){const t=this.geometry,i=this.count;this.boundingSphere===null&&(this.boundingSphere=new Wr),t.boundingSphere===null&&t.computeBoundingSphere(),this.boundingSphere.makeEmpty();for(let s=0;s<i;s++)this.getMatrixAt(s,Lr),Zo.copy(t.boundingSphere).applyMatrix4(Lr),this.boundingSphere.union(Zo)}copy(t,i){return super.copy(t,i),this.instanceMatrix.copy(t.instanceMatrix),t.morphTexture!==null&&(this.morphTexture=t.morphTexture.clone()),t.instanceColor!==null&&(this.instanceColor=t.instanceColor.clone()),this.count=t.count,t.boundingBox!==null&&(this.boundingBox=t.boundingBox.clone()),t.boundingSphere!==null&&(this.boundingSphere=t.boundingSphere.clone()),this}getColorAt(t,i){i.fromArray(this.instanceColor.array,t*3)}getMatrixAt(t,i){i.fromArray(this.instanceMatrix.array,t*16)}getMorphAt(t,i){const s=i.morphTargetInfluences,l=this.morphTexture.source.data.data,c=s.length+1,h=t*c+1;for(let d=0;d<s.length;d++)s[d]=l[h+d]}raycast(t,i){const s=this.matrixWorld,l=this.count;if(qo.geometry=this.geometry,qo.material=this.material,qo.material!==void 0&&(this.boundingSphere===null&&this.computeBoundingSphere(),Zo.copy(this.boundingSphere),Zo.applyMatrix4(s),t.ray.intersectsSphere(Zo)!==!1))for(let c=0;c<l;c++){this.getMatrixAt(c,Lr),F0.multiplyMatrices(s,Lr),qo.matrixWorld=F0,qo.raycast(t,Gc);for(let h=0,d=Gc.length;h<d;h++){const m=Gc[h];m.instanceId=c,m.object=this,i.push(m)}Gc.length=0}}setColorAt(t,i){this.instanceColor===null&&(this.instanceColor=new I0(new Float32Array(this.instanceMatrix.count*3).fill(1),3)),i.toArray(this.instanceColor.array,t*3)}setMatrixAt(t,i){i.toArray(this.instanceMatrix.array,t*16)}setMorphAt(t,i){const s=i.morphTargetInfluences,l=s.length+1;this.morphTexture===null&&(this.morphTexture=new mE(new Float32Array(l*this.count),l,this.count,rp,zi));const c=this.morphTexture.source.data.data;let h=0;for(let p=0;p<s.length;p++)h+=s[p];const d=this.geometry.morphTargetsRelative?1:1-h,m=l*t;c[m]=d,c.set(s,m+1)}updateMorphTargets(){}dispose(){this.dispatchEvent({type:"dispose"}),this.morphTexture!==null&&(this.morphTexture.dispose(),this.morphTexture=null)}}const Jh=new X,_E=new X,vE=new me;class Xa{constructor(t=new X(1,0,0),i=0){this.isPlane=!0,this.normal=t,this.constant=i}set(t,i){return this.normal.copy(t),this.constant=i,this}setComponents(t,i,s,l){return this.normal.set(t,i,s),this.constant=l,this}setFromNormalAndCoplanarPoint(t,i){return this.normal.copy(t),this.constant=-i.dot(this.normal),this}setFromCoplanarPoints(t,i,s){const l=Jh.subVectors(s,i).cross(_E.subVectors(t,i)).normalize();return this.setFromNormalAndCoplanarPoint(l,t),this}copy(t){return this.normal.copy(t.normal),this.constant=t.constant,this}normalize(){const t=1/this.normal.length();return this.normal.multiplyScalar(t),this.constant*=t,this}negate(){return this.constant*=-1,this.normal.negate(),this}distanceToPoint(t){return this.normal.dot(t)+this.constant}distanceToSphere(t){return this.distanceToPoint(t.center)-t.radius}projectPoint(t,i){return i.copy(t).addScaledVector(this.normal,-this.distanceToPoint(t))}intersectLine(t,i){const s=t.delta(Jh),l=this.normal.dot(s);if(l===0)return this.distanceToPoint(t.start)===0?i.copy(t.start):null;const c=-(t.start.dot(this.normal)+this.constant)/l;return c<0||c>1?null:i.copy(t.start).addScaledVector(s,c)}intersectsLine(t){const i=this.distanceToPoint(t.start),s=this.distanceToPoint(t.end);return i<0&&s>0||s<0&&i>0}intersectsBox(t){return t.intersectsPlane(this)}intersectsSphere(t){return t.intersectsPlane(this)}coplanarPoint(t){return t.copy(this.normal).multiplyScalar(-this.constant)}applyMatrix4(t,i){const s=i||vE.getNormalMatrix(t),l=this.coplanarPoint(Jh).applyMatrix4(t),c=this.normal.applyMatrix3(s).normalize();return this.constant=-l.dot(c),this}translate(t){return this.constant-=t.dot(this.normal),this}equals(t){return t.normal.equals(this.normal)&&t.constant===this.constant}clone(){return new this.constructor().copy(this)}}const vs=new Wr,yE=new re(.5,.5),Vc=new X;class dp{constructor(t=new Xa,i=new Xa,s=new Xa,l=new Xa,c=new Xa,h=new Xa){this.planes=[t,i,s,l,c,h]}set(t,i,s,l,c,h){const d=this.planes;return d[0].copy(t),d[1].copy(i),d[2].copy(s),d[3].copy(l),d[4].copy(c),d[5].copy(h),this}copy(t){const i=this.planes;for(let s=0;s<6;s++)i[s].copy(t.planes[s]);return this}setFromProjectionMatrix(t,i=Bi,s=!1){const l=this.planes,c=t.elements,h=c[0],d=c[1],m=c[2],p=c[3],v=c[4],g=c[5],x=c[6],S=c[7],b=c[8],R=c[9],M=c[10],_=c[11],L=c[12],U=c[13],T=c[14],P=c[15];if(l[0].setComponents(p-h,S-v,_-b,P-L).normalize(),l[1].setComponents(p+h,S+v,_+b,P+L).normalize(),l[2].setComponents(p+d,S+g,_+R,P+U).normalize(),l[3].setComponents(p-d,S-g,_-R,P-U).normalize(),s)l[4].setComponents(m,x,M,T).normalize(),l[5].setComponents(p-m,S-x,_-M,P-T).normalize();else if(l[4].setComponents(p-m,S-x,_-M,P-T).normalize(),i===Bi)l[5].setComponents(p+m,S+x,_+M,P+T).normalize();else if(i===iu)l[5].setComponents(m,x,M,T).normalize();else throw new Error("THREE.Frustum.setFromProjectionMatrix(): Invalid coordinate system: "+i);return this}intersectsObject(t){if(t.boundingSphere!==void 0)t.boundingSphere===null&&t.computeBoundingSphere(),vs.copy(t.boundingSphere).applyMatrix4(t.matrixWorld);else{const i=t.geometry;i.boundingSphere===null&&i.computeBoundingSphere(),vs.copy(i.boundingSphere).applyMatrix4(t.matrixWorld)}return this.intersectsSphere(vs)}intersectsSprite(t){vs.center.set(0,0,0);const i=yE.distanceTo(t.center);return vs.radius=.7071067811865476+i,vs.applyMatrix4(t.matrixWorld),this.intersectsSphere(vs)}intersectsSphere(t){const i=this.planes,s=t.center,l=-t.radius;for(let c=0;c<6;c++)if(i[c].distanceToPoint(s)<l)return!1;return!0}intersectsBox(t){const i=this.planes;for(let s=0;s<6;s++){const l=i[s];if(Vc.x=l.normal.x>0?t.max.x:t.min.x,Vc.y=l.normal.y>0?t.max.y:t.min.y,Vc.z=l.normal.z>0?t.max.z:t.min.z,l.distanceToPoint(Vc)<0)return!1}return!0}containsPoint(t){const i=this.planes;for(let s=0;s<6;s++)if(i[s].distanceToPoint(t)<0)return!1;return!0}clone(){return new this.constructor().copy(this)}}class ay extends Ls{constructor(t){super(),this.isLineBasicMaterial=!0,this.type="LineBasicMaterial",this.color=new Me(16777215),this.map=null,this.linewidth=1,this.linecap="round",this.linejoin="round",this.fog=!0,this.setValues(t)}copy(t){return super.copy(t),this.color.copy(t.color),this.map=t.map,this.linewidth=t.linewidth,this.linecap=t.linecap,this.linejoin=t.linejoin,this.fog=t.fog,this}}const ou=new X,lu=new X,V0=new qe,Ko=new hu,kc=new Wr,$h=new X,k0=new X;class xE extends Mn{constructor(t=new ri,i=new ay){super(),this.isLine=!0,this.type="Line",this.geometry=t,this.material=i,this.morphTargetDictionary=void 0,this.morphTargetInfluences=void 0,this.updateMorphTargets()}copy(t,i){return super.copy(t,i),this.material=Array.isArray(t.material)?t.material.slice():t.material,this.geometry=t.geometry,this}computeLineDistances(){const t=this.geometry;if(t.index===null){const i=t.attributes.position,s=[0];for(let l=1,c=i.count;l<c;l++)ou.fromBufferAttribute(i,l-1),lu.fromBufferAttribute(i,l),s[l]=s[l-1],s[l]+=ou.distanceTo(lu);t.setAttribute("lineDistance",new dn(s,1))}else console.warn("THREE.Line.computeLineDistances(): Computation only possible with non-indexed BufferGeometry.");return this}raycast(t,i){const s=this.geometry,l=this.matrixWorld,c=t.params.Line.threshold,h=s.drawRange;if(s.boundingSphere===null&&s.computeBoundingSphere(),kc.copy(s.boundingSphere),kc.applyMatrix4(l),kc.radius+=c,t.ray.intersectsSphere(kc)===!1)return;V0.copy(l).invert(),Ko.copy(t.ray).applyMatrix4(V0);const d=c/((this.scale.x+this.scale.y+this.scale.z)/3),m=d*d,p=this.isLineSegments?2:1,v=s.index,x=s.attributes.position;if(v!==null){const S=Math.max(0,h.start),b=Math.min(v.count,h.start+h.count);for(let R=S,M=b-1;R<M;R+=p){const _=v.getX(R),L=v.getX(R+1),U=Xc(this,t,Ko,m,_,L,R);U&&i.push(U)}if(this.isLineLoop){const R=v.getX(b-1),M=v.getX(S),_=Xc(this,t,Ko,m,R,M,b-1);_&&i.push(_)}}else{const S=Math.max(0,h.start),b=Math.min(x.count,h.start+h.count);for(let R=S,M=b-1;R<M;R+=p){const _=Xc(this,t,Ko,m,R,R+1,R);_&&i.push(_)}if(this.isLineLoop){const R=Xc(this,t,Ko,m,b-1,S,b-1);R&&i.push(R)}}}updateMorphTargets(){const i=this.geometry.morphAttributes,s=Object.keys(i);if(s.length>0){const l=i[s[0]];if(l!==void 0){this.morphTargetInfluences=[],this.morphTargetDictionary={};for(let c=0,h=l.length;c<h;c++){const d=l[c].name||String(c);this.morphTargetInfluences.push(0),this.morphTargetDictionary[d]=c}}}}}function Xc(r,t,i,s,l,c,h){const d=r.geometry.attributes.position;if(ou.fromBufferAttribute(d,l),lu.fromBufferAttribute(d,c),i.distanceSqToSegment(ou,lu,$h,k0)>s)return;$h.applyMatrix4(r.matrixWorld);const p=t.ray.origin.distanceTo($h);if(!(p<t.near||p>t.far))return{distance:p,point:k0.clone().applyMatrix4(r.matrixWorld),index:h,face:null,faceIndex:null,barycoord:null,object:r}}const X0=new X,W0=new X;class SE extends xE{constructor(t,i){super(t,i),this.isLineSegments=!0,this.type="LineSegments"}computeLineDistances(){const t=this.geometry;if(t.index===null){const i=t.attributes.position,s=[];for(let l=0,c=i.count;l<c;l+=2)X0.fromBufferAttribute(i,l),W0.fromBufferAttribute(i,l+1),s[l]=l===0?0:s[l-1],s[l+1]=s[l]+X0.distanceTo(W0);t.setAttribute("lineDistance",new dn(s,1))}else console.warn("THREE.LineSegments.computeLineDistances(): Computation only possible with non-indexed BufferGeometry.");return this}}class ME extends On{constructor(t,i,s,l,c,h,d,m,p){super(t,i,s,l,c,h,d,m,p),this.isCanvasTexture=!0,this.needsUpdate=!0}}class sy extends On{constructor(t,i,s=Cs,l,c,h,d=si,m=si,p,v=sl,g=1){if(v!==sl&&v!==rl)throw new Error("DepthTexture format must be either THREE.DepthFormat or THREE.DepthStencilFormat");const x={width:t,height:i,depth:g};super(x,l,c,h,d,m,v,s,p),this.isDepthTexture=!0,this.flipY=!1,this.generateMipmaps=!1,this.compareFunction=null}copy(t){return super.copy(t),this.source=new fp(Object.assign({},t.image)),this.compareFunction=t.compareFunction,this}toJSON(t){const i=super.toJSON(t);return this.compareFunction!==null&&(i.compareFunction=this.compareFunction),i}}class ry extends On{constructor(t=null){super(),this.sourceTexture=t,this.isExternalTexture=!0}copy(t){return super.copy(t),this.sourceTexture=t.sourceTexture,this}}class pp extends ri{constructor(t=[],i=[],s=1,l=0){super(),this.type="PolyhedronGeometry",this.parameters={vertices:t,indices:i,radius:s,detail:l};const c=[],h=[];d(l),p(s),v(),this.setAttribute("position",new dn(c,3)),this.setAttribute("normal",new dn(c.slice(),3)),this.setAttribute("uv",new dn(h,2)),l===0?this.computeVertexNormals():this.normalizeNormals();function d(L){const U=new X,T=new X,P=new X;for(let B=0;B<i.length;B+=3)S(i[B+0],U),S(i[B+1],T),S(i[B+2],P),m(U,T,P,L)}function m(L,U,T,P){const B=P+1,F=[];for(let G=0;G<=B;G++){F[G]=[];const D=L.clone().lerp(T,G/B),w=U.clone().lerp(T,G/B),V=B-G;for(let K=0;K<=V;K++)K===0&&G===B?F[G][K]=D:F[G][K]=D.clone().lerp(w,K/V)}for(let G=0;G<B;G++)for(let D=0;D<2*(B-G)-1;D++){const w=Math.floor(D/2);D%2===0?(x(F[G][w+1]),x(F[G+1][w]),x(F[G][w])):(x(F[G][w+1]),x(F[G+1][w+1]),x(F[G+1][w]))}}function p(L){const U=new X;for(let T=0;T<c.length;T+=3)U.x=c[T+0],U.y=c[T+1],U.z=c[T+2],U.normalize().multiplyScalar(L),c[T+0]=U.x,c[T+1]=U.y,c[T+2]=U.z}function v(){const L=new X;for(let U=0;U<c.length;U+=3){L.x=c[U+0],L.y=c[U+1],L.z=c[U+2];const T=M(L)/2/Math.PI+.5,P=_(L)/Math.PI+.5;h.push(T,1-P)}b(),g()}function g(){for(let L=0;L<h.length;L+=6){const U=h[L+0],T=h[L+2],P=h[L+4],B=Math.max(U,T,P),F=Math.min(U,T,P);B>.9&&F<.1&&(U<.2&&(h[L+0]+=1),T<.2&&(h[L+2]+=1),P<.2&&(h[L+4]+=1))}}function x(L){c.push(L.x,L.y,L.z)}function S(L,U){const T=L*3;U.x=t[T+0],U.y=t[T+1],U.z=t[T+2]}function b(){const L=new X,U=new X,T=new X,P=new X,B=new re,F=new re,G=new re;for(let D=0,w=0;D<c.length;D+=9,w+=6){L.set(c[D+0],c[D+1],c[D+2]),U.set(c[D+3],c[D+4],c[D+5]),T.set(c[D+6],c[D+7],c[D+8]),B.set(h[w+0],h[w+1]),F.set(h[w+2],h[w+3]),G.set(h[w+4],h[w+5]),P.copy(L).add(U).add(T).divideScalar(3);const V=M(P);R(B,w+0,L,V),R(F,w+2,U,V),R(G,w+4,T,V)}}function R(L,U,T,P){P<0&&L.x===1&&(h[U]=L.x-1),T.x===0&&T.z===0&&(h[U]=P/2/Math.PI+.5)}function M(L){return Math.atan2(L.z,-L.x)}function _(L){return Math.atan2(-L.y,Math.sqrt(L.x*L.x+L.z*L.z))}}copy(t){return super.copy(t),this.parameters=Object.assign({},t.parameters),this}static fromJSON(t){return new pp(t.vertices,t.indices,t.radius,t.details)}}class mp extends pp{constructor(t=1,i=0){const s=[1,0,0,-1,0,0,0,1,0,0,-1,0,0,0,1,0,0,-1],l=[0,2,4,0,4,3,0,3,5,0,5,2,1,2,5,1,5,3,1,3,4,1,4,2];super(s,l,t,i),this.type="OctahedronGeometry",this.parameters={radius:t,detail:i}}static fromJSON(t){return new mp(t.radius,t.detail)}}class du extends ri{constructor(t=1,i=1,s=1,l=1){super(),this.type="PlaneGeometry",this.parameters={width:t,height:i,widthSegments:s,heightSegments:l};const c=t/2,h=i/2,d=Math.floor(s),m=Math.floor(l),p=d+1,v=m+1,g=t/d,x=i/m,S=[],b=[],R=[],M=[];for(let _=0;_<v;_++){const L=_*x-h;for(let U=0;U<p;U++){const T=U*g-c;b.push(T,-L,0),R.push(0,0,1),M.push(U/d),M.push(1-_/m)}}for(let _=0;_<m;_++)for(let L=0;L<d;L++){const U=L+p*_,T=L+p*(_+1),P=L+1+p*(_+1),B=L+1+p*_;S.push(U,T,B),S.push(T,P,B)}this.setIndex(S),this.setAttribute("position",new dn(b,3)),this.setAttribute("normal",new dn(R,3)),this.setAttribute("uv",new dn(M,2))}copy(t){return super.copy(t),this.parameters=Object.assign({},t.parameters),this}static fromJSON(t){return new du(t.width,t.height,t.widthSegments,t.heightSegments)}}class cu extends ri{constructor(t=1,i=32,s=16,l=0,c=Math.PI*2,h=0,d=Math.PI){super(),this.type="SphereGeometry",this.parameters={radius:t,widthSegments:i,heightSegments:s,phiStart:l,phiLength:c,thetaStart:h,thetaLength:d},i=Math.max(3,Math.floor(i)),s=Math.max(2,Math.floor(s));const m=Math.min(h+d,Math.PI);let p=0;const v=[],g=new X,x=new X,S=[],b=[],R=[],M=[];for(let _=0;_<=s;_++){const L=[],U=_/s;let T=0;_===0&&h===0?T=.5/i:_===s&&m===Math.PI&&(T=-.5/i);for(let P=0;P<=i;P++){const B=P/i;g.x=-t*Math.cos(l+B*c)*Math.sin(h+U*d),g.y=t*Math.cos(h+U*d),g.z=t*Math.sin(l+B*c)*Math.sin(h+U*d),b.push(g.x,g.y,g.z),x.copy(g).normalize(),R.push(x.x,x.y,x.z),M.push(B+T,1-U),L.push(p++)}v.push(L)}for(let _=0;_<s;_++)for(let L=0;L<i;L++){const U=v[_][L+1],T=v[_][L],P=v[_+1][L],B=v[_+1][L+1];(_!==0||h>0)&&S.push(U,T,B),(_!==s-1||m<Math.PI)&&S.push(T,P,B)}this.setIndex(S),this.setAttribute("position",new dn(b,3)),this.setAttribute("normal",new dn(R,3)),this.setAttribute("uv",new dn(M,2))}copy(t){return super.copy(t),this.parameters=Object.assign({},t.parameters),this}static fromJSON(t){return new cu(t.radius,t.widthSegments,t.heightSegments,t.phiStart,t.phiLength,t.thetaStart,t.thetaLength)}}class uu extends ri{constructor(t=1,i=.4,s=12,l=48,c=Math.PI*2){super(),this.type="TorusGeometry",this.parameters={radius:t,tube:i,radialSegments:s,tubularSegments:l,arc:c},s=Math.floor(s),l=Math.floor(l);const h=[],d=[],m=[],p=[],v=new X,g=new X,x=new X;for(let S=0;S<=s;S++)for(let b=0;b<=l;b++){const R=b/l*c,M=S/s*Math.PI*2;g.x=(t+i*Math.cos(M))*Math.cos(R),g.y=(t+i*Math.cos(M))*Math.sin(R),g.z=i*Math.sin(M),d.push(g.x,g.y,g.z),v.x=t*Math.cos(R),v.y=t*Math.sin(R),x.subVectors(g,v).normalize(),m.push(x.x,x.y,x.z),p.push(b/l),p.push(S/s)}for(let S=1;S<=s;S++)for(let b=1;b<=l;b++){const R=(l+1)*S+b-1,M=(l+1)*(S-1)+b-1,_=(l+1)*(S-1)+b,L=(l+1)*S+b;h.push(R,M,L),h.push(M,_,L)}this.setIndex(h),this.setAttribute("position",new dn(d,3)),this.setAttribute("normal",new dn(m,3)),this.setAttribute("uv",new dn(p,2))}copy(t){return super.copy(t),this.parameters=Object.assign({},t.parameters),this}static fromJSON(t){return new uu(t.radius,t.tube,t.radialSegments,t.tubularSegments,t.arc)}}class EE extends Ls{constructor(t){super(),this.isMeshStandardMaterial=!0,this.type="MeshStandardMaterial",this.defines={STANDARD:""},this.color=new Me(16777215),this.roughness=1,this.metalness=0,this.map=null,this.lightMap=null,this.lightMapIntensity=1,this.aoMap=null,this.aoMapIntensity=1,this.emissive=new Me(0),this.emissiveIntensity=1,this.emissiveMap=null,this.bumpMap=null,this.bumpScale=1,this.normalMap=null,this.normalMapType=Wv,this.normalScale=new re(1,1),this.displacementMap=null,this.displacementScale=1,this.displacementBias=0,this.roughnessMap=null,this.metalnessMap=null,this.alphaMap=null,this.envMap=null,this.envMapRotation=new Fi,this.envMapIntensity=1,this.wireframe=!1,this.wireframeLinewidth=1,this.wireframeLinecap="round",this.wireframeLinejoin="round",this.flatShading=!1,this.fog=!0,this.setValues(t)}copy(t){return super.copy(t),this.defines={STANDARD:""},this.color.copy(t.color),this.roughness=t.roughness,this.metalness=t.metalness,this.map=t.map,this.lightMap=t.lightMap,this.lightMapIntensity=t.lightMapIntensity,this.aoMap=t.aoMap,this.aoMapIntensity=t.aoMapIntensity,this.emissive.copy(t.emissive),this.emissiveMap=t.emissiveMap,this.emissiveIntensity=t.emissiveIntensity,this.bumpMap=t.bumpMap,this.bumpScale=t.bumpScale,this.normalMap=t.normalMap,this.normalMapType=t.normalMapType,this.normalScale.copy(t.normalScale),this.displacementMap=t.displacementMap,this.displacementScale=t.displacementScale,this.displacementBias=t.displacementBias,this.roughnessMap=t.roughnessMap,this.metalnessMap=t.metalnessMap,this.alphaMap=t.alphaMap,this.envMap=t.envMap,this.envMapRotation.copy(t.envMapRotation),this.envMapIntensity=t.envMapIntensity,this.wireframe=t.wireframe,this.wireframeLinewidth=t.wireframeLinewidth,this.wireframeLinecap=t.wireframeLinecap,this.wireframeLinejoin=t.wireframeLinejoin,this.flatShading=t.flatShading,this.fog=t.fog,this}}class TE extends EE{constructor(t){super(),this.isMeshPhysicalMaterial=!0,this.defines={STANDARD:"",PHYSICAL:""},this.type="MeshPhysicalMaterial",this.anisotropyRotation=0,this.anisotropyMap=null,this.clearcoatMap=null,this.clearcoatRoughness=0,this.clearcoatRoughnessMap=null,this.clearcoatNormalScale=new re(1,1),this.clearcoatNormalMap=null,this.ior=1.5,Object.defineProperty(this,"reflectivity",{get:function(){return ye(2.5*(this.ior-1)/(this.ior+1),0,1)},set:function(i){this.ior=(1+.4*i)/(1-.4*i)}}),this.iridescenceMap=null,this.iridescenceIOR=1.3,this.iridescenceThicknessRange=[100,400],this.iridescenceThicknessMap=null,this.sheenColor=new Me(0),this.sheenColorMap=null,this.sheenRoughness=1,this.sheenRoughnessMap=null,this.transmissionMap=null,this.thickness=0,this.thicknessMap=null,this.attenuationDistance=1/0,this.attenuationColor=new Me(1,1,1),this.specularIntensity=1,this.specularIntensityMap=null,this.specularColor=new Me(1,1,1),this.specularColorMap=null,this._anisotropy=0,this._clearcoat=0,this._dispersion=0,this._iridescence=0,this._sheen=0,this._transmission=0,this.setValues(t)}get anisotropy(){return this._anisotropy}set anisotropy(t){this._anisotropy>0!=t>0&&this.version++,this._anisotropy=t}get clearcoat(){return this._clearcoat}set clearcoat(t){this._clearcoat>0!=t>0&&this.version++,this._clearcoat=t}get iridescence(){return this._iridescence}set iridescence(t){this._iridescence>0!=t>0&&this.version++,this._iridescence=t}get dispersion(){return this._dispersion}set dispersion(t){this._dispersion>0!=t>0&&this.version++,this._dispersion=t}get sheen(){return this._sheen}set sheen(t){this._sheen>0!=t>0&&this.version++,this._sheen=t}get transmission(){return this._transmission}set transmission(t){this._transmission>0!=t>0&&this.version++,this._transmission=t}copy(t){return super.copy(t),this.defines={STANDARD:"",PHYSICAL:""},this.anisotropy=t.anisotropy,this.anisotropyRotation=t.anisotropyRotation,this.anisotropyMap=t.anisotropyMap,this.clearcoat=t.clearcoat,this.clearcoatMap=t.clearcoatMap,this.clearcoatRoughness=t.clearcoatRoughness,this.clearcoatRoughnessMap=t.clearcoatRoughnessMap,this.clearcoatNormalMap=t.clearcoatNormalMap,this.clearcoatNormalScale.copy(t.clearcoatNormalScale),this.dispersion=t.dispersion,this.ior=t.ior,this.iridescence=t.iridescence,this.iridescenceMap=t.iridescenceMap,this.iridescenceIOR=t.iridescenceIOR,this.iridescenceThicknessRange=[...t.iridescenceThicknessRange],this.iridescenceThicknessMap=t.iridescenceThicknessMap,this.sheen=t.sheen,this.sheenColor.copy(t.sheenColor),this.sheenColorMap=t.sheenColorMap,this.sheenRoughness=t.sheenRoughness,this.sheenRoughnessMap=t.sheenRoughnessMap,this.transmission=t.transmission,this.transmissionMap=t.transmissionMap,this.thickness=t.thickness,this.thicknessMap=t.thicknessMap,this.attenuationDistance=t.attenuationDistance,this.attenuationColor.copy(t.attenuationColor),this.specularIntensity=t.specularIntensity,this.specularIntensityMap=t.specularIntensityMap,this.specularColor.copy(t.specularColor),this.specularColorMap=t.specularColorMap,this}}class bE extends Ls{constructor(t){super(),this.isMeshDepthMaterial=!0,this.type="MeshDepthMaterial",this.depthPacking=pM,this.map=null,this.alphaMap=null,this.displacementMap=null,this.displacementScale=1,this.displacementBias=0,this.wireframe=!1,this.wireframeLinewidth=1,this.setValues(t)}copy(t){return super.copy(t),this.depthPacking=t.depthPacking,this.map=t.map,this.alphaMap=t.alphaMap,this.displacementMap=t.displacementMap,this.displacementScale=t.displacementScale,this.displacementBias=t.displacementBias,this.wireframe=t.wireframe,this.wireframeLinewidth=t.wireframeLinewidth,this}}class AE extends Ls{constructor(t){super(),this.isMeshDistanceMaterial=!0,this.type="MeshDistanceMaterial",this.map=null,this.alphaMap=null,this.displacementMap=null,this.displacementScale=1,this.displacementBias=0,this.setValues(t)}copy(t){return super.copy(t),this.map=t.map,this.alphaMap=t.alphaMap,this.displacementMap=t.displacementMap,this.displacementScale=t.displacementScale,this.displacementBias=t.displacementBias,this}}class oy extends Mn{constructor(t,i=1){super(),this.isLight=!0,this.type="Light",this.color=new Me(t),this.intensity=i}dispose(){}copy(t,i){return super.copy(t,i),this.color.copy(t.color),this.intensity=t.intensity,this}toJSON(t){const i=super.toJSON(t);return i.object.color=this.color.getHex(),i.object.intensity=this.intensity,this.groundColor!==void 0&&(i.object.groundColor=this.groundColor.getHex()),this.distance!==void 0&&(i.object.distance=this.distance),this.angle!==void 0&&(i.object.angle=this.angle),this.decay!==void 0&&(i.object.decay=this.decay),this.penumbra!==void 0&&(i.object.penumbra=this.penumbra),this.shadow!==void 0&&(i.object.shadow=this.shadow.toJSON()),this.target!==void 0&&(i.object.target=this.target.uuid),i}}class RE extends oy{constructor(t,i,s){super(t,s),this.isHemisphereLight=!0,this.type="HemisphereLight",this.position.copy(Mn.DEFAULT_UP),this.updateMatrix(),this.groundColor=new Me(i)}copy(t,i){return super.copy(t,i),this.groundColor.copy(t.groundColor),this}}const td=new qe,j0=new X,Y0=new X;class CE{constructor(t){this.camera=t,this.intensity=1,this.bias=0,this.normalBias=0,this.radius=1,this.blurSamples=8,this.mapSize=new re(512,512),this.mapType=Ii,this.map=null,this.mapPass=null,this.matrix=new qe,this.autoUpdate=!0,this.needsUpdate=!1,this._frustum=new dp,this._frameExtents=new re(1,1),this._viewportCount=1,this._viewports=[new Ye(0,0,1,1)]}getViewportCount(){return this._viewportCount}getFrustum(){return this._frustum}updateMatrices(t){const i=this.camera,s=this.matrix;j0.setFromMatrixPosition(t.matrixWorld),i.position.copy(j0),Y0.setFromMatrixPosition(t.target.matrixWorld),i.lookAt(Y0),i.updateMatrixWorld(),td.multiplyMatrices(i.projectionMatrix,i.matrixWorldInverse),this._frustum.setFromProjectionMatrix(td,i.coordinateSystem,i.reversedDepth),i.reversedDepth?s.set(.5,0,0,.5,0,.5,0,.5,0,0,1,0,0,0,0,1):s.set(.5,0,0,.5,0,.5,0,.5,0,0,.5,.5,0,0,0,1),s.multiply(td)}getViewport(t){return this._viewports[t]}getFrameExtents(){return this._frameExtents}dispose(){this.map&&this.map.dispose(),this.mapPass&&this.mapPass.dispose()}copy(t){return this.camera=t.camera.clone(),this.intensity=t.intensity,this.bias=t.bias,this.radius=t.radius,this.autoUpdate=t.autoUpdate,this.needsUpdate=t.needsUpdate,this.normalBias=t.normalBias,this.blurSamples=t.blurSamples,this.mapSize.copy(t.mapSize),this}clone(){return new this.constructor().copy(this)}toJSON(){const t={};return this.intensity!==1&&(t.intensity=this.intensity),this.bias!==0&&(t.bias=this.bias),this.normalBias!==0&&(t.normalBias=this.normalBias),this.radius!==1&&(t.radius=this.radius),(this.mapSize.x!==512||this.mapSize.y!==512)&&(t.mapSize=this.mapSize.toArray()),t.camera=this.camera.toJSON(!1).object,delete t.camera.matrix,t}}const q0=new qe,Qo=new X,ed=new X;class wE extends CE{constructor(){super(new ai(90,1,.5,500)),this.isPointLightShadow=!0,this._frameExtents=new re(4,2),this._viewportCount=6,this._viewports=[new Ye(2,1,1,1),new Ye(0,1,1,1),new Ye(3,1,1,1),new Ye(1,1,1,1),new Ye(3,0,1,1),new Ye(1,0,1,1)],this._cubeDirections=[new X(1,0,0),new X(-1,0,0),new X(0,0,1),new X(0,0,-1),new X(0,1,0),new X(0,-1,0)],this._cubeUps=[new X(0,1,0),new X(0,1,0),new X(0,1,0),new X(0,1,0),new X(0,0,1),new X(0,0,-1)]}updateMatrices(t,i=0){const s=this.camera,l=this.matrix,c=t.distance||s.far;c!==s.far&&(s.far=c,s.updateProjectionMatrix()),Qo.setFromMatrixPosition(t.matrixWorld),s.position.copy(Qo),ed.copy(s.position),ed.add(this._cubeDirections[i]),s.up.copy(this._cubeUps[i]),s.lookAt(ed),s.updateMatrixWorld(),l.makeTranslation(-Qo.x,-Qo.y,-Qo.z),q0.multiplyMatrices(s.projectionMatrix,s.matrixWorldInverse),this._frustum.setFromProjectionMatrix(q0,s.coordinateSystem,s.reversedDepth)}}class Z0 extends oy{constructor(t,i,s=0,l=2){super(t,i),this.isPointLight=!0,this.type="PointLight",this.distance=s,this.decay=l,this.shadow=new wE}get power(){return this.intensity*4*Math.PI}set power(t){this.intensity=t/(4*Math.PI)}dispose(){this.shadow.dispose()}copy(t,i){return super.copy(t,i),this.distance=t.distance,this.decay=t.decay,this.shadow=t.shadow.clone(),this}}class DE extends ty{constructor(t=-1,i=1,s=1,l=-1,c=.1,h=2e3){super(),this.isOrthographicCamera=!0,this.type="OrthographicCamera",this.zoom=1,this.view=null,this.left=t,this.right=i,this.top=s,this.bottom=l,this.near=c,this.far=h,this.updateProjectionMatrix()}copy(t,i){return super.copy(t,i),this.left=t.left,this.right=t.right,this.top=t.top,this.bottom=t.bottom,this.near=t.near,this.far=t.far,this.zoom=t.zoom,this.view=t.view===null?null:Object.assign({},t.view),this}setViewOffset(t,i,s,l,c,h){this.view===null&&(this.view={enabled:!0,fullWidth:1,fullHeight:1,offsetX:0,offsetY:0,width:1,height:1}),this.view.enabled=!0,this.view.fullWidth=t,this.view.fullHeight=i,this.view.offsetX=s,this.view.offsetY=l,this.view.width=c,this.view.height=h,this.updateProjectionMatrix()}clearViewOffset(){this.view!==null&&(this.view.enabled=!1),this.updateProjectionMatrix()}updateProjectionMatrix(){const t=(this.right-this.left)/(2*this.zoom),i=(this.top-this.bottom)/(2*this.zoom),s=(this.right+this.left)/2,l=(this.top+this.bottom)/2;let c=s-t,h=s+t,d=l+i,m=l-i;if(this.view!==null&&this.view.enabled){const p=(this.right-this.left)/this.view.fullWidth/this.zoom,v=(this.top-this.bottom)/this.view.fullHeight/this.zoom;c+=p*this.view.offsetX,h=c+p*this.view.width,d-=v*this.view.offsetY,m=d-v*this.view.height}this.projectionMatrix.makeOrthographic(c,h,d,m,this.near,this.far,this.coordinateSystem,this.reversedDepth),this.projectionMatrixInverse.copy(this.projectionMatrix).invert()}toJSON(t){const i=super.toJSON(t);return i.object.zoom=this.zoom,i.object.left=this.left,i.object.right=this.right,i.object.top=this.top,i.object.bottom=this.bottom,i.object.near=this.near,i.object.far=this.far,this.view!==null&&(i.object.view=Object.assign({},this.view)),i}}class UE extends ai{constructor(t=[]){super(),this.isArrayCamera=!0,this.isMultiViewCamera=!1,this.cameras=t}}const K0=new qe;class LE{constructor(t,i,s=0,l=1/0){this.ray=new hu(t,i),this.near=s,this.far=l,this.camera=null,this.layers=new hp,this.params={Mesh:{},Line:{threshold:1},LOD:{},Points:{threshold:1},Sprite:{}}}set(t,i){this.ray.set(t,i)}setFromCamera(t,i){i.isPerspectiveCamera?(this.ray.origin.setFromMatrixPosition(i.matrixWorld),this.ray.direction.set(t.x,t.y,.5).unproject(i).sub(this.ray.origin).normalize(),this.camera=i):i.isOrthographicCamera?(this.ray.origin.set(t.x,t.y,(i.near+i.far)/(i.near-i.far)).unproject(i),this.ray.direction.set(0,0,-1).transformDirection(i.matrixWorld),this.camera=i):console.error("THREE.Raycaster: Unsupported camera type: "+i.type)}setFromXRController(t){return K0.identity().extractRotation(t.matrixWorld),this.ray.origin.setFromMatrixPosition(t.matrixWorld),this.ray.direction.set(0,0,-1).applyMatrix4(K0),this}intersectObject(t,i=!0,s=[]){return $d(t,this,s,i),s.sort(Q0),s}intersectObjects(t,i=!0,s=[]){for(let l=0,c=t.length;l<c;l++)$d(t[l],this,s,i);return s.sort(Q0),s}}function Q0(r,t){return r.distance-t.distance}function $d(r,t,i,s){let l=!0;if(r.layers.test(t.layers)&&r.raycast(t,i)===!1&&(l=!1),l===!0&&s===!0){const c=r.children;for(let h=0,d=c.length;h<d;h++)$d(c[h],t,i,!0)}}class J0{constructor(t=1,i=0,s=0){this.radius=t,this.phi=i,this.theta=s}set(t,i,s){return this.radius=t,this.phi=i,this.theta=s,this}copy(t){return this.radius=t.radius,this.phi=t.phi,this.theta=t.theta,this}makeSafe(){return this.phi=ye(this.phi,1e-6,Math.PI-1e-6),this}setFromVector3(t){return this.setFromCartesianCoords(t.x,t.y,t.z)}setFromCartesianCoords(t,i,s){return this.radius=Math.sqrt(t*t+i*i+s*s),this.radius===0?(this.theta=0,this.phi=0):(this.theta=Math.atan2(t,s),this.phi=Math.acos(ye(i/this.radius,-1,1))),this}clone(){return new this.constructor().copy(this)}}class NE extends Us{constructor(t,i=null){super(),this.object=t,this.domElement=i,this.enabled=!0,this.state=-1,this.keys={},this.mouseButtons={LEFT:null,MIDDLE:null,RIGHT:null},this.touches={ONE:null,TWO:null}}connect(t){if(t===void 0){console.warn("THREE.Controls: connect() now requires an element.");return}this.domElement!==null&&this.disconnect(),this.domElement=t}disconnect(){}dispose(){}update(){}}function $0(r,t,i,s){const l=OE(s);switch(i){case Vv:return r*t;case rp:return r*t/l.components*l.byteLength;case op:return r*t/l.components*l.byteLength;case Xv:return r*t*2/l.components*l.byteLength;case lp:return r*t*2/l.components*l.byteLength;case kv:return r*t*3/l.components*l.byteLength;case Ri:return r*t*4/l.components*l.byteLength;case cp:return r*t*4/l.components*l.byteLength;case Qc:case Jc:return Math.floor((r+3)/4)*Math.floor((t+3)/4)*8;case $c:case tu:return Math.floor((r+3)/4)*Math.floor((t+3)/4)*16;case bd:case Rd:return Math.max(r,16)*Math.max(t,8)/4;case Td:case Ad:return Math.max(r,8)*Math.max(t,8)/2;case Cd:case wd:return Math.floor((r+3)/4)*Math.floor((t+3)/4)*8;case Dd:return Math.floor((r+3)/4)*Math.floor((t+3)/4)*16;case Ud:return Math.floor((r+3)/4)*Math.floor((t+3)/4)*16;case Ld:return Math.floor((r+4)/5)*Math.floor((t+3)/4)*16;case Nd:return Math.floor((r+4)/5)*Math.floor((t+4)/5)*16;case Od:return Math.floor((r+5)/6)*Math.floor((t+4)/5)*16;case Pd:return Math.floor((r+5)/6)*Math.floor((t+5)/6)*16;case zd:return Math.floor((r+7)/8)*Math.floor((t+4)/5)*16;case Bd:return Math.floor((r+7)/8)*Math.floor((t+5)/6)*16;case Id:return Math.floor((r+7)/8)*Math.floor((t+7)/8)*16;case Fd:return Math.floor((r+9)/10)*Math.floor((t+4)/5)*16;case Hd:return Math.floor((r+9)/10)*Math.floor((t+5)/6)*16;case Gd:return Math.floor((r+9)/10)*Math.floor((t+7)/8)*16;case Vd:return Math.floor((r+9)/10)*Math.floor((t+9)/10)*16;case kd:return Math.floor((r+11)/12)*Math.floor((t+9)/10)*16;case Xd:return Math.floor((r+11)/12)*Math.floor((t+11)/12)*16;case Wd:case jd:case Yd:return Math.ceil(r/4)*Math.ceil(t/4)*16;case qd:case Zd:return Math.ceil(r/4)*Math.ceil(t/4)*8;case Kd:case Qd:return Math.ceil(r/4)*Math.ceil(t/4)*16}throw new Error(`Unable to determine texture byte length for ${i} format.`)}function OE(r){switch(r){case Ii:case Iv:return{byteLength:1,components:1};case il:case Fv:case cl:return{byteLength:2,components:1};case ap:case sp:return{byteLength:2,components:4};case Cs:case ip:case zi:return{byteLength:4,components:1};case Hv:case Gv:return{byteLength:4,components:3}}throw new Error(`Unknown texture type ${r}.`)}typeof __THREE_DEVTOOLS__<"u"&&__THREE_DEVTOOLS__.dispatchEvent(new CustomEvent("register",{detail:{revision:np}}));typeof window<"u"&&(window.__THREE__?console.warn("WARNING: Multiple instances of Three.js being imported."):window.__THREE__=np);/**
 * @license
 * Copyright 2010-2025 Three.js Authors
 * SPDX-License-Identifier: MIT
 */function ly(){let r=null,t=!1,i=null,s=null;function l(c,h){i(c,h),s=r.requestAnimationFrame(l)}return{start:function(){t!==!0&&i!==null&&(s=r.requestAnimationFrame(l),t=!0)},stop:function(){r.cancelAnimationFrame(s),t=!1},setAnimationLoop:function(c){i=c},setContext:function(c){r=c}}}function PE(r){const t=new WeakMap;function i(d,m){const p=d.array,v=d.usage,g=p.byteLength,x=r.createBuffer();r.bindBuffer(m,x),r.bufferData(m,p,v),d.onUploadCallback();let S;if(p instanceof Float32Array)S=r.FLOAT;else if(typeof Float16Array<"u"&&p instanceof Float16Array)S=r.HALF_FLOAT;else if(p instanceof Uint16Array)d.isFloat16BufferAttribute?S=r.HALF_FLOAT:S=r.UNSIGNED_SHORT;else if(p instanceof Int16Array)S=r.SHORT;else if(p instanceof Uint32Array)S=r.UNSIGNED_INT;else if(p instanceof Int32Array)S=r.INT;else if(p instanceof Int8Array)S=r.BYTE;else if(p instanceof Uint8Array)S=r.UNSIGNED_BYTE;else if(p instanceof Uint8ClampedArray)S=r.UNSIGNED_BYTE;else throw new Error("THREE.WebGLAttributes: Unsupported buffer data format: "+p);return{buffer:x,type:S,bytesPerElement:p.BYTES_PER_ELEMENT,version:d.version,size:g}}function s(d,m,p){const v=m.array,g=m.updateRanges;if(r.bindBuffer(p,d),g.length===0)r.bufferSubData(p,0,v);else{g.sort((S,b)=>S.start-b.start);let x=0;for(let S=1;S<g.length;S++){const b=g[x],R=g[S];R.start<=b.start+b.count+1?b.count=Math.max(b.count,R.start+R.count-b.start):(++x,g[x]=R)}g.length=x+1;for(let S=0,b=g.length;S<b;S++){const R=g[S];r.bufferSubData(p,R.start*v.BYTES_PER_ELEMENT,v,R.start,R.count)}m.clearUpdateRanges()}m.onUploadCallback()}function l(d){return d.isInterleavedBufferAttribute&&(d=d.data),t.get(d)}function c(d){d.isInterleavedBufferAttribute&&(d=d.data);const m=t.get(d);m&&(r.deleteBuffer(m.buffer),t.delete(d))}function h(d,m){if(d.isInterleavedBufferAttribute&&(d=d.data),d.isGLBufferAttribute){const v=t.get(d);(!v||v.version<d.version)&&t.set(d,{buffer:d.buffer,type:d.type,bytesPerElement:d.elementSize,version:d.version});return}const p=t.get(d);if(p===void 0)t.set(d,i(d,m));else if(p.version<d.version){if(p.size!==d.array.byteLength)throw new Error("THREE.WebGLAttributes: The size of the buffer attribute's array buffer does not match the original size. Resizing buffer attributes is not supported.");s(p.buffer,d,m),p.version=d.version}}return{get:l,remove:c,update:h}}var zE=`#ifdef USE_ALPHAHASH
	if ( diffuseColor.a < getAlphaHashThreshold( vPosition ) ) discard;
#endif`,BE=`#ifdef USE_ALPHAHASH
	const float ALPHA_HASH_SCALE = 0.05;
	float hash2D( vec2 value ) {
		return fract( 1.0e4 * sin( 17.0 * value.x + 0.1 * value.y ) * ( 0.1 + abs( sin( 13.0 * value.y + value.x ) ) ) );
	}
	float hash3D( vec3 value ) {
		return hash2D( vec2( hash2D( value.xy ), value.z ) );
	}
	float getAlphaHashThreshold( vec3 position ) {
		float maxDeriv = max(
			length( dFdx( position.xyz ) ),
			length( dFdy( position.xyz ) )
		);
		float pixScale = 1.0 / ( ALPHA_HASH_SCALE * maxDeriv );
		vec2 pixScales = vec2(
			exp2( floor( log2( pixScale ) ) ),
			exp2( ceil( log2( pixScale ) ) )
		);
		vec2 alpha = vec2(
			hash3D( floor( pixScales.x * position.xyz ) ),
			hash3D( floor( pixScales.y * position.xyz ) )
		);
		float lerpFactor = fract( log2( pixScale ) );
		float x = ( 1.0 - lerpFactor ) * alpha.x + lerpFactor * alpha.y;
		float a = min( lerpFactor, 1.0 - lerpFactor );
		vec3 cases = vec3(
			x * x / ( 2.0 * a * ( 1.0 - a ) ),
			( x - 0.5 * a ) / ( 1.0 - a ),
			1.0 - ( ( 1.0 - x ) * ( 1.0 - x ) / ( 2.0 * a * ( 1.0 - a ) ) )
		);
		float threshold = ( x < ( 1.0 - a ) )
			? ( ( x < a ) ? cases.x : cases.y )
			: cases.z;
		return clamp( threshold , 1.0e-6, 1.0 );
	}
#endif`,IE=`#ifdef USE_ALPHAMAP
	diffuseColor.a *= texture2D( alphaMap, vAlphaMapUv ).g;
#endif`,FE=`#ifdef USE_ALPHAMAP
	uniform sampler2D alphaMap;
#endif`,HE=`#ifdef USE_ALPHATEST
	#ifdef ALPHA_TO_COVERAGE
	diffuseColor.a = smoothstep( alphaTest, alphaTest + fwidth( diffuseColor.a ), diffuseColor.a );
	if ( diffuseColor.a == 0.0 ) discard;
	#else
	if ( diffuseColor.a < alphaTest ) discard;
	#endif
#endif`,GE=`#ifdef USE_ALPHATEST
	uniform float alphaTest;
#endif`,VE=`#ifdef USE_AOMAP
	float ambientOcclusion = ( texture2D( aoMap, vAoMapUv ).r - 1.0 ) * aoMapIntensity + 1.0;
	reflectedLight.indirectDiffuse *= ambientOcclusion;
	#if defined( USE_CLEARCOAT ) 
		clearcoatSpecularIndirect *= ambientOcclusion;
	#endif
	#if defined( USE_SHEEN ) 
		sheenSpecularIndirect *= ambientOcclusion;
	#endif
	#if defined( USE_ENVMAP ) && defined( STANDARD )
		float dotNV = saturate( dot( geometryNormal, geometryViewDir ) );
		reflectedLight.indirectSpecular *= computeSpecularOcclusion( dotNV, ambientOcclusion, material.roughness );
	#endif
#endif`,kE=`#ifdef USE_AOMAP
	uniform sampler2D aoMap;
	uniform float aoMapIntensity;
#endif`,XE=`#ifdef USE_BATCHING
	#if ! defined( GL_ANGLE_multi_draw )
	#define gl_DrawID _gl_DrawID
	uniform int _gl_DrawID;
	#endif
	uniform highp sampler2D batchingTexture;
	uniform highp usampler2D batchingIdTexture;
	mat4 getBatchingMatrix( const in float i ) {
		int size = textureSize( batchingTexture, 0 ).x;
		int j = int( i ) * 4;
		int x = j % size;
		int y = j / size;
		vec4 v1 = texelFetch( batchingTexture, ivec2( x, y ), 0 );
		vec4 v2 = texelFetch( batchingTexture, ivec2( x + 1, y ), 0 );
		vec4 v3 = texelFetch( batchingTexture, ivec2( x + 2, y ), 0 );
		vec4 v4 = texelFetch( batchingTexture, ivec2( x + 3, y ), 0 );
		return mat4( v1, v2, v3, v4 );
	}
	float getIndirectIndex( const in int i ) {
		int size = textureSize( batchingIdTexture, 0 ).x;
		int x = i % size;
		int y = i / size;
		return float( texelFetch( batchingIdTexture, ivec2( x, y ), 0 ).r );
	}
#endif
#ifdef USE_BATCHING_COLOR
	uniform sampler2D batchingColorTexture;
	vec3 getBatchingColor( const in float i ) {
		int size = textureSize( batchingColorTexture, 0 ).x;
		int j = int( i );
		int x = j % size;
		int y = j / size;
		return texelFetch( batchingColorTexture, ivec2( x, y ), 0 ).rgb;
	}
#endif`,WE=`#ifdef USE_BATCHING
	mat4 batchingMatrix = getBatchingMatrix( getIndirectIndex( gl_DrawID ) );
#endif`,jE=`vec3 transformed = vec3( position );
#ifdef USE_ALPHAHASH
	vPosition = vec3( position );
#endif`,YE=`vec3 objectNormal = vec3( normal );
#ifdef USE_TANGENT
	vec3 objectTangent = vec3( tangent.xyz );
#endif`,qE=`float G_BlinnPhong_Implicit( ) {
	return 0.25;
}
float D_BlinnPhong( const in float shininess, const in float dotNH ) {
	return RECIPROCAL_PI * ( shininess * 0.5 + 1.0 ) * pow( dotNH, shininess );
}
vec3 BRDF_BlinnPhong( const in vec3 lightDir, const in vec3 viewDir, const in vec3 normal, const in vec3 specularColor, const in float shininess ) {
	vec3 halfDir = normalize( lightDir + viewDir );
	float dotNH = saturate( dot( normal, halfDir ) );
	float dotVH = saturate( dot( viewDir, halfDir ) );
	vec3 F = F_Schlick( specularColor, 1.0, dotVH );
	float G = G_BlinnPhong_Implicit( );
	float D = D_BlinnPhong( shininess, dotNH );
	return F * ( G * D );
} // validated`,ZE=`#ifdef USE_IRIDESCENCE
	const mat3 XYZ_TO_REC709 = mat3(
		 3.2404542, -0.9692660,  0.0556434,
		-1.5371385,  1.8760108, -0.2040259,
		-0.4985314,  0.0415560,  1.0572252
	);
	vec3 Fresnel0ToIor( vec3 fresnel0 ) {
		vec3 sqrtF0 = sqrt( fresnel0 );
		return ( vec3( 1.0 ) + sqrtF0 ) / ( vec3( 1.0 ) - sqrtF0 );
	}
	vec3 IorToFresnel0( vec3 transmittedIor, float incidentIor ) {
		return pow2( ( transmittedIor - vec3( incidentIor ) ) / ( transmittedIor + vec3( incidentIor ) ) );
	}
	float IorToFresnel0( float transmittedIor, float incidentIor ) {
		return pow2( ( transmittedIor - incidentIor ) / ( transmittedIor + incidentIor ));
	}
	vec3 evalSensitivity( float OPD, vec3 shift ) {
		float phase = 2.0 * PI * OPD * 1.0e-9;
		vec3 val = vec3( 5.4856e-13, 4.4201e-13, 5.2481e-13 );
		vec3 pos = vec3( 1.6810e+06, 1.7953e+06, 2.2084e+06 );
		vec3 var = vec3( 4.3278e+09, 9.3046e+09, 6.6121e+09 );
		vec3 xyz = val * sqrt( 2.0 * PI * var ) * cos( pos * phase + shift ) * exp( - pow2( phase ) * var );
		xyz.x += 9.7470e-14 * sqrt( 2.0 * PI * 4.5282e+09 ) * cos( 2.2399e+06 * phase + shift[ 0 ] ) * exp( - 4.5282e+09 * pow2( phase ) );
		xyz /= 1.0685e-7;
		vec3 rgb = XYZ_TO_REC709 * xyz;
		return rgb;
	}
	vec3 evalIridescence( float outsideIOR, float eta2, float cosTheta1, float thinFilmThickness, vec3 baseF0 ) {
		vec3 I;
		float iridescenceIOR = mix( outsideIOR, eta2, smoothstep( 0.0, 0.03, thinFilmThickness ) );
		float sinTheta2Sq = pow2( outsideIOR / iridescenceIOR ) * ( 1.0 - pow2( cosTheta1 ) );
		float cosTheta2Sq = 1.0 - sinTheta2Sq;
		if ( cosTheta2Sq < 0.0 ) {
			return vec3( 1.0 );
		}
		float cosTheta2 = sqrt( cosTheta2Sq );
		float R0 = IorToFresnel0( iridescenceIOR, outsideIOR );
		float R12 = F_Schlick( R0, 1.0, cosTheta1 );
		float T121 = 1.0 - R12;
		float phi12 = 0.0;
		if ( iridescenceIOR < outsideIOR ) phi12 = PI;
		float phi21 = PI - phi12;
		vec3 baseIOR = Fresnel0ToIor( clamp( baseF0, 0.0, 0.9999 ) );		vec3 R1 = IorToFresnel0( baseIOR, iridescenceIOR );
		vec3 R23 = F_Schlick( R1, 1.0, cosTheta2 );
		vec3 phi23 = vec3( 0.0 );
		if ( baseIOR[ 0 ] < iridescenceIOR ) phi23[ 0 ] = PI;
		if ( baseIOR[ 1 ] < iridescenceIOR ) phi23[ 1 ] = PI;
		if ( baseIOR[ 2 ] < iridescenceIOR ) phi23[ 2 ] = PI;
		float OPD = 2.0 * iridescenceIOR * thinFilmThickness * cosTheta2;
		vec3 phi = vec3( phi21 ) + phi23;
		vec3 R123 = clamp( R12 * R23, 1e-5, 0.9999 );
		vec3 r123 = sqrt( R123 );
		vec3 Rs = pow2( T121 ) * R23 / ( vec3( 1.0 ) - R123 );
		vec3 C0 = R12 + Rs;
		I = C0;
		vec3 Cm = Rs - T121;
		for ( int m = 1; m <= 2; ++ m ) {
			Cm *= r123;
			vec3 Sm = 2.0 * evalSensitivity( float( m ) * OPD, float( m ) * phi );
			I += Cm * Sm;
		}
		return max( I, vec3( 0.0 ) );
	}
#endif`,KE=`#ifdef USE_BUMPMAP
	uniform sampler2D bumpMap;
	uniform float bumpScale;
	vec2 dHdxy_fwd() {
		vec2 dSTdx = dFdx( vBumpMapUv );
		vec2 dSTdy = dFdy( vBumpMapUv );
		float Hll = bumpScale * texture2D( bumpMap, vBumpMapUv ).x;
		float dBx = bumpScale * texture2D( bumpMap, vBumpMapUv + dSTdx ).x - Hll;
		float dBy = bumpScale * texture2D( bumpMap, vBumpMapUv + dSTdy ).x - Hll;
		return vec2( dBx, dBy );
	}
	vec3 perturbNormalArb( vec3 surf_pos, vec3 surf_norm, vec2 dHdxy, float faceDirection ) {
		vec3 vSigmaX = normalize( dFdx( surf_pos.xyz ) );
		vec3 vSigmaY = normalize( dFdy( surf_pos.xyz ) );
		vec3 vN = surf_norm;
		vec3 R1 = cross( vSigmaY, vN );
		vec3 R2 = cross( vN, vSigmaX );
		float fDet = dot( vSigmaX, R1 ) * faceDirection;
		vec3 vGrad = sign( fDet ) * ( dHdxy.x * R1 + dHdxy.y * R2 );
		return normalize( abs( fDet ) * surf_norm - vGrad );
	}
#endif`,QE=`#if NUM_CLIPPING_PLANES > 0
	vec4 plane;
	#ifdef ALPHA_TO_COVERAGE
		float distanceToPlane, distanceGradient;
		float clipOpacity = 1.0;
		#pragma unroll_loop_start
		for ( int i = 0; i < UNION_CLIPPING_PLANES; i ++ ) {
			plane = clippingPlanes[ i ];
			distanceToPlane = - dot( vClipPosition, plane.xyz ) + plane.w;
			distanceGradient = fwidth( distanceToPlane ) / 2.0;
			clipOpacity *= smoothstep( - distanceGradient, distanceGradient, distanceToPlane );
			if ( clipOpacity == 0.0 ) discard;
		}
		#pragma unroll_loop_end
		#if UNION_CLIPPING_PLANES < NUM_CLIPPING_PLANES
			float unionClipOpacity = 1.0;
			#pragma unroll_loop_start
			for ( int i = UNION_CLIPPING_PLANES; i < NUM_CLIPPING_PLANES; i ++ ) {
				plane = clippingPlanes[ i ];
				distanceToPlane = - dot( vClipPosition, plane.xyz ) + plane.w;
				distanceGradient = fwidth( distanceToPlane ) / 2.0;
				unionClipOpacity *= 1.0 - smoothstep( - distanceGradient, distanceGradient, distanceToPlane );
			}
			#pragma unroll_loop_end
			clipOpacity *= 1.0 - unionClipOpacity;
		#endif
		diffuseColor.a *= clipOpacity;
		if ( diffuseColor.a == 0.0 ) discard;
	#else
		#pragma unroll_loop_start
		for ( int i = 0; i < UNION_CLIPPING_PLANES; i ++ ) {
			plane = clippingPlanes[ i ];
			if ( dot( vClipPosition, plane.xyz ) > plane.w ) discard;
		}
		#pragma unroll_loop_end
		#if UNION_CLIPPING_PLANES < NUM_CLIPPING_PLANES
			bool clipped = true;
			#pragma unroll_loop_start
			for ( int i = UNION_CLIPPING_PLANES; i < NUM_CLIPPING_PLANES; i ++ ) {
				plane = clippingPlanes[ i ];
				clipped = ( dot( vClipPosition, plane.xyz ) > plane.w ) && clipped;
			}
			#pragma unroll_loop_end
			if ( clipped ) discard;
		#endif
	#endif
#endif`,JE=`#if NUM_CLIPPING_PLANES > 0
	varying vec3 vClipPosition;
	uniform vec4 clippingPlanes[ NUM_CLIPPING_PLANES ];
#endif`,$E=`#if NUM_CLIPPING_PLANES > 0
	varying vec3 vClipPosition;
#endif`,tT=`#if NUM_CLIPPING_PLANES > 0
	vClipPosition = - mvPosition.xyz;
#endif`,eT=`#if defined( USE_COLOR_ALPHA )
	diffuseColor *= vColor;
#elif defined( USE_COLOR )
	diffuseColor.rgb *= vColor;
#endif`,nT=`#if defined( USE_COLOR_ALPHA )
	varying vec4 vColor;
#elif defined( USE_COLOR )
	varying vec3 vColor;
#endif`,iT=`#if defined( USE_COLOR_ALPHA )
	varying vec4 vColor;
#elif defined( USE_COLOR ) || defined( USE_INSTANCING_COLOR ) || defined( USE_BATCHING_COLOR )
	varying vec3 vColor;
#endif`,aT=`#if defined( USE_COLOR_ALPHA )
	vColor = vec4( 1.0 );
#elif defined( USE_COLOR ) || defined( USE_INSTANCING_COLOR ) || defined( USE_BATCHING_COLOR )
	vColor = vec3( 1.0 );
#endif
#ifdef USE_COLOR
	vColor *= color;
#endif
#ifdef USE_INSTANCING_COLOR
	vColor.xyz *= instanceColor.xyz;
#endif
#ifdef USE_BATCHING_COLOR
	vec3 batchingColor = getBatchingColor( getIndirectIndex( gl_DrawID ) );
	vColor.xyz *= batchingColor.xyz;
#endif`,sT=`#define PI 3.141592653589793
#define PI2 6.283185307179586
#define PI_HALF 1.5707963267948966
#define RECIPROCAL_PI 0.3183098861837907
#define RECIPROCAL_PI2 0.15915494309189535
#define EPSILON 1e-6
#ifndef saturate
#define saturate( a ) clamp( a, 0.0, 1.0 )
#endif
#define whiteComplement( a ) ( 1.0 - saturate( a ) )
float pow2( const in float x ) { return x*x; }
vec3 pow2( const in vec3 x ) { return x*x; }
float pow3( const in float x ) { return x*x*x; }
float pow4( const in float x ) { float x2 = x*x; return x2*x2; }
float max3( const in vec3 v ) { return max( max( v.x, v.y ), v.z ); }
float average( const in vec3 v ) { return dot( v, vec3( 0.3333333 ) ); }
highp float rand( const in vec2 uv ) {
	const highp float a = 12.9898, b = 78.233, c = 43758.5453;
	highp float dt = dot( uv.xy, vec2( a,b ) ), sn = mod( dt, PI );
	return fract( sin( sn ) * c );
}
#ifdef HIGH_PRECISION
	float precisionSafeLength( vec3 v ) { return length( v ); }
#else
	float precisionSafeLength( vec3 v ) {
		float maxComponent = max3( abs( v ) );
		return length( v / maxComponent ) * maxComponent;
	}
#endif
struct IncidentLight {
	vec3 color;
	vec3 direction;
	bool visible;
};
struct ReflectedLight {
	vec3 directDiffuse;
	vec3 directSpecular;
	vec3 indirectDiffuse;
	vec3 indirectSpecular;
};
#ifdef USE_ALPHAHASH
	varying vec3 vPosition;
#endif
vec3 transformDirection( in vec3 dir, in mat4 matrix ) {
	return normalize( ( matrix * vec4( dir, 0.0 ) ).xyz );
}
vec3 inverseTransformDirection( in vec3 dir, in mat4 matrix ) {
	return normalize( ( vec4( dir, 0.0 ) * matrix ).xyz );
}
mat3 transposeMat3( const in mat3 m ) {
	mat3 tmp;
	tmp[ 0 ] = vec3( m[ 0 ].x, m[ 1 ].x, m[ 2 ].x );
	tmp[ 1 ] = vec3( m[ 0 ].y, m[ 1 ].y, m[ 2 ].y );
	tmp[ 2 ] = vec3( m[ 0 ].z, m[ 1 ].z, m[ 2 ].z );
	return tmp;
}
bool isPerspectiveMatrix( mat4 m ) {
	return m[ 2 ][ 3 ] == - 1.0;
}
vec2 equirectUv( in vec3 dir ) {
	float u = atan( dir.z, dir.x ) * RECIPROCAL_PI2 + 0.5;
	float v = asin( clamp( dir.y, - 1.0, 1.0 ) ) * RECIPROCAL_PI + 0.5;
	return vec2( u, v );
}
vec3 BRDF_Lambert( const in vec3 diffuseColor ) {
	return RECIPROCAL_PI * diffuseColor;
}
vec3 F_Schlick( const in vec3 f0, const in float f90, const in float dotVH ) {
	float fresnel = exp2( ( - 5.55473 * dotVH - 6.98316 ) * dotVH );
	return f0 * ( 1.0 - fresnel ) + ( f90 * fresnel );
}
float F_Schlick( const in float f0, const in float f90, const in float dotVH ) {
	float fresnel = exp2( ( - 5.55473 * dotVH - 6.98316 ) * dotVH );
	return f0 * ( 1.0 - fresnel ) + ( f90 * fresnel );
} // validated`,rT=`#ifdef ENVMAP_TYPE_CUBE_UV
	#define cubeUV_minMipLevel 4.0
	#define cubeUV_minTileSize 16.0
	float getFace( vec3 direction ) {
		vec3 absDirection = abs( direction );
		float face = - 1.0;
		if ( absDirection.x > absDirection.z ) {
			if ( absDirection.x > absDirection.y )
				face = direction.x > 0.0 ? 0.0 : 3.0;
			else
				face = direction.y > 0.0 ? 1.0 : 4.0;
		} else {
			if ( absDirection.z > absDirection.y )
				face = direction.z > 0.0 ? 2.0 : 5.0;
			else
				face = direction.y > 0.0 ? 1.0 : 4.0;
		}
		return face;
	}
	vec2 getUV( vec3 direction, float face ) {
		vec2 uv;
		if ( face == 0.0 ) {
			uv = vec2( direction.z, direction.y ) / abs( direction.x );
		} else if ( face == 1.0 ) {
			uv = vec2( - direction.x, - direction.z ) / abs( direction.y );
		} else if ( face == 2.0 ) {
			uv = vec2( - direction.x, direction.y ) / abs( direction.z );
		} else if ( face == 3.0 ) {
			uv = vec2( - direction.z, direction.y ) / abs( direction.x );
		} else if ( face == 4.0 ) {
			uv = vec2( - direction.x, direction.z ) / abs( direction.y );
		} else {
			uv = vec2( direction.x, direction.y ) / abs( direction.z );
		}
		return 0.5 * ( uv + 1.0 );
	}
	vec3 bilinearCubeUV( sampler2D envMap, vec3 direction, float mipInt ) {
		float face = getFace( direction );
		float filterInt = max( cubeUV_minMipLevel - mipInt, 0.0 );
		mipInt = max( mipInt, cubeUV_minMipLevel );
		float faceSize = exp2( mipInt );
		highp vec2 uv = getUV( direction, face ) * ( faceSize - 2.0 ) + 1.0;
		if ( face > 2.0 ) {
			uv.y += faceSize;
			face -= 3.0;
		}
		uv.x += face * faceSize;
		uv.x += filterInt * 3.0 * cubeUV_minTileSize;
		uv.y += 4.0 * ( exp2( CUBEUV_MAX_MIP ) - faceSize );
		uv.x *= CUBEUV_TEXEL_WIDTH;
		uv.y *= CUBEUV_TEXEL_HEIGHT;
		#ifdef texture2DGradEXT
			return texture2DGradEXT( envMap, uv, vec2( 0.0 ), vec2( 0.0 ) ).rgb;
		#else
			return texture2D( envMap, uv ).rgb;
		#endif
	}
	#define cubeUV_r0 1.0
	#define cubeUV_m0 - 2.0
	#define cubeUV_r1 0.8
	#define cubeUV_m1 - 1.0
	#define cubeUV_r4 0.4
	#define cubeUV_m4 2.0
	#define cubeUV_r5 0.305
	#define cubeUV_m5 3.0
	#define cubeUV_r6 0.21
	#define cubeUV_m6 4.0
	float roughnessToMip( float roughness ) {
		float mip = 0.0;
		if ( roughness >= cubeUV_r1 ) {
			mip = ( cubeUV_r0 - roughness ) * ( cubeUV_m1 - cubeUV_m0 ) / ( cubeUV_r0 - cubeUV_r1 ) + cubeUV_m0;
		} else if ( roughness >= cubeUV_r4 ) {
			mip = ( cubeUV_r1 - roughness ) * ( cubeUV_m4 - cubeUV_m1 ) / ( cubeUV_r1 - cubeUV_r4 ) + cubeUV_m1;
		} else if ( roughness >= cubeUV_r5 ) {
			mip = ( cubeUV_r4 - roughness ) * ( cubeUV_m5 - cubeUV_m4 ) / ( cubeUV_r4 - cubeUV_r5 ) + cubeUV_m4;
		} else if ( roughness >= cubeUV_r6 ) {
			mip = ( cubeUV_r5 - roughness ) * ( cubeUV_m6 - cubeUV_m5 ) / ( cubeUV_r5 - cubeUV_r6 ) + cubeUV_m5;
		} else {
			mip = - 2.0 * log2( 1.16 * roughness );		}
		return mip;
	}
	vec4 textureCubeUV( sampler2D envMap, vec3 sampleDir, float roughness ) {
		float mip = clamp( roughnessToMip( roughness ), cubeUV_m0, CUBEUV_MAX_MIP );
		float mipF = fract( mip );
		float mipInt = floor( mip );
		vec3 color0 = bilinearCubeUV( envMap, sampleDir, mipInt );
		if ( mipF == 0.0 ) {
			return vec4( color0, 1.0 );
		} else {
			vec3 color1 = bilinearCubeUV( envMap, sampleDir, mipInt + 1.0 );
			return vec4( mix( color0, color1, mipF ), 1.0 );
		}
	}
#endif`,oT=`vec3 transformedNormal = objectNormal;
#ifdef USE_TANGENT
	vec3 transformedTangent = objectTangent;
#endif
#ifdef USE_BATCHING
	mat3 bm = mat3( batchingMatrix );
	transformedNormal /= vec3( dot( bm[ 0 ], bm[ 0 ] ), dot( bm[ 1 ], bm[ 1 ] ), dot( bm[ 2 ], bm[ 2 ] ) );
	transformedNormal = bm * transformedNormal;
	#ifdef USE_TANGENT
		transformedTangent = bm * transformedTangent;
	#endif
#endif
#ifdef USE_INSTANCING
	mat3 im = mat3( instanceMatrix );
	transformedNormal /= vec3( dot( im[ 0 ], im[ 0 ] ), dot( im[ 1 ], im[ 1 ] ), dot( im[ 2 ], im[ 2 ] ) );
	transformedNormal = im * transformedNormal;
	#ifdef USE_TANGENT
		transformedTangent = im * transformedTangent;
	#endif
#endif
transformedNormal = normalMatrix * transformedNormal;
#ifdef FLIP_SIDED
	transformedNormal = - transformedNormal;
#endif
#ifdef USE_TANGENT
	transformedTangent = ( modelViewMatrix * vec4( transformedTangent, 0.0 ) ).xyz;
	#ifdef FLIP_SIDED
		transformedTangent = - transformedTangent;
	#endif
#endif`,lT=`#ifdef USE_DISPLACEMENTMAP
	uniform sampler2D displacementMap;
	uniform float displacementScale;
	uniform float displacementBias;
#endif`,cT=`#ifdef USE_DISPLACEMENTMAP
	transformed += normalize( objectNormal ) * ( texture2D( displacementMap, vDisplacementMapUv ).x * displacementScale + displacementBias );
#endif`,uT=`#ifdef USE_EMISSIVEMAP
	vec4 emissiveColor = texture2D( emissiveMap, vEmissiveMapUv );
	#ifdef DECODE_VIDEO_TEXTURE_EMISSIVE
		emissiveColor = sRGBTransferEOTF( emissiveColor );
	#endif
	totalEmissiveRadiance *= emissiveColor.rgb;
#endif`,fT=`#ifdef USE_EMISSIVEMAP
	uniform sampler2D emissiveMap;
#endif`,hT="gl_FragColor = linearToOutputTexel( gl_FragColor );",dT=`vec4 LinearTransferOETF( in vec4 value ) {
	return value;
}
vec4 sRGBTransferEOTF( in vec4 value ) {
	return vec4( mix( pow( value.rgb * 0.9478672986 + vec3( 0.0521327014 ), vec3( 2.4 ) ), value.rgb * 0.0773993808, vec3( lessThanEqual( value.rgb, vec3( 0.04045 ) ) ) ), value.a );
}
vec4 sRGBTransferOETF( in vec4 value ) {
	return vec4( mix( pow( value.rgb, vec3( 0.41666 ) ) * 1.055 - vec3( 0.055 ), value.rgb * 12.92, vec3( lessThanEqual( value.rgb, vec3( 0.0031308 ) ) ) ), value.a );
}`,pT=`#ifdef USE_ENVMAP
	#ifdef ENV_WORLDPOS
		vec3 cameraToFrag;
		if ( isOrthographic ) {
			cameraToFrag = normalize( vec3( - viewMatrix[ 0 ][ 2 ], - viewMatrix[ 1 ][ 2 ], - viewMatrix[ 2 ][ 2 ] ) );
		} else {
			cameraToFrag = normalize( vWorldPosition - cameraPosition );
		}
		vec3 worldNormal = inverseTransformDirection( normal, viewMatrix );
		#ifdef ENVMAP_MODE_REFLECTION
			vec3 reflectVec = reflect( cameraToFrag, worldNormal );
		#else
			vec3 reflectVec = refract( cameraToFrag, worldNormal, refractionRatio );
		#endif
	#else
		vec3 reflectVec = vReflect;
	#endif
	#ifdef ENVMAP_TYPE_CUBE
		vec4 envColor = textureCube( envMap, envMapRotation * vec3( flipEnvMap * reflectVec.x, reflectVec.yz ) );
	#else
		vec4 envColor = vec4( 0.0 );
	#endif
	#ifdef ENVMAP_BLENDING_MULTIPLY
		outgoingLight = mix( outgoingLight, outgoingLight * envColor.xyz, specularStrength * reflectivity );
	#elif defined( ENVMAP_BLENDING_MIX )
		outgoingLight = mix( outgoingLight, envColor.xyz, specularStrength * reflectivity );
	#elif defined( ENVMAP_BLENDING_ADD )
		outgoingLight += envColor.xyz * specularStrength * reflectivity;
	#endif
#endif`,mT=`#ifdef USE_ENVMAP
	uniform float envMapIntensity;
	uniform float flipEnvMap;
	uniform mat3 envMapRotation;
	#ifdef ENVMAP_TYPE_CUBE
		uniform samplerCube envMap;
	#else
		uniform sampler2D envMap;
	#endif
	
#endif`,gT=`#ifdef USE_ENVMAP
	uniform float reflectivity;
	#if defined( USE_BUMPMAP ) || defined( USE_NORMALMAP ) || defined( PHONG ) || defined( LAMBERT )
		#define ENV_WORLDPOS
	#endif
	#ifdef ENV_WORLDPOS
		varying vec3 vWorldPosition;
		uniform float refractionRatio;
	#else
		varying vec3 vReflect;
	#endif
#endif`,_T=`#ifdef USE_ENVMAP
	#if defined( USE_BUMPMAP ) || defined( USE_NORMALMAP ) || defined( PHONG ) || defined( LAMBERT )
		#define ENV_WORLDPOS
	#endif
	#ifdef ENV_WORLDPOS
		
		varying vec3 vWorldPosition;
	#else
		varying vec3 vReflect;
		uniform float refractionRatio;
	#endif
#endif`,vT=`#ifdef USE_ENVMAP
	#ifdef ENV_WORLDPOS
		vWorldPosition = worldPosition.xyz;
	#else
		vec3 cameraToVertex;
		if ( isOrthographic ) {
			cameraToVertex = normalize( vec3( - viewMatrix[ 0 ][ 2 ], - viewMatrix[ 1 ][ 2 ], - viewMatrix[ 2 ][ 2 ] ) );
		} else {
			cameraToVertex = normalize( worldPosition.xyz - cameraPosition );
		}
		vec3 worldNormal = inverseTransformDirection( transformedNormal, viewMatrix );
		#ifdef ENVMAP_MODE_REFLECTION
			vReflect = reflect( cameraToVertex, worldNormal );
		#else
			vReflect = refract( cameraToVertex, worldNormal, refractionRatio );
		#endif
	#endif
#endif`,yT=`#ifdef USE_FOG
	vFogDepth = - mvPosition.z;
#endif`,xT=`#ifdef USE_FOG
	varying float vFogDepth;
#endif`,ST=`#ifdef USE_FOG
	#ifdef FOG_EXP2
		float fogFactor = 1.0 - exp( - fogDensity * fogDensity * vFogDepth * vFogDepth );
	#else
		float fogFactor = smoothstep( fogNear, fogFar, vFogDepth );
	#endif
	gl_FragColor.rgb = mix( gl_FragColor.rgb, fogColor, fogFactor );
#endif`,MT=`#ifdef USE_FOG
	uniform vec3 fogColor;
	varying float vFogDepth;
	#ifdef FOG_EXP2
		uniform float fogDensity;
	#else
		uniform float fogNear;
		uniform float fogFar;
	#endif
#endif`,ET=`#ifdef USE_GRADIENTMAP
	uniform sampler2D gradientMap;
#endif
vec3 getGradientIrradiance( vec3 normal, vec3 lightDirection ) {
	float dotNL = dot( normal, lightDirection );
	vec2 coord = vec2( dotNL * 0.5 + 0.5, 0.0 );
	#ifdef USE_GRADIENTMAP
		return vec3( texture2D( gradientMap, coord ).r );
	#else
		vec2 fw = fwidth( coord ) * 0.5;
		return mix( vec3( 0.7 ), vec3( 1.0 ), smoothstep( 0.7 - fw.x, 0.7 + fw.x, coord.x ) );
	#endif
}`,TT=`#ifdef USE_LIGHTMAP
	uniform sampler2D lightMap;
	uniform float lightMapIntensity;
#endif`,bT=`LambertMaterial material;
material.diffuseColor = diffuseColor.rgb;
material.specularStrength = specularStrength;`,AT=`varying vec3 vViewPosition;
struct LambertMaterial {
	vec3 diffuseColor;
	float specularStrength;
};
void RE_Direct_Lambert( const in IncidentLight directLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in LambertMaterial material, inout ReflectedLight reflectedLight ) {
	float dotNL = saturate( dot( geometryNormal, directLight.direction ) );
	vec3 irradiance = dotNL * directLight.color;
	reflectedLight.directDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
void RE_IndirectDiffuse_Lambert( const in vec3 irradiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in LambertMaterial material, inout ReflectedLight reflectedLight ) {
	reflectedLight.indirectDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
#define RE_Direct				RE_Direct_Lambert
#define RE_IndirectDiffuse		RE_IndirectDiffuse_Lambert`,RT=`uniform bool receiveShadow;
uniform vec3 ambientLightColor;
#if defined( USE_LIGHT_PROBES )
	uniform vec3 lightProbe[ 9 ];
#endif
vec3 shGetIrradianceAt( in vec3 normal, in vec3 shCoefficients[ 9 ] ) {
	float x = normal.x, y = normal.y, z = normal.z;
	vec3 result = shCoefficients[ 0 ] * 0.886227;
	result += shCoefficients[ 1 ] * 2.0 * 0.511664 * y;
	result += shCoefficients[ 2 ] * 2.0 * 0.511664 * z;
	result += shCoefficients[ 3 ] * 2.0 * 0.511664 * x;
	result += shCoefficients[ 4 ] * 2.0 * 0.429043 * x * y;
	result += shCoefficients[ 5 ] * 2.0 * 0.429043 * y * z;
	result += shCoefficients[ 6 ] * ( 0.743125 * z * z - 0.247708 );
	result += shCoefficients[ 7 ] * 2.0 * 0.429043 * x * z;
	result += shCoefficients[ 8 ] * 0.429043 * ( x * x - y * y );
	return result;
}
vec3 getLightProbeIrradiance( const in vec3 lightProbe[ 9 ], const in vec3 normal ) {
	vec3 worldNormal = inverseTransformDirection( normal, viewMatrix );
	vec3 irradiance = shGetIrradianceAt( worldNormal, lightProbe );
	return irradiance;
}
vec3 getAmbientLightIrradiance( const in vec3 ambientLightColor ) {
	vec3 irradiance = ambientLightColor;
	return irradiance;
}
float getDistanceAttenuation( const in float lightDistance, const in float cutoffDistance, const in float decayExponent ) {
	float distanceFalloff = 1.0 / max( pow( lightDistance, decayExponent ), 0.01 );
	if ( cutoffDistance > 0.0 ) {
		distanceFalloff *= pow2( saturate( 1.0 - pow4( lightDistance / cutoffDistance ) ) );
	}
	return distanceFalloff;
}
float getSpotAttenuation( const in float coneCosine, const in float penumbraCosine, const in float angleCosine ) {
	return smoothstep( coneCosine, penumbraCosine, angleCosine );
}
#if NUM_DIR_LIGHTS > 0
	struct DirectionalLight {
		vec3 direction;
		vec3 color;
	};
	uniform DirectionalLight directionalLights[ NUM_DIR_LIGHTS ];
	void getDirectionalLightInfo( const in DirectionalLight directionalLight, out IncidentLight light ) {
		light.color = directionalLight.color;
		light.direction = directionalLight.direction;
		light.visible = true;
	}
#endif
#if NUM_POINT_LIGHTS > 0
	struct PointLight {
		vec3 position;
		vec3 color;
		float distance;
		float decay;
	};
	uniform PointLight pointLights[ NUM_POINT_LIGHTS ];
	void getPointLightInfo( const in PointLight pointLight, const in vec3 geometryPosition, out IncidentLight light ) {
		vec3 lVector = pointLight.position - geometryPosition;
		light.direction = normalize( lVector );
		float lightDistance = length( lVector );
		light.color = pointLight.color;
		light.color *= getDistanceAttenuation( lightDistance, pointLight.distance, pointLight.decay );
		light.visible = ( light.color != vec3( 0.0 ) );
	}
#endif
#if NUM_SPOT_LIGHTS > 0
	struct SpotLight {
		vec3 position;
		vec3 direction;
		vec3 color;
		float distance;
		float decay;
		float coneCos;
		float penumbraCos;
	};
	uniform SpotLight spotLights[ NUM_SPOT_LIGHTS ];
	void getSpotLightInfo( const in SpotLight spotLight, const in vec3 geometryPosition, out IncidentLight light ) {
		vec3 lVector = spotLight.position - geometryPosition;
		light.direction = normalize( lVector );
		float angleCos = dot( light.direction, spotLight.direction );
		float spotAttenuation = getSpotAttenuation( spotLight.coneCos, spotLight.penumbraCos, angleCos );
		if ( spotAttenuation > 0.0 ) {
			float lightDistance = length( lVector );
			light.color = spotLight.color * spotAttenuation;
			light.color *= getDistanceAttenuation( lightDistance, spotLight.distance, spotLight.decay );
			light.visible = ( light.color != vec3( 0.0 ) );
		} else {
			light.color = vec3( 0.0 );
			light.visible = false;
		}
	}
#endif
#if NUM_RECT_AREA_LIGHTS > 0
	struct RectAreaLight {
		vec3 color;
		vec3 position;
		vec3 halfWidth;
		vec3 halfHeight;
	};
	uniform sampler2D ltc_1;	uniform sampler2D ltc_2;
	uniform RectAreaLight rectAreaLights[ NUM_RECT_AREA_LIGHTS ];
#endif
#if NUM_HEMI_LIGHTS > 0
	struct HemisphereLight {
		vec3 direction;
		vec3 skyColor;
		vec3 groundColor;
	};
	uniform HemisphereLight hemisphereLights[ NUM_HEMI_LIGHTS ];
	vec3 getHemisphereLightIrradiance( const in HemisphereLight hemiLight, const in vec3 normal ) {
		float dotNL = dot( normal, hemiLight.direction );
		float hemiDiffuseWeight = 0.5 * dotNL + 0.5;
		vec3 irradiance = mix( hemiLight.groundColor, hemiLight.skyColor, hemiDiffuseWeight );
		return irradiance;
	}
#endif`,CT=`#ifdef USE_ENVMAP
	vec3 getIBLIrradiance( const in vec3 normal ) {
		#ifdef ENVMAP_TYPE_CUBE_UV
			vec3 worldNormal = inverseTransformDirection( normal, viewMatrix );
			vec4 envMapColor = textureCubeUV( envMap, envMapRotation * worldNormal, 1.0 );
			return PI * envMapColor.rgb * envMapIntensity;
		#else
			return vec3( 0.0 );
		#endif
	}
	vec3 getIBLRadiance( const in vec3 viewDir, const in vec3 normal, const in float roughness ) {
		#ifdef ENVMAP_TYPE_CUBE_UV
			vec3 reflectVec = reflect( - viewDir, normal );
			reflectVec = normalize( mix( reflectVec, normal, roughness * roughness) );
			reflectVec = inverseTransformDirection( reflectVec, viewMatrix );
			vec4 envMapColor = textureCubeUV( envMap, envMapRotation * reflectVec, roughness );
			return envMapColor.rgb * envMapIntensity;
		#else
			return vec3( 0.0 );
		#endif
	}
	#ifdef USE_ANISOTROPY
		vec3 getIBLAnisotropyRadiance( const in vec3 viewDir, const in vec3 normal, const in float roughness, const in vec3 bitangent, const in float anisotropy ) {
			#ifdef ENVMAP_TYPE_CUBE_UV
				vec3 bentNormal = cross( bitangent, viewDir );
				bentNormal = normalize( cross( bentNormal, bitangent ) );
				bentNormal = normalize( mix( bentNormal, normal, pow2( pow2( 1.0 - anisotropy * ( 1.0 - roughness ) ) ) ) );
				return getIBLRadiance( viewDir, bentNormal, roughness );
			#else
				return vec3( 0.0 );
			#endif
		}
	#endif
#endif`,wT=`ToonMaterial material;
material.diffuseColor = diffuseColor.rgb;`,DT=`varying vec3 vViewPosition;
struct ToonMaterial {
	vec3 diffuseColor;
};
void RE_Direct_Toon( const in IncidentLight directLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in ToonMaterial material, inout ReflectedLight reflectedLight ) {
	vec3 irradiance = getGradientIrradiance( geometryNormal, directLight.direction ) * directLight.color;
	reflectedLight.directDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
void RE_IndirectDiffuse_Toon( const in vec3 irradiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in ToonMaterial material, inout ReflectedLight reflectedLight ) {
	reflectedLight.indirectDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
#define RE_Direct				RE_Direct_Toon
#define RE_IndirectDiffuse		RE_IndirectDiffuse_Toon`,UT=`BlinnPhongMaterial material;
material.diffuseColor = diffuseColor.rgb;
material.specularColor = specular;
material.specularShininess = shininess;
material.specularStrength = specularStrength;`,LT=`varying vec3 vViewPosition;
struct BlinnPhongMaterial {
	vec3 diffuseColor;
	vec3 specularColor;
	float specularShininess;
	float specularStrength;
};
void RE_Direct_BlinnPhong( const in IncidentLight directLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in BlinnPhongMaterial material, inout ReflectedLight reflectedLight ) {
	float dotNL = saturate( dot( geometryNormal, directLight.direction ) );
	vec3 irradiance = dotNL * directLight.color;
	reflectedLight.directDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
	reflectedLight.directSpecular += irradiance * BRDF_BlinnPhong( directLight.direction, geometryViewDir, geometryNormal, material.specularColor, material.specularShininess ) * material.specularStrength;
}
void RE_IndirectDiffuse_BlinnPhong( const in vec3 irradiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in BlinnPhongMaterial material, inout ReflectedLight reflectedLight ) {
	reflectedLight.indirectDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
#define RE_Direct				RE_Direct_BlinnPhong
#define RE_IndirectDiffuse		RE_IndirectDiffuse_BlinnPhong`,NT=`PhysicalMaterial material;
material.diffuseColor = diffuseColor.rgb * ( 1.0 - metalnessFactor );
vec3 dxy = max( abs( dFdx( nonPerturbedNormal ) ), abs( dFdy( nonPerturbedNormal ) ) );
float geometryRoughness = max( max( dxy.x, dxy.y ), dxy.z );
material.roughness = max( roughnessFactor, 0.0525 );material.roughness += geometryRoughness;
material.roughness = min( material.roughness, 1.0 );
#ifdef IOR
	material.ior = ior;
	#ifdef USE_SPECULAR
		float specularIntensityFactor = specularIntensity;
		vec3 specularColorFactor = specularColor;
		#ifdef USE_SPECULAR_COLORMAP
			specularColorFactor *= texture2D( specularColorMap, vSpecularColorMapUv ).rgb;
		#endif
		#ifdef USE_SPECULAR_INTENSITYMAP
			specularIntensityFactor *= texture2D( specularIntensityMap, vSpecularIntensityMapUv ).a;
		#endif
		material.specularF90 = mix( specularIntensityFactor, 1.0, metalnessFactor );
	#else
		float specularIntensityFactor = 1.0;
		vec3 specularColorFactor = vec3( 1.0 );
		material.specularF90 = 1.0;
	#endif
	material.specularColor = mix( min( pow2( ( material.ior - 1.0 ) / ( material.ior + 1.0 ) ) * specularColorFactor, vec3( 1.0 ) ) * specularIntensityFactor, diffuseColor.rgb, metalnessFactor );
#else
	material.specularColor = mix( vec3( 0.04 ), diffuseColor.rgb, metalnessFactor );
	material.specularF90 = 1.0;
#endif
#ifdef USE_CLEARCOAT
	material.clearcoat = clearcoat;
	material.clearcoatRoughness = clearcoatRoughness;
	material.clearcoatF0 = vec3( 0.04 );
	material.clearcoatF90 = 1.0;
	#ifdef USE_CLEARCOATMAP
		material.clearcoat *= texture2D( clearcoatMap, vClearcoatMapUv ).x;
	#endif
	#ifdef USE_CLEARCOAT_ROUGHNESSMAP
		material.clearcoatRoughness *= texture2D( clearcoatRoughnessMap, vClearcoatRoughnessMapUv ).y;
	#endif
	material.clearcoat = saturate( material.clearcoat );	material.clearcoatRoughness = max( material.clearcoatRoughness, 0.0525 );
	material.clearcoatRoughness += geometryRoughness;
	material.clearcoatRoughness = min( material.clearcoatRoughness, 1.0 );
#endif
#ifdef USE_DISPERSION
	material.dispersion = dispersion;
#endif
#ifdef USE_IRIDESCENCE
	material.iridescence = iridescence;
	material.iridescenceIOR = iridescenceIOR;
	#ifdef USE_IRIDESCENCEMAP
		material.iridescence *= texture2D( iridescenceMap, vIridescenceMapUv ).r;
	#endif
	#ifdef USE_IRIDESCENCE_THICKNESSMAP
		material.iridescenceThickness = (iridescenceThicknessMaximum - iridescenceThicknessMinimum) * texture2D( iridescenceThicknessMap, vIridescenceThicknessMapUv ).g + iridescenceThicknessMinimum;
	#else
		material.iridescenceThickness = iridescenceThicknessMaximum;
	#endif
#endif
#ifdef USE_SHEEN
	material.sheenColor = sheenColor;
	#ifdef USE_SHEEN_COLORMAP
		material.sheenColor *= texture2D( sheenColorMap, vSheenColorMapUv ).rgb;
	#endif
	material.sheenRoughness = clamp( sheenRoughness, 0.07, 1.0 );
	#ifdef USE_SHEEN_ROUGHNESSMAP
		material.sheenRoughness *= texture2D( sheenRoughnessMap, vSheenRoughnessMapUv ).a;
	#endif
#endif
#ifdef USE_ANISOTROPY
	#ifdef USE_ANISOTROPYMAP
		mat2 anisotropyMat = mat2( anisotropyVector.x, anisotropyVector.y, - anisotropyVector.y, anisotropyVector.x );
		vec3 anisotropyPolar = texture2D( anisotropyMap, vAnisotropyMapUv ).rgb;
		vec2 anisotropyV = anisotropyMat * normalize( 2.0 * anisotropyPolar.rg - vec2( 1.0 ) ) * anisotropyPolar.b;
	#else
		vec2 anisotropyV = anisotropyVector;
	#endif
	material.anisotropy = length( anisotropyV );
	if( material.anisotropy == 0.0 ) {
		anisotropyV = vec2( 1.0, 0.0 );
	} else {
		anisotropyV /= material.anisotropy;
		material.anisotropy = saturate( material.anisotropy );
	}
	material.alphaT = mix( pow2( material.roughness ), 1.0, pow2( material.anisotropy ) );
	material.anisotropyT = tbn[ 0 ] * anisotropyV.x + tbn[ 1 ] * anisotropyV.y;
	material.anisotropyB = tbn[ 1 ] * anisotropyV.x - tbn[ 0 ] * anisotropyV.y;
#endif`,OT=`struct PhysicalMaterial {
	vec3 diffuseColor;
	float roughness;
	vec3 specularColor;
	float specularF90;
	float dispersion;
	#ifdef USE_CLEARCOAT
		float clearcoat;
		float clearcoatRoughness;
		vec3 clearcoatF0;
		float clearcoatF90;
	#endif
	#ifdef USE_IRIDESCENCE
		float iridescence;
		float iridescenceIOR;
		float iridescenceThickness;
		vec3 iridescenceFresnel;
		vec3 iridescenceF0;
	#endif
	#ifdef USE_SHEEN
		vec3 sheenColor;
		float sheenRoughness;
	#endif
	#ifdef IOR
		float ior;
	#endif
	#ifdef USE_TRANSMISSION
		float transmission;
		float transmissionAlpha;
		float thickness;
		float attenuationDistance;
		vec3 attenuationColor;
	#endif
	#ifdef USE_ANISOTROPY
		float anisotropy;
		float alphaT;
		vec3 anisotropyT;
		vec3 anisotropyB;
	#endif
};
vec3 clearcoatSpecularDirect = vec3( 0.0 );
vec3 clearcoatSpecularIndirect = vec3( 0.0 );
vec3 sheenSpecularDirect = vec3( 0.0 );
vec3 sheenSpecularIndirect = vec3(0.0 );
vec3 Schlick_to_F0( const in vec3 f, const in float f90, const in float dotVH ) {
    float x = clamp( 1.0 - dotVH, 0.0, 1.0 );
    float x2 = x * x;
    float x5 = clamp( x * x2 * x2, 0.0, 0.9999 );
    return ( f - vec3( f90 ) * x5 ) / ( 1.0 - x5 );
}
float V_GGX_SmithCorrelated( const in float alpha, const in float dotNL, const in float dotNV ) {
	float a2 = pow2( alpha );
	float gv = dotNL * sqrt( a2 + ( 1.0 - a2 ) * pow2( dotNV ) );
	float gl = dotNV * sqrt( a2 + ( 1.0 - a2 ) * pow2( dotNL ) );
	return 0.5 / max( gv + gl, EPSILON );
}
float D_GGX( const in float alpha, const in float dotNH ) {
	float a2 = pow2( alpha );
	float denom = pow2( dotNH ) * ( a2 - 1.0 ) + 1.0;
	return RECIPROCAL_PI * a2 / pow2( denom );
}
#ifdef USE_ANISOTROPY
	float V_GGX_SmithCorrelated_Anisotropic( const in float alphaT, const in float alphaB, const in float dotTV, const in float dotBV, const in float dotTL, const in float dotBL, const in float dotNV, const in float dotNL ) {
		float gv = dotNL * length( vec3( alphaT * dotTV, alphaB * dotBV, dotNV ) );
		float gl = dotNV * length( vec3( alphaT * dotTL, alphaB * dotBL, dotNL ) );
		float v = 0.5 / ( gv + gl );
		return saturate(v);
	}
	float D_GGX_Anisotropic( const in float alphaT, const in float alphaB, const in float dotNH, const in float dotTH, const in float dotBH ) {
		float a2 = alphaT * alphaB;
		highp vec3 v = vec3( alphaB * dotTH, alphaT * dotBH, a2 * dotNH );
		highp float v2 = dot( v, v );
		float w2 = a2 / v2;
		return RECIPROCAL_PI * a2 * pow2 ( w2 );
	}
#endif
#ifdef USE_CLEARCOAT
	vec3 BRDF_GGX_Clearcoat( const in vec3 lightDir, const in vec3 viewDir, const in vec3 normal, const in PhysicalMaterial material) {
		vec3 f0 = material.clearcoatF0;
		float f90 = material.clearcoatF90;
		float roughness = material.clearcoatRoughness;
		float alpha = pow2( roughness );
		vec3 halfDir = normalize( lightDir + viewDir );
		float dotNL = saturate( dot( normal, lightDir ) );
		float dotNV = saturate( dot( normal, viewDir ) );
		float dotNH = saturate( dot( normal, halfDir ) );
		float dotVH = saturate( dot( viewDir, halfDir ) );
		vec3 F = F_Schlick( f0, f90, dotVH );
		float V = V_GGX_SmithCorrelated( alpha, dotNL, dotNV );
		float D = D_GGX( alpha, dotNH );
		return F * ( V * D );
	}
#endif
vec3 BRDF_GGX( const in vec3 lightDir, const in vec3 viewDir, const in vec3 normal, const in PhysicalMaterial material ) {
	vec3 f0 = material.specularColor;
	float f90 = material.specularF90;
	float roughness = material.roughness;
	float alpha = pow2( roughness );
	vec3 halfDir = normalize( lightDir + viewDir );
	float dotNL = saturate( dot( normal, lightDir ) );
	float dotNV = saturate( dot( normal, viewDir ) );
	float dotNH = saturate( dot( normal, halfDir ) );
	float dotVH = saturate( dot( viewDir, halfDir ) );
	vec3 F = F_Schlick( f0, f90, dotVH );
	#ifdef USE_IRIDESCENCE
		F = mix( F, material.iridescenceFresnel, material.iridescence );
	#endif
	#ifdef USE_ANISOTROPY
		float dotTL = dot( material.anisotropyT, lightDir );
		float dotTV = dot( material.anisotropyT, viewDir );
		float dotTH = dot( material.anisotropyT, halfDir );
		float dotBL = dot( material.anisotropyB, lightDir );
		float dotBV = dot( material.anisotropyB, viewDir );
		float dotBH = dot( material.anisotropyB, halfDir );
		float V = V_GGX_SmithCorrelated_Anisotropic( material.alphaT, alpha, dotTV, dotBV, dotTL, dotBL, dotNV, dotNL );
		float D = D_GGX_Anisotropic( material.alphaT, alpha, dotNH, dotTH, dotBH );
	#else
		float V = V_GGX_SmithCorrelated( alpha, dotNL, dotNV );
		float D = D_GGX( alpha, dotNH );
	#endif
	return F * ( V * D );
}
vec2 LTC_Uv( const in vec3 N, const in vec3 V, const in float roughness ) {
	const float LUT_SIZE = 64.0;
	const float LUT_SCALE = ( LUT_SIZE - 1.0 ) / LUT_SIZE;
	const float LUT_BIAS = 0.5 / LUT_SIZE;
	float dotNV = saturate( dot( N, V ) );
	vec2 uv = vec2( roughness, sqrt( 1.0 - dotNV ) );
	uv = uv * LUT_SCALE + LUT_BIAS;
	return uv;
}
float LTC_ClippedSphereFormFactor( const in vec3 f ) {
	float l = length( f );
	return max( ( l * l + f.z ) / ( l + 1.0 ), 0.0 );
}
vec3 LTC_EdgeVectorFormFactor( const in vec3 v1, const in vec3 v2 ) {
	float x = dot( v1, v2 );
	float y = abs( x );
	float a = 0.8543985 + ( 0.4965155 + 0.0145206 * y ) * y;
	float b = 3.4175940 + ( 4.1616724 + y ) * y;
	float v = a / b;
	float theta_sintheta = ( x > 0.0 ) ? v : 0.5 * inversesqrt( max( 1.0 - x * x, 1e-7 ) ) - v;
	return cross( v1, v2 ) * theta_sintheta;
}
vec3 LTC_Evaluate( const in vec3 N, const in vec3 V, const in vec3 P, const in mat3 mInv, const in vec3 rectCoords[ 4 ] ) {
	vec3 v1 = rectCoords[ 1 ] - rectCoords[ 0 ];
	vec3 v2 = rectCoords[ 3 ] - rectCoords[ 0 ];
	vec3 lightNormal = cross( v1, v2 );
	if( dot( lightNormal, P - rectCoords[ 0 ] ) < 0.0 ) return vec3( 0.0 );
	vec3 T1, T2;
	T1 = normalize( V - N * dot( V, N ) );
	T2 = - cross( N, T1 );
	mat3 mat = mInv * transposeMat3( mat3( T1, T2, N ) );
	vec3 coords[ 4 ];
	coords[ 0 ] = mat * ( rectCoords[ 0 ] - P );
	coords[ 1 ] = mat * ( rectCoords[ 1 ] - P );
	coords[ 2 ] = mat * ( rectCoords[ 2 ] - P );
	coords[ 3 ] = mat * ( rectCoords[ 3 ] - P );
	coords[ 0 ] = normalize( coords[ 0 ] );
	coords[ 1 ] = normalize( coords[ 1 ] );
	coords[ 2 ] = normalize( coords[ 2 ] );
	coords[ 3 ] = normalize( coords[ 3 ] );
	vec3 vectorFormFactor = vec3( 0.0 );
	vectorFormFactor += LTC_EdgeVectorFormFactor( coords[ 0 ], coords[ 1 ] );
	vectorFormFactor += LTC_EdgeVectorFormFactor( coords[ 1 ], coords[ 2 ] );
	vectorFormFactor += LTC_EdgeVectorFormFactor( coords[ 2 ], coords[ 3 ] );
	vectorFormFactor += LTC_EdgeVectorFormFactor( coords[ 3 ], coords[ 0 ] );
	float result = LTC_ClippedSphereFormFactor( vectorFormFactor );
	return vec3( result );
}
#if defined( USE_SHEEN )
float D_Charlie( float roughness, float dotNH ) {
	float alpha = pow2( roughness );
	float invAlpha = 1.0 / alpha;
	float cos2h = dotNH * dotNH;
	float sin2h = max( 1.0 - cos2h, 0.0078125 );
	return ( 2.0 + invAlpha ) * pow( sin2h, invAlpha * 0.5 ) / ( 2.0 * PI );
}
float V_Neubelt( float dotNV, float dotNL ) {
	return saturate( 1.0 / ( 4.0 * ( dotNL + dotNV - dotNL * dotNV ) ) );
}
vec3 BRDF_Sheen( const in vec3 lightDir, const in vec3 viewDir, const in vec3 normal, vec3 sheenColor, const in float sheenRoughness ) {
	vec3 halfDir = normalize( lightDir + viewDir );
	float dotNL = saturate( dot( normal, lightDir ) );
	float dotNV = saturate( dot( normal, viewDir ) );
	float dotNH = saturate( dot( normal, halfDir ) );
	float D = D_Charlie( sheenRoughness, dotNH );
	float V = V_Neubelt( dotNV, dotNL );
	return sheenColor * ( D * V );
}
#endif
float IBLSheenBRDF( const in vec3 normal, const in vec3 viewDir, const in float roughness ) {
	float dotNV = saturate( dot( normal, viewDir ) );
	float r2 = roughness * roughness;
	float a = roughness < 0.25 ? -339.2 * r2 + 161.4 * roughness - 25.9 : -8.48 * r2 + 14.3 * roughness - 9.95;
	float b = roughness < 0.25 ? 44.0 * r2 - 23.7 * roughness + 3.26 : 1.97 * r2 - 3.27 * roughness + 0.72;
	float DG = exp( a * dotNV + b ) + ( roughness < 0.25 ? 0.0 : 0.1 * ( roughness - 0.25 ) );
	return saturate( DG * RECIPROCAL_PI );
}
vec2 DFGApprox( const in vec3 normal, const in vec3 viewDir, const in float roughness ) {
	float dotNV = saturate( dot( normal, viewDir ) );
	const vec4 c0 = vec4( - 1, - 0.0275, - 0.572, 0.022 );
	const vec4 c1 = vec4( 1, 0.0425, 1.04, - 0.04 );
	vec4 r = roughness * c0 + c1;
	float a004 = min( r.x * r.x, exp2( - 9.28 * dotNV ) ) * r.x + r.y;
	vec2 fab = vec2( - 1.04, 1.04 ) * a004 + r.zw;
	return fab;
}
vec3 EnvironmentBRDF( const in vec3 normal, const in vec3 viewDir, const in vec3 specularColor, const in float specularF90, const in float roughness ) {
	vec2 fab = DFGApprox( normal, viewDir, roughness );
	return specularColor * fab.x + specularF90 * fab.y;
}
#ifdef USE_IRIDESCENCE
void computeMultiscatteringIridescence( const in vec3 normal, const in vec3 viewDir, const in vec3 specularColor, const in float specularF90, const in float iridescence, const in vec3 iridescenceF0, const in float roughness, inout vec3 singleScatter, inout vec3 multiScatter ) {
#else
void computeMultiscattering( const in vec3 normal, const in vec3 viewDir, const in vec3 specularColor, const in float specularF90, const in float roughness, inout vec3 singleScatter, inout vec3 multiScatter ) {
#endif
	vec2 fab = DFGApprox( normal, viewDir, roughness );
	#ifdef USE_IRIDESCENCE
		vec3 Fr = mix( specularColor, iridescenceF0, iridescence );
	#else
		vec3 Fr = specularColor;
	#endif
	vec3 FssEss = Fr * fab.x + specularF90 * fab.y;
	float Ess = fab.x + fab.y;
	float Ems = 1.0 - Ess;
	vec3 Favg = Fr + ( 1.0 - Fr ) * 0.047619;	vec3 Fms = FssEss * Favg / ( 1.0 - Ems * Favg );
	singleScatter += FssEss;
	multiScatter += Fms * Ems;
}
#if NUM_RECT_AREA_LIGHTS > 0
	void RE_Direct_RectArea_Physical( const in RectAreaLight rectAreaLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in PhysicalMaterial material, inout ReflectedLight reflectedLight ) {
		vec3 normal = geometryNormal;
		vec3 viewDir = geometryViewDir;
		vec3 position = geometryPosition;
		vec3 lightPos = rectAreaLight.position;
		vec3 halfWidth = rectAreaLight.halfWidth;
		vec3 halfHeight = rectAreaLight.halfHeight;
		vec3 lightColor = rectAreaLight.color;
		float roughness = material.roughness;
		vec3 rectCoords[ 4 ];
		rectCoords[ 0 ] = lightPos + halfWidth - halfHeight;		rectCoords[ 1 ] = lightPos - halfWidth - halfHeight;
		rectCoords[ 2 ] = lightPos - halfWidth + halfHeight;
		rectCoords[ 3 ] = lightPos + halfWidth + halfHeight;
		vec2 uv = LTC_Uv( normal, viewDir, roughness );
		vec4 t1 = texture2D( ltc_1, uv );
		vec4 t2 = texture2D( ltc_2, uv );
		mat3 mInv = mat3(
			vec3( t1.x, 0, t1.y ),
			vec3(    0, 1,    0 ),
			vec3( t1.z, 0, t1.w )
		);
		vec3 fresnel = ( material.specularColor * t2.x + ( vec3( 1.0 ) - material.specularColor ) * t2.y );
		reflectedLight.directSpecular += lightColor * fresnel * LTC_Evaluate( normal, viewDir, position, mInv, rectCoords );
		reflectedLight.directDiffuse += lightColor * material.diffuseColor * LTC_Evaluate( normal, viewDir, position, mat3( 1.0 ), rectCoords );
	}
#endif
void RE_Direct_Physical( const in IncidentLight directLight, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in PhysicalMaterial material, inout ReflectedLight reflectedLight ) {
	float dotNL = saturate( dot( geometryNormal, directLight.direction ) );
	vec3 irradiance = dotNL * directLight.color;
	#ifdef USE_CLEARCOAT
		float dotNLcc = saturate( dot( geometryClearcoatNormal, directLight.direction ) );
		vec3 ccIrradiance = dotNLcc * directLight.color;
		clearcoatSpecularDirect += ccIrradiance * BRDF_GGX_Clearcoat( directLight.direction, geometryViewDir, geometryClearcoatNormal, material );
	#endif
	#ifdef USE_SHEEN
		sheenSpecularDirect += irradiance * BRDF_Sheen( directLight.direction, geometryViewDir, geometryNormal, material.sheenColor, material.sheenRoughness );
	#endif
	reflectedLight.directSpecular += irradiance * BRDF_GGX( directLight.direction, geometryViewDir, geometryNormal, material );
	reflectedLight.directDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
void RE_IndirectDiffuse_Physical( const in vec3 irradiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in PhysicalMaterial material, inout ReflectedLight reflectedLight ) {
	reflectedLight.indirectDiffuse += irradiance * BRDF_Lambert( material.diffuseColor );
}
void RE_IndirectSpecular_Physical( const in vec3 radiance, const in vec3 irradiance, const in vec3 clearcoatRadiance, const in vec3 geometryPosition, const in vec3 geometryNormal, const in vec3 geometryViewDir, const in vec3 geometryClearcoatNormal, const in PhysicalMaterial material, inout ReflectedLight reflectedLight) {
	#ifdef USE_CLEARCOAT
		clearcoatSpecularIndirect += clearcoatRadiance * EnvironmentBRDF( geometryClearcoatNormal, geometryViewDir, material.clearcoatF0, material.clearcoatF90, material.clearcoatRoughness );
	#endif
	#ifdef USE_SHEEN
		sheenSpecularIndirect += irradiance * material.sheenColor * IBLSheenBRDF( geometryNormal, geometryViewDir, material.sheenRoughness );
	#endif
	vec3 singleScattering = vec3( 0.0 );
	vec3 multiScattering = vec3( 0.0 );
	vec3 cosineWeightedIrradiance = irradiance * RECIPROCAL_PI;
	#ifdef USE_IRIDESCENCE
		computeMultiscatteringIridescence( geometryNormal, geometryViewDir, material.specularColor, material.specularF90, material.iridescence, material.iridescenceFresnel, material.roughness, singleScattering, multiScattering );
	#else
		computeMultiscattering( geometryNormal, geometryViewDir, material.specularColor, material.specularF90, material.roughness, singleScattering, multiScattering );
	#endif
	vec3 totalScattering = singleScattering + multiScattering;
	vec3 diffuse = material.diffuseColor * ( 1.0 - max( max( totalScattering.r, totalScattering.g ), totalScattering.b ) );
	reflectedLight.indirectSpecular += radiance * singleScattering;
	reflectedLight.indirectSpecular += multiScattering * cosineWeightedIrradiance;
	reflectedLight.indirectDiffuse += diffuse * cosineWeightedIrradiance;
}
#define RE_Direct				RE_Direct_Physical
#define RE_Direct_RectArea		RE_Direct_RectArea_Physical
#define RE_IndirectDiffuse		RE_IndirectDiffuse_Physical
#define RE_IndirectSpecular		RE_IndirectSpecular_Physical
float computeSpecularOcclusion( const in float dotNV, const in float ambientOcclusion, const in float roughness ) {
	return saturate( pow( dotNV + ambientOcclusion, exp2( - 16.0 * roughness - 1.0 ) ) - 1.0 + ambientOcclusion );
}`,PT=`
vec3 geometryPosition = - vViewPosition;
vec3 geometryNormal = normal;
vec3 geometryViewDir = ( isOrthographic ) ? vec3( 0, 0, 1 ) : normalize( vViewPosition );
vec3 geometryClearcoatNormal = vec3( 0.0 );
#ifdef USE_CLEARCOAT
	geometryClearcoatNormal = clearcoatNormal;
#endif
#ifdef USE_IRIDESCENCE
	float dotNVi = saturate( dot( normal, geometryViewDir ) );
	if ( material.iridescenceThickness == 0.0 ) {
		material.iridescence = 0.0;
	} else {
		material.iridescence = saturate( material.iridescence );
	}
	if ( material.iridescence > 0.0 ) {
		material.iridescenceFresnel = evalIridescence( 1.0, material.iridescenceIOR, dotNVi, material.iridescenceThickness, material.specularColor );
		material.iridescenceF0 = Schlick_to_F0( material.iridescenceFresnel, 1.0, dotNVi );
	}
#endif
IncidentLight directLight;
#if ( NUM_POINT_LIGHTS > 0 ) && defined( RE_Direct )
	PointLight pointLight;
	#if defined( USE_SHADOWMAP ) && NUM_POINT_LIGHT_SHADOWS > 0
	PointLightShadow pointLightShadow;
	#endif
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_POINT_LIGHTS; i ++ ) {
		pointLight = pointLights[ i ];
		getPointLightInfo( pointLight, geometryPosition, directLight );
		#if defined( USE_SHADOWMAP ) && ( UNROLLED_LOOP_INDEX < NUM_POINT_LIGHT_SHADOWS )
		pointLightShadow = pointLightShadows[ i ];
		directLight.color *= ( directLight.visible && receiveShadow ) ? getPointShadow( pointShadowMap[ i ], pointLightShadow.shadowMapSize, pointLightShadow.shadowIntensity, pointLightShadow.shadowBias, pointLightShadow.shadowRadius, vPointShadowCoord[ i ], pointLightShadow.shadowCameraNear, pointLightShadow.shadowCameraFar ) : 1.0;
		#endif
		RE_Direct( directLight, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
	}
	#pragma unroll_loop_end
#endif
#if ( NUM_SPOT_LIGHTS > 0 ) && defined( RE_Direct )
	SpotLight spotLight;
	vec4 spotColor;
	vec3 spotLightCoord;
	bool inSpotLightMap;
	#if defined( USE_SHADOWMAP ) && NUM_SPOT_LIGHT_SHADOWS > 0
	SpotLightShadow spotLightShadow;
	#endif
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_SPOT_LIGHTS; i ++ ) {
		spotLight = spotLights[ i ];
		getSpotLightInfo( spotLight, geometryPosition, directLight );
		#if ( UNROLLED_LOOP_INDEX < NUM_SPOT_LIGHT_SHADOWS_WITH_MAPS )
		#define SPOT_LIGHT_MAP_INDEX UNROLLED_LOOP_INDEX
		#elif ( UNROLLED_LOOP_INDEX < NUM_SPOT_LIGHT_SHADOWS )
		#define SPOT_LIGHT_MAP_INDEX NUM_SPOT_LIGHT_MAPS
		#else
		#define SPOT_LIGHT_MAP_INDEX ( UNROLLED_LOOP_INDEX - NUM_SPOT_LIGHT_SHADOWS + NUM_SPOT_LIGHT_SHADOWS_WITH_MAPS )
		#endif
		#if ( SPOT_LIGHT_MAP_INDEX < NUM_SPOT_LIGHT_MAPS )
			spotLightCoord = vSpotLightCoord[ i ].xyz / vSpotLightCoord[ i ].w;
			inSpotLightMap = all( lessThan( abs( spotLightCoord * 2. - 1. ), vec3( 1.0 ) ) );
			spotColor = texture2D( spotLightMap[ SPOT_LIGHT_MAP_INDEX ], spotLightCoord.xy );
			directLight.color = inSpotLightMap ? directLight.color * spotColor.rgb : directLight.color;
		#endif
		#undef SPOT_LIGHT_MAP_INDEX
		#if defined( USE_SHADOWMAP ) && ( UNROLLED_LOOP_INDEX < NUM_SPOT_LIGHT_SHADOWS )
		spotLightShadow = spotLightShadows[ i ];
		directLight.color *= ( directLight.visible && receiveShadow ) ? getShadow( spotShadowMap[ i ], spotLightShadow.shadowMapSize, spotLightShadow.shadowIntensity, spotLightShadow.shadowBias, spotLightShadow.shadowRadius, vSpotLightCoord[ i ] ) : 1.0;
		#endif
		RE_Direct( directLight, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
	}
	#pragma unroll_loop_end
#endif
#if ( NUM_DIR_LIGHTS > 0 ) && defined( RE_Direct )
	DirectionalLight directionalLight;
	#if defined( USE_SHADOWMAP ) && NUM_DIR_LIGHT_SHADOWS > 0
	DirectionalLightShadow directionalLightShadow;
	#endif
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_DIR_LIGHTS; i ++ ) {
		directionalLight = directionalLights[ i ];
		getDirectionalLightInfo( directionalLight, directLight );
		#if defined( USE_SHADOWMAP ) && ( UNROLLED_LOOP_INDEX < NUM_DIR_LIGHT_SHADOWS )
		directionalLightShadow = directionalLightShadows[ i ];
		directLight.color *= ( directLight.visible && receiveShadow ) ? getShadow( directionalShadowMap[ i ], directionalLightShadow.shadowMapSize, directionalLightShadow.shadowIntensity, directionalLightShadow.shadowBias, directionalLightShadow.shadowRadius, vDirectionalShadowCoord[ i ] ) : 1.0;
		#endif
		RE_Direct( directLight, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
	}
	#pragma unroll_loop_end
#endif
#if ( NUM_RECT_AREA_LIGHTS > 0 ) && defined( RE_Direct_RectArea )
	RectAreaLight rectAreaLight;
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_RECT_AREA_LIGHTS; i ++ ) {
		rectAreaLight = rectAreaLights[ i ];
		RE_Direct_RectArea( rectAreaLight, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
	}
	#pragma unroll_loop_end
#endif
#if defined( RE_IndirectDiffuse )
	vec3 iblIrradiance = vec3( 0.0 );
	vec3 irradiance = getAmbientLightIrradiance( ambientLightColor );
	#if defined( USE_LIGHT_PROBES )
		irradiance += getLightProbeIrradiance( lightProbe, geometryNormal );
	#endif
	#if ( NUM_HEMI_LIGHTS > 0 )
		#pragma unroll_loop_start
		for ( int i = 0; i < NUM_HEMI_LIGHTS; i ++ ) {
			irradiance += getHemisphereLightIrradiance( hemisphereLights[ i ], geometryNormal );
		}
		#pragma unroll_loop_end
	#endif
#endif
#if defined( RE_IndirectSpecular )
	vec3 radiance = vec3( 0.0 );
	vec3 clearcoatRadiance = vec3( 0.0 );
#endif`,zT=`#if defined( RE_IndirectDiffuse )
	#ifdef USE_LIGHTMAP
		vec4 lightMapTexel = texture2D( lightMap, vLightMapUv );
		vec3 lightMapIrradiance = lightMapTexel.rgb * lightMapIntensity;
		irradiance += lightMapIrradiance;
	#endif
	#if defined( USE_ENVMAP ) && defined( STANDARD ) && defined( ENVMAP_TYPE_CUBE_UV )
		iblIrradiance += getIBLIrradiance( geometryNormal );
	#endif
#endif
#if defined( USE_ENVMAP ) && defined( RE_IndirectSpecular )
	#ifdef USE_ANISOTROPY
		radiance += getIBLAnisotropyRadiance( geometryViewDir, geometryNormal, material.roughness, material.anisotropyB, material.anisotropy );
	#else
		radiance += getIBLRadiance( geometryViewDir, geometryNormal, material.roughness );
	#endif
	#ifdef USE_CLEARCOAT
		clearcoatRadiance += getIBLRadiance( geometryViewDir, geometryClearcoatNormal, material.clearcoatRoughness );
	#endif
#endif`,BT=`#if defined( RE_IndirectDiffuse )
	RE_IndirectDiffuse( irradiance, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
#endif
#if defined( RE_IndirectSpecular )
	RE_IndirectSpecular( radiance, iblIrradiance, clearcoatRadiance, geometryPosition, geometryNormal, geometryViewDir, geometryClearcoatNormal, material, reflectedLight );
#endif`,IT=`#if defined( USE_LOGARITHMIC_DEPTH_BUFFER )
	gl_FragDepth = vIsPerspective == 0.0 ? gl_FragCoord.z : log2( vFragDepth ) * logDepthBufFC * 0.5;
#endif`,FT=`#if defined( USE_LOGARITHMIC_DEPTH_BUFFER )
	uniform float logDepthBufFC;
	varying float vFragDepth;
	varying float vIsPerspective;
#endif`,HT=`#ifdef USE_LOGARITHMIC_DEPTH_BUFFER
	varying float vFragDepth;
	varying float vIsPerspective;
#endif`,GT=`#ifdef USE_LOGARITHMIC_DEPTH_BUFFER
	vFragDepth = 1.0 + gl_Position.w;
	vIsPerspective = float( isPerspectiveMatrix( projectionMatrix ) );
#endif`,VT=`#ifdef USE_MAP
	vec4 sampledDiffuseColor = texture2D( map, vMapUv );
	#ifdef DECODE_VIDEO_TEXTURE
		sampledDiffuseColor = sRGBTransferEOTF( sampledDiffuseColor );
	#endif
	diffuseColor *= sampledDiffuseColor;
#endif`,kT=`#ifdef USE_MAP
	uniform sampler2D map;
#endif`,XT=`#if defined( USE_MAP ) || defined( USE_ALPHAMAP )
	#if defined( USE_POINTS_UV )
		vec2 uv = vUv;
	#else
		vec2 uv = ( uvTransform * vec3( gl_PointCoord.x, 1.0 - gl_PointCoord.y, 1 ) ).xy;
	#endif
#endif
#ifdef USE_MAP
	diffuseColor *= texture2D( map, uv );
#endif
#ifdef USE_ALPHAMAP
	diffuseColor.a *= texture2D( alphaMap, uv ).g;
#endif`,WT=`#if defined( USE_POINTS_UV )
	varying vec2 vUv;
#else
	#if defined( USE_MAP ) || defined( USE_ALPHAMAP )
		uniform mat3 uvTransform;
	#endif
#endif
#ifdef USE_MAP
	uniform sampler2D map;
#endif
#ifdef USE_ALPHAMAP
	uniform sampler2D alphaMap;
#endif`,jT=`float metalnessFactor = metalness;
#ifdef USE_METALNESSMAP
	vec4 texelMetalness = texture2D( metalnessMap, vMetalnessMapUv );
	metalnessFactor *= texelMetalness.b;
#endif`,YT=`#ifdef USE_METALNESSMAP
	uniform sampler2D metalnessMap;
#endif`,qT=`#ifdef USE_INSTANCING_MORPH
	float morphTargetInfluences[ MORPHTARGETS_COUNT ];
	float morphTargetBaseInfluence = texelFetch( morphTexture, ivec2( 0, gl_InstanceID ), 0 ).r;
	for ( int i = 0; i < MORPHTARGETS_COUNT; i ++ ) {
		morphTargetInfluences[i] =  texelFetch( morphTexture, ivec2( i + 1, gl_InstanceID ), 0 ).r;
	}
#endif`,ZT=`#if defined( USE_MORPHCOLORS )
	vColor *= morphTargetBaseInfluence;
	for ( int i = 0; i < MORPHTARGETS_COUNT; i ++ ) {
		#if defined( USE_COLOR_ALPHA )
			if ( morphTargetInfluences[ i ] != 0.0 ) vColor += getMorph( gl_VertexID, i, 2 ) * morphTargetInfluences[ i ];
		#elif defined( USE_COLOR )
			if ( morphTargetInfluences[ i ] != 0.0 ) vColor += getMorph( gl_VertexID, i, 2 ).rgb * morphTargetInfluences[ i ];
		#endif
	}
#endif`,KT=`#ifdef USE_MORPHNORMALS
	objectNormal *= morphTargetBaseInfluence;
	for ( int i = 0; i < MORPHTARGETS_COUNT; i ++ ) {
		if ( morphTargetInfluences[ i ] != 0.0 ) objectNormal += getMorph( gl_VertexID, i, 1 ).xyz * morphTargetInfluences[ i ];
	}
#endif`,QT=`#ifdef USE_MORPHTARGETS
	#ifndef USE_INSTANCING_MORPH
		uniform float morphTargetBaseInfluence;
		uniform float morphTargetInfluences[ MORPHTARGETS_COUNT ];
	#endif
	uniform sampler2DArray morphTargetsTexture;
	uniform ivec2 morphTargetsTextureSize;
	vec4 getMorph( const in int vertexIndex, const in int morphTargetIndex, const in int offset ) {
		int texelIndex = vertexIndex * MORPHTARGETS_TEXTURE_STRIDE + offset;
		int y = texelIndex / morphTargetsTextureSize.x;
		int x = texelIndex - y * morphTargetsTextureSize.x;
		ivec3 morphUV = ivec3( x, y, morphTargetIndex );
		return texelFetch( morphTargetsTexture, morphUV, 0 );
	}
#endif`,JT=`#ifdef USE_MORPHTARGETS
	transformed *= morphTargetBaseInfluence;
	for ( int i = 0; i < MORPHTARGETS_COUNT; i ++ ) {
		if ( morphTargetInfluences[ i ] != 0.0 ) transformed += getMorph( gl_VertexID, i, 0 ).xyz * morphTargetInfluences[ i ];
	}
#endif`,$T=`float faceDirection = gl_FrontFacing ? 1.0 : - 1.0;
#ifdef FLAT_SHADED
	vec3 fdx = dFdx( vViewPosition );
	vec3 fdy = dFdy( vViewPosition );
	vec3 normal = normalize( cross( fdx, fdy ) );
#else
	vec3 normal = normalize( vNormal );
	#ifdef DOUBLE_SIDED
		normal *= faceDirection;
	#endif
#endif
#if defined( USE_NORMALMAP_TANGENTSPACE ) || defined( USE_CLEARCOAT_NORMALMAP ) || defined( USE_ANISOTROPY )
	#ifdef USE_TANGENT
		mat3 tbn = mat3( normalize( vTangent ), normalize( vBitangent ), normal );
	#else
		mat3 tbn = getTangentFrame( - vViewPosition, normal,
		#if defined( USE_NORMALMAP )
			vNormalMapUv
		#elif defined( USE_CLEARCOAT_NORMALMAP )
			vClearcoatNormalMapUv
		#else
			vUv
		#endif
		);
	#endif
	#if defined( DOUBLE_SIDED ) && ! defined( FLAT_SHADED )
		tbn[0] *= faceDirection;
		tbn[1] *= faceDirection;
	#endif
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	#ifdef USE_TANGENT
		mat3 tbn2 = mat3( normalize( vTangent ), normalize( vBitangent ), normal );
	#else
		mat3 tbn2 = getTangentFrame( - vViewPosition, normal, vClearcoatNormalMapUv );
	#endif
	#if defined( DOUBLE_SIDED ) && ! defined( FLAT_SHADED )
		tbn2[0] *= faceDirection;
		tbn2[1] *= faceDirection;
	#endif
#endif
vec3 nonPerturbedNormal = normal;`,tb=`#ifdef USE_NORMALMAP_OBJECTSPACE
	normal = texture2D( normalMap, vNormalMapUv ).xyz * 2.0 - 1.0;
	#ifdef FLIP_SIDED
		normal = - normal;
	#endif
	#ifdef DOUBLE_SIDED
		normal = normal * faceDirection;
	#endif
	normal = normalize( normalMatrix * normal );
#elif defined( USE_NORMALMAP_TANGENTSPACE )
	vec3 mapN = texture2D( normalMap, vNormalMapUv ).xyz * 2.0 - 1.0;
	mapN.xy *= normalScale;
	normal = normalize( tbn * mapN );
#elif defined( USE_BUMPMAP )
	normal = perturbNormalArb( - vViewPosition, normal, dHdxy_fwd(), faceDirection );
#endif`,eb=`#ifndef FLAT_SHADED
	varying vec3 vNormal;
	#ifdef USE_TANGENT
		varying vec3 vTangent;
		varying vec3 vBitangent;
	#endif
#endif`,nb=`#ifndef FLAT_SHADED
	varying vec3 vNormal;
	#ifdef USE_TANGENT
		varying vec3 vTangent;
		varying vec3 vBitangent;
	#endif
#endif`,ib=`#ifndef FLAT_SHADED
	vNormal = normalize( transformedNormal );
	#ifdef USE_TANGENT
		vTangent = normalize( transformedTangent );
		vBitangent = normalize( cross( vNormal, vTangent ) * tangent.w );
	#endif
#endif`,ab=`#ifdef USE_NORMALMAP
	uniform sampler2D normalMap;
	uniform vec2 normalScale;
#endif
#ifdef USE_NORMALMAP_OBJECTSPACE
	uniform mat3 normalMatrix;
#endif
#if ! defined ( USE_TANGENT ) && ( defined ( USE_NORMALMAP_TANGENTSPACE ) || defined ( USE_CLEARCOAT_NORMALMAP ) || defined( USE_ANISOTROPY ) )
	mat3 getTangentFrame( vec3 eye_pos, vec3 surf_norm, vec2 uv ) {
		vec3 q0 = dFdx( eye_pos.xyz );
		vec3 q1 = dFdy( eye_pos.xyz );
		vec2 st0 = dFdx( uv.st );
		vec2 st1 = dFdy( uv.st );
		vec3 N = surf_norm;
		vec3 q1perp = cross( q1, N );
		vec3 q0perp = cross( N, q0 );
		vec3 T = q1perp * st0.x + q0perp * st1.x;
		vec3 B = q1perp * st0.y + q0perp * st1.y;
		float det = max( dot( T, T ), dot( B, B ) );
		float scale = ( det == 0.0 ) ? 0.0 : inversesqrt( det );
		return mat3( T * scale, B * scale, N );
	}
#endif`,sb=`#ifdef USE_CLEARCOAT
	vec3 clearcoatNormal = nonPerturbedNormal;
#endif`,rb=`#ifdef USE_CLEARCOAT_NORMALMAP
	vec3 clearcoatMapN = texture2D( clearcoatNormalMap, vClearcoatNormalMapUv ).xyz * 2.0 - 1.0;
	clearcoatMapN.xy *= clearcoatNormalScale;
	clearcoatNormal = normalize( tbn2 * clearcoatMapN );
#endif`,ob=`#ifdef USE_CLEARCOATMAP
	uniform sampler2D clearcoatMap;
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	uniform sampler2D clearcoatNormalMap;
	uniform vec2 clearcoatNormalScale;
#endif
#ifdef USE_CLEARCOAT_ROUGHNESSMAP
	uniform sampler2D clearcoatRoughnessMap;
#endif`,lb=`#ifdef USE_IRIDESCENCEMAP
	uniform sampler2D iridescenceMap;
#endif
#ifdef USE_IRIDESCENCE_THICKNESSMAP
	uniform sampler2D iridescenceThicknessMap;
#endif`,cb=`#ifdef OPAQUE
diffuseColor.a = 1.0;
#endif
#ifdef USE_TRANSMISSION
diffuseColor.a *= material.transmissionAlpha;
#endif
gl_FragColor = vec4( outgoingLight, diffuseColor.a );`,ub=`vec3 packNormalToRGB( const in vec3 normal ) {
	return normalize( normal ) * 0.5 + 0.5;
}
vec3 unpackRGBToNormal( const in vec3 rgb ) {
	return 2.0 * rgb.xyz - 1.0;
}
const float PackUpscale = 256. / 255.;const float UnpackDownscale = 255. / 256.;const float ShiftRight8 = 1. / 256.;
const float Inv255 = 1. / 255.;
const vec4 PackFactors = vec4( 1.0, 256.0, 256.0 * 256.0, 256.0 * 256.0 * 256.0 );
const vec2 UnpackFactors2 = vec2( UnpackDownscale, 1.0 / PackFactors.g );
const vec3 UnpackFactors3 = vec3( UnpackDownscale / PackFactors.rg, 1.0 / PackFactors.b );
const vec4 UnpackFactors4 = vec4( UnpackDownscale / PackFactors.rgb, 1.0 / PackFactors.a );
vec4 packDepthToRGBA( const in float v ) {
	if( v <= 0.0 )
		return vec4( 0., 0., 0., 0. );
	if( v >= 1.0 )
		return vec4( 1., 1., 1., 1. );
	float vuf;
	float af = modf( v * PackFactors.a, vuf );
	float bf = modf( vuf * ShiftRight8, vuf );
	float gf = modf( vuf * ShiftRight8, vuf );
	return vec4( vuf * Inv255, gf * PackUpscale, bf * PackUpscale, af );
}
vec3 packDepthToRGB( const in float v ) {
	if( v <= 0.0 )
		return vec3( 0., 0., 0. );
	if( v >= 1.0 )
		return vec3( 1., 1., 1. );
	float vuf;
	float bf = modf( v * PackFactors.b, vuf );
	float gf = modf( vuf * ShiftRight8, vuf );
	return vec3( vuf * Inv255, gf * PackUpscale, bf );
}
vec2 packDepthToRG( const in float v ) {
	if( v <= 0.0 )
		return vec2( 0., 0. );
	if( v >= 1.0 )
		return vec2( 1., 1. );
	float vuf;
	float gf = modf( v * 256., vuf );
	return vec2( vuf * Inv255, gf );
}
float unpackRGBAToDepth( const in vec4 v ) {
	return dot( v, UnpackFactors4 );
}
float unpackRGBToDepth( const in vec3 v ) {
	return dot( v, UnpackFactors3 );
}
float unpackRGToDepth( const in vec2 v ) {
	return v.r * UnpackFactors2.r + v.g * UnpackFactors2.g;
}
vec4 pack2HalfToRGBA( const in vec2 v ) {
	vec4 r = vec4( v.x, fract( v.x * 255.0 ), v.y, fract( v.y * 255.0 ) );
	return vec4( r.x - r.y / 255.0, r.y, r.z - r.w / 255.0, r.w );
}
vec2 unpackRGBATo2Half( const in vec4 v ) {
	return vec2( v.x + ( v.y / 255.0 ), v.z + ( v.w / 255.0 ) );
}
float viewZToOrthographicDepth( const in float viewZ, const in float near, const in float far ) {
	return ( viewZ + near ) / ( near - far );
}
float orthographicDepthToViewZ( const in float depth, const in float near, const in float far ) {
	return depth * ( near - far ) - near;
}
float viewZToPerspectiveDepth( const in float viewZ, const in float near, const in float far ) {
	return ( ( near + viewZ ) * far ) / ( ( far - near ) * viewZ );
}
float perspectiveDepthToViewZ( const in float depth, const in float near, const in float far ) {
	return ( near * far ) / ( ( far - near ) * depth - far );
}`,fb=`#ifdef PREMULTIPLIED_ALPHA
	gl_FragColor.rgb *= gl_FragColor.a;
#endif`,hb=`vec4 mvPosition = vec4( transformed, 1.0 );
#ifdef USE_BATCHING
	mvPosition = batchingMatrix * mvPosition;
#endif
#ifdef USE_INSTANCING
	mvPosition = instanceMatrix * mvPosition;
#endif
mvPosition = modelViewMatrix * mvPosition;
gl_Position = projectionMatrix * mvPosition;`,db=`#ifdef DITHERING
	gl_FragColor.rgb = dithering( gl_FragColor.rgb );
#endif`,pb=`#ifdef DITHERING
	vec3 dithering( vec3 color ) {
		float grid_position = rand( gl_FragCoord.xy );
		vec3 dither_shift_RGB = vec3( 0.25 / 255.0, -0.25 / 255.0, 0.25 / 255.0 );
		dither_shift_RGB = mix( 2.0 * dither_shift_RGB, -2.0 * dither_shift_RGB, grid_position );
		return color + dither_shift_RGB;
	}
#endif`,mb=`float roughnessFactor = roughness;
#ifdef USE_ROUGHNESSMAP
	vec4 texelRoughness = texture2D( roughnessMap, vRoughnessMapUv );
	roughnessFactor *= texelRoughness.g;
#endif`,gb=`#ifdef USE_ROUGHNESSMAP
	uniform sampler2D roughnessMap;
#endif`,_b=`#if NUM_SPOT_LIGHT_COORDS > 0
	varying vec4 vSpotLightCoord[ NUM_SPOT_LIGHT_COORDS ];
#endif
#if NUM_SPOT_LIGHT_MAPS > 0
	uniform sampler2D spotLightMap[ NUM_SPOT_LIGHT_MAPS ];
#endif
#ifdef USE_SHADOWMAP
	#if NUM_DIR_LIGHT_SHADOWS > 0
		uniform sampler2D directionalShadowMap[ NUM_DIR_LIGHT_SHADOWS ];
		varying vec4 vDirectionalShadowCoord[ NUM_DIR_LIGHT_SHADOWS ];
		struct DirectionalLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
		};
		uniform DirectionalLightShadow directionalLightShadows[ NUM_DIR_LIGHT_SHADOWS ];
	#endif
	#if NUM_SPOT_LIGHT_SHADOWS > 0
		uniform sampler2D spotShadowMap[ NUM_SPOT_LIGHT_SHADOWS ];
		struct SpotLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
		};
		uniform SpotLightShadow spotLightShadows[ NUM_SPOT_LIGHT_SHADOWS ];
	#endif
	#if NUM_POINT_LIGHT_SHADOWS > 0
		uniform sampler2D pointShadowMap[ NUM_POINT_LIGHT_SHADOWS ];
		varying vec4 vPointShadowCoord[ NUM_POINT_LIGHT_SHADOWS ];
		struct PointLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
			float shadowCameraNear;
			float shadowCameraFar;
		};
		uniform PointLightShadow pointLightShadows[ NUM_POINT_LIGHT_SHADOWS ];
	#endif
	float texture2DCompare( sampler2D depths, vec2 uv, float compare ) {
		float depth = unpackRGBAToDepth( texture2D( depths, uv ) );
		#ifdef USE_REVERSED_DEPTH_BUFFER
			return step( depth, compare );
		#else
			return step( compare, depth );
		#endif
	}
	vec2 texture2DDistribution( sampler2D shadow, vec2 uv ) {
		return unpackRGBATo2Half( texture2D( shadow, uv ) );
	}
	float VSMShadow( sampler2D shadow, vec2 uv, float compare ) {
		float occlusion = 1.0;
		vec2 distribution = texture2DDistribution( shadow, uv );
		#ifdef USE_REVERSED_DEPTH_BUFFER
			float hard_shadow = step( distribution.x, compare );
		#else
			float hard_shadow = step( compare, distribution.x );
		#endif
		if ( hard_shadow != 1.0 ) {
			float distance = compare - distribution.x;
			float variance = max( 0.00000, distribution.y * distribution.y );
			float softness_probability = variance / (variance + distance * distance );			softness_probability = clamp( ( softness_probability - 0.3 ) / ( 0.95 - 0.3 ), 0.0, 1.0 );			occlusion = clamp( max( hard_shadow, softness_probability ), 0.0, 1.0 );
		}
		return occlusion;
	}
	float getShadow( sampler2D shadowMap, vec2 shadowMapSize, float shadowIntensity, float shadowBias, float shadowRadius, vec4 shadowCoord ) {
		float shadow = 1.0;
		shadowCoord.xyz /= shadowCoord.w;
		shadowCoord.z += shadowBias;
		bool inFrustum = shadowCoord.x >= 0.0 && shadowCoord.x <= 1.0 && shadowCoord.y >= 0.0 && shadowCoord.y <= 1.0;
		bool frustumTest = inFrustum && shadowCoord.z <= 1.0;
		if ( frustumTest ) {
		#if defined( SHADOWMAP_TYPE_PCF )
			vec2 texelSize = vec2( 1.0 ) / shadowMapSize;
			float dx0 = - texelSize.x * shadowRadius;
			float dy0 = - texelSize.y * shadowRadius;
			float dx1 = + texelSize.x * shadowRadius;
			float dy1 = + texelSize.y * shadowRadius;
			float dx2 = dx0 / 2.0;
			float dy2 = dy0 / 2.0;
			float dx3 = dx1 / 2.0;
			float dy3 = dy1 / 2.0;
			shadow = (
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx0, dy0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( 0.0, dy0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx1, dy0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx2, dy2 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( 0.0, dy2 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx3, dy2 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx0, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx2, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy, shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx3, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx1, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx2, dy3 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( 0.0, dy3 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx3, dy3 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx0, dy1 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( 0.0, dy1 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, shadowCoord.xy + vec2( dx1, dy1 ), shadowCoord.z )
			) * ( 1.0 / 17.0 );
		#elif defined( SHADOWMAP_TYPE_PCF_SOFT )
			vec2 texelSize = vec2( 1.0 ) / shadowMapSize;
			float dx = texelSize.x;
			float dy = texelSize.y;
			vec2 uv = shadowCoord.xy;
			vec2 f = fract( uv * shadowMapSize + 0.5 );
			uv -= f * texelSize;
			shadow = (
				texture2DCompare( shadowMap, uv, shadowCoord.z ) +
				texture2DCompare( shadowMap, uv + vec2( dx, 0.0 ), shadowCoord.z ) +
				texture2DCompare( shadowMap, uv + vec2( 0.0, dy ), shadowCoord.z ) +
				texture2DCompare( shadowMap, uv + texelSize, shadowCoord.z ) +
				mix( texture2DCompare( shadowMap, uv + vec2( -dx, 0.0 ), shadowCoord.z ),
					 texture2DCompare( shadowMap, uv + vec2( 2.0 * dx, 0.0 ), shadowCoord.z ),
					 f.x ) +
				mix( texture2DCompare( shadowMap, uv + vec2( -dx, dy ), shadowCoord.z ),
					 texture2DCompare( shadowMap, uv + vec2( 2.0 * dx, dy ), shadowCoord.z ),
					 f.x ) +
				mix( texture2DCompare( shadowMap, uv + vec2( 0.0, -dy ), shadowCoord.z ),
					 texture2DCompare( shadowMap, uv + vec2( 0.0, 2.0 * dy ), shadowCoord.z ),
					 f.y ) +
				mix( texture2DCompare( shadowMap, uv + vec2( dx, -dy ), shadowCoord.z ),
					 texture2DCompare( shadowMap, uv + vec2( dx, 2.0 * dy ), shadowCoord.z ),
					 f.y ) +
				mix( mix( texture2DCompare( shadowMap, uv + vec2( -dx, -dy ), shadowCoord.z ),
						  texture2DCompare( shadowMap, uv + vec2( 2.0 * dx, -dy ), shadowCoord.z ),
						  f.x ),
					 mix( texture2DCompare( shadowMap, uv + vec2( -dx, 2.0 * dy ), shadowCoord.z ),
						  texture2DCompare( shadowMap, uv + vec2( 2.0 * dx, 2.0 * dy ), shadowCoord.z ),
						  f.x ),
					 f.y )
			) * ( 1.0 / 9.0 );
		#elif defined( SHADOWMAP_TYPE_VSM )
			shadow = VSMShadow( shadowMap, shadowCoord.xy, shadowCoord.z );
		#else
			shadow = texture2DCompare( shadowMap, shadowCoord.xy, shadowCoord.z );
		#endif
		}
		return mix( 1.0, shadow, shadowIntensity );
	}
	vec2 cubeToUV( vec3 v, float texelSizeY ) {
		vec3 absV = abs( v );
		float scaleToCube = 1.0 / max( absV.x, max( absV.y, absV.z ) );
		absV *= scaleToCube;
		v *= scaleToCube * ( 1.0 - 2.0 * texelSizeY );
		vec2 planar = v.xy;
		float almostATexel = 1.5 * texelSizeY;
		float almostOne = 1.0 - almostATexel;
		if ( absV.z >= almostOne ) {
			if ( v.z > 0.0 )
				planar.x = 4.0 - v.x;
		} else if ( absV.x >= almostOne ) {
			float signX = sign( v.x );
			planar.x = v.z * signX + 2.0 * signX;
		} else if ( absV.y >= almostOne ) {
			float signY = sign( v.y );
			planar.x = v.x + 2.0 * signY + 2.0;
			planar.y = v.z * signY - 2.0;
		}
		return vec2( 0.125, 0.25 ) * planar + vec2( 0.375, 0.75 );
	}
	float getPointShadow( sampler2D shadowMap, vec2 shadowMapSize, float shadowIntensity, float shadowBias, float shadowRadius, vec4 shadowCoord, float shadowCameraNear, float shadowCameraFar ) {
		float shadow = 1.0;
		vec3 lightToPosition = shadowCoord.xyz;
		
		float lightToPositionLength = length( lightToPosition );
		if ( lightToPositionLength - shadowCameraFar <= 0.0 && lightToPositionLength - shadowCameraNear >= 0.0 ) {
			float dp = ( lightToPositionLength - shadowCameraNear ) / ( shadowCameraFar - shadowCameraNear );			dp += shadowBias;
			vec3 bd3D = normalize( lightToPosition );
			vec2 texelSize = vec2( 1.0 ) / ( shadowMapSize * vec2( 4.0, 2.0 ) );
			#if defined( SHADOWMAP_TYPE_PCF ) || defined( SHADOWMAP_TYPE_PCF_SOFT ) || defined( SHADOWMAP_TYPE_VSM )
				vec2 offset = vec2( - 1, 1 ) * shadowRadius * texelSize.y;
				shadow = (
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.xyy, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.yyy, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.xyx, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.yyx, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.xxy, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.yxy, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.xxx, texelSize.y ), dp ) +
					texture2DCompare( shadowMap, cubeToUV( bd3D + offset.yxx, texelSize.y ), dp )
				) * ( 1.0 / 9.0 );
			#else
				shadow = texture2DCompare( shadowMap, cubeToUV( bd3D, texelSize.y ), dp );
			#endif
		}
		return mix( 1.0, shadow, shadowIntensity );
	}
#endif`,vb=`#if NUM_SPOT_LIGHT_COORDS > 0
	uniform mat4 spotLightMatrix[ NUM_SPOT_LIGHT_COORDS ];
	varying vec4 vSpotLightCoord[ NUM_SPOT_LIGHT_COORDS ];
#endif
#ifdef USE_SHADOWMAP
	#if NUM_DIR_LIGHT_SHADOWS > 0
		uniform mat4 directionalShadowMatrix[ NUM_DIR_LIGHT_SHADOWS ];
		varying vec4 vDirectionalShadowCoord[ NUM_DIR_LIGHT_SHADOWS ];
		struct DirectionalLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
		};
		uniform DirectionalLightShadow directionalLightShadows[ NUM_DIR_LIGHT_SHADOWS ];
	#endif
	#if NUM_SPOT_LIGHT_SHADOWS > 0
		struct SpotLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
		};
		uniform SpotLightShadow spotLightShadows[ NUM_SPOT_LIGHT_SHADOWS ];
	#endif
	#if NUM_POINT_LIGHT_SHADOWS > 0
		uniform mat4 pointShadowMatrix[ NUM_POINT_LIGHT_SHADOWS ];
		varying vec4 vPointShadowCoord[ NUM_POINT_LIGHT_SHADOWS ];
		struct PointLightShadow {
			float shadowIntensity;
			float shadowBias;
			float shadowNormalBias;
			float shadowRadius;
			vec2 shadowMapSize;
			float shadowCameraNear;
			float shadowCameraFar;
		};
		uniform PointLightShadow pointLightShadows[ NUM_POINT_LIGHT_SHADOWS ];
	#endif
#endif`,yb=`#if ( defined( USE_SHADOWMAP ) && ( NUM_DIR_LIGHT_SHADOWS > 0 || NUM_POINT_LIGHT_SHADOWS > 0 ) ) || ( NUM_SPOT_LIGHT_COORDS > 0 )
	vec3 shadowWorldNormal = inverseTransformDirection( transformedNormal, viewMatrix );
	vec4 shadowWorldPosition;
#endif
#if defined( USE_SHADOWMAP )
	#if NUM_DIR_LIGHT_SHADOWS > 0
		#pragma unroll_loop_start
		for ( int i = 0; i < NUM_DIR_LIGHT_SHADOWS; i ++ ) {
			shadowWorldPosition = worldPosition + vec4( shadowWorldNormal * directionalLightShadows[ i ].shadowNormalBias, 0 );
			vDirectionalShadowCoord[ i ] = directionalShadowMatrix[ i ] * shadowWorldPosition;
		}
		#pragma unroll_loop_end
	#endif
	#if NUM_POINT_LIGHT_SHADOWS > 0
		#pragma unroll_loop_start
		for ( int i = 0; i < NUM_POINT_LIGHT_SHADOWS; i ++ ) {
			shadowWorldPosition = worldPosition + vec4( shadowWorldNormal * pointLightShadows[ i ].shadowNormalBias, 0 );
			vPointShadowCoord[ i ] = pointShadowMatrix[ i ] * shadowWorldPosition;
		}
		#pragma unroll_loop_end
	#endif
#endif
#if NUM_SPOT_LIGHT_COORDS > 0
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_SPOT_LIGHT_COORDS; i ++ ) {
		shadowWorldPosition = worldPosition;
		#if ( defined( USE_SHADOWMAP ) && UNROLLED_LOOP_INDEX < NUM_SPOT_LIGHT_SHADOWS )
			shadowWorldPosition.xyz += shadowWorldNormal * spotLightShadows[ i ].shadowNormalBias;
		#endif
		vSpotLightCoord[ i ] = spotLightMatrix[ i ] * shadowWorldPosition;
	}
	#pragma unroll_loop_end
#endif`,xb=`float getShadowMask() {
	float shadow = 1.0;
	#ifdef USE_SHADOWMAP
	#if NUM_DIR_LIGHT_SHADOWS > 0
	DirectionalLightShadow directionalLight;
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_DIR_LIGHT_SHADOWS; i ++ ) {
		directionalLight = directionalLightShadows[ i ];
		shadow *= receiveShadow ? getShadow( directionalShadowMap[ i ], directionalLight.shadowMapSize, directionalLight.shadowIntensity, directionalLight.shadowBias, directionalLight.shadowRadius, vDirectionalShadowCoord[ i ] ) : 1.0;
	}
	#pragma unroll_loop_end
	#endif
	#if NUM_SPOT_LIGHT_SHADOWS > 0
	SpotLightShadow spotLight;
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_SPOT_LIGHT_SHADOWS; i ++ ) {
		spotLight = spotLightShadows[ i ];
		shadow *= receiveShadow ? getShadow( spotShadowMap[ i ], spotLight.shadowMapSize, spotLight.shadowIntensity, spotLight.shadowBias, spotLight.shadowRadius, vSpotLightCoord[ i ] ) : 1.0;
	}
	#pragma unroll_loop_end
	#endif
	#if NUM_POINT_LIGHT_SHADOWS > 0
	PointLightShadow pointLight;
	#pragma unroll_loop_start
	for ( int i = 0; i < NUM_POINT_LIGHT_SHADOWS; i ++ ) {
		pointLight = pointLightShadows[ i ];
		shadow *= receiveShadow ? getPointShadow( pointShadowMap[ i ], pointLight.shadowMapSize, pointLight.shadowIntensity, pointLight.shadowBias, pointLight.shadowRadius, vPointShadowCoord[ i ], pointLight.shadowCameraNear, pointLight.shadowCameraFar ) : 1.0;
	}
	#pragma unroll_loop_end
	#endif
	#endif
	return shadow;
}`,Sb=`#ifdef USE_SKINNING
	mat4 boneMatX = getBoneMatrix( skinIndex.x );
	mat4 boneMatY = getBoneMatrix( skinIndex.y );
	mat4 boneMatZ = getBoneMatrix( skinIndex.z );
	mat4 boneMatW = getBoneMatrix( skinIndex.w );
#endif`,Mb=`#ifdef USE_SKINNING
	uniform mat4 bindMatrix;
	uniform mat4 bindMatrixInverse;
	uniform highp sampler2D boneTexture;
	mat4 getBoneMatrix( const in float i ) {
		int size = textureSize( boneTexture, 0 ).x;
		int j = int( i ) * 4;
		int x = j % size;
		int y = j / size;
		vec4 v1 = texelFetch( boneTexture, ivec2( x, y ), 0 );
		vec4 v2 = texelFetch( boneTexture, ivec2( x + 1, y ), 0 );
		vec4 v3 = texelFetch( boneTexture, ivec2( x + 2, y ), 0 );
		vec4 v4 = texelFetch( boneTexture, ivec2( x + 3, y ), 0 );
		return mat4( v1, v2, v3, v4 );
	}
#endif`,Eb=`#ifdef USE_SKINNING
	vec4 skinVertex = bindMatrix * vec4( transformed, 1.0 );
	vec4 skinned = vec4( 0.0 );
	skinned += boneMatX * skinVertex * skinWeight.x;
	skinned += boneMatY * skinVertex * skinWeight.y;
	skinned += boneMatZ * skinVertex * skinWeight.z;
	skinned += boneMatW * skinVertex * skinWeight.w;
	transformed = ( bindMatrixInverse * skinned ).xyz;
#endif`,Tb=`#ifdef USE_SKINNING
	mat4 skinMatrix = mat4( 0.0 );
	skinMatrix += skinWeight.x * boneMatX;
	skinMatrix += skinWeight.y * boneMatY;
	skinMatrix += skinWeight.z * boneMatZ;
	skinMatrix += skinWeight.w * boneMatW;
	skinMatrix = bindMatrixInverse * skinMatrix * bindMatrix;
	objectNormal = vec4( skinMatrix * vec4( objectNormal, 0.0 ) ).xyz;
	#ifdef USE_TANGENT
		objectTangent = vec4( skinMatrix * vec4( objectTangent, 0.0 ) ).xyz;
	#endif
#endif`,bb=`float specularStrength;
#ifdef USE_SPECULARMAP
	vec4 texelSpecular = texture2D( specularMap, vSpecularMapUv );
	specularStrength = texelSpecular.r;
#else
	specularStrength = 1.0;
#endif`,Ab=`#ifdef USE_SPECULARMAP
	uniform sampler2D specularMap;
#endif`,Rb=`#if defined( TONE_MAPPING )
	gl_FragColor.rgb = toneMapping( gl_FragColor.rgb );
#endif`,Cb=`#ifndef saturate
#define saturate( a ) clamp( a, 0.0, 1.0 )
#endif
uniform float toneMappingExposure;
vec3 LinearToneMapping( vec3 color ) {
	return saturate( toneMappingExposure * color );
}
vec3 ReinhardToneMapping( vec3 color ) {
	color *= toneMappingExposure;
	return saturate( color / ( vec3( 1.0 ) + color ) );
}
vec3 CineonToneMapping( vec3 color ) {
	color *= toneMappingExposure;
	color = max( vec3( 0.0 ), color - 0.004 );
	return pow( ( color * ( 6.2 * color + 0.5 ) ) / ( color * ( 6.2 * color + 1.7 ) + 0.06 ), vec3( 2.2 ) );
}
vec3 RRTAndODTFit( vec3 v ) {
	vec3 a = v * ( v + 0.0245786 ) - 0.000090537;
	vec3 b = v * ( 0.983729 * v + 0.4329510 ) + 0.238081;
	return a / b;
}
vec3 ACESFilmicToneMapping( vec3 color ) {
	const mat3 ACESInputMat = mat3(
		vec3( 0.59719, 0.07600, 0.02840 ),		vec3( 0.35458, 0.90834, 0.13383 ),
		vec3( 0.04823, 0.01566, 0.83777 )
	);
	const mat3 ACESOutputMat = mat3(
		vec3(  1.60475, -0.10208, -0.00327 ),		vec3( -0.53108,  1.10813, -0.07276 ),
		vec3( -0.07367, -0.00605,  1.07602 )
	);
	color *= toneMappingExposure / 0.6;
	color = ACESInputMat * color;
	color = RRTAndODTFit( color );
	color = ACESOutputMat * color;
	return saturate( color );
}
const mat3 LINEAR_REC2020_TO_LINEAR_SRGB = mat3(
	vec3( 1.6605, - 0.1246, - 0.0182 ),
	vec3( - 0.5876, 1.1329, - 0.1006 ),
	vec3( - 0.0728, - 0.0083, 1.1187 )
);
const mat3 LINEAR_SRGB_TO_LINEAR_REC2020 = mat3(
	vec3( 0.6274, 0.0691, 0.0164 ),
	vec3( 0.3293, 0.9195, 0.0880 ),
	vec3( 0.0433, 0.0113, 0.8956 )
);
vec3 agxDefaultContrastApprox( vec3 x ) {
	vec3 x2 = x * x;
	vec3 x4 = x2 * x2;
	return + 15.5 * x4 * x2
		- 40.14 * x4 * x
		+ 31.96 * x4
		- 6.868 * x2 * x
		+ 0.4298 * x2
		+ 0.1191 * x
		- 0.00232;
}
vec3 AgXToneMapping( vec3 color ) {
	const mat3 AgXInsetMatrix = mat3(
		vec3( 0.856627153315983, 0.137318972929847, 0.11189821299995 ),
		vec3( 0.0951212405381588, 0.761241990602591, 0.0767994186031903 ),
		vec3( 0.0482516061458583, 0.101439036467562, 0.811302368396859 )
	);
	const mat3 AgXOutsetMatrix = mat3(
		vec3( 1.1271005818144368, - 0.1413297634984383, - 0.14132976349843826 ),
		vec3( - 0.11060664309660323, 1.157823702216272, - 0.11060664309660294 ),
		vec3( - 0.016493938717834573, - 0.016493938717834257, 1.2519364065950405 )
	);
	const float AgxMinEv = - 12.47393;	const float AgxMaxEv = 4.026069;
	color *= toneMappingExposure;
	color = LINEAR_SRGB_TO_LINEAR_REC2020 * color;
	color = AgXInsetMatrix * color;
	color = max( color, 1e-10 );	color = log2( color );
	color = ( color - AgxMinEv ) / ( AgxMaxEv - AgxMinEv );
	color = clamp( color, 0.0, 1.0 );
	color = agxDefaultContrastApprox( color );
	color = AgXOutsetMatrix * color;
	color = pow( max( vec3( 0.0 ), color ), vec3( 2.2 ) );
	color = LINEAR_REC2020_TO_LINEAR_SRGB * color;
	color = clamp( color, 0.0, 1.0 );
	return color;
}
vec3 NeutralToneMapping( vec3 color ) {
	const float StartCompression = 0.8 - 0.04;
	const float Desaturation = 0.15;
	color *= toneMappingExposure;
	float x = min( color.r, min( color.g, color.b ) );
	float offset = x < 0.08 ? x - 6.25 * x * x : 0.04;
	color -= offset;
	float peak = max( color.r, max( color.g, color.b ) );
	if ( peak < StartCompression ) return color;
	float d = 1. - StartCompression;
	float newPeak = 1. - d * d / ( peak + d - StartCompression );
	color *= newPeak / peak;
	float g = 1. - 1. / ( Desaturation * ( peak - newPeak ) + 1. );
	return mix( color, vec3( newPeak ), g );
}
vec3 CustomToneMapping( vec3 color ) { return color; }`,wb=`#ifdef USE_TRANSMISSION
	material.transmission = transmission;
	material.transmissionAlpha = 1.0;
	material.thickness = thickness;
	material.attenuationDistance = attenuationDistance;
	material.attenuationColor = attenuationColor;
	#ifdef USE_TRANSMISSIONMAP
		material.transmission *= texture2D( transmissionMap, vTransmissionMapUv ).r;
	#endif
	#ifdef USE_THICKNESSMAP
		material.thickness *= texture2D( thicknessMap, vThicknessMapUv ).g;
	#endif
	vec3 pos = vWorldPosition;
	vec3 v = normalize( cameraPosition - pos );
	vec3 n = inverseTransformDirection( normal, viewMatrix );
	vec4 transmitted = getIBLVolumeRefraction(
		n, v, material.roughness, material.diffuseColor, material.specularColor, material.specularF90,
		pos, modelMatrix, viewMatrix, projectionMatrix, material.dispersion, material.ior, material.thickness,
		material.attenuationColor, material.attenuationDistance );
	material.transmissionAlpha = mix( material.transmissionAlpha, transmitted.a, material.transmission );
	totalDiffuse = mix( totalDiffuse, transmitted.rgb, material.transmission );
#endif`,Db=`#ifdef USE_TRANSMISSION
	uniform float transmission;
	uniform float thickness;
	uniform float attenuationDistance;
	uniform vec3 attenuationColor;
	#ifdef USE_TRANSMISSIONMAP
		uniform sampler2D transmissionMap;
	#endif
	#ifdef USE_THICKNESSMAP
		uniform sampler2D thicknessMap;
	#endif
	uniform vec2 transmissionSamplerSize;
	uniform sampler2D transmissionSamplerMap;
	uniform mat4 modelMatrix;
	uniform mat4 projectionMatrix;
	varying vec3 vWorldPosition;
	float w0( float a ) {
		return ( 1.0 / 6.0 ) * ( a * ( a * ( - a + 3.0 ) - 3.0 ) + 1.0 );
	}
	float w1( float a ) {
		return ( 1.0 / 6.0 ) * ( a *  a * ( 3.0 * a - 6.0 ) + 4.0 );
	}
	float w2( float a ){
		return ( 1.0 / 6.0 ) * ( a * ( a * ( - 3.0 * a + 3.0 ) + 3.0 ) + 1.0 );
	}
	float w3( float a ) {
		return ( 1.0 / 6.0 ) * ( a * a * a );
	}
	float g0( float a ) {
		return w0( a ) + w1( a );
	}
	float g1( float a ) {
		return w2( a ) + w3( a );
	}
	float h0( float a ) {
		return - 1.0 + w1( a ) / ( w0( a ) + w1( a ) );
	}
	float h1( float a ) {
		return 1.0 + w3( a ) / ( w2( a ) + w3( a ) );
	}
	vec4 bicubic( sampler2D tex, vec2 uv, vec4 texelSize, float lod ) {
		uv = uv * texelSize.zw + 0.5;
		vec2 iuv = floor( uv );
		vec2 fuv = fract( uv );
		float g0x = g0( fuv.x );
		float g1x = g1( fuv.x );
		float h0x = h0( fuv.x );
		float h1x = h1( fuv.x );
		float h0y = h0( fuv.y );
		float h1y = h1( fuv.y );
		vec2 p0 = ( vec2( iuv.x + h0x, iuv.y + h0y ) - 0.5 ) * texelSize.xy;
		vec2 p1 = ( vec2( iuv.x + h1x, iuv.y + h0y ) - 0.5 ) * texelSize.xy;
		vec2 p2 = ( vec2( iuv.x + h0x, iuv.y + h1y ) - 0.5 ) * texelSize.xy;
		vec2 p3 = ( vec2( iuv.x + h1x, iuv.y + h1y ) - 0.5 ) * texelSize.xy;
		return g0( fuv.y ) * ( g0x * textureLod( tex, p0, lod ) + g1x * textureLod( tex, p1, lod ) ) +
			g1( fuv.y ) * ( g0x * textureLod( tex, p2, lod ) + g1x * textureLod( tex, p3, lod ) );
	}
	vec4 textureBicubic( sampler2D sampler, vec2 uv, float lod ) {
		vec2 fLodSize = vec2( textureSize( sampler, int( lod ) ) );
		vec2 cLodSize = vec2( textureSize( sampler, int( lod + 1.0 ) ) );
		vec2 fLodSizeInv = 1.0 / fLodSize;
		vec2 cLodSizeInv = 1.0 / cLodSize;
		vec4 fSample = bicubic( sampler, uv, vec4( fLodSizeInv, fLodSize ), floor( lod ) );
		vec4 cSample = bicubic( sampler, uv, vec4( cLodSizeInv, cLodSize ), ceil( lod ) );
		return mix( fSample, cSample, fract( lod ) );
	}
	vec3 getVolumeTransmissionRay( const in vec3 n, const in vec3 v, const in float thickness, const in float ior, const in mat4 modelMatrix ) {
		vec3 refractionVector = refract( - v, normalize( n ), 1.0 / ior );
		vec3 modelScale;
		modelScale.x = length( vec3( modelMatrix[ 0 ].xyz ) );
		modelScale.y = length( vec3( modelMatrix[ 1 ].xyz ) );
		modelScale.z = length( vec3( modelMatrix[ 2 ].xyz ) );
		return normalize( refractionVector ) * thickness * modelScale;
	}
	float applyIorToRoughness( const in float roughness, const in float ior ) {
		return roughness * clamp( ior * 2.0 - 2.0, 0.0, 1.0 );
	}
	vec4 getTransmissionSample( const in vec2 fragCoord, const in float roughness, const in float ior ) {
		float lod = log2( transmissionSamplerSize.x ) * applyIorToRoughness( roughness, ior );
		return textureBicubic( transmissionSamplerMap, fragCoord.xy, lod );
	}
	vec3 volumeAttenuation( const in float transmissionDistance, const in vec3 attenuationColor, const in float attenuationDistance ) {
		if ( isinf( attenuationDistance ) ) {
			return vec3( 1.0 );
		} else {
			vec3 attenuationCoefficient = -log( attenuationColor ) / attenuationDistance;
			vec3 transmittance = exp( - attenuationCoefficient * transmissionDistance );			return transmittance;
		}
	}
	vec4 getIBLVolumeRefraction( const in vec3 n, const in vec3 v, const in float roughness, const in vec3 diffuseColor,
		const in vec3 specularColor, const in float specularF90, const in vec3 position, const in mat4 modelMatrix,
		const in mat4 viewMatrix, const in mat4 projMatrix, const in float dispersion, const in float ior, const in float thickness,
		const in vec3 attenuationColor, const in float attenuationDistance ) {
		vec4 transmittedLight;
		vec3 transmittance;
		#ifdef USE_DISPERSION
			float halfSpread = ( ior - 1.0 ) * 0.025 * dispersion;
			vec3 iors = vec3( ior - halfSpread, ior, ior + halfSpread );
			for ( int i = 0; i < 3; i ++ ) {
				vec3 transmissionRay = getVolumeTransmissionRay( n, v, thickness, iors[ i ], modelMatrix );
				vec3 refractedRayExit = position + transmissionRay;
				vec4 ndcPos = projMatrix * viewMatrix * vec4( refractedRayExit, 1.0 );
				vec2 refractionCoords = ndcPos.xy / ndcPos.w;
				refractionCoords += 1.0;
				refractionCoords /= 2.0;
				vec4 transmissionSample = getTransmissionSample( refractionCoords, roughness, iors[ i ] );
				transmittedLight[ i ] = transmissionSample[ i ];
				transmittedLight.a += transmissionSample.a;
				transmittance[ i ] = diffuseColor[ i ] * volumeAttenuation( length( transmissionRay ), attenuationColor, attenuationDistance )[ i ];
			}
			transmittedLight.a /= 3.0;
		#else
			vec3 transmissionRay = getVolumeTransmissionRay( n, v, thickness, ior, modelMatrix );
			vec3 refractedRayExit = position + transmissionRay;
			vec4 ndcPos = projMatrix * viewMatrix * vec4( refractedRayExit, 1.0 );
			vec2 refractionCoords = ndcPos.xy / ndcPos.w;
			refractionCoords += 1.0;
			refractionCoords /= 2.0;
			transmittedLight = getTransmissionSample( refractionCoords, roughness, ior );
			transmittance = diffuseColor * volumeAttenuation( length( transmissionRay ), attenuationColor, attenuationDistance );
		#endif
		vec3 attenuatedColor = transmittance * transmittedLight.rgb;
		vec3 F = EnvironmentBRDF( n, v, specularColor, specularF90, roughness );
		float transmittanceFactor = ( transmittance.r + transmittance.g + transmittance.b ) / 3.0;
		return vec4( ( 1.0 - F ) * attenuatedColor, 1.0 - ( 1.0 - transmittedLight.a ) * transmittanceFactor );
	}
#endif`,Ub=`#if defined( USE_UV ) || defined( USE_ANISOTROPY )
	varying vec2 vUv;
#endif
#ifdef USE_MAP
	varying vec2 vMapUv;
#endif
#ifdef USE_ALPHAMAP
	varying vec2 vAlphaMapUv;
#endif
#ifdef USE_LIGHTMAP
	varying vec2 vLightMapUv;
#endif
#ifdef USE_AOMAP
	varying vec2 vAoMapUv;
#endif
#ifdef USE_BUMPMAP
	varying vec2 vBumpMapUv;
#endif
#ifdef USE_NORMALMAP
	varying vec2 vNormalMapUv;
#endif
#ifdef USE_EMISSIVEMAP
	varying vec2 vEmissiveMapUv;
#endif
#ifdef USE_METALNESSMAP
	varying vec2 vMetalnessMapUv;
#endif
#ifdef USE_ROUGHNESSMAP
	varying vec2 vRoughnessMapUv;
#endif
#ifdef USE_ANISOTROPYMAP
	varying vec2 vAnisotropyMapUv;
#endif
#ifdef USE_CLEARCOATMAP
	varying vec2 vClearcoatMapUv;
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	varying vec2 vClearcoatNormalMapUv;
#endif
#ifdef USE_CLEARCOAT_ROUGHNESSMAP
	varying vec2 vClearcoatRoughnessMapUv;
#endif
#ifdef USE_IRIDESCENCEMAP
	varying vec2 vIridescenceMapUv;
#endif
#ifdef USE_IRIDESCENCE_THICKNESSMAP
	varying vec2 vIridescenceThicknessMapUv;
#endif
#ifdef USE_SHEEN_COLORMAP
	varying vec2 vSheenColorMapUv;
#endif
#ifdef USE_SHEEN_ROUGHNESSMAP
	varying vec2 vSheenRoughnessMapUv;
#endif
#ifdef USE_SPECULARMAP
	varying vec2 vSpecularMapUv;
#endif
#ifdef USE_SPECULAR_COLORMAP
	varying vec2 vSpecularColorMapUv;
#endif
#ifdef USE_SPECULAR_INTENSITYMAP
	varying vec2 vSpecularIntensityMapUv;
#endif
#ifdef USE_TRANSMISSIONMAP
	uniform mat3 transmissionMapTransform;
	varying vec2 vTransmissionMapUv;
#endif
#ifdef USE_THICKNESSMAP
	uniform mat3 thicknessMapTransform;
	varying vec2 vThicknessMapUv;
#endif`,Lb=`#if defined( USE_UV ) || defined( USE_ANISOTROPY )
	varying vec2 vUv;
#endif
#ifdef USE_MAP
	uniform mat3 mapTransform;
	varying vec2 vMapUv;
#endif
#ifdef USE_ALPHAMAP
	uniform mat3 alphaMapTransform;
	varying vec2 vAlphaMapUv;
#endif
#ifdef USE_LIGHTMAP
	uniform mat3 lightMapTransform;
	varying vec2 vLightMapUv;
#endif
#ifdef USE_AOMAP
	uniform mat3 aoMapTransform;
	varying vec2 vAoMapUv;
#endif
#ifdef USE_BUMPMAP
	uniform mat3 bumpMapTransform;
	varying vec2 vBumpMapUv;
#endif
#ifdef USE_NORMALMAP
	uniform mat3 normalMapTransform;
	varying vec2 vNormalMapUv;
#endif
#ifdef USE_DISPLACEMENTMAP
	uniform mat3 displacementMapTransform;
	varying vec2 vDisplacementMapUv;
#endif
#ifdef USE_EMISSIVEMAP
	uniform mat3 emissiveMapTransform;
	varying vec2 vEmissiveMapUv;
#endif
#ifdef USE_METALNESSMAP
	uniform mat3 metalnessMapTransform;
	varying vec2 vMetalnessMapUv;
#endif
#ifdef USE_ROUGHNESSMAP
	uniform mat3 roughnessMapTransform;
	varying vec2 vRoughnessMapUv;
#endif
#ifdef USE_ANISOTROPYMAP
	uniform mat3 anisotropyMapTransform;
	varying vec2 vAnisotropyMapUv;
#endif
#ifdef USE_CLEARCOATMAP
	uniform mat3 clearcoatMapTransform;
	varying vec2 vClearcoatMapUv;
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	uniform mat3 clearcoatNormalMapTransform;
	varying vec2 vClearcoatNormalMapUv;
#endif
#ifdef USE_CLEARCOAT_ROUGHNESSMAP
	uniform mat3 clearcoatRoughnessMapTransform;
	varying vec2 vClearcoatRoughnessMapUv;
#endif
#ifdef USE_SHEEN_COLORMAP
	uniform mat3 sheenColorMapTransform;
	varying vec2 vSheenColorMapUv;
#endif
#ifdef USE_SHEEN_ROUGHNESSMAP
	uniform mat3 sheenRoughnessMapTransform;
	varying vec2 vSheenRoughnessMapUv;
#endif
#ifdef USE_IRIDESCENCEMAP
	uniform mat3 iridescenceMapTransform;
	varying vec2 vIridescenceMapUv;
#endif
#ifdef USE_IRIDESCENCE_THICKNESSMAP
	uniform mat3 iridescenceThicknessMapTransform;
	varying vec2 vIridescenceThicknessMapUv;
#endif
#ifdef USE_SPECULARMAP
	uniform mat3 specularMapTransform;
	varying vec2 vSpecularMapUv;
#endif
#ifdef USE_SPECULAR_COLORMAP
	uniform mat3 specularColorMapTransform;
	varying vec2 vSpecularColorMapUv;
#endif
#ifdef USE_SPECULAR_INTENSITYMAP
	uniform mat3 specularIntensityMapTransform;
	varying vec2 vSpecularIntensityMapUv;
#endif
#ifdef USE_TRANSMISSIONMAP
	uniform mat3 transmissionMapTransform;
	varying vec2 vTransmissionMapUv;
#endif
#ifdef USE_THICKNESSMAP
	uniform mat3 thicknessMapTransform;
	varying vec2 vThicknessMapUv;
#endif`,Nb=`#if defined( USE_UV ) || defined( USE_ANISOTROPY )
	vUv = vec3( uv, 1 ).xy;
#endif
#ifdef USE_MAP
	vMapUv = ( mapTransform * vec3( MAP_UV, 1 ) ).xy;
#endif
#ifdef USE_ALPHAMAP
	vAlphaMapUv = ( alphaMapTransform * vec3( ALPHAMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_LIGHTMAP
	vLightMapUv = ( lightMapTransform * vec3( LIGHTMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_AOMAP
	vAoMapUv = ( aoMapTransform * vec3( AOMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_BUMPMAP
	vBumpMapUv = ( bumpMapTransform * vec3( BUMPMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_NORMALMAP
	vNormalMapUv = ( normalMapTransform * vec3( NORMALMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_DISPLACEMENTMAP
	vDisplacementMapUv = ( displacementMapTransform * vec3( DISPLACEMENTMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_EMISSIVEMAP
	vEmissiveMapUv = ( emissiveMapTransform * vec3( EMISSIVEMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_METALNESSMAP
	vMetalnessMapUv = ( metalnessMapTransform * vec3( METALNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_ROUGHNESSMAP
	vRoughnessMapUv = ( roughnessMapTransform * vec3( ROUGHNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_ANISOTROPYMAP
	vAnisotropyMapUv = ( anisotropyMapTransform * vec3( ANISOTROPYMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_CLEARCOATMAP
	vClearcoatMapUv = ( clearcoatMapTransform * vec3( CLEARCOATMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_CLEARCOAT_NORMALMAP
	vClearcoatNormalMapUv = ( clearcoatNormalMapTransform * vec3( CLEARCOAT_NORMALMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_CLEARCOAT_ROUGHNESSMAP
	vClearcoatRoughnessMapUv = ( clearcoatRoughnessMapTransform * vec3( CLEARCOAT_ROUGHNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_IRIDESCENCEMAP
	vIridescenceMapUv = ( iridescenceMapTransform * vec3( IRIDESCENCEMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_IRIDESCENCE_THICKNESSMAP
	vIridescenceThicknessMapUv = ( iridescenceThicknessMapTransform * vec3( IRIDESCENCE_THICKNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SHEEN_COLORMAP
	vSheenColorMapUv = ( sheenColorMapTransform * vec3( SHEEN_COLORMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SHEEN_ROUGHNESSMAP
	vSheenRoughnessMapUv = ( sheenRoughnessMapTransform * vec3( SHEEN_ROUGHNESSMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SPECULARMAP
	vSpecularMapUv = ( specularMapTransform * vec3( SPECULARMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SPECULAR_COLORMAP
	vSpecularColorMapUv = ( specularColorMapTransform * vec3( SPECULAR_COLORMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_SPECULAR_INTENSITYMAP
	vSpecularIntensityMapUv = ( specularIntensityMapTransform * vec3( SPECULAR_INTENSITYMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_TRANSMISSIONMAP
	vTransmissionMapUv = ( transmissionMapTransform * vec3( TRANSMISSIONMAP_UV, 1 ) ).xy;
#endif
#ifdef USE_THICKNESSMAP
	vThicknessMapUv = ( thicknessMapTransform * vec3( THICKNESSMAP_UV, 1 ) ).xy;
#endif`,Ob=`#if defined( USE_ENVMAP ) || defined( DISTANCE ) || defined ( USE_SHADOWMAP ) || defined ( USE_TRANSMISSION ) || NUM_SPOT_LIGHT_COORDS > 0
	vec4 worldPosition = vec4( transformed, 1.0 );
	#ifdef USE_BATCHING
		worldPosition = batchingMatrix * worldPosition;
	#endif
	#ifdef USE_INSTANCING
		worldPosition = instanceMatrix * worldPosition;
	#endif
	worldPosition = modelMatrix * worldPosition;
#endif`;const Pb=`varying vec2 vUv;
uniform mat3 uvTransform;
void main() {
	vUv = ( uvTransform * vec3( uv, 1 ) ).xy;
	gl_Position = vec4( position.xy, 1.0, 1.0 );
}`,zb=`uniform sampler2D t2D;
uniform float backgroundIntensity;
varying vec2 vUv;
void main() {
	vec4 texColor = texture2D( t2D, vUv );
	#ifdef DECODE_VIDEO_TEXTURE
		texColor = vec4( mix( pow( texColor.rgb * 0.9478672986 + vec3( 0.0521327014 ), vec3( 2.4 ) ), texColor.rgb * 0.0773993808, vec3( lessThanEqual( texColor.rgb, vec3( 0.04045 ) ) ) ), texColor.w );
	#endif
	texColor.rgb *= backgroundIntensity;
	gl_FragColor = texColor;
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
}`,Bb=`varying vec3 vWorldDirection;
#include <common>
void main() {
	vWorldDirection = transformDirection( position, modelMatrix );
	#include <begin_vertex>
	#include <project_vertex>
	gl_Position.z = gl_Position.w;
}`,Ib=`#ifdef ENVMAP_TYPE_CUBE
	uniform samplerCube envMap;
#elif defined( ENVMAP_TYPE_CUBE_UV )
	uniform sampler2D envMap;
#endif
uniform float flipEnvMap;
uniform float backgroundBlurriness;
uniform float backgroundIntensity;
uniform mat3 backgroundRotation;
varying vec3 vWorldDirection;
#include <cube_uv_reflection_fragment>
void main() {
	#ifdef ENVMAP_TYPE_CUBE
		vec4 texColor = textureCube( envMap, backgroundRotation * vec3( flipEnvMap * vWorldDirection.x, vWorldDirection.yz ) );
	#elif defined( ENVMAP_TYPE_CUBE_UV )
		vec4 texColor = textureCubeUV( envMap, backgroundRotation * vWorldDirection, backgroundBlurriness );
	#else
		vec4 texColor = vec4( 0.0, 0.0, 0.0, 1.0 );
	#endif
	texColor.rgb *= backgroundIntensity;
	gl_FragColor = texColor;
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
}`,Fb=`varying vec3 vWorldDirection;
#include <common>
void main() {
	vWorldDirection = transformDirection( position, modelMatrix );
	#include <begin_vertex>
	#include <project_vertex>
	gl_Position.z = gl_Position.w;
}`,Hb=`uniform samplerCube tCube;
uniform float tFlip;
uniform float opacity;
varying vec3 vWorldDirection;
void main() {
	vec4 texColor = textureCube( tCube, vec3( tFlip * vWorldDirection.x, vWorldDirection.yz ) );
	gl_FragColor = texColor;
	gl_FragColor.a *= opacity;
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
}`,Gb=`#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
varying vec2 vHighPrecisionZW;
void main() {
	#include <uv_vertex>
	#include <batching_vertex>
	#include <skinbase_vertex>
	#include <morphinstance_vertex>
	#ifdef USE_DISPLACEMENTMAP
		#include <beginnormal_vertex>
		#include <morphnormal_vertex>
		#include <skinnormal_vertex>
	#endif
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vHighPrecisionZW = gl_Position.zw;
}`,Vb=`#if DEPTH_PACKING == 3200
	uniform float opacity;
#endif
#include <common>
#include <packing>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
varying vec2 vHighPrecisionZW;
void main() {
	vec4 diffuseColor = vec4( 1.0 );
	#include <clipping_planes_fragment>
	#if DEPTH_PACKING == 3200
		diffuseColor.a = opacity;
	#endif
	#include <map_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <logdepthbuf_fragment>
	#ifdef USE_REVERSED_DEPTH_BUFFER
		float fragCoordZ = vHighPrecisionZW[ 0 ] / vHighPrecisionZW[ 1 ];
	#else
		float fragCoordZ = 0.5 * vHighPrecisionZW[ 0 ] / vHighPrecisionZW[ 1 ] + 0.5;
	#endif
	#if DEPTH_PACKING == 3200
		gl_FragColor = vec4( vec3( 1.0 - fragCoordZ ), opacity );
	#elif DEPTH_PACKING == 3201
		gl_FragColor = packDepthToRGBA( fragCoordZ );
	#elif DEPTH_PACKING == 3202
		gl_FragColor = vec4( packDepthToRGB( fragCoordZ ), 1.0 );
	#elif DEPTH_PACKING == 3203
		gl_FragColor = vec4( packDepthToRG( fragCoordZ ), 0.0, 1.0 );
	#endif
}`,kb=`#define DISTANCE
varying vec3 vWorldPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <batching_vertex>
	#include <skinbase_vertex>
	#include <morphinstance_vertex>
	#ifdef USE_DISPLACEMENTMAP
		#include <beginnormal_vertex>
		#include <morphnormal_vertex>
		#include <skinnormal_vertex>
	#endif
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <worldpos_vertex>
	#include <clipping_planes_vertex>
	vWorldPosition = worldPosition.xyz;
}`,Xb=`#define DISTANCE
uniform vec3 referencePosition;
uniform float nearDistance;
uniform float farDistance;
varying vec3 vWorldPosition;
#include <common>
#include <packing>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <clipping_planes_pars_fragment>
void main () {
	vec4 diffuseColor = vec4( 1.0 );
	#include <clipping_planes_fragment>
	#include <map_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	float dist = length( vWorldPosition - referencePosition );
	dist = ( dist - nearDistance ) / ( farDistance - nearDistance );
	dist = saturate( dist );
	gl_FragColor = packDepthToRGBA( dist );
}`,Wb=`varying vec3 vWorldDirection;
#include <common>
void main() {
	vWorldDirection = transformDirection( position, modelMatrix );
	#include <begin_vertex>
	#include <project_vertex>
}`,jb=`uniform sampler2D tEquirect;
varying vec3 vWorldDirection;
#include <common>
void main() {
	vec3 direction = normalize( vWorldDirection );
	vec2 sampleUV = equirectUv( direction );
	gl_FragColor = texture2D( tEquirect, sampleUV );
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
}`,Yb=`uniform float scale;
attribute float lineDistance;
varying float vLineDistance;
#include <common>
#include <uv_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <morphtarget_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	vLineDistance = scale * lineDistance;
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <fog_vertex>
}`,qb=`uniform vec3 diffuse;
uniform float opacity;
uniform float dashSize;
uniform float totalSize;
varying float vLineDistance;
#include <common>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <fog_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	if ( mod( vLineDistance, totalSize ) > dashSize ) {
		discard;
	}
	vec3 outgoingLight = vec3( 0.0 );
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	outgoingLight = diffuseColor.rgb;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
}`,Zb=`#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <envmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#if defined ( USE_ENVMAP ) || defined ( USE_SKINNING )
		#include <beginnormal_vertex>
		#include <morphnormal_vertex>
		#include <skinbase_vertex>
		#include <skinnormal_vertex>
		#include <defaultnormal_vertex>
	#endif
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <worldpos_vertex>
	#include <envmap_vertex>
	#include <fog_vertex>
}`,Kb=`uniform vec3 diffuse;
uniform float opacity;
#ifndef FLAT_SHADED
	varying vec3 vNormal;
#endif
#include <common>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <envmap_common_pars_fragment>
#include <envmap_pars_fragment>
#include <fog_pars_fragment>
#include <specularmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <specularmap_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	#ifdef USE_LIGHTMAP
		vec4 lightMapTexel = texture2D( lightMap, vLightMapUv );
		reflectedLight.indirectDiffuse += lightMapTexel.rgb * lightMapIntensity * RECIPROCAL_PI;
	#else
		reflectedLight.indirectDiffuse += vec3( 1.0 );
	#endif
	#include <aomap_fragment>
	reflectedLight.indirectDiffuse *= diffuseColor.rgb;
	vec3 outgoingLight = reflectedLight.indirectDiffuse;
	#include <envmap_fragment>
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,Qb=`#define LAMBERT
varying vec3 vViewPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <envmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <shadowmap_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vViewPosition = - mvPosition.xyz;
	#include <worldpos_vertex>
	#include <envmap_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
}`,Jb=`#define LAMBERT
uniform vec3 diffuse;
uniform vec3 emissive;
uniform float opacity;
#include <common>
#include <packing>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <emissivemap_pars_fragment>
#include <envmap_common_pars_fragment>
#include <envmap_pars_fragment>
#include <fog_pars_fragment>
#include <bsdfs>
#include <lights_pars_begin>
#include <normal_pars_fragment>
#include <lights_lambert_pars_fragment>
#include <shadowmap_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <specularmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	vec3 totalEmissiveRadiance = emissive;
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <specularmap_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	#include <emissivemap_fragment>
	#include <lights_lambert_fragment>
	#include <lights_fragment_begin>
	#include <lights_fragment_maps>
	#include <lights_fragment_end>
	#include <aomap_fragment>
	vec3 outgoingLight = reflectedLight.directDiffuse + reflectedLight.indirectDiffuse + totalEmissiveRadiance;
	#include <envmap_fragment>
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,$b=`#define MATCAP
varying vec3 vViewPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <color_pars_vertex>
#include <displacementmap_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <fog_vertex>
	vViewPosition = - mvPosition.xyz;
}`,tA=`#define MATCAP
uniform vec3 diffuse;
uniform float opacity;
uniform sampler2D matcap;
varying vec3 vViewPosition;
#include <common>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <fog_pars_fragment>
#include <normal_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	vec3 viewDir = normalize( vViewPosition );
	vec3 x = normalize( vec3( viewDir.z, 0.0, - viewDir.x ) );
	vec3 y = cross( viewDir, x );
	vec2 uv = vec2( dot( x, normal ), dot( y, normal ) ) * 0.495 + 0.5;
	#ifdef USE_MATCAP
		vec4 matcapColor = texture2D( matcap, uv );
	#else
		vec4 matcapColor = vec4( vec3( mix( 0.2, 0.8, uv.y ) ), 1.0 );
	#endif
	vec3 outgoingLight = diffuseColor.rgb * matcapColor.rgb;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,eA=`#define NORMAL
#if defined( FLAT_SHADED ) || defined( USE_BUMPMAP ) || defined( USE_NORMALMAP_TANGENTSPACE )
	varying vec3 vViewPosition;
#endif
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphinstance_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
#if defined( FLAT_SHADED ) || defined( USE_BUMPMAP ) || defined( USE_NORMALMAP_TANGENTSPACE )
	vViewPosition = - mvPosition.xyz;
#endif
}`,nA=`#define NORMAL
uniform float opacity;
#if defined( FLAT_SHADED ) || defined( USE_BUMPMAP ) || defined( USE_NORMALMAP_TANGENTSPACE )
	varying vec3 vViewPosition;
#endif
#include <packing>
#include <uv_pars_fragment>
#include <normal_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( 0.0, 0.0, 0.0, opacity );
	#include <clipping_planes_fragment>
	#include <logdepthbuf_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	gl_FragColor = vec4( packNormalToRGB( normal ), diffuseColor.a );
	#ifdef OPAQUE
		gl_FragColor.a = 1.0;
	#endif
}`,iA=`#define PHONG
varying vec3 vViewPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <envmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <shadowmap_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphinstance_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vViewPosition = - mvPosition.xyz;
	#include <worldpos_vertex>
	#include <envmap_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
}`,aA=`#define PHONG
uniform vec3 diffuse;
uniform vec3 emissive;
uniform vec3 specular;
uniform float shininess;
uniform float opacity;
#include <common>
#include <packing>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <emissivemap_pars_fragment>
#include <envmap_common_pars_fragment>
#include <envmap_pars_fragment>
#include <fog_pars_fragment>
#include <bsdfs>
#include <lights_pars_begin>
#include <normal_pars_fragment>
#include <lights_phong_pars_fragment>
#include <shadowmap_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <specularmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	vec3 totalEmissiveRadiance = emissive;
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <specularmap_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	#include <emissivemap_fragment>
	#include <lights_phong_fragment>
	#include <lights_fragment_begin>
	#include <lights_fragment_maps>
	#include <lights_fragment_end>
	#include <aomap_fragment>
	vec3 outgoingLight = reflectedLight.directDiffuse + reflectedLight.indirectDiffuse + reflectedLight.directSpecular + reflectedLight.indirectSpecular + totalEmissiveRadiance;
	#include <envmap_fragment>
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,sA=`#define STANDARD
varying vec3 vViewPosition;
#ifdef USE_TRANSMISSION
	varying vec3 vWorldPosition;
#endif
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <shadowmap_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vViewPosition = - mvPosition.xyz;
	#include <worldpos_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
#ifdef USE_TRANSMISSION
	vWorldPosition = worldPosition.xyz;
#endif
}`,rA=`#define STANDARD
#ifdef PHYSICAL
	#define IOR
	#define USE_SPECULAR
#endif
uniform vec3 diffuse;
uniform vec3 emissive;
uniform float roughness;
uniform float metalness;
uniform float opacity;
#ifdef IOR
	uniform float ior;
#endif
#ifdef USE_SPECULAR
	uniform float specularIntensity;
	uniform vec3 specularColor;
	#ifdef USE_SPECULAR_COLORMAP
		uniform sampler2D specularColorMap;
	#endif
	#ifdef USE_SPECULAR_INTENSITYMAP
		uniform sampler2D specularIntensityMap;
	#endif
#endif
#ifdef USE_CLEARCOAT
	uniform float clearcoat;
	uniform float clearcoatRoughness;
#endif
#ifdef USE_DISPERSION
	uniform float dispersion;
#endif
#ifdef USE_IRIDESCENCE
	uniform float iridescence;
	uniform float iridescenceIOR;
	uniform float iridescenceThicknessMinimum;
	uniform float iridescenceThicknessMaximum;
#endif
#ifdef USE_SHEEN
	uniform vec3 sheenColor;
	uniform float sheenRoughness;
	#ifdef USE_SHEEN_COLORMAP
		uniform sampler2D sheenColorMap;
	#endif
	#ifdef USE_SHEEN_ROUGHNESSMAP
		uniform sampler2D sheenRoughnessMap;
	#endif
#endif
#ifdef USE_ANISOTROPY
	uniform vec2 anisotropyVector;
	#ifdef USE_ANISOTROPYMAP
		uniform sampler2D anisotropyMap;
	#endif
#endif
varying vec3 vViewPosition;
#include <common>
#include <packing>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <emissivemap_pars_fragment>
#include <iridescence_fragment>
#include <cube_uv_reflection_fragment>
#include <envmap_common_pars_fragment>
#include <envmap_physical_pars_fragment>
#include <fog_pars_fragment>
#include <lights_pars_begin>
#include <normal_pars_fragment>
#include <lights_physical_pars_fragment>
#include <transmission_pars_fragment>
#include <shadowmap_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <clearcoat_pars_fragment>
#include <iridescence_pars_fragment>
#include <roughnessmap_pars_fragment>
#include <metalnessmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	vec3 totalEmissiveRadiance = emissive;
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <roughnessmap_fragment>
	#include <metalnessmap_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	#include <clearcoat_normal_fragment_begin>
	#include <clearcoat_normal_fragment_maps>
	#include <emissivemap_fragment>
	#include <lights_physical_fragment>
	#include <lights_fragment_begin>
	#include <lights_fragment_maps>
	#include <lights_fragment_end>
	#include <aomap_fragment>
	vec3 totalDiffuse = reflectedLight.directDiffuse + reflectedLight.indirectDiffuse;
	vec3 totalSpecular = reflectedLight.directSpecular + reflectedLight.indirectSpecular;
	#include <transmission_fragment>
	vec3 outgoingLight = totalDiffuse + totalSpecular + totalEmissiveRadiance;
	#ifdef USE_SHEEN
		float sheenEnergyComp = 1.0 - 0.157 * max3( material.sheenColor );
		outgoingLight = outgoingLight * sheenEnergyComp + sheenSpecularDirect + sheenSpecularIndirect;
	#endif
	#ifdef USE_CLEARCOAT
		float dotNVcc = saturate( dot( geometryClearcoatNormal, geometryViewDir ) );
		vec3 Fcc = F_Schlick( material.clearcoatF0, material.clearcoatF90, dotNVcc );
		outgoingLight = outgoingLight * ( 1.0 - material.clearcoat * Fcc ) + ( clearcoatSpecularDirect + clearcoatSpecularIndirect ) * material.clearcoat;
	#endif
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,oA=`#define TOON
varying vec3 vViewPosition;
#include <common>
#include <batching_pars_vertex>
#include <uv_pars_vertex>
#include <displacementmap_pars_vertex>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <normal_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <shadowmap_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <normal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <displacementmap_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	vViewPosition = - mvPosition.xyz;
	#include <worldpos_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
}`,lA=`#define TOON
uniform vec3 diffuse;
uniform vec3 emissive;
uniform float opacity;
#include <common>
#include <packing>
#include <dithering_pars_fragment>
#include <color_pars_fragment>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <aomap_pars_fragment>
#include <lightmap_pars_fragment>
#include <emissivemap_pars_fragment>
#include <gradientmap_pars_fragment>
#include <fog_pars_fragment>
#include <bsdfs>
#include <lights_pars_begin>
#include <normal_pars_fragment>
#include <lights_toon_pars_fragment>
#include <shadowmap_pars_fragment>
#include <bumpmap_pars_fragment>
#include <normalmap_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	ReflectedLight reflectedLight = ReflectedLight( vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ), vec3( 0.0 ) );
	vec3 totalEmissiveRadiance = emissive;
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <color_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	#include <normal_fragment_begin>
	#include <normal_fragment_maps>
	#include <emissivemap_fragment>
	#include <lights_toon_fragment>
	#include <lights_fragment_begin>
	#include <lights_fragment_maps>
	#include <lights_fragment_end>
	#include <aomap_fragment>
	vec3 outgoingLight = reflectedLight.directDiffuse + reflectedLight.indirectDiffuse + totalEmissiveRadiance;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
	#include <dithering_fragment>
}`,cA=`uniform float size;
uniform float scale;
#include <common>
#include <color_pars_vertex>
#include <fog_pars_vertex>
#include <morphtarget_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
#ifdef USE_POINTS_UV
	varying vec2 vUv;
	uniform mat3 uvTransform;
#endif
void main() {
	#ifdef USE_POINTS_UV
		vUv = ( uvTransform * vec3( uv, 1 ) ).xy;
	#endif
	#include <color_vertex>
	#include <morphinstance_vertex>
	#include <morphcolor_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <project_vertex>
	gl_PointSize = size;
	#ifdef USE_SIZEATTENUATION
		bool isPerspective = isPerspectiveMatrix( projectionMatrix );
		if ( isPerspective ) gl_PointSize *= ( scale / - mvPosition.z );
	#endif
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <worldpos_vertex>
	#include <fog_vertex>
}`,uA=`uniform vec3 diffuse;
uniform float opacity;
#include <common>
#include <color_pars_fragment>
#include <map_particle_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <fog_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	vec3 outgoingLight = vec3( 0.0 );
	#include <logdepthbuf_fragment>
	#include <map_particle_fragment>
	#include <color_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	outgoingLight = diffuseColor.rgb;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
	#include <premultiplied_alpha_fragment>
}`,fA=`#include <common>
#include <batching_pars_vertex>
#include <fog_pars_vertex>
#include <morphtarget_pars_vertex>
#include <skinning_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <shadowmap_pars_vertex>
void main() {
	#include <batching_vertex>
	#include <beginnormal_vertex>
	#include <morphinstance_vertex>
	#include <morphnormal_vertex>
	#include <skinbase_vertex>
	#include <skinnormal_vertex>
	#include <defaultnormal_vertex>
	#include <begin_vertex>
	#include <morphtarget_vertex>
	#include <skinning_vertex>
	#include <project_vertex>
	#include <logdepthbuf_vertex>
	#include <worldpos_vertex>
	#include <shadowmap_vertex>
	#include <fog_vertex>
}`,hA=`uniform vec3 color;
uniform float opacity;
#include <common>
#include <packing>
#include <fog_pars_fragment>
#include <bsdfs>
#include <lights_pars_begin>
#include <logdepthbuf_pars_fragment>
#include <shadowmap_pars_fragment>
#include <shadowmask_pars_fragment>
void main() {
	#include <logdepthbuf_fragment>
	gl_FragColor = vec4( color, opacity * ( 1.0 - getShadowMask() ) );
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
}`,dA=`uniform float rotation;
uniform vec2 center;
#include <common>
#include <uv_pars_vertex>
#include <fog_pars_vertex>
#include <logdepthbuf_pars_vertex>
#include <clipping_planes_pars_vertex>
void main() {
	#include <uv_vertex>
	vec4 mvPosition = modelViewMatrix[ 3 ];
	vec2 scale = vec2( length( modelMatrix[ 0 ].xyz ), length( modelMatrix[ 1 ].xyz ) );
	#ifndef USE_SIZEATTENUATION
		bool isPerspective = isPerspectiveMatrix( projectionMatrix );
		if ( isPerspective ) scale *= - mvPosition.z;
	#endif
	vec2 alignedPosition = ( position.xy - ( center - vec2( 0.5 ) ) ) * scale;
	vec2 rotatedPosition;
	rotatedPosition.x = cos( rotation ) * alignedPosition.x - sin( rotation ) * alignedPosition.y;
	rotatedPosition.y = sin( rotation ) * alignedPosition.x + cos( rotation ) * alignedPosition.y;
	mvPosition.xy += rotatedPosition;
	gl_Position = projectionMatrix * mvPosition;
	#include <logdepthbuf_vertex>
	#include <clipping_planes_vertex>
	#include <fog_vertex>
}`,pA=`uniform vec3 diffuse;
uniform float opacity;
#include <common>
#include <uv_pars_fragment>
#include <map_pars_fragment>
#include <alphamap_pars_fragment>
#include <alphatest_pars_fragment>
#include <alphahash_pars_fragment>
#include <fog_pars_fragment>
#include <logdepthbuf_pars_fragment>
#include <clipping_planes_pars_fragment>
void main() {
	vec4 diffuseColor = vec4( diffuse, opacity );
	#include <clipping_planes_fragment>
	vec3 outgoingLight = vec3( 0.0 );
	#include <logdepthbuf_fragment>
	#include <map_fragment>
	#include <alphamap_fragment>
	#include <alphatest_fragment>
	#include <alphahash_fragment>
	outgoingLight = diffuseColor.rgb;
	#include <opaque_fragment>
	#include <tonemapping_fragment>
	#include <colorspace_fragment>
	#include <fog_fragment>
}`,ge={alphahash_fragment:zE,alphahash_pars_fragment:BE,alphamap_fragment:IE,alphamap_pars_fragment:FE,alphatest_fragment:HE,alphatest_pars_fragment:GE,aomap_fragment:VE,aomap_pars_fragment:kE,batching_pars_vertex:XE,batching_vertex:WE,begin_vertex:jE,beginnormal_vertex:YE,bsdfs:qE,iridescence_fragment:ZE,bumpmap_pars_fragment:KE,clipping_planes_fragment:QE,clipping_planes_pars_fragment:JE,clipping_planes_pars_vertex:$E,clipping_planes_vertex:tT,color_fragment:eT,color_pars_fragment:nT,color_pars_vertex:iT,color_vertex:aT,common:sT,cube_uv_reflection_fragment:rT,defaultnormal_vertex:oT,displacementmap_pars_vertex:lT,displacementmap_vertex:cT,emissivemap_fragment:uT,emissivemap_pars_fragment:fT,colorspace_fragment:hT,colorspace_pars_fragment:dT,envmap_fragment:pT,envmap_common_pars_fragment:mT,envmap_pars_fragment:gT,envmap_pars_vertex:_T,envmap_physical_pars_fragment:CT,envmap_vertex:vT,fog_vertex:yT,fog_pars_vertex:xT,fog_fragment:ST,fog_pars_fragment:MT,gradientmap_pars_fragment:ET,lightmap_pars_fragment:TT,lights_lambert_fragment:bT,lights_lambert_pars_fragment:AT,lights_pars_begin:RT,lights_toon_fragment:wT,lights_toon_pars_fragment:DT,lights_phong_fragment:UT,lights_phong_pars_fragment:LT,lights_physical_fragment:NT,lights_physical_pars_fragment:OT,lights_fragment_begin:PT,lights_fragment_maps:zT,lights_fragment_end:BT,logdepthbuf_fragment:IT,logdepthbuf_pars_fragment:FT,logdepthbuf_pars_vertex:HT,logdepthbuf_vertex:GT,map_fragment:VT,map_pars_fragment:kT,map_particle_fragment:XT,map_particle_pars_fragment:WT,metalnessmap_fragment:jT,metalnessmap_pars_fragment:YT,morphinstance_vertex:qT,morphcolor_vertex:ZT,morphnormal_vertex:KT,morphtarget_pars_vertex:QT,morphtarget_vertex:JT,normal_fragment_begin:$T,normal_fragment_maps:tb,normal_pars_fragment:eb,normal_pars_vertex:nb,normal_vertex:ib,normalmap_pars_fragment:ab,clearcoat_normal_fragment_begin:sb,clearcoat_normal_fragment_maps:rb,clearcoat_pars_fragment:ob,iridescence_pars_fragment:lb,opaque_fragment:cb,packing:ub,premultiplied_alpha_fragment:fb,project_vertex:hb,dithering_fragment:db,dithering_pars_fragment:pb,roughnessmap_fragment:mb,roughnessmap_pars_fragment:gb,shadowmap_pars_fragment:_b,shadowmap_pars_vertex:vb,shadowmap_vertex:yb,shadowmask_pars_fragment:xb,skinbase_vertex:Sb,skinning_pars_vertex:Mb,skinning_vertex:Eb,skinnormal_vertex:Tb,specularmap_fragment:bb,specularmap_pars_fragment:Ab,tonemapping_fragment:Rb,tonemapping_pars_fragment:Cb,transmission_fragment:wb,transmission_pars_fragment:Db,uv_pars_fragment:Ub,uv_pars_vertex:Lb,uv_vertex:Nb,worldpos_vertex:Ob,background_vert:Pb,background_frag:zb,backgroundCube_vert:Bb,backgroundCube_frag:Ib,cube_vert:Fb,cube_frag:Hb,depth_vert:Gb,depth_frag:Vb,distanceRGBA_vert:kb,distanceRGBA_frag:Xb,equirect_vert:Wb,equirect_frag:jb,linedashed_vert:Yb,linedashed_frag:qb,meshbasic_vert:Zb,meshbasic_frag:Kb,meshlambert_vert:Qb,meshlambert_frag:Jb,meshmatcap_vert:$b,meshmatcap_frag:tA,meshnormal_vert:eA,meshnormal_frag:nA,meshphong_vert:iA,meshphong_frag:aA,meshphysical_vert:sA,meshphysical_frag:rA,meshtoon_vert:oA,meshtoon_frag:lA,points_vert:cA,points_frag:uA,shadow_vert:fA,shadow_frag:hA,sprite_vert:dA,sprite_frag:pA},Vt={common:{diffuse:{value:new Me(16777215)},opacity:{value:1},map:{value:null},mapTransform:{value:new me},alphaMap:{value:null},alphaMapTransform:{value:new me},alphaTest:{value:0}},specularmap:{specularMap:{value:null},specularMapTransform:{value:new me}},envmap:{envMap:{value:null},envMapRotation:{value:new me},flipEnvMap:{value:-1},reflectivity:{value:1},ior:{value:1.5},refractionRatio:{value:.98}},aomap:{aoMap:{value:null},aoMapIntensity:{value:1},aoMapTransform:{value:new me}},lightmap:{lightMap:{value:null},lightMapIntensity:{value:1},lightMapTransform:{value:new me}},bumpmap:{bumpMap:{value:null},bumpMapTransform:{value:new me},bumpScale:{value:1}},normalmap:{normalMap:{value:null},normalMapTransform:{value:new me},normalScale:{value:new re(1,1)}},displacementmap:{displacementMap:{value:null},displacementMapTransform:{value:new me},displacementScale:{value:1},displacementBias:{value:0}},emissivemap:{emissiveMap:{value:null},emissiveMapTransform:{value:new me}},metalnessmap:{metalnessMap:{value:null},metalnessMapTransform:{value:new me}},roughnessmap:{roughnessMap:{value:null},roughnessMapTransform:{value:new me}},gradientmap:{gradientMap:{value:null}},fog:{fogDensity:{value:25e-5},fogNear:{value:1},fogFar:{value:2e3},fogColor:{value:new Me(16777215)}},lights:{ambientLightColor:{value:[]},lightProbe:{value:[]},directionalLights:{value:[],properties:{direction:{},color:{}}},directionalLightShadows:{value:[],properties:{shadowIntensity:1,shadowBias:{},shadowNormalBias:{},shadowRadius:{},shadowMapSize:{}}},directionalShadowMap:{value:[]},directionalShadowMatrix:{value:[]},spotLights:{value:[],properties:{color:{},position:{},direction:{},distance:{},coneCos:{},penumbraCos:{},decay:{}}},spotLightShadows:{value:[],properties:{shadowIntensity:1,shadowBias:{},shadowNormalBias:{},shadowRadius:{},shadowMapSize:{}}},spotLightMap:{value:[]},spotShadowMap:{value:[]},spotLightMatrix:{value:[]},pointLights:{value:[],properties:{color:{},position:{},decay:{},distance:{}}},pointLightShadows:{value:[],properties:{shadowIntensity:1,shadowBias:{},shadowNormalBias:{},shadowRadius:{},shadowMapSize:{},shadowCameraNear:{},shadowCameraFar:{}}},pointShadowMap:{value:[]},pointShadowMatrix:{value:[]},hemisphereLights:{value:[],properties:{direction:{},skyColor:{},groundColor:{}}},rectAreaLights:{value:[],properties:{color:{},position:{},width:{},height:{}}},ltc_1:{value:null},ltc_2:{value:null}},points:{diffuse:{value:new Me(16777215)},opacity:{value:1},size:{value:1},scale:{value:1},map:{value:null},alphaMap:{value:null},alphaMapTransform:{value:new me},alphaTest:{value:0},uvTransform:{value:new me}},sprite:{diffuse:{value:new Me(16777215)},opacity:{value:1},center:{value:new re(.5,.5)},rotation:{value:0},map:{value:null},mapTransform:{value:new me},alphaMap:{value:null},alphaMapTransform:{value:new me},alphaTest:{value:0}}},Oi={basic:{uniforms:In([Vt.common,Vt.specularmap,Vt.envmap,Vt.aomap,Vt.lightmap,Vt.fog]),vertexShader:ge.meshbasic_vert,fragmentShader:ge.meshbasic_frag},lambert:{uniforms:In([Vt.common,Vt.specularmap,Vt.envmap,Vt.aomap,Vt.lightmap,Vt.emissivemap,Vt.bumpmap,Vt.normalmap,Vt.displacementmap,Vt.fog,Vt.lights,{emissive:{value:new Me(0)}}]),vertexShader:ge.meshlambert_vert,fragmentShader:ge.meshlambert_frag},phong:{uniforms:In([Vt.common,Vt.specularmap,Vt.envmap,Vt.aomap,Vt.lightmap,Vt.emissivemap,Vt.bumpmap,Vt.normalmap,Vt.displacementmap,Vt.fog,Vt.lights,{emissive:{value:new Me(0)},specular:{value:new Me(1118481)},shininess:{value:30}}]),vertexShader:ge.meshphong_vert,fragmentShader:ge.meshphong_frag},standard:{uniforms:In([Vt.common,Vt.envmap,Vt.aomap,Vt.lightmap,Vt.emissivemap,Vt.bumpmap,Vt.normalmap,Vt.displacementmap,Vt.roughnessmap,Vt.metalnessmap,Vt.fog,Vt.lights,{emissive:{value:new Me(0)},roughness:{value:1},metalness:{value:0},envMapIntensity:{value:1}}]),vertexShader:ge.meshphysical_vert,fragmentShader:ge.meshphysical_frag},toon:{uniforms:In([Vt.common,Vt.aomap,Vt.lightmap,Vt.emissivemap,Vt.bumpmap,Vt.normalmap,Vt.displacementmap,Vt.gradientmap,Vt.fog,Vt.lights,{emissive:{value:new Me(0)}}]),vertexShader:ge.meshtoon_vert,fragmentShader:ge.meshtoon_frag},matcap:{uniforms:In([Vt.common,Vt.bumpmap,Vt.normalmap,Vt.displacementmap,Vt.fog,{matcap:{value:null}}]),vertexShader:ge.meshmatcap_vert,fragmentShader:ge.meshmatcap_frag},points:{uniforms:In([Vt.points,Vt.fog]),vertexShader:ge.points_vert,fragmentShader:ge.points_frag},dashed:{uniforms:In([Vt.common,Vt.fog,{scale:{value:1},dashSize:{value:1},totalSize:{value:2}}]),vertexShader:ge.linedashed_vert,fragmentShader:ge.linedashed_frag},depth:{uniforms:In([Vt.common,Vt.displacementmap]),vertexShader:ge.depth_vert,fragmentShader:ge.depth_frag},normal:{uniforms:In([Vt.common,Vt.bumpmap,Vt.normalmap,Vt.displacementmap,{opacity:{value:1}}]),vertexShader:ge.meshnormal_vert,fragmentShader:ge.meshnormal_frag},sprite:{uniforms:In([Vt.sprite,Vt.fog]),vertexShader:ge.sprite_vert,fragmentShader:ge.sprite_frag},background:{uniforms:{uvTransform:{value:new me},t2D:{value:null},backgroundIntensity:{value:1}},vertexShader:ge.background_vert,fragmentShader:ge.background_frag},backgroundCube:{uniforms:{envMap:{value:null},flipEnvMap:{value:-1},backgroundBlurriness:{value:0},backgroundIntensity:{value:1},backgroundRotation:{value:new me}},vertexShader:ge.backgroundCube_vert,fragmentShader:ge.backgroundCube_frag},cube:{uniforms:{tCube:{value:null},tFlip:{value:-1},opacity:{value:1}},vertexShader:ge.cube_vert,fragmentShader:ge.cube_frag},equirect:{uniforms:{tEquirect:{value:null}},vertexShader:ge.equirect_vert,fragmentShader:ge.equirect_frag},distanceRGBA:{uniforms:In([Vt.common,Vt.displacementmap,{referencePosition:{value:new X},nearDistance:{value:1},farDistance:{value:1e3}}]),vertexShader:ge.distanceRGBA_vert,fragmentShader:ge.distanceRGBA_frag},shadow:{uniforms:In([Vt.lights,Vt.fog,{color:{value:new Me(0)},opacity:{value:1}}]),vertexShader:ge.shadow_vert,fragmentShader:ge.shadow_frag}};Oi.physical={uniforms:In([Oi.standard.uniforms,{clearcoat:{value:0},clearcoatMap:{value:null},clearcoatMapTransform:{value:new me},clearcoatNormalMap:{value:null},clearcoatNormalMapTransform:{value:new me},clearcoatNormalScale:{value:new re(1,1)},clearcoatRoughness:{value:0},clearcoatRoughnessMap:{value:null},clearcoatRoughnessMapTransform:{value:new me},dispersion:{value:0},iridescence:{value:0},iridescenceMap:{value:null},iridescenceMapTransform:{value:new me},iridescenceIOR:{value:1.3},iridescenceThicknessMinimum:{value:100},iridescenceThicknessMaximum:{value:400},iridescenceThicknessMap:{value:null},iridescenceThicknessMapTransform:{value:new me},sheen:{value:0},sheenColor:{value:new Me(0)},sheenColorMap:{value:null},sheenColorMapTransform:{value:new me},sheenRoughness:{value:1},sheenRoughnessMap:{value:null},sheenRoughnessMapTransform:{value:new me},transmission:{value:0},transmissionMap:{value:null},transmissionMapTransform:{value:new me},transmissionSamplerSize:{value:new re},transmissionSamplerMap:{value:null},thickness:{value:0},thicknessMap:{value:null},thicknessMapTransform:{value:new me},attenuationDistance:{value:0},attenuationColor:{value:new Me(0)},specularColor:{value:new Me(1,1,1)},specularColorMap:{value:null},specularColorMapTransform:{value:new me},specularIntensity:{value:1},specularIntensityMap:{value:null},specularIntensityMapTransform:{value:new me},anisotropyVector:{value:new re},anisotropyMap:{value:null},anisotropyMapTransform:{value:new me}}]),vertexShader:ge.meshphysical_vert,fragmentShader:ge.meshphysical_frag};const Wc={r:0,b:0,g:0},ys=new Fi,mA=new qe;function gA(r,t,i,s,l,c,h){const d=new Me(0);let m=c===!0?0:1,p,v,g=null,x=0,S=null;function b(U){let T=U.isScene===!0?U.background:null;return T&&T.isTexture&&(T=(U.backgroundBlurriness>0?i:t).get(T)),T}function R(U){let T=!1;const P=b(U);P===null?_(d,m):P&&P.isColor&&(_(P,1),T=!0);const B=r.xr.getEnvironmentBlendMode();B==="additive"?s.buffers.color.setClear(0,0,0,1,h):B==="alpha-blend"&&s.buffers.color.setClear(0,0,0,0,h),(r.autoClear||T)&&(s.buffers.depth.setTest(!0),s.buffers.depth.setMask(!0),s.buffers.color.setMask(!0),r.clear(r.autoClearColor,r.autoClearDepth,r.autoClearStencil))}function M(U,T){const P=b(T);P&&(P.isCubeTexture||P.mapping===fu)?(v===void 0&&(v=new vi(new jr(1,1,1),new Za({name:"BackgroundCubeMaterial",uniforms:Xr(Oi.backgroundCube.uniforms),vertexShader:Oi.backgroundCube.vertexShader,fragmentShader:Oi.backgroundCube.fragmentShader,side:Yn,depthTest:!1,depthWrite:!1,fog:!1,allowOverride:!1})),v.geometry.deleteAttribute("normal"),v.geometry.deleteAttribute("uv"),v.onBeforeRender=function(B,F,G){this.matrixWorld.copyPosition(G.matrixWorld)},Object.defineProperty(v.material,"envMap",{get:function(){return this.uniforms.envMap.value}}),l.update(v)),ys.copy(T.backgroundRotation),ys.x*=-1,ys.y*=-1,ys.z*=-1,P.isCubeTexture&&P.isRenderTargetTexture===!1&&(ys.y*=-1,ys.z*=-1),v.material.uniforms.envMap.value=P,v.material.uniforms.flipEnvMap.value=P.isCubeTexture&&P.isRenderTargetTexture===!1?-1:1,v.material.uniforms.backgroundBlurriness.value=T.backgroundBlurriness,v.material.uniforms.backgroundIntensity.value=T.backgroundIntensity,v.material.uniforms.backgroundRotation.value.setFromMatrix4(mA.makeRotationFromEuler(ys)),v.material.toneMapped=Pe.getTransfer(P.colorSpace)!==We,(g!==P||x!==P.version||S!==r.toneMapping)&&(v.material.needsUpdate=!0,g=P,x=P.version,S=r.toneMapping),v.layers.enableAll(),U.unshift(v,v.geometry,v.material,0,0,null)):P&&P.isTexture&&(p===void 0&&(p=new vi(new du(2,2),new Za({name:"BackgroundMaterial",uniforms:Xr(Oi.background.uniforms),vertexShader:Oi.background.vertexShader,fragmentShader:Oi.background.fragmentShader,side:qa,depthTest:!1,depthWrite:!1,fog:!1,allowOverride:!1})),p.geometry.deleteAttribute("normal"),Object.defineProperty(p.material,"map",{get:function(){return this.uniforms.t2D.value}}),l.update(p)),p.material.uniforms.t2D.value=P,p.material.uniforms.backgroundIntensity.value=T.backgroundIntensity,p.material.toneMapped=Pe.getTransfer(P.colorSpace)!==We,P.matrixAutoUpdate===!0&&P.updateMatrix(),p.material.uniforms.uvTransform.value.copy(P.matrix),(g!==P||x!==P.version||S!==r.toneMapping)&&(p.material.needsUpdate=!0,g=P,x=P.version,S=r.toneMapping),p.layers.enableAll(),U.unshift(p,p.geometry,p.material,0,0,null))}function _(U,T){U.getRGB(Wc,$v(r)),s.buffers.color.setClear(Wc.r,Wc.g,Wc.b,T,h)}function L(){v!==void 0&&(v.geometry.dispose(),v.material.dispose(),v=void 0),p!==void 0&&(p.geometry.dispose(),p.material.dispose(),p=void 0)}return{getClearColor:function(){return d},setClearColor:function(U,T=1){d.set(U),m=T,_(d,m)},getClearAlpha:function(){return m},setClearAlpha:function(U){m=U,_(d,m)},render:R,addToRenderList:M,dispose:L}}function _A(r,t){const i=r.getParameter(r.MAX_VERTEX_ATTRIBS),s={},l=x(null);let c=l,h=!1;function d(w,V,K,ot,ct){let ht=!1;const N=g(ot,K,V);c!==N&&(c=N,p(c.object)),ht=S(w,ot,K,ct),ht&&b(w,ot,K,ct),ct!==null&&t.update(ct,r.ELEMENT_ARRAY_BUFFER),(ht||h)&&(h=!1,T(w,V,K,ot),ct!==null&&r.bindBuffer(r.ELEMENT_ARRAY_BUFFER,t.get(ct).buffer))}function m(){return r.createVertexArray()}function p(w){return r.bindVertexArray(w)}function v(w){return r.deleteVertexArray(w)}function g(w,V,K){const ot=K.wireframe===!0;let ct=s[w.id];ct===void 0&&(ct={},s[w.id]=ct);let ht=ct[V.id];ht===void 0&&(ht={},ct[V.id]=ht);let N=ht[ot];return N===void 0&&(N=x(m()),ht[ot]=N),N}function x(w){const V=[],K=[],ot=[];for(let ct=0;ct<i;ct++)V[ct]=0,K[ct]=0,ot[ct]=0;return{geometry:null,program:null,wireframe:!1,newAttributes:V,enabledAttributes:K,attributeDivisors:ot,object:w,attributes:{},index:null}}function S(w,V,K,ot){const ct=c.attributes,ht=V.attributes;let N=0;const J=K.getAttributes();for(const Y in J)if(J[Y].location>=0){const Ct=ct[Y];let z=ht[Y];if(z===void 0&&(Y==="instanceMatrix"&&w.instanceMatrix&&(z=w.instanceMatrix),Y==="instanceColor"&&w.instanceColor&&(z=w.instanceColor)),Ct===void 0||Ct.attribute!==z||z&&Ct.data!==z.data)return!0;N++}return c.attributesNum!==N||c.index!==ot}function b(w,V,K,ot){const ct={},ht=V.attributes;let N=0;const J=K.getAttributes();for(const Y in J)if(J[Y].location>=0){let Ct=ht[Y];Ct===void 0&&(Y==="instanceMatrix"&&w.instanceMatrix&&(Ct=w.instanceMatrix),Y==="instanceColor"&&w.instanceColor&&(Ct=w.instanceColor));const z={};z.attribute=Ct,Ct&&Ct.data&&(z.data=Ct.data),ct[Y]=z,N++}c.attributes=ct,c.attributesNum=N,c.index=ot}function R(){const w=c.newAttributes;for(let V=0,K=w.length;V<K;V++)w[V]=0}function M(w){_(w,0)}function _(w,V){const K=c.newAttributes,ot=c.enabledAttributes,ct=c.attributeDivisors;K[w]=1,ot[w]===0&&(r.enableVertexAttribArray(w),ot[w]=1),ct[w]!==V&&(r.vertexAttribDivisor(w,V),ct[w]=V)}function L(){const w=c.newAttributes,V=c.enabledAttributes;for(let K=0,ot=V.length;K<ot;K++)V[K]!==w[K]&&(r.disableVertexAttribArray(K),V[K]=0)}function U(w,V,K,ot,ct,ht,N){N===!0?r.vertexAttribIPointer(w,V,K,ct,ht):r.vertexAttribPointer(w,V,K,ot,ct,ht)}function T(w,V,K,ot){R();const ct=ot.attributes,ht=K.getAttributes(),N=V.defaultAttributeValues;for(const J in ht){const Y=ht[J];if(Y.location>=0){let Tt=ct[J];if(Tt===void 0&&(J==="instanceMatrix"&&w.instanceMatrix&&(Tt=w.instanceMatrix),J==="instanceColor"&&w.instanceColor&&(Tt=w.instanceColor)),Tt!==void 0){const Ct=Tt.normalized,z=Tt.itemSize,at=t.get(Tt);if(at===void 0)continue;const Mt=at.buffer,wt=at.type,mt=at.bytesPerElement,q=wt===r.INT||wt===r.UNSIGNED_INT||Tt.gpuType===ip;if(Tt.isInterleavedBufferAttribute){const it=Tt.data,xt=it.stride,Lt=Tt.offset;if(it.isInstancedInterleavedBuffer){for(let Bt=0;Bt<Y.locationSize;Bt++)_(Y.location+Bt,it.meshPerAttribute);w.isInstancedMesh!==!0&&ot._maxInstanceCount===void 0&&(ot._maxInstanceCount=it.meshPerAttribute*it.count)}else for(let Bt=0;Bt<Y.locationSize;Bt++)M(Y.location+Bt);r.bindBuffer(r.ARRAY_BUFFER,Mt);for(let Bt=0;Bt<Y.locationSize;Bt++)U(Y.location+Bt,z/Y.locationSize,wt,Ct,xt*mt,(Lt+z/Y.locationSize*Bt)*mt,q)}else{if(Tt.isInstancedBufferAttribute){for(let it=0;it<Y.locationSize;it++)_(Y.location+it,Tt.meshPerAttribute);w.isInstancedMesh!==!0&&ot._maxInstanceCount===void 0&&(ot._maxInstanceCount=Tt.meshPerAttribute*Tt.count)}else for(let it=0;it<Y.locationSize;it++)M(Y.location+it);r.bindBuffer(r.ARRAY_BUFFER,Mt);for(let it=0;it<Y.locationSize;it++)U(Y.location+it,z/Y.locationSize,wt,Ct,z*mt,z/Y.locationSize*it*mt,q)}}else if(N!==void 0){const Ct=N[J];if(Ct!==void 0)switch(Ct.length){case 2:r.vertexAttrib2fv(Y.location,Ct);break;case 3:r.vertexAttrib3fv(Y.location,Ct);break;case 4:r.vertexAttrib4fv(Y.location,Ct);break;default:r.vertexAttrib1fv(Y.location,Ct)}}}}L()}function P(){G();for(const w in s){const V=s[w];for(const K in V){const ot=V[K];for(const ct in ot)v(ot[ct].object),delete ot[ct];delete V[K]}delete s[w]}}function B(w){if(s[w.id]===void 0)return;const V=s[w.id];for(const K in V){const ot=V[K];for(const ct in ot)v(ot[ct].object),delete ot[ct];delete V[K]}delete s[w.id]}function F(w){for(const V in s){const K=s[V];if(K[w.id]===void 0)continue;const ot=K[w.id];for(const ct in ot)v(ot[ct].object),delete ot[ct];delete K[w.id]}}function G(){D(),h=!0,c!==l&&(c=l,p(c.object))}function D(){l.geometry=null,l.program=null,l.wireframe=!1}return{setup:d,reset:G,resetDefaultState:D,dispose:P,releaseStatesOfGeometry:B,releaseStatesOfProgram:F,initAttributes:R,enableAttribute:M,disableUnusedAttributes:L}}function vA(r,t,i){let s;function l(p){s=p}function c(p,v){r.drawArrays(s,p,v),i.update(v,s,1)}function h(p,v,g){g!==0&&(r.drawArraysInstanced(s,p,v,g),i.update(v,s,g))}function d(p,v,g){if(g===0)return;t.get("WEBGL_multi_draw").multiDrawArraysWEBGL(s,p,0,v,0,g);let S=0;for(let b=0;b<g;b++)S+=v[b];i.update(S,s,1)}function m(p,v,g,x){if(g===0)return;const S=t.get("WEBGL_multi_draw");if(S===null)for(let b=0;b<p.length;b++)h(p[b],v[b],x[b]);else{S.multiDrawArraysInstancedWEBGL(s,p,0,v,0,x,0,g);let b=0;for(let R=0;R<g;R++)b+=v[R]*x[R];i.update(b,s,1)}}this.setMode=l,this.render=c,this.renderInstances=h,this.renderMultiDraw=d,this.renderMultiDrawInstances=m}function yA(r,t,i,s){let l;function c(){if(l!==void 0)return l;if(t.has("EXT_texture_filter_anisotropic")===!0){const F=t.get("EXT_texture_filter_anisotropic");l=r.getParameter(F.MAX_TEXTURE_MAX_ANISOTROPY_EXT)}else l=0;return l}function h(F){return!(F!==Ri&&s.convert(F)!==r.getParameter(r.IMPLEMENTATION_COLOR_READ_FORMAT))}function d(F){const G=F===cl&&(t.has("EXT_color_buffer_half_float")||t.has("EXT_color_buffer_float"));return!(F!==Ii&&s.convert(F)!==r.getParameter(r.IMPLEMENTATION_COLOR_READ_TYPE)&&F!==zi&&!G)}function m(F){if(F==="highp"){if(r.getShaderPrecisionFormat(r.VERTEX_SHADER,r.HIGH_FLOAT).precision>0&&r.getShaderPrecisionFormat(r.FRAGMENT_SHADER,r.HIGH_FLOAT).precision>0)return"highp";F="mediump"}return F==="mediump"&&r.getShaderPrecisionFormat(r.VERTEX_SHADER,r.MEDIUM_FLOAT).precision>0&&r.getShaderPrecisionFormat(r.FRAGMENT_SHADER,r.MEDIUM_FLOAT).precision>0?"mediump":"lowp"}let p=i.precision!==void 0?i.precision:"highp";const v=m(p);v!==p&&(console.warn("THREE.WebGLRenderer:",p,"not supported, using",v,"instead."),p=v);const g=i.logarithmicDepthBuffer===!0,x=i.reversedDepthBuffer===!0&&t.has("EXT_clip_control"),S=r.getParameter(r.MAX_TEXTURE_IMAGE_UNITS),b=r.getParameter(r.MAX_VERTEX_TEXTURE_IMAGE_UNITS),R=r.getParameter(r.MAX_TEXTURE_SIZE),M=r.getParameter(r.MAX_CUBE_MAP_TEXTURE_SIZE),_=r.getParameter(r.MAX_VERTEX_ATTRIBS),L=r.getParameter(r.MAX_VERTEX_UNIFORM_VECTORS),U=r.getParameter(r.MAX_VARYING_VECTORS),T=r.getParameter(r.MAX_FRAGMENT_UNIFORM_VECTORS),P=b>0,B=r.getParameter(r.MAX_SAMPLES);return{isWebGL2:!0,getMaxAnisotropy:c,getMaxPrecision:m,textureFormatReadable:h,textureTypeReadable:d,precision:p,logarithmicDepthBuffer:g,reversedDepthBuffer:x,maxTextures:S,maxVertexTextures:b,maxTextureSize:R,maxCubemapSize:M,maxAttributes:_,maxVertexUniforms:L,maxVaryings:U,maxFragmentUniforms:T,vertexTextures:P,maxSamples:B}}function xA(r){const t=this;let i=null,s=0,l=!1,c=!1;const h=new Xa,d=new me,m={value:null,needsUpdate:!1};this.uniform=m,this.numPlanes=0,this.numIntersection=0,this.init=function(g,x){const S=g.length!==0||x||s!==0||l;return l=x,s=g.length,S},this.beginShadows=function(){c=!0,v(null)},this.endShadows=function(){c=!1},this.setGlobalState=function(g,x){i=v(g,x,0)},this.setState=function(g,x,S){const b=g.clippingPlanes,R=g.clipIntersection,M=g.clipShadows,_=r.get(g);if(!l||b===null||b.length===0||c&&!M)c?v(null):p();else{const L=c?0:s,U=L*4;let T=_.clippingState||null;m.value=T,T=v(b,x,U,S);for(let P=0;P!==U;++P)T[P]=i[P];_.clippingState=T,this.numIntersection=R?this.numPlanes:0,this.numPlanes+=L}};function p(){m.value!==i&&(m.value=i,m.needsUpdate=s>0),t.numPlanes=s,t.numIntersection=0}function v(g,x,S,b){const R=g!==null?g.length:0;let M=null;if(R!==0){if(M=m.value,b!==!0||M===null){const _=S+R*4,L=x.matrixWorldInverse;d.getNormalMatrix(L),(M===null||M.length<_)&&(M=new Float32Array(_));for(let U=0,T=S;U!==R;++U,T+=4)h.copy(g[U]).applyMatrix4(L,d),h.normal.toArray(M,T),M[T+3]=h.constant}m.value=M,m.needsUpdate=!0}return t.numPlanes=R,t.numIntersection=0,M}}function SA(r){let t=new WeakMap;function i(h,d){return d===xd?h.mapping=Gr:d===Sd&&(h.mapping=Vr),h}function s(h){if(h&&h.isTexture){const d=h.mapping;if(d===xd||d===Sd)if(t.has(h)){const m=t.get(h).texture;return i(m,h.mapping)}else{const m=h.image;if(m&&m.height>0){const p=new uE(m.height);return p.fromEquirectangularTexture(r,h),t.set(h,p),h.addEventListener("dispose",l),i(p.texture,h.mapping)}else return null}}return h}function l(h){const d=h.target;d.removeEventListener("dispose",l);const m=t.get(d);m!==void 0&&(t.delete(d),m.dispose())}function c(){t=new WeakMap}return{get:s,dispose:c}}const zr=4,tv=[.125,.215,.35,.446,.526,.582],bs=20,nd=new DE,ev=new Me;let id=null,ad=0,sd=0,rd=!1;const Es=(1+Math.sqrt(5))/2,Nr=1/Es,nv=[new X(-Es,Nr,0),new X(Es,Nr,0),new X(-Nr,0,Es),new X(Nr,0,Es),new X(0,Es,-Nr),new X(0,Es,Nr),new X(-1,1,-1),new X(1,1,-1),new X(-1,1,1),new X(1,1,1)],MA=new X;class iv{constructor(t){this._renderer=t,this._pingPongRenderTarget=null,this._lodMax=0,this._cubeSize=0,this._lodPlanes=[],this._sizeLods=[],this._sigmas=[],this._blurMaterial=null,this._cubemapMaterial=null,this._equirectMaterial=null,this._compileMaterial(this._blurMaterial)}fromScene(t,i=0,s=.1,l=100,c={}){const{size:h=256,position:d=MA}=c;id=this._renderer.getRenderTarget(),ad=this._renderer.getActiveCubeFace(),sd=this._renderer.getActiveMipmapLevel(),rd=this._renderer.xr.enabled,this._renderer.xr.enabled=!1,this._setSize(h);const m=this._allocateTargets();return m.depthBuffer=!0,this._sceneToCubeUV(t,s,l,m,d),i>0&&this._blur(m,0,0,i),this._applyPMREM(m),this._cleanup(m),m}fromEquirectangular(t,i=null){return this._fromTexture(t,i)}fromCubemap(t,i=null){return this._fromTexture(t,i)}compileCubemapShader(){this._cubemapMaterial===null&&(this._cubemapMaterial=rv(),this._compileMaterial(this._cubemapMaterial))}compileEquirectangularShader(){this._equirectMaterial===null&&(this._equirectMaterial=sv(),this._compileMaterial(this._equirectMaterial))}dispose(){this._dispose(),this._cubemapMaterial!==null&&this._cubemapMaterial.dispose(),this._equirectMaterial!==null&&this._equirectMaterial.dispose()}_setSize(t){this._lodMax=Math.floor(Math.log2(t)),this._cubeSize=Math.pow(2,this._lodMax)}_dispose(){this._blurMaterial!==null&&this._blurMaterial.dispose(),this._pingPongRenderTarget!==null&&this._pingPongRenderTarget.dispose();for(let t=0;t<this._lodPlanes.length;t++)this._lodPlanes[t].dispose()}_cleanup(t){this._renderer.setRenderTarget(id,ad,sd),this._renderer.xr.enabled=rd,t.scissorTest=!1,jc(t,0,0,t.width,t.height)}_fromTexture(t,i){t.mapping===Gr||t.mapping===Vr?this._setSize(t.image.length===0?16:t.image[0].width||t.image[0].image.width):this._setSize(t.image.width/4),id=this._renderer.getRenderTarget(),ad=this._renderer.getActiveCubeFace(),sd=this._renderer.getActiveMipmapLevel(),rd=this._renderer.xr.enabled,this._renderer.xr.enabled=!1;const s=i||this._allocateTargets();return this._textureToCubeUV(t,s),this._applyPMREM(s),this._cleanup(s),s}_allocateTargets(){const t=3*Math.max(this._cubeSize,112),i=4*this._cubeSize,s={magFilter:Pi,minFilter:Pi,generateMipmaps:!1,type:cl,format:Ri,colorSpace:kr,depthBuffer:!1},l=av(t,i,s);if(this._pingPongRenderTarget===null||this._pingPongRenderTarget.width!==t||this._pingPongRenderTarget.height!==i){this._pingPongRenderTarget!==null&&this._dispose(),this._pingPongRenderTarget=av(t,i,s);const{_lodMax:c}=this;({sizeLods:this._sizeLods,lodPlanes:this._lodPlanes,sigmas:this._sigmas}=EA(c)),this._blurMaterial=TA(c,t,i)}return l}_compileMaterial(t){const i=new vi(this._lodPlanes[0],t);this._renderer.compile(i,nd)}_sceneToCubeUV(t,i,s,l,c){const m=new ai(90,1,i,s),p=[1,-1,1,1,1,1],v=[1,1,1,-1,-1,-1],g=this._renderer,x=g.autoClear,S=g.toneMapping;g.getClearColor(ev),g.toneMapping=Ya,g.autoClear=!1,g.state.buffers.depth.getReversed()&&(g.setRenderTarget(l),g.clearDepth(),g.setRenderTarget(null));const R=new su({name:"PMREM.Background",side:Yn,depthWrite:!1,depthTest:!1}),M=new vi(new jr,R);let _=!1;const L=t.background;L?L.isColor&&(R.color.copy(L),t.background=null,_=!0):(R.color.copy(ev),_=!0);for(let U=0;U<6;U++){const T=U%3;T===0?(m.up.set(0,p[U],0),m.position.set(c.x,c.y,c.z),m.lookAt(c.x+v[U],c.y,c.z)):T===1?(m.up.set(0,0,p[U]),m.position.set(c.x,c.y,c.z),m.lookAt(c.x,c.y+v[U],c.z)):(m.up.set(0,p[U],0),m.position.set(c.x,c.y,c.z),m.lookAt(c.x,c.y,c.z+v[U]));const P=this._cubeSize;jc(l,T*P,U>2?P:0,P,P),g.setRenderTarget(l),_&&g.render(M,m),g.render(t,m)}M.geometry.dispose(),M.material.dispose(),g.toneMapping=S,g.autoClear=x,t.background=L}_textureToCubeUV(t,i){const s=this._renderer,l=t.mapping===Gr||t.mapping===Vr;l?(this._cubemapMaterial===null&&(this._cubemapMaterial=rv()),this._cubemapMaterial.uniforms.flipEnvMap.value=t.isRenderTargetTexture===!1?-1:1):this._equirectMaterial===null&&(this._equirectMaterial=sv());const c=l?this._cubemapMaterial:this._equirectMaterial,h=new vi(this._lodPlanes[0],c),d=c.uniforms;d.envMap.value=t;const m=this._cubeSize;jc(i,0,0,3*m,2*m),s.setRenderTarget(i),s.render(h,nd)}_applyPMREM(t){const i=this._renderer,s=i.autoClear;i.autoClear=!1;const l=this._lodPlanes.length;for(let c=1;c<l;c++){const h=Math.sqrt(this._sigmas[c]*this._sigmas[c]-this._sigmas[c-1]*this._sigmas[c-1]),d=nv[(l-c-1)%nv.length];this._blur(t,c-1,c,h,d)}i.autoClear=s}_blur(t,i,s,l,c){const h=this._pingPongRenderTarget;this._halfBlur(t,h,i,s,l,"latitudinal",c),this._halfBlur(h,t,s,s,l,"longitudinal",c)}_halfBlur(t,i,s,l,c,h,d){const m=this._renderer,p=this._blurMaterial;h!=="latitudinal"&&h!=="longitudinal"&&console.error("blur direction must be either latitudinal or longitudinal!");const v=3,g=new vi(this._lodPlanes[l],p),x=p.uniforms,S=this._sizeLods[s]-1,b=isFinite(c)?Math.PI/(2*S):2*Math.PI/(2*bs-1),R=c/b,M=isFinite(c)?1+Math.floor(v*R):bs;M>bs&&console.warn(`sigmaRadians, ${c}, is too large and will clip, as it requested ${M} samples when the maximum is set to ${bs}`);const _=[];let L=0;for(let F=0;F<bs;++F){const G=F/R,D=Math.exp(-G*G/2);_.push(D),F===0?L+=D:F<M&&(L+=2*D)}for(let F=0;F<_.length;F++)_[F]=_[F]/L;x.envMap.value=t.texture,x.samples.value=M,x.weights.value=_,x.latitudinal.value=h==="latitudinal",d&&(x.poleAxis.value=d);const{_lodMax:U}=this;x.dTheta.value=b,x.mipInt.value=U-s;const T=this._sizeLods[l],P=3*T*(l>U-zr?l-U+zr:0),B=4*(this._cubeSize-T);jc(i,P,B,3*T,2*T),m.setRenderTarget(i),m.render(g,nd)}}function EA(r){const t=[],i=[],s=[];let l=r;const c=r-zr+1+tv.length;for(let h=0;h<c;h++){const d=Math.pow(2,l);i.push(d);let m=1/d;h>r-zr?m=tv[h-r+zr-1]:h===0&&(m=0),s.push(m);const p=1/(d-2),v=-p,g=1+p,x=[v,v,g,v,g,g,v,v,g,g,v,g],S=6,b=6,R=3,M=2,_=1,L=new Float32Array(R*b*S),U=new Float32Array(M*b*S),T=new Float32Array(_*b*S);for(let B=0;B<S;B++){const F=B%3*2/3-1,G=B>2?0:-1,D=[F,G,0,F+2/3,G,0,F+2/3,G+1,0,F,G,0,F+2/3,G+1,0,F,G+1,0];L.set(D,R*b*B),U.set(x,M*b*B);const w=[B,B,B,B,B,B];T.set(w,_*b*B)}const P=new ri;P.setAttribute("position",new yi(L,R)),P.setAttribute("uv",new yi(U,M)),P.setAttribute("faceIndex",new yi(T,_)),t.push(P),l>zr&&l--}return{lodPlanes:t,sizeLods:i,sigmas:s}}function av(r,t,i){const s=new Ds(r,t,i);return s.texture.mapping=fu,s.texture.name="PMREM.cubeUv",s.scissorTest=!0,s}function jc(r,t,i,s,l){r.viewport.set(t,i,s,l),r.scissor.set(t,i,s,l)}function TA(r,t,i){const s=new Float32Array(bs),l=new X(0,1,0);return new Za({name:"SphericalGaussianBlur",defines:{n:bs,CUBEUV_TEXEL_WIDTH:1/t,CUBEUV_TEXEL_HEIGHT:1/i,CUBEUV_MAX_MIP:`${r}.0`},uniforms:{envMap:{value:null},samples:{value:1},weights:{value:s},latitudinal:{value:!1},dTheta:{value:0},mipInt:{value:0},poleAxis:{value:l}},vertexShader:gp(),fragmentShader:`

			precision mediump float;
			precision mediump int;

			varying vec3 vOutputDirection;

			uniform sampler2D envMap;
			uniform int samples;
			uniform float weights[ n ];
			uniform bool latitudinal;
			uniform float dTheta;
			uniform float mipInt;
			uniform vec3 poleAxis;

			#define ENVMAP_TYPE_CUBE_UV
			#include <cube_uv_reflection_fragment>

			vec3 getSample( float theta, vec3 axis ) {

				float cosTheta = cos( theta );
				// Rodrigues' axis-angle rotation
				vec3 sampleDirection = vOutputDirection * cosTheta
					+ cross( axis, vOutputDirection ) * sin( theta )
					+ axis * dot( axis, vOutputDirection ) * ( 1.0 - cosTheta );

				return bilinearCubeUV( envMap, sampleDirection, mipInt );

			}

			void main() {

				vec3 axis = latitudinal ? poleAxis : cross( poleAxis, vOutputDirection );

				if ( all( equal( axis, vec3( 0.0 ) ) ) ) {

					axis = vec3( vOutputDirection.z, 0.0, - vOutputDirection.x );

				}

				axis = normalize( axis );

				gl_FragColor = vec4( 0.0, 0.0, 0.0, 1.0 );
				gl_FragColor.rgb += weights[ 0 ] * getSample( 0.0, axis );

				for ( int i = 1; i < n; i++ ) {

					if ( i >= samples ) {

						break;

					}

					float theta = dTheta * float( i );
					gl_FragColor.rgb += weights[ i ] * getSample( -1.0 * theta, axis );
					gl_FragColor.rgb += weights[ i ] * getSample( theta, axis );

				}

			}
		`,blending:ja,depthTest:!1,depthWrite:!1})}function sv(){return new Za({name:"EquirectangularToCubeUV",uniforms:{envMap:{value:null}},vertexShader:gp(),fragmentShader:`

			precision mediump float;
			precision mediump int;

			varying vec3 vOutputDirection;

			uniform sampler2D envMap;

			#include <common>

			void main() {

				vec3 outputDirection = normalize( vOutputDirection );
				vec2 uv = equirectUv( outputDirection );

				gl_FragColor = vec4( texture2D ( envMap, uv ).rgb, 1.0 );

			}
		`,blending:ja,depthTest:!1,depthWrite:!1})}function rv(){return new Za({name:"CubemapToCubeUV",uniforms:{envMap:{value:null},flipEnvMap:{value:-1}},vertexShader:gp(),fragmentShader:`

			precision mediump float;
			precision mediump int;

			uniform float flipEnvMap;

			varying vec3 vOutputDirection;

			uniform samplerCube envMap;

			void main() {

				gl_FragColor = textureCube( envMap, vec3( flipEnvMap * vOutputDirection.x, vOutputDirection.yz ) );

			}
		`,blending:ja,depthTest:!1,depthWrite:!1})}function gp(){return`

		precision mediump float;
		precision mediump int;

		attribute float faceIndex;

		varying vec3 vOutputDirection;

		// RH coordinate system; PMREM face-indexing convention
		vec3 getDirection( vec2 uv, float face ) {

			uv = 2.0 * uv - 1.0;

			vec3 direction = vec3( uv, 1.0 );

			if ( face == 0.0 ) {

				direction = direction.zyx; // ( 1, v, u ) pos x

			} else if ( face == 1.0 ) {

				direction = direction.xzy;
				direction.xz *= -1.0; // ( -u, 1, -v ) pos y

			} else if ( face == 2.0 ) {

				direction.x *= -1.0; // ( -u, v, 1 ) pos z

			} else if ( face == 3.0 ) {

				direction = direction.zyx;
				direction.xz *= -1.0; // ( -1, v, -u ) neg x

			} else if ( face == 4.0 ) {

				direction = direction.xzy;
				direction.xy *= -1.0; // ( -u, -1, v ) neg y

			} else if ( face == 5.0 ) {

				direction.z *= -1.0; // ( u, v, -1 ) neg z

			}

			return direction;

		}

		void main() {

			vOutputDirection = getDirection( uv, faceIndex );
			gl_Position = vec4( position, 1.0 );

		}
	`}function bA(r){let t=new WeakMap,i=null;function s(d){if(d&&d.isTexture){const m=d.mapping,p=m===xd||m===Sd,v=m===Gr||m===Vr;if(p||v){let g=t.get(d);const x=g!==void 0?g.texture.pmremVersion:0;if(d.isRenderTargetTexture&&d.pmremVersion!==x)return i===null&&(i=new iv(r)),g=p?i.fromEquirectangular(d,g):i.fromCubemap(d,g),g.texture.pmremVersion=d.pmremVersion,t.set(d,g),g.texture;if(g!==void 0)return g.texture;{const S=d.image;return p&&S&&S.height>0||v&&S&&l(S)?(i===null&&(i=new iv(r)),g=p?i.fromEquirectangular(d):i.fromCubemap(d),g.texture.pmremVersion=d.pmremVersion,t.set(d,g),d.addEventListener("dispose",c),g.texture):null}}}return d}function l(d){let m=0;const p=6;for(let v=0;v<p;v++)d[v]!==void 0&&m++;return m===p}function c(d){const m=d.target;m.removeEventListener("dispose",c);const p=t.get(m);p!==void 0&&(t.delete(m),p.dispose())}function h(){t=new WeakMap,i!==null&&(i.dispose(),i=null)}return{get:s,dispose:h}}function AA(r){const t={};function i(s){if(t[s]!==void 0)return t[s];let l;switch(s){case"WEBGL_depth_texture":l=r.getExtension("WEBGL_depth_texture")||r.getExtension("MOZ_WEBGL_depth_texture")||r.getExtension("WEBKIT_WEBGL_depth_texture");break;case"EXT_texture_filter_anisotropic":l=r.getExtension("EXT_texture_filter_anisotropic")||r.getExtension("MOZ_EXT_texture_filter_anisotropic")||r.getExtension("WEBKIT_EXT_texture_filter_anisotropic");break;case"WEBGL_compressed_texture_s3tc":l=r.getExtension("WEBGL_compressed_texture_s3tc")||r.getExtension("MOZ_WEBGL_compressed_texture_s3tc")||r.getExtension("WEBKIT_WEBGL_compressed_texture_s3tc");break;case"WEBGL_compressed_texture_pvrtc":l=r.getExtension("WEBGL_compressed_texture_pvrtc")||r.getExtension("WEBKIT_WEBGL_compressed_texture_pvrtc");break;default:l=r.getExtension(s)}return t[s]=l,l}return{has:function(s){return i(s)!==null},init:function(){i("EXT_color_buffer_float"),i("WEBGL_clip_cull_distance"),i("OES_texture_float_linear"),i("EXT_color_buffer_half_float"),i("WEBGL_multisampled_render_to_texture"),i("WEBGL_render_shared_exponent")},get:function(s){const l=i(s);return l===null&&ll("THREE.WebGLRenderer: "+s+" extension not supported."),l}}}function RA(r,t,i,s){const l={},c=new WeakMap;function h(g){const x=g.target;x.index!==null&&t.remove(x.index);for(const b in x.attributes)t.remove(x.attributes[b]);x.removeEventListener("dispose",h),delete l[x.id];const S=c.get(x);S&&(t.remove(S),c.delete(x)),s.releaseStatesOfGeometry(x),x.isInstancedBufferGeometry===!0&&delete x._maxInstanceCount,i.memory.geometries--}function d(g,x){return l[x.id]===!0||(x.addEventListener("dispose",h),l[x.id]=!0,i.memory.geometries++),x}function m(g){const x=g.attributes;for(const S in x)t.update(x[S],r.ARRAY_BUFFER)}function p(g){const x=[],S=g.index,b=g.attributes.position;let R=0;if(S!==null){const L=S.array;R=S.version;for(let U=0,T=L.length;U<T;U+=3){const P=L[U+0],B=L[U+1],F=L[U+2];x.push(P,B,B,F,F,P)}}else if(b!==void 0){const L=b.array;R=b.version;for(let U=0,T=L.length/3-1;U<T;U+=3){const P=U+0,B=U+1,F=U+2;x.push(P,B,B,F,F,P)}}else return;const M=new(qv(x)?Jv:Qv)(x,1);M.version=R;const _=c.get(g);_&&t.remove(_),c.set(g,M)}function v(g){const x=c.get(g);if(x){const S=g.index;S!==null&&x.version<S.version&&p(g)}else p(g);return c.get(g)}return{get:d,update:m,getWireframeAttribute:v}}function CA(r,t,i){let s;function l(x){s=x}let c,h;function d(x){c=x.type,h=x.bytesPerElement}function m(x,S){r.drawElements(s,S,c,x*h),i.update(S,s,1)}function p(x,S,b){b!==0&&(r.drawElementsInstanced(s,S,c,x*h,b),i.update(S,s,b))}function v(x,S,b){if(b===0)return;t.get("WEBGL_multi_draw").multiDrawElementsWEBGL(s,S,0,c,x,0,b);let M=0;for(let _=0;_<b;_++)M+=S[_];i.update(M,s,1)}function g(x,S,b,R){if(b===0)return;const M=t.get("WEBGL_multi_draw");if(M===null)for(let _=0;_<x.length;_++)p(x[_]/h,S[_],R[_]);else{M.multiDrawElementsInstancedWEBGL(s,S,0,c,x,0,R,0,b);let _=0;for(let L=0;L<b;L++)_+=S[L]*R[L];i.update(_,s,1)}}this.setMode=l,this.setIndex=d,this.render=m,this.renderInstances=p,this.renderMultiDraw=v,this.renderMultiDrawInstances=g}function wA(r){const t={geometries:0,textures:0},i={frame:0,calls:0,triangles:0,points:0,lines:0};function s(c,h,d){switch(i.calls++,h){case r.TRIANGLES:i.triangles+=d*(c/3);break;case r.LINES:i.lines+=d*(c/2);break;case r.LINE_STRIP:i.lines+=d*(c-1);break;case r.LINE_LOOP:i.lines+=d*c;break;case r.POINTS:i.points+=d*c;break;default:console.error("THREE.WebGLInfo: Unknown draw mode:",h);break}}function l(){i.calls=0,i.triangles=0,i.points=0,i.lines=0}return{memory:t,render:i,programs:null,autoReset:!0,reset:l,update:s}}function DA(r,t,i){const s=new WeakMap,l=new Ye;function c(h,d,m){const p=h.morphTargetInfluences,v=d.morphAttributes.position||d.morphAttributes.normal||d.morphAttributes.color,g=v!==void 0?v.length:0;let x=s.get(d);if(x===void 0||x.count!==g){let D=function(){F.dispose(),s.delete(d),d.removeEventListener("dispose",D)};x!==void 0&&x.texture.dispose();const S=d.morphAttributes.position!==void 0,b=d.morphAttributes.normal!==void 0,R=d.morphAttributes.color!==void 0,M=d.morphAttributes.position||[],_=d.morphAttributes.normal||[],L=d.morphAttributes.color||[];let U=0;S===!0&&(U=1),b===!0&&(U=2),R===!0&&(U=3);let T=d.attributes.position.count*U,P=1;T>t.maxTextureSize&&(P=Math.ceil(T/t.maxTextureSize),T=t.maxTextureSize);const B=new Float32Array(T*P*4*g),F=new Zv(B,T,P,g);F.type=zi,F.needsUpdate=!0;const G=U*4;for(let w=0;w<g;w++){const V=M[w],K=_[w],ot=L[w],ct=T*P*4*w;for(let ht=0;ht<V.count;ht++){const N=ht*G;S===!0&&(l.fromBufferAttribute(V,ht),B[ct+N+0]=l.x,B[ct+N+1]=l.y,B[ct+N+2]=l.z,B[ct+N+3]=0),b===!0&&(l.fromBufferAttribute(K,ht),B[ct+N+4]=l.x,B[ct+N+5]=l.y,B[ct+N+6]=l.z,B[ct+N+7]=0),R===!0&&(l.fromBufferAttribute(ot,ht),B[ct+N+8]=l.x,B[ct+N+9]=l.y,B[ct+N+10]=l.z,B[ct+N+11]=ot.itemSize===4?l.w:1)}}x={count:g,texture:F,size:new re(T,P)},s.set(d,x),d.addEventListener("dispose",D)}if(h.isInstancedMesh===!0&&h.morphTexture!==null)m.getUniforms().setValue(r,"morphTexture",h.morphTexture,i);else{let S=0;for(let R=0;R<p.length;R++)S+=p[R];const b=d.morphTargetsRelative?1:1-S;m.getUniforms().setValue(r,"morphTargetBaseInfluence",b),m.getUniforms().setValue(r,"morphTargetInfluences",p)}m.getUniforms().setValue(r,"morphTargetsTexture",x.texture,i),m.getUniforms().setValue(r,"morphTargetsTextureSize",x.size)}return{update:c}}function UA(r,t,i,s){let l=new WeakMap;function c(m){const p=s.render.frame,v=m.geometry,g=t.get(m,v);if(l.get(g)!==p&&(t.update(g),l.set(g,p)),m.isInstancedMesh&&(m.hasEventListener("dispose",d)===!1&&m.addEventListener("dispose",d),l.get(m)!==p&&(i.update(m.instanceMatrix,r.ARRAY_BUFFER),m.instanceColor!==null&&i.update(m.instanceColor,r.ARRAY_BUFFER),l.set(m,p))),m.isSkinnedMesh){const x=m.skeleton;l.get(x)!==p&&(x.update(),l.set(x,p))}return g}function h(){l=new WeakMap}function d(m){const p=m.target;p.removeEventListener("dispose",d),i.remove(p.instanceMatrix),p.instanceColor!==null&&i.remove(p.instanceColor)}return{update:c,dispose:h}}const cy=new On,ov=new sy(1,1),uy=new Zv,fy=new YM,hy=new ey,lv=[],cv=[],uv=new Float32Array(16),fv=new Float32Array(9),hv=new Float32Array(4);function Yr(r,t,i){const s=r[0];if(s<=0||s>0)return r;const l=t*i;let c=lv[l];if(c===void 0&&(c=new Float32Array(l),lv[l]=c),t!==0){s.toArray(c,0);for(let h=1,d=0;h!==t;++h)d+=i,r[h].toArray(c,d)}return c}function _n(r,t){if(r.length!==t.length)return!1;for(let i=0,s=r.length;i<s;i++)if(r[i]!==t[i])return!1;return!0}function vn(r,t){for(let i=0,s=t.length;i<s;i++)r[i]=t[i]}function pu(r,t){let i=cv[t];i===void 0&&(i=new Int32Array(t),cv[t]=i);for(let s=0;s!==t;++s)i[s]=r.allocateTextureUnit();return i}function LA(r,t){const i=this.cache;i[0]!==t&&(r.uniform1f(this.addr,t),i[0]=t)}function NA(r,t){const i=this.cache;if(t.x!==void 0)(i[0]!==t.x||i[1]!==t.y)&&(r.uniform2f(this.addr,t.x,t.y),i[0]=t.x,i[1]=t.y);else{if(_n(i,t))return;r.uniform2fv(this.addr,t),vn(i,t)}}function OA(r,t){const i=this.cache;if(t.x!==void 0)(i[0]!==t.x||i[1]!==t.y||i[2]!==t.z)&&(r.uniform3f(this.addr,t.x,t.y,t.z),i[0]=t.x,i[1]=t.y,i[2]=t.z);else if(t.r!==void 0)(i[0]!==t.r||i[1]!==t.g||i[2]!==t.b)&&(r.uniform3f(this.addr,t.r,t.g,t.b),i[0]=t.r,i[1]=t.g,i[2]=t.b);else{if(_n(i,t))return;r.uniform3fv(this.addr,t),vn(i,t)}}function PA(r,t){const i=this.cache;if(t.x!==void 0)(i[0]!==t.x||i[1]!==t.y||i[2]!==t.z||i[3]!==t.w)&&(r.uniform4f(this.addr,t.x,t.y,t.z,t.w),i[0]=t.x,i[1]=t.y,i[2]=t.z,i[3]=t.w);else{if(_n(i,t))return;r.uniform4fv(this.addr,t),vn(i,t)}}function zA(r,t){const i=this.cache,s=t.elements;if(s===void 0){if(_n(i,t))return;r.uniformMatrix2fv(this.addr,!1,t),vn(i,t)}else{if(_n(i,s))return;hv.set(s),r.uniformMatrix2fv(this.addr,!1,hv),vn(i,s)}}function BA(r,t){const i=this.cache,s=t.elements;if(s===void 0){if(_n(i,t))return;r.uniformMatrix3fv(this.addr,!1,t),vn(i,t)}else{if(_n(i,s))return;fv.set(s),r.uniformMatrix3fv(this.addr,!1,fv),vn(i,s)}}function IA(r,t){const i=this.cache,s=t.elements;if(s===void 0){if(_n(i,t))return;r.uniformMatrix4fv(this.addr,!1,t),vn(i,t)}else{if(_n(i,s))return;uv.set(s),r.uniformMatrix4fv(this.addr,!1,uv),vn(i,s)}}function FA(r,t){const i=this.cache;i[0]!==t&&(r.uniform1i(this.addr,t),i[0]=t)}function HA(r,t){const i=this.cache;if(t.x!==void 0)(i[0]!==t.x||i[1]!==t.y)&&(r.uniform2i(this.addr,t.x,t.y),i[0]=t.x,i[1]=t.y);else{if(_n(i,t))return;r.uniform2iv(this.addr,t),vn(i,t)}}function GA(r,t){const i=this.cache;if(t.x!==void 0)(i[0]!==t.x||i[1]!==t.y||i[2]!==t.z)&&(r.uniform3i(this.addr,t.x,t.y,t.z),i[0]=t.x,i[1]=t.y,i[2]=t.z);else{if(_n(i,t))return;r.uniform3iv(this.addr,t),vn(i,t)}}function VA(r,t){const i=this.cache;if(t.x!==void 0)(i[0]!==t.x||i[1]!==t.y||i[2]!==t.z||i[3]!==t.w)&&(r.uniform4i(this.addr,t.x,t.y,t.z,t.w),i[0]=t.x,i[1]=t.y,i[2]=t.z,i[3]=t.w);else{if(_n(i,t))return;r.uniform4iv(this.addr,t),vn(i,t)}}function kA(r,t){const i=this.cache;i[0]!==t&&(r.uniform1ui(this.addr,t),i[0]=t)}function XA(r,t){const i=this.cache;if(t.x!==void 0)(i[0]!==t.x||i[1]!==t.y)&&(r.uniform2ui(this.addr,t.x,t.y),i[0]=t.x,i[1]=t.y);else{if(_n(i,t))return;r.uniform2uiv(this.addr,t),vn(i,t)}}function WA(r,t){const i=this.cache;if(t.x!==void 0)(i[0]!==t.x||i[1]!==t.y||i[2]!==t.z)&&(r.uniform3ui(this.addr,t.x,t.y,t.z),i[0]=t.x,i[1]=t.y,i[2]=t.z);else{if(_n(i,t))return;r.uniform3uiv(this.addr,t),vn(i,t)}}function jA(r,t){const i=this.cache;if(t.x!==void 0)(i[0]!==t.x||i[1]!==t.y||i[2]!==t.z||i[3]!==t.w)&&(r.uniform4ui(this.addr,t.x,t.y,t.z,t.w),i[0]=t.x,i[1]=t.y,i[2]=t.z,i[3]=t.w);else{if(_n(i,t))return;r.uniform4uiv(this.addr,t),vn(i,t)}}function YA(r,t,i){const s=this.cache,l=i.allocateTextureUnit();s[0]!==l&&(r.uniform1i(this.addr,l),s[0]=l);let c;this.type===r.SAMPLER_2D_SHADOW?(ov.compareFunction=jv,c=ov):c=cy,i.setTexture2D(t||c,l)}function qA(r,t,i){const s=this.cache,l=i.allocateTextureUnit();s[0]!==l&&(r.uniform1i(this.addr,l),s[0]=l),i.setTexture3D(t||fy,l)}function ZA(r,t,i){const s=this.cache,l=i.allocateTextureUnit();s[0]!==l&&(r.uniform1i(this.addr,l),s[0]=l),i.setTextureCube(t||hy,l)}function KA(r,t,i){const s=this.cache,l=i.allocateTextureUnit();s[0]!==l&&(r.uniform1i(this.addr,l),s[0]=l),i.setTexture2DArray(t||uy,l)}function QA(r){switch(r){case 5126:return LA;case 35664:return NA;case 35665:return OA;case 35666:return PA;case 35674:return zA;case 35675:return BA;case 35676:return IA;case 5124:case 35670:return FA;case 35667:case 35671:return HA;case 35668:case 35672:return GA;case 35669:case 35673:return VA;case 5125:return kA;case 36294:return XA;case 36295:return WA;case 36296:return jA;case 35678:case 36198:case 36298:case 36306:case 35682:return YA;case 35679:case 36299:case 36307:return qA;case 35680:case 36300:case 36308:case 36293:return ZA;case 36289:case 36303:case 36311:case 36292:return KA}}function JA(r,t){r.uniform1fv(this.addr,t)}function $A(r,t){const i=Yr(t,this.size,2);r.uniform2fv(this.addr,i)}function t1(r,t){const i=Yr(t,this.size,3);r.uniform3fv(this.addr,i)}function e1(r,t){const i=Yr(t,this.size,4);r.uniform4fv(this.addr,i)}function n1(r,t){const i=Yr(t,this.size,4);r.uniformMatrix2fv(this.addr,!1,i)}function i1(r,t){const i=Yr(t,this.size,9);r.uniformMatrix3fv(this.addr,!1,i)}function a1(r,t){const i=Yr(t,this.size,16);r.uniformMatrix4fv(this.addr,!1,i)}function s1(r,t){r.uniform1iv(this.addr,t)}function r1(r,t){r.uniform2iv(this.addr,t)}function o1(r,t){r.uniform3iv(this.addr,t)}function l1(r,t){r.uniform4iv(this.addr,t)}function c1(r,t){r.uniform1uiv(this.addr,t)}function u1(r,t){r.uniform2uiv(this.addr,t)}function f1(r,t){r.uniform3uiv(this.addr,t)}function h1(r,t){r.uniform4uiv(this.addr,t)}function d1(r,t,i){const s=this.cache,l=t.length,c=pu(i,l);_n(s,c)||(r.uniform1iv(this.addr,c),vn(s,c));for(let h=0;h!==l;++h)i.setTexture2D(t[h]||cy,c[h])}function p1(r,t,i){const s=this.cache,l=t.length,c=pu(i,l);_n(s,c)||(r.uniform1iv(this.addr,c),vn(s,c));for(let h=0;h!==l;++h)i.setTexture3D(t[h]||fy,c[h])}function m1(r,t,i){const s=this.cache,l=t.length,c=pu(i,l);_n(s,c)||(r.uniform1iv(this.addr,c),vn(s,c));for(let h=0;h!==l;++h)i.setTextureCube(t[h]||hy,c[h])}function g1(r,t,i){const s=this.cache,l=t.length,c=pu(i,l);_n(s,c)||(r.uniform1iv(this.addr,c),vn(s,c));for(let h=0;h!==l;++h)i.setTexture2DArray(t[h]||uy,c[h])}function _1(r){switch(r){case 5126:return JA;case 35664:return $A;case 35665:return t1;case 35666:return e1;case 35674:return n1;case 35675:return i1;case 35676:return a1;case 5124:case 35670:return s1;case 35667:case 35671:return r1;case 35668:case 35672:return o1;case 35669:case 35673:return l1;case 5125:return c1;case 36294:return u1;case 36295:return f1;case 36296:return h1;case 35678:case 36198:case 36298:case 36306:case 35682:return d1;case 35679:case 36299:case 36307:return p1;case 35680:case 36300:case 36308:case 36293:return m1;case 36289:case 36303:case 36311:case 36292:return g1}}class v1{constructor(t,i,s){this.id=t,this.addr=s,this.cache=[],this.type=i.type,this.setValue=QA(i.type)}}class y1{constructor(t,i,s){this.id=t,this.addr=s,this.cache=[],this.type=i.type,this.size=i.size,this.setValue=_1(i.type)}}class x1{constructor(t){this.id=t,this.seq=[],this.map={}}setValue(t,i,s){const l=this.seq;for(let c=0,h=l.length;c!==h;++c){const d=l[c];d.setValue(t,i[d.id],s)}}}const od=/(\w+)(\])?(\[|\.)?/g;function dv(r,t){r.seq.push(t),r.map[t.id]=t}function S1(r,t,i){const s=r.name,l=s.length;for(od.lastIndex=0;;){const c=od.exec(s),h=od.lastIndex;let d=c[1];const m=c[2]==="]",p=c[3];if(m&&(d=d|0),p===void 0||p==="["&&h+2===l){dv(i,p===void 0?new v1(d,r,t):new y1(d,r,t));break}else{let g=i.map[d];g===void 0&&(g=new x1(d),dv(i,g)),i=g}}}class eu{constructor(t,i){this.seq=[],this.map={};const s=t.getProgramParameter(i,t.ACTIVE_UNIFORMS);for(let l=0;l<s;++l){const c=t.getActiveUniform(i,l),h=t.getUniformLocation(i,c.name);S1(c,h,this)}}setValue(t,i,s,l){const c=this.map[i];c!==void 0&&c.setValue(t,s,l)}setOptional(t,i,s){const l=i[s];l!==void 0&&this.setValue(t,s,l)}static upload(t,i,s,l){for(let c=0,h=i.length;c!==h;++c){const d=i[c],m=s[d.id];m.needsUpdate!==!1&&d.setValue(t,m.value,l)}}static seqWithValue(t,i){const s=[];for(let l=0,c=t.length;l!==c;++l){const h=t[l];h.id in i&&s.push(h)}return s}}function pv(r,t,i){const s=r.createShader(t);return r.shaderSource(s,i),r.compileShader(s),s}const M1=37297;let E1=0;function T1(r,t){const i=r.split(`
`),s=[],l=Math.max(t-6,0),c=Math.min(t+6,i.length);for(let h=l;h<c;h++){const d=h+1;s.push(`${d===t?">":" "} ${d}: ${i[h]}`)}return s.join(`
`)}const mv=new me;function b1(r){Pe._getMatrix(mv,Pe.workingColorSpace,r);const t=`mat3( ${mv.elements.map(i=>i.toFixed(4))} )`;switch(Pe.getTransfer(r)){case nu:return[t,"LinearTransferOETF"];case We:return[t,"sRGBTransferOETF"];default:return console.warn("THREE.WebGLProgram: Unsupported color space: ",r),[t,"LinearTransferOETF"]}}function gv(r,t,i){const s=r.getShaderParameter(t,r.COMPILE_STATUS),c=(r.getShaderInfoLog(t)||"").trim();if(s&&c==="")return"";const h=/ERROR: 0:(\d+)/.exec(c);if(h){const d=parseInt(h[1]);return i.toUpperCase()+`

`+c+`

`+T1(r.getShaderSource(t),d)}else return c}function A1(r,t){const i=b1(t);return[`vec4 ${r}( vec4 value ) {`,`	return ${i[1]}( vec4( value.rgb * ${i[0]}, value.a ) );`,"}"].join(`
`)}function R1(r,t){let i;switch(t){case oM:i="Linear";break;case lM:i="Reinhard";break;case cM:i="Cineon";break;case zv:i="ACESFilmic";break;case fM:i="AgX";break;case hM:i="Neutral";break;case uM:i="Custom";break;default:console.warn("THREE.WebGLProgram: Unsupported toneMapping:",t),i="Linear"}return"vec3 "+r+"( vec3 color ) { return "+i+"ToneMapping( color ); }"}const Yc=new X;function C1(){Pe.getLuminanceCoefficients(Yc);const r=Yc.x.toFixed(4),t=Yc.y.toFixed(4),i=Yc.z.toFixed(4);return["float luminance( const in vec3 rgb ) {",`	const vec3 weights = vec3( ${r}, ${t}, ${i} );`,"	return dot( weights, rgb );","}"].join(`
`)}function w1(r){return[r.extensionClipCullDistance?"#extension GL_ANGLE_clip_cull_distance : require":"",r.extensionMultiDraw?"#extension GL_ANGLE_multi_draw : require":""].filter(tl).join(`
`)}function D1(r){const t=[];for(const i in r){const s=r[i];s!==!1&&t.push("#define "+i+" "+s)}return t.join(`
`)}function U1(r,t){const i={},s=r.getProgramParameter(t,r.ACTIVE_ATTRIBUTES);for(let l=0;l<s;l++){const c=r.getActiveAttrib(t,l),h=c.name;let d=1;c.type===r.FLOAT_MAT2&&(d=2),c.type===r.FLOAT_MAT3&&(d=3),c.type===r.FLOAT_MAT4&&(d=4),i[h]={type:c.type,location:r.getAttribLocation(t,h),locationSize:d}}return i}function tl(r){return r!==""}function _v(r,t){const i=t.numSpotLightShadows+t.numSpotLightMaps-t.numSpotLightShadowsWithMaps;return r.replace(/NUM_DIR_LIGHTS/g,t.numDirLights).replace(/NUM_SPOT_LIGHTS/g,t.numSpotLights).replace(/NUM_SPOT_LIGHT_MAPS/g,t.numSpotLightMaps).replace(/NUM_SPOT_LIGHT_COORDS/g,i).replace(/NUM_RECT_AREA_LIGHTS/g,t.numRectAreaLights).replace(/NUM_POINT_LIGHTS/g,t.numPointLights).replace(/NUM_HEMI_LIGHTS/g,t.numHemiLights).replace(/NUM_DIR_LIGHT_SHADOWS/g,t.numDirLightShadows).replace(/NUM_SPOT_LIGHT_SHADOWS_WITH_MAPS/g,t.numSpotLightShadowsWithMaps).replace(/NUM_SPOT_LIGHT_SHADOWS/g,t.numSpotLightShadows).replace(/NUM_POINT_LIGHT_SHADOWS/g,t.numPointLightShadows)}function vv(r,t){return r.replace(/NUM_CLIPPING_PLANES/g,t.numClippingPlanes).replace(/UNION_CLIPPING_PLANES/g,t.numClippingPlanes-t.numClipIntersection)}const L1=/^[ \t]*#include +<([\w\d./]+)>/gm;function tp(r){return r.replace(L1,O1)}const N1=new Map;function O1(r,t){let i=ge[t];if(i===void 0){const s=N1.get(t);if(s!==void 0)i=ge[s],console.warn('THREE.WebGLRenderer: Shader chunk "%s" has been deprecated. Use "%s" instead.',t,s);else throw new Error("Can not resolve #include <"+t+">")}return tp(i)}const P1=/#pragma unroll_loop_start\s+for\s*\(\s*int\s+i\s*=\s*(\d+)\s*;\s*i\s*<\s*(\d+)\s*;\s*i\s*\+\+\s*\)\s*{([\s\S]+?)}\s+#pragma unroll_loop_end/g;function yv(r){return r.replace(P1,z1)}function z1(r,t,i,s){let l="";for(let c=parseInt(t);c<parseInt(i);c++)l+=s.replace(/\[\s*i\s*\]/g,"[ "+c+" ]").replace(/UNROLLED_LOOP_INDEX/g,c);return l}function xv(r){let t=`precision ${r.precision} float;
	precision ${r.precision} int;
	precision ${r.precision} sampler2D;
	precision ${r.precision} samplerCube;
	precision ${r.precision} sampler3D;
	precision ${r.precision} sampler2DArray;
	precision ${r.precision} sampler2DShadow;
	precision ${r.precision} samplerCubeShadow;
	precision ${r.precision} sampler2DArrayShadow;
	precision ${r.precision} isampler2D;
	precision ${r.precision} isampler3D;
	precision ${r.precision} isamplerCube;
	precision ${r.precision} isampler2DArray;
	precision ${r.precision} usampler2D;
	precision ${r.precision} usampler3D;
	precision ${r.precision} usamplerCube;
	precision ${r.precision} usampler2DArray;
	`;return r.precision==="highp"?t+=`
#define HIGH_PRECISION`:r.precision==="mediump"?t+=`
#define MEDIUM_PRECISION`:r.precision==="lowp"&&(t+=`
#define LOW_PRECISION`),t}function B1(r){let t="SHADOWMAP_TYPE_BASIC";return r.shadowMapType===Ov?t="SHADOWMAP_TYPE_PCF":r.shadowMapType===HS?t="SHADOWMAP_TYPE_PCF_SOFT":r.shadowMapType===ca&&(t="SHADOWMAP_TYPE_VSM"),t}function I1(r){let t="ENVMAP_TYPE_CUBE";if(r.envMap)switch(r.envMapMode){case Gr:case Vr:t="ENVMAP_TYPE_CUBE";break;case fu:t="ENVMAP_TYPE_CUBE_UV";break}return t}function F1(r){let t="ENVMAP_MODE_REFLECTION";if(r.envMap)switch(r.envMapMode){case Vr:t="ENVMAP_MODE_REFRACTION";break}return t}function H1(r){let t="ENVMAP_BLENDING_NONE";if(r.envMap)switch(r.combine){case Pv:t="ENVMAP_BLENDING_MULTIPLY";break;case sM:t="ENVMAP_BLENDING_MIX";break;case rM:t="ENVMAP_BLENDING_ADD";break}return t}function G1(r){const t=r.envMapCubeUVHeight;if(t===null)return null;const i=Math.log2(t)-2,s=1/t;return{texelWidth:1/(3*Math.max(Math.pow(2,i),112)),texelHeight:s,maxMip:i}}function V1(r,t,i,s){const l=r.getContext(),c=i.defines;let h=i.vertexShader,d=i.fragmentShader;const m=B1(i),p=I1(i),v=F1(i),g=H1(i),x=G1(i),S=w1(i),b=D1(c),R=l.createProgram();let M,_,L=i.glslVersion?"#version "+i.glslVersion+`
`:"";i.isRawShaderMaterial?(M=["#define SHADER_TYPE "+i.shaderType,"#define SHADER_NAME "+i.shaderName,b].filter(tl).join(`
`),M.length>0&&(M+=`
`),_=["#define SHADER_TYPE "+i.shaderType,"#define SHADER_NAME "+i.shaderName,b].filter(tl).join(`
`),_.length>0&&(_+=`
`)):(M=[xv(i),"#define SHADER_TYPE "+i.shaderType,"#define SHADER_NAME "+i.shaderName,b,i.extensionClipCullDistance?"#define USE_CLIP_DISTANCE":"",i.batching?"#define USE_BATCHING":"",i.batchingColor?"#define USE_BATCHING_COLOR":"",i.instancing?"#define USE_INSTANCING":"",i.instancingColor?"#define USE_INSTANCING_COLOR":"",i.instancingMorph?"#define USE_INSTANCING_MORPH":"",i.useFog&&i.fog?"#define USE_FOG":"",i.useFog&&i.fogExp2?"#define FOG_EXP2":"",i.map?"#define USE_MAP":"",i.envMap?"#define USE_ENVMAP":"",i.envMap?"#define "+v:"",i.lightMap?"#define USE_LIGHTMAP":"",i.aoMap?"#define USE_AOMAP":"",i.bumpMap?"#define USE_BUMPMAP":"",i.normalMap?"#define USE_NORMALMAP":"",i.normalMapObjectSpace?"#define USE_NORMALMAP_OBJECTSPACE":"",i.normalMapTangentSpace?"#define USE_NORMALMAP_TANGENTSPACE":"",i.displacementMap?"#define USE_DISPLACEMENTMAP":"",i.emissiveMap?"#define USE_EMISSIVEMAP":"",i.anisotropy?"#define USE_ANISOTROPY":"",i.anisotropyMap?"#define USE_ANISOTROPYMAP":"",i.clearcoatMap?"#define USE_CLEARCOATMAP":"",i.clearcoatRoughnessMap?"#define USE_CLEARCOAT_ROUGHNESSMAP":"",i.clearcoatNormalMap?"#define USE_CLEARCOAT_NORMALMAP":"",i.iridescenceMap?"#define USE_IRIDESCENCEMAP":"",i.iridescenceThicknessMap?"#define USE_IRIDESCENCE_THICKNESSMAP":"",i.specularMap?"#define USE_SPECULARMAP":"",i.specularColorMap?"#define USE_SPECULAR_COLORMAP":"",i.specularIntensityMap?"#define USE_SPECULAR_INTENSITYMAP":"",i.roughnessMap?"#define USE_ROUGHNESSMAP":"",i.metalnessMap?"#define USE_METALNESSMAP":"",i.alphaMap?"#define USE_ALPHAMAP":"",i.alphaHash?"#define USE_ALPHAHASH":"",i.transmission?"#define USE_TRANSMISSION":"",i.transmissionMap?"#define USE_TRANSMISSIONMAP":"",i.thicknessMap?"#define USE_THICKNESSMAP":"",i.sheenColorMap?"#define USE_SHEEN_COLORMAP":"",i.sheenRoughnessMap?"#define USE_SHEEN_ROUGHNESSMAP":"",i.mapUv?"#define MAP_UV "+i.mapUv:"",i.alphaMapUv?"#define ALPHAMAP_UV "+i.alphaMapUv:"",i.lightMapUv?"#define LIGHTMAP_UV "+i.lightMapUv:"",i.aoMapUv?"#define AOMAP_UV "+i.aoMapUv:"",i.emissiveMapUv?"#define EMISSIVEMAP_UV "+i.emissiveMapUv:"",i.bumpMapUv?"#define BUMPMAP_UV "+i.bumpMapUv:"",i.normalMapUv?"#define NORMALMAP_UV "+i.normalMapUv:"",i.displacementMapUv?"#define DISPLACEMENTMAP_UV "+i.displacementMapUv:"",i.metalnessMapUv?"#define METALNESSMAP_UV "+i.metalnessMapUv:"",i.roughnessMapUv?"#define ROUGHNESSMAP_UV "+i.roughnessMapUv:"",i.anisotropyMapUv?"#define ANISOTROPYMAP_UV "+i.anisotropyMapUv:"",i.clearcoatMapUv?"#define CLEARCOATMAP_UV "+i.clearcoatMapUv:"",i.clearcoatNormalMapUv?"#define CLEARCOAT_NORMALMAP_UV "+i.clearcoatNormalMapUv:"",i.clearcoatRoughnessMapUv?"#define CLEARCOAT_ROUGHNESSMAP_UV "+i.clearcoatRoughnessMapUv:"",i.iridescenceMapUv?"#define IRIDESCENCEMAP_UV "+i.iridescenceMapUv:"",i.iridescenceThicknessMapUv?"#define IRIDESCENCE_THICKNESSMAP_UV "+i.iridescenceThicknessMapUv:"",i.sheenColorMapUv?"#define SHEEN_COLORMAP_UV "+i.sheenColorMapUv:"",i.sheenRoughnessMapUv?"#define SHEEN_ROUGHNESSMAP_UV "+i.sheenRoughnessMapUv:"",i.specularMapUv?"#define SPECULARMAP_UV "+i.specularMapUv:"",i.specularColorMapUv?"#define SPECULAR_COLORMAP_UV "+i.specularColorMapUv:"",i.specularIntensityMapUv?"#define SPECULAR_INTENSITYMAP_UV "+i.specularIntensityMapUv:"",i.transmissionMapUv?"#define TRANSMISSIONMAP_UV "+i.transmissionMapUv:"",i.thicknessMapUv?"#define THICKNESSMAP_UV "+i.thicknessMapUv:"",i.vertexTangents&&i.flatShading===!1?"#define USE_TANGENT":"",i.vertexColors?"#define USE_COLOR":"",i.vertexAlphas?"#define USE_COLOR_ALPHA":"",i.vertexUv1s?"#define USE_UV1":"",i.vertexUv2s?"#define USE_UV2":"",i.vertexUv3s?"#define USE_UV3":"",i.pointsUvs?"#define USE_POINTS_UV":"",i.flatShading?"#define FLAT_SHADED":"",i.skinning?"#define USE_SKINNING":"",i.morphTargets?"#define USE_MORPHTARGETS":"",i.morphNormals&&i.flatShading===!1?"#define USE_MORPHNORMALS":"",i.morphColors?"#define USE_MORPHCOLORS":"",i.morphTargetsCount>0?"#define MORPHTARGETS_TEXTURE_STRIDE "+i.morphTextureStride:"",i.morphTargetsCount>0?"#define MORPHTARGETS_COUNT "+i.morphTargetsCount:"",i.doubleSided?"#define DOUBLE_SIDED":"",i.flipSided?"#define FLIP_SIDED":"",i.shadowMapEnabled?"#define USE_SHADOWMAP":"",i.shadowMapEnabled?"#define "+m:"",i.sizeAttenuation?"#define USE_SIZEATTENUATION":"",i.numLightProbes>0?"#define USE_LIGHT_PROBES":"",i.logarithmicDepthBuffer?"#define USE_LOGARITHMIC_DEPTH_BUFFER":"",i.reversedDepthBuffer?"#define USE_REVERSED_DEPTH_BUFFER":"","uniform mat4 modelMatrix;","uniform mat4 modelViewMatrix;","uniform mat4 projectionMatrix;","uniform mat4 viewMatrix;","uniform mat3 normalMatrix;","uniform vec3 cameraPosition;","uniform bool isOrthographic;","#ifdef USE_INSTANCING","	attribute mat4 instanceMatrix;","#endif","#ifdef USE_INSTANCING_COLOR","	attribute vec3 instanceColor;","#endif","#ifdef USE_INSTANCING_MORPH","	uniform sampler2D morphTexture;","#endif","attribute vec3 position;","attribute vec3 normal;","attribute vec2 uv;","#ifdef USE_UV1","	attribute vec2 uv1;","#endif","#ifdef USE_UV2","	attribute vec2 uv2;","#endif","#ifdef USE_UV3","	attribute vec2 uv3;","#endif","#ifdef USE_TANGENT","	attribute vec4 tangent;","#endif","#if defined( USE_COLOR_ALPHA )","	attribute vec4 color;","#elif defined( USE_COLOR )","	attribute vec3 color;","#endif","#ifdef USE_SKINNING","	attribute vec4 skinIndex;","	attribute vec4 skinWeight;","#endif",`
`].filter(tl).join(`
`),_=[xv(i),"#define SHADER_TYPE "+i.shaderType,"#define SHADER_NAME "+i.shaderName,b,i.useFog&&i.fog?"#define USE_FOG":"",i.useFog&&i.fogExp2?"#define FOG_EXP2":"",i.alphaToCoverage?"#define ALPHA_TO_COVERAGE":"",i.map?"#define USE_MAP":"",i.matcap?"#define USE_MATCAP":"",i.envMap?"#define USE_ENVMAP":"",i.envMap?"#define "+p:"",i.envMap?"#define "+v:"",i.envMap?"#define "+g:"",x?"#define CUBEUV_TEXEL_WIDTH "+x.texelWidth:"",x?"#define CUBEUV_TEXEL_HEIGHT "+x.texelHeight:"",x?"#define CUBEUV_MAX_MIP "+x.maxMip+".0":"",i.lightMap?"#define USE_LIGHTMAP":"",i.aoMap?"#define USE_AOMAP":"",i.bumpMap?"#define USE_BUMPMAP":"",i.normalMap?"#define USE_NORMALMAP":"",i.normalMapObjectSpace?"#define USE_NORMALMAP_OBJECTSPACE":"",i.normalMapTangentSpace?"#define USE_NORMALMAP_TANGENTSPACE":"",i.emissiveMap?"#define USE_EMISSIVEMAP":"",i.anisotropy?"#define USE_ANISOTROPY":"",i.anisotropyMap?"#define USE_ANISOTROPYMAP":"",i.clearcoat?"#define USE_CLEARCOAT":"",i.clearcoatMap?"#define USE_CLEARCOATMAP":"",i.clearcoatRoughnessMap?"#define USE_CLEARCOAT_ROUGHNESSMAP":"",i.clearcoatNormalMap?"#define USE_CLEARCOAT_NORMALMAP":"",i.dispersion?"#define USE_DISPERSION":"",i.iridescence?"#define USE_IRIDESCENCE":"",i.iridescenceMap?"#define USE_IRIDESCENCEMAP":"",i.iridescenceThicknessMap?"#define USE_IRIDESCENCE_THICKNESSMAP":"",i.specularMap?"#define USE_SPECULARMAP":"",i.specularColorMap?"#define USE_SPECULAR_COLORMAP":"",i.specularIntensityMap?"#define USE_SPECULAR_INTENSITYMAP":"",i.roughnessMap?"#define USE_ROUGHNESSMAP":"",i.metalnessMap?"#define USE_METALNESSMAP":"",i.alphaMap?"#define USE_ALPHAMAP":"",i.alphaTest?"#define USE_ALPHATEST":"",i.alphaHash?"#define USE_ALPHAHASH":"",i.sheen?"#define USE_SHEEN":"",i.sheenColorMap?"#define USE_SHEEN_COLORMAP":"",i.sheenRoughnessMap?"#define USE_SHEEN_ROUGHNESSMAP":"",i.transmission?"#define USE_TRANSMISSION":"",i.transmissionMap?"#define USE_TRANSMISSIONMAP":"",i.thicknessMap?"#define USE_THICKNESSMAP":"",i.vertexTangents&&i.flatShading===!1?"#define USE_TANGENT":"",i.vertexColors||i.instancingColor||i.batchingColor?"#define USE_COLOR":"",i.vertexAlphas?"#define USE_COLOR_ALPHA":"",i.vertexUv1s?"#define USE_UV1":"",i.vertexUv2s?"#define USE_UV2":"",i.vertexUv3s?"#define USE_UV3":"",i.pointsUvs?"#define USE_POINTS_UV":"",i.gradientMap?"#define USE_GRADIENTMAP":"",i.flatShading?"#define FLAT_SHADED":"",i.doubleSided?"#define DOUBLE_SIDED":"",i.flipSided?"#define FLIP_SIDED":"",i.shadowMapEnabled?"#define USE_SHADOWMAP":"",i.shadowMapEnabled?"#define "+m:"",i.premultipliedAlpha?"#define PREMULTIPLIED_ALPHA":"",i.numLightProbes>0?"#define USE_LIGHT_PROBES":"",i.decodeVideoTexture?"#define DECODE_VIDEO_TEXTURE":"",i.decodeVideoTextureEmissive?"#define DECODE_VIDEO_TEXTURE_EMISSIVE":"",i.logarithmicDepthBuffer?"#define USE_LOGARITHMIC_DEPTH_BUFFER":"",i.reversedDepthBuffer?"#define USE_REVERSED_DEPTH_BUFFER":"","uniform mat4 viewMatrix;","uniform vec3 cameraPosition;","uniform bool isOrthographic;",i.toneMapping!==Ya?"#define TONE_MAPPING":"",i.toneMapping!==Ya?ge.tonemapping_pars_fragment:"",i.toneMapping!==Ya?R1("toneMapping",i.toneMapping):"",i.dithering?"#define DITHERING":"",i.opaque?"#define OPAQUE":"",ge.colorspace_pars_fragment,A1("linearToOutputTexel",i.outputColorSpace),C1(),i.useDepthPacking?"#define DEPTH_PACKING "+i.depthPacking:"",`
`].filter(tl).join(`
`)),h=tp(h),h=_v(h,i),h=vv(h,i),d=tp(d),d=_v(d,i),d=vv(d,i),h=yv(h),d=yv(d),i.isRawShaderMaterial!==!0&&(L=`#version 300 es
`,M=[S,"#define attribute in","#define varying out","#define texture2D texture"].join(`
`)+`
`+M,_=["#define varying in",i.glslVersion===_0?"":"layout(location = 0) out highp vec4 pc_fragColor;",i.glslVersion===_0?"":"#define gl_FragColor pc_fragColor","#define gl_FragDepthEXT gl_FragDepth","#define texture2D texture","#define textureCube texture","#define texture2DProj textureProj","#define texture2DLodEXT textureLod","#define texture2DProjLodEXT textureProjLod","#define textureCubeLodEXT textureLod","#define texture2DGradEXT textureGrad","#define texture2DProjGradEXT textureProjGrad","#define textureCubeGradEXT textureGrad"].join(`
`)+`
`+_);const U=L+M+h,T=L+_+d,P=pv(l,l.VERTEX_SHADER,U),B=pv(l,l.FRAGMENT_SHADER,T);l.attachShader(R,P),l.attachShader(R,B),i.index0AttributeName!==void 0?l.bindAttribLocation(R,0,i.index0AttributeName):i.morphTargets===!0&&l.bindAttribLocation(R,0,"position"),l.linkProgram(R);function F(V){if(r.debug.checkShaderErrors){const K=l.getProgramInfoLog(R)||"",ot=l.getShaderInfoLog(P)||"",ct=l.getShaderInfoLog(B)||"",ht=K.trim(),N=ot.trim(),J=ct.trim();let Y=!0,Tt=!0;if(l.getProgramParameter(R,l.LINK_STATUS)===!1)if(Y=!1,typeof r.debug.onShaderError=="function")r.debug.onShaderError(l,R,P,B);else{const Ct=gv(l,P,"vertex"),z=gv(l,B,"fragment");console.error("THREE.WebGLProgram: Shader Error "+l.getError()+" - VALIDATE_STATUS "+l.getProgramParameter(R,l.VALIDATE_STATUS)+`

Material Name: `+V.name+`
Material Type: `+V.type+`

Program Info Log: `+ht+`
`+Ct+`
`+z)}else ht!==""?console.warn("THREE.WebGLProgram: Program Info Log:",ht):(N===""||J==="")&&(Tt=!1);Tt&&(V.diagnostics={runnable:Y,programLog:ht,vertexShader:{log:N,prefix:M},fragmentShader:{log:J,prefix:_}})}l.deleteShader(P),l.deleteShader(B),G=new eu(l,R),D=U1(l,R)}let G;this.getUniforms=function(){return G===void 0&&F(this),G};let D;this.getAttributes=function(){return D===void 0&&F(this),D};let w=i.rendererExtensionParallelShaderCompile===!1;return this.isReady=function(){return w===!1&&(w=l.getProgramParameter(R,M1)),w},this.destroy=function(){s.releaseStatesOfProgram(this),l.deleteProgram(R),this.program=void 0},this.type=i.shaderType,this.name=i.shaderName,this.id=E1++,this.cacheKey=t,this.usedTimes=1,this.program=R,this.vertexShader=P,this.fragmentShader=B,this}let k1=0;class X1{constructor(){this.shaderCache=new Map,this.materialCache=new Map}update(t){const i=t.vertexShader,s=t.fragmentShader,l=this._getShaderStage(i),c=this._getShaderStage(s),h=this._getShaderCacheForMaterial(t);return h.has(l)===!1&&(h.add(l),l.usedTimes++),h.has(c)===!1&&(h.add(c),c.usedTimes++),this}remove(t){const i=this.materialCache.get(t);for(const s of i)s.usedTimes--,s.usedTimes===0&&this.shaderCache.delete(s.code);return this.materialCache.delete(t),this}getVertexShaderID(t){return this._getShaderStage(t.vertexShader).id}getFragmentShaderID(t){return this._getShaderStage(t.fragmentShader).id}dispose(){this.shaderCache.clear(),this.materialCache.clear()}_getShaderCacheForMaterial(t){const i=this.materialCache;let s=i.get(t);return s===void 0&&(s=new Set,i.set(t,s)),s}_getShaderStage(t){const i=this.shaderCache;let s=i.get(t);return s===void 0&&(s=new W1(t),i.set(t,s)),s}}class W1{constructor(t){this.id=k1++,this.code=t,this.usedTimes=0}}function j1(r,t,i,s,l,c,h){const d=new hp,m=new X1,p=new Set,v=[],g=l.logarithmicDepthBuffer,x=l.vertexTextures;let S=l.precision;const b={MeshDepthMaterial:"depth",MeshDistanceMaterial:"distanceRGBA",MeshNormalMaterial:"normal",MeshBasicMaterial:"basic",MeshLambertMaterial:"lambert",MeshPhongMaterial:"phong",MeshToonMaterial:"toon",MeshStandardMaterial:"physical",MeshPhysicalMaterial:"physical",MeshMatcapMaterial:"matcap",LineBasicMaterial:"basic",LineDashedMaterial:"dashed",PointsMaterial:"points",ShadowMaterial:"shadow",SpriteMaterial:"sprite"};function R(D){return p.add(D),D===0?"uv":`uv${D}`}function M(D,w,V,K,ot){const ct=K.fog,ht=ot.geometry,N=D.isMeshStandardMaterial?K.environment:null,J=(D.isMeshStandardMaterial?i:t).get(D.envMap||N),Y=J&&J.mapping===fu?J.image.height:null,Tt=b[D.type];D.precision!==null&&(S=l.getMaxPrecision(D.precision),S!==D.precision&&console.warn("THREE.WebGLProgram.getParameters:",D.precision,"not supported, using",S,"instead."));const Ct=ht.morphAttributes.position||ht.morphAttributes.normal||ht.morphAttributes.color,z=Ct!==void 0?Ct.length:0;let at=0;ht.morphAttributes.position!==void 0&&(at=1),ht.morphAttributes.normal!==void 0&&(at=2),ht.morphAttributes.color!==void 0&&(at=3);let Mt,wt,mt,q;if(Tt){const Re=Oi[Tt];Mt=Re.vertexShader,wt=Re.fragmentShader}else Mt=D.vertexShader,wt=D.fragmentShader,m.update(D),mt=m.getVertexShaderID(D),q=m.getFragmentShaderID(D);const it=r.getRenderTarget(),xt=r.state.buffers.depth.getReversed(),Lt=ot.isInstancedMesh===!0,Bt=ot.isBatchedMesh===!0,le=!!D.map,ft=!!D.matcap,I=!!J,jt=!!D.aoMap,Kt=!!D.lightMap,Qt=!!D.bumpMap,Xt=!!D.normalMap,Ee=!!D.displacementMap,Dt=!!D.emissiveMap,Ht=!!D.metalnessMap,xe=!!D.roughnessMap,Se=D.anisotropy>0,O=D.clearcoat>0,E=D.dispersion>0,tt=D.iridescence>0,gt=D.sheen>0,At=D.transmission>0,dt=Se&&!!D.anisotropyMap,Jt=O&&!!D.clearcoatMap,Nt=O&&!!D.clearcoatNormalMap,Wt=O&&!!D.clearcoatRoughnessMap,$t=tt&&!!D.iridescenceMap,Ut=tt&&!!D.iridescenceThicknessMap,Ot=gt&&!!D.sheenColorMap,ee=gt&&!!D.sheenRoughnessMap,It=!!D.specularMap,Ft=!!D.specularColorMap,fe=!!D.specularIntensityMap,W=At&&!!D.transmissionMap,_t=At&&!!D.thicknessMap,Rt=!!D.gradientMap,Pt=!!D.alphaMap,Et=D.alphaTest>0,pt=!!D.alphaHash,zt=!!D.extensions;let ne=Ya;D.toneMapped&&(it===null||it.isXRRenderTarget===!0)&&(ne=r.toneMapping);const _e={shaderID:Tt,shaderType:D.type,shaderName:D.name,vertexShader:Mt,fragmentShader:wt,defines:D.defines,customVertexShaderID:mt,customFragmentShaderID:q,isRawShaderMaterial:D.isRawShaderMaterial===!0,glslVersion:D.glslVersion,precision:S,batching:Bt,batchingColor:Bt&&ot._colorsTexture!==null,instancing:Lt,instancingColor:Lt&&ot.instanceColor!==null,instancingMorph:Lt&&ot.morphTexture!==null,supportsVertexTextures:x,outputColorSpace:it===null?r.outputColorSpace:it.isXRRenderTarget===!0?it.texture.colorSpace:kr,alphaToCoverage:!!D.alphaToCoverage,map:le,matcap:ft,envMap:I,envMapMode:I&&J.mapping,envMapCubeUVHeight:Y,aoMap:jt,lightMap:Kt,bumpMap:Qt,normalMap:Xt,displacementMap:x&&Ee,emissiveMap:Dt,normalMapObjectSpace:Xt&&D.normalMapType===gM,normalMapTangentSpace:Xt&&D.normalMapType===Wv,metalnessMap:Ht,roughnessMap:xe,anisotropy:Se,anisotropyMap:dt,clearcoat:O,clearcoatMap:Jt,clearcoatNormalMap:Nt,clearcoatRoughnessMap:Wt,dispersion:E,iridescence:tt,iridescenceMap:$t,iridescenceThicknessMap:Ut,sheen:gt,sheenColorMap:Ot,sheenRoughnessMap:ee,specularMap:It,specularColorMap:Ft,specularIntensityMap:fe,transmission:At,transmissionMap:W,thicknessMap:_t,gradientMap:Rt,opaque:D.transparent===!1&&D.blending===Ir&&D.alphaToCoverage===!1,alphaMap:Pt,alphaTest:Et,alphaHash:pt,combine:D.combine,mapUv:le&&R(D.map.channel),aoMapUv:jt&&R(D.aoMap.channel),lightMapUv:Kt&&R(D.lightMap.channel),bumpMapUv:Qt&&R(D.bumpMap.channel),normalMapUv:Xt&&R(D.normalMap.channel),displacementMapUv:Ee&&R(D.displacementMap.channel),emissiveMapUv:Dt&&R(D.emissiveMap.channel),metalnessMapUv:Ht&&R(D.metalnessMap.channel),roughnessMapUv:xe&&R(D.roughnessMap.channel),anisotropyMapUv:dt&&R(D.anisotropyMap.channel),clearcoatMapUv:Jt&&R(D.clearcoatMap.channel),clearcoatNormalMapUv:Nt&&R(D.clearcoatNormalMap.channel),clearcoatRoughnessMapUv:Wt&&R(D.clearcoatRoughnessMap.channel),iridescenceMapUv:$t&&R(D.iridescenceMap.channel),iridescenceThicknessMapUv:Ut&&R(D.iridescenceThicknessMap.channel),sheenColorMapUv:Ot&&R(D.sheenColorMap.channel),sheenRoughnessMapUv:ee&&R(D.sheenRoughnessMap.channel),specularMapUv:It&&R(D.specularMap.channel),specularColorMapUv:Ft&&R(D.specularColorMap.channel),specularIntensityMapUv:fe&&R(D.specularIntensityMap.channel),transmissionMapUv:W&&R(D.transmissionMap.channel),thicknessMapUv:_t&&R(D.thicknessMap.channel),alphaMapUv:Pt&&R(D.alphaMap.channel),vertexTangents:!!ht.attributes.tangent&&(Xt||Se),vertexColors:D.vertexColors,vertexAlphas:D.vertexColors===!0&&!!ht.attributes.color&&ht.attributes.color.itemSize===4,pointsUvs:ot.isPoints===!0&&!!ht.attributes.uv&&(le||Pt),fog:!!ct,useFog:D.fog===!0,fogExp2:!!ct&&ct.isFogExp2,flatShading:D.flatShading===!0&&D.wireframe===!1,sizeAttenuation:D.sizeAttenuation===!0,logarithmicDepthBuffer:g,reversedDepthBuffer:xt,skinning:ot.isSkinnedMesh===!0,morphTargets:ht.morphAttributes.position!==void 0,morphNormals:ht.morphAttributes.normal!==void 0,morphColors:ht.morphAttributes.color!==void 0,morphTargetsCount:z,morphTextureStride:at,numDirLights:w.directional.length,numPointLights:w.point.length,numSpotLights:w.spot.length,numSpotLightMaps:w.spotLightMap.length,numRectAreaLights:w.rectArea.length,numHemiLights:w.hemi.length,numDirLightShadows:w.directionalShadowMap.length,numPointLightShadows:w.pointShadowMap.length,numSpotLightShadows:w.spotShadowMap.length,numSpotLightShadowsWithMaps:w.numSpotLightShadowsWithMaps,numLightProbes:w.numLightProbes,numClippingPlanes:h.numPlanes,numClipIntersection:h.numIntersection,dithering:D.dithering,shadowMapEnabled:r.shadowMap.enabled&&V.length>0,shadowMapType:r.shadowMap.type,toneMapping:ne,decodeVideoTexture:le&&D.map.isVideoTexture===!0&&Pe.getTransfer(D.map.colorSpace)===We,decodeVideoTextureEmissive:Dt&&D.emissiveMap.isVideoTexture===!0&&Pe.getTransfer(D.emissiveMap.colorSpace)===We,premultipliedAlpha:D.premultipliedAlpha,doubleSided:D.side===ua,flipSided:D.side===Yn,useDepthPacking:D.depthPacking>=0,depthPacking:D.depthPacking||0,index0AttributeName:D.index0AttributeName,extensionClipCullDistance:zt&&D.extensions.clipCullDistance===!0&&s.has("WEBGL_clip_cull_distance"),extensionMultiDraw:(zt&&D.extensions.multiDraw===!0||Bt)&&s.has("WEBGL_multi_draw"),rendererExtensionParallelShaderCompile:s.has("KHR_parallel_shader_compile"),customProgramCacheKey:D.customProgramCacheKey()};return _e.vertexUv1s=p.has(1),_e.vertexUv2s=p.has(2),_e.vertexUv3s=p.has(3),p.clear(),_e}function _(D){const w=[];if(D.shaderID?w.push(D.shaderID):(w.push(D.customVertexShaderID),w.push(D.customFragmentShaderID)),D.defines!==void 0)for(const V in D.defines)w.push(V),w.push(D.defines[V]);return D.isRawShaderMaterial===!1&&(L(w,D),U(w,D),w.push(r.outputColorSpace)),w.push(D.customProgramCacheKey),w.join()}function L(D,w){D.push(w.precision),D.push(w.outputColorSpace),D.push(w.envMapMode),D.push(w.envMapCubeUVHeight),D.push(w.mapUv),D.push(w.alphaMapUv),D.push(w.lightMapUv),D.push(w.aoMapUv),D.push(w.bumpMapUv),D.push(w.normalMapUv),D.push(w.displacementMapUv),D.push(w.emissiveMapUv),D.push(w.metalnessMapUv),D.push(w.roughnessMapUv),D.push(w.anisotropyMapUv),D.push(w.clearcoatMapUv),D.push(w.clearcoatNormalMapUv),D.push(w.clearcoatRoughnessMapUv),D.push(w.iridescenceMapUv),D.push(w.iridescenceThicknessMapUv),D.push(w.sheenColorMapUv),D.push(w.sheenRoughnessMapUv),D.push(w.specularMapUv),D.push(w.specularColorMapUv),D.push(w.specularIntensityMapUv),D.push(w.transmissionMapUv),D.push(w.thicknessMapUv),D.push(w.combine),D.push(w.fogExp2),D.push(w.sizeAttenuation),D.push(w.morphTargetsCount),D.push(w.morphAttributeCount),D.push(w.numDirLights),D.push(w.numPointLights),D.push(w.numSpotLights),D.push(w.numSpotLightMaps),D.push(w.numHemiLights),D.push(w.numRectAreaLights),D.push(w.numDirLightShadows),D.push(w.numPointLightShadows),D.push(w.numSpotLightShadows),D.push(w.numSpotLightShadowsWithMaps),D.push(w.numLightProbes),D.push(w.shadowMapType),D.push(w.toneMapping),D.push(w.numClippingPlanes),D.push(w.numClipIntersection),D.push(w.depthPacking)}function U(D,w){d.disableAll(),w.supportsVertexTextures&&d.enable(0),w.instancing&&d.enable(1),w.instancingColor&&d.enable(2),w.instancingMorph&&d.enable(3),w.matcap&&d.enable(4),w.envMap&&d.enable(5),w.normalMapObjectSpace&&d.enable(6),w.normalMapTangentSpace&&d.enable(7),w.clearcoat&&d.enable(8),w.iridescence&&d.enable(9),w.alphaTest&&d.enable(10),w.vertexColors&&d.enable(11),w.vertexAlphas&&d.enable(12),w.vertexUv1s&&d.enable(13),w.vertexUv2s&&d.enable(14),w.vertexUv3s&&d.enable(15),w.vertexTangents&&d.enable(16),w.anisotropy&&d.enable(17),w.alphaHash&&d.enable(18),w.batching&&d.enable(19),w.dispersion&&d.enable(20),w.batchingColor&&d.enable(21),w.gradientMap&&d.enable(22),D.push(d.mask),d.disableAll(),w.fog&&d.enable(0),w.useFog&&d.enable(1),w.flatShading&&d.enable(2),w.logarithmicDepthBuffer&&d.enable(3),w.reversedDepthBuffer&&d.enable(4),w.skinning&&d.enable(5),w.morphTargets&&d.enable(6),w.morphNormals&&d.enable(7),w.morphColors&&d.enable(8),w.premultipliedAlpha&&d.enable(9),w.shadowMapEnabled&&d.enable(10),w.doubleSided&&d.enable(11),w.flipSided&&d.enable(12),w.useDepthPacking&&d.enable(13),w.dithering&&d.enable(14),w.transmission&&d.enable(15),w.sheen&&d.enable(16),w.opaque&&d.enable(17),w.pointsUvs&&d.enable(18),w.decodeVideoTexture&&d.enable(19),w.decodeVideoTextureEmissive&&d.enable(20),w.alphaToCoverage&&d.enable(21),D.push(d.mask)}function T(D){const w=b[D.type];let V;if(w){const K=Oi[w];V=rE.clone(K.uniforms)}else V=D.uniforms;return V}function P(D,w){let V;for(let K=0,ot=v.length;K<ot;K++){const ct=v[K];if(ct.cacheKey===w){V=ct,++V.usedTimes;break}}return V===void 0&&(V=new V1(r,w,D,c),v.push(V)),V}function B(D){if(--D.usedTimes===0){const w=v.indexOf(D);v[w]=v[v.length-1],v.pop(),D.destroy()}}function F(D){m.remove(D)}function G(){m.dispose()}return{getParameters:M,getProgramCacheKey:_,getUniforms:T,acquireProgram:P,releaseProgram:B,releaseShaderCache:F,programs:v,dispose:G}}function Y1(){let r=new WeakMap;function t(h){return r.has(h)}function i(h){let d=r.get(h);return d===void 0&&(d={},r.set(h,d)),d}function s(h){r.delete(h)}function l(h,d,m){r.get(h)[d]=m}function c(){r=new WeakMap}return{has:t,get:i,remove:s,update:l,dispose:c}}function q1(r,t){return r.groupOrder!==t.groupOrder?r.groupOrder-t.groupOrder:r.renderOrder!==t.renderOrder?r.renderOrder-t.renderOrder:r.material.id!==t.material.id?r.material.id-t.material.id:r.z!==t.z?r.z-t.z:r.id-t.id}function Sv(r,t){return r.groupOrder!==t.groupOrder?r.groupOrder-t.groupOrder:r.renderOrder!==t.renderOrder?r.renderOrder-t.renderOrder:r.z!==t.z?t.z-r.z:r.id-t.id}function Mv(){const r=[];let t=0;const i=[],s=[],l=[];function c(){t=0,i.length=0,s.length=0,l.length=0}function h(g,x,S,b,R,M){let _=r[t];return _===void 0?(_={id:g.id,object:g,geometry:x,material:S,groupOrder:b,renderOrder:g.renderOrder,z:R,group:M},r[t]=_):(_.id=g.id,_.object=g,_.geometry=x,_.material=S,_.groupOrder=b,_.renderOrder=g.renderOrder,_.z=R,_.group=M),t++,_}function d(g,x,S,b,R,M){const _=h(g,x,S,b,R,M);S.transmission>0?s.push(_):S.transparent===!0?l.push(_):i.push(_)}function m(g,x,S,b,R,M){const _=h(g,x,S,b,R,M);S.transmission>0?s.unshift(_):S.transparent===!0?l.unshift(_):i.unshift(_)}function p(g,x){i.length>1&&i.sort(g||q1),s.length>1&&s.sort(x||Sv),l.length>1&&l.sort(x||Sv)}function v(){for(let g=t,x=r.length;g<x;g++){const S=r[g];if(S.id===null)break;S.id=null,S.object=null,S.geometry=null,S.material=null,S.group=null}}return{opaque:i,transmissive:s,transparent:l,init:c,push:d,unshift:m,finish:v,sort:p}}function Z1(){let r=new WeakMap;function t(s,l){const c=r.get(s);let h;return c===void 0?(h=new Mv,r.set(s,[h])):l>=c.length?(h=new Mv,c.push(h)):h=c[l],h}function i(){r=new WeakMap}return{get:t,dispose:i}}function K1(){const r={};return{get:function(t){if(r[t.id]!==void 0)return r[t.id];let i;switch(t.type){case"DirectionalLight":i={direction:new X,color:new Me};break;case"SpotLight":i={position:new X,direction:new X,color:new Me,distance:0,coneCos:0,penumbraCos:0,decay:0};break;case"PointLight":i={position:new X,color:new Me,distance:0,decay:0};break;case"HemisphereLight":i={direction:new X,skyColor:new Me,groundColor:new Me};break;case"RectAreaLight":i={color:new Me,position:new X,halfWidth:new X,halfHeight:new X};break}return r[t.id]=i,i}}}function Q1(){const r={};return{get:function(t){if(r[t.id]!==void 0)return r[t.id];let i;switch(t.type){case"DirectionalLight":i={shadowIntensity:1,shadowBias:0,shadowNormalBias:0,shadowRadius:1,shadowMapSize:new re};break;case"SpotLight":i={shadowIntensity:1,shadowBias:0,shadowNormalBias:0,shadowRadius:1,shadowMapSize:new re};break;case"PointLight":i={shadowIntensity:1,shadowBias:0,shadowNormalBias:0,shadowRadius:1,shadowMapSize:new re,shadowCameraNear:1,shadowCameraFar:1e3};break}return r[t.id]=i,i}}}let J1=0;function $1(r,t){return(t.castShadow?2:0)-(r.castShadow?2:0)+(t.map?1:0)-(r.map?1:0)}function tR(r){const t=new K1,i=Q1(),s={version:0,hash:{directionalLength:-1,pointLength:-1,spotLength:-1,rectAreaLength:-1,hemiLength:-1,numDirectionalShadows:-1,numPointShadows:-1,numSpotShadows:-1,numSpotMaps:-1,numLightProbes:-1},ambient:[0,0,0],probe:[],directional:[],directionalShadow:[],directionalShadowMap:[],directionalShadowMatrix:[],spot:[],spotLightMap:[],spotShadow:[],spotShadowMap:[],spotLightMatrix:[],rectArea:[],rectAreaLTC1:null,rectAreaLTC2:null,point:[],pointShadow:[],pointShadowMap:[],pointShadowMatrix:[],hemi:[],numSpotLightShadowsWithMaps:0,numLightProbes:0};for(let p=0;p<9;p++)s.probe.push(new X);const l=new X,c=new qe,h=new qe;function d(p){let v=0,g=0,x=0;for(let D=0;D<9;D++)s.probe[D].set(0,0,0);let S=0,b=0,R=0,M=0,_=0,L=0,U=0,T=0,P=0,B=0,F=0;p.sort($1);for(let D=0,w=p.length;D<w;D++){const V=p[D],K=V.color,ot=V.intensity,ct=V.distance,ht=V.shadow&&V.shadow.map?V.shadow.map.texture:null;if(V.isAmbientLight)v+=K.r*ot,g+=K.g*ot,x+=K.b*ot;else if(V.isLightProbe){for(let N=0;N<9;N++)s.probe[N].addScaledVector(V.sh.coefficients[N],ot);F++}else if(V.isDirectionalLight){const N=t.get(V);if(N.color.copy(V.color).multiplyScalar(V.intensity),V.castShadow){const J=V.shadow,Y=i.get(V);Y.shadowIntensity=J.intensity,Y.shadowBias=J.bias,Y.shadowNormalBias=J.normalBias,Y.shadowRadius=J.radius,Y.shadowMapSize=J.mapSize,s.directionalShadow[S]=Y,s.directionalShadowMap[S]=ht,s.directionalShadowMatrix[S]=V.shadow.matrix,L++}s.directional[S]=N,S++}else if(V.isSpotLight){const N=t.get(V);N.position.setFromMatrixPosition(V.matrixWorld),N.color.copy(K).multiplyScalar(ot),N.distance=ct,N.coneCos=Math.cos(V.angle),N.penumbraCos=Math.cos(V.angle*(1-V.penumbra)),N.decay=V.decay,s.spot[R]=N;const J=V.shadow;if(V.map&&(s.spotLightMap[P]=V.map,P++,J.updateMatrices(V),V.castShadow&&B++),s.spotLightMatrix[R]=J.matrix,V.castShadow){const Y=i.get(V);Y.shadowIntensity=J.intensity,Y.shadowBias=J.bias,Y.shadowNormalBias=J.normalBias,Y.shadowRadius=J.radius,Y.shadowMapSize=J.mapSize,s.spotShadow[R]=Y,s.spotShadowMap[R]=ht,T++}R++}else if(V.isRectAreaLight){const N=t.get(V);N.color.copy(K).multiplyScalar(ot),N.halfWidth.set(V.width*.5,0,0),N.halfHeight.set(0,V.height*.5,0),s.rectArea[M]=N,M++}else if(V.isPointLight){const N=t.get(V);if(N.color.copy(V.color).multiplyScalar(V.intensity),N.distance=V.distance,N.decay=V.decay,V.castShadow){const J=V.shadow,Y=i.get(V);Y.shadowIntensity=J.intensity,Y.shadowBias=J.bias,Y.shadowNormalBias=J.normalBias,Y.shadowRadius=J.radius,Y.shadowMapSize=J.mapSize,Y.shadowCameraNear=J.camera.near,Y.shadowCameraFar=J.camera.far,s.pointShadow[b]=Y,s.pointShadowMap[b]=ht,s.pointShadowMatrix[b]=V.shadow.matrix,U++}s.point[b]=N,b++}else if(V.isHemisphereLight){const N=t.get(V);N.skyColor.copy(V.color).multiplyScalar(ot),N.groundColor.copy(V.groundColor).multiplyScalar(ot),s.hemi[_]=N,_++}}M>0&&(r.has("OES_texture_float_linear")===!0?(s.rectAreaLTC1=Vt.LTC_FLOAT_1,s.rectAreaLTC2=Vt.LTC_FLOAT_2):(s.rectAreaLTC1=Vt.LTC_HALF_1,s.rectAreaLTC2=Vt.LTC_HALF_2)),s.ambient[0]=v,s.ambient[1]=g,s.ambient[2]=x;const G=s.hash;(G.directionalLength!==S||G.pointLength!==b||G.spotLength!==R||G.rectAreaLength!==M||G.hemiLength!==_||G.numDirectionalShadows!==L||G.numPointShadows!==U||G.numSpotShadows!==T||G.numSpotMaps!==P||G.numLightProbes!==F)&&(s.directional.length=S,s.spot.length=R,s.rectArea.length=M,s.point.length=b,s.hemi.length=_,s.directionalShadow.length=L,s.directionalShadowMap.length=L,s.pointShadow.length=U,s.pointShadowMap.length=U,s.spotShadow.length=T,s.spotShadowMap.length=T,s.directionalShadowMatrix.length=L,s.pointShadowMatrix.length=U,s.spotLightMatrix.length=T+P-B,s.spotLightMap.length=P,s.numSpotLightShadowsWithMaps=B,s.numLightProbes=F,G.directionalLength=S,G.pointLength=b,G.spotLength=R,G.rectAreaLength=M,G.hemiLength=_,G.numDirectionalShadows=L,G.numPointShadows=U,G.numSpotShadows=T,G.numSpotMaps=P,G.numLightProbes=F,s.version=J1++)}function m(p,v){let g=0,x=0,S=0,b=0,R=0;const M=v.matrixWorldInverse;for(let _=0,L=p.length;_<L;_++){const U=p[_];if(U.isDirectionalLight){const T=s.directional[g];T.direction.setFromMatrixPosition(U.matrixWorld),l.setFromMatrixPosition(U.target.matrixWorld),T.direction.sub(l),T.direction.transformDirection(M),g++}else if(U.isSpotLight){const T=s.spot[S];T.position.setFromMatrixPosition(U.matrixWorld),T.position.applyMatrix4(M),T.direction.setFromMatrixPosition(U.matrixWorld),l.setFromMatrixPosition(U.target.matrixWorld),T.direction.sub(l),T.direction.transformDirection(M),S++}else if(U.isRectAreaLight){const T=s.rectArea[b];T.position.setFromMatrixPosition(U.matrixWorld),T.position.applyMatrix4(M),h.identity(),c.copy(U.matrixWorld),c.premultiply(M),h.extractRotation(c),T.halfWidth.set(U.width*.5,0,0),T.halfHeight.set(0,U.height*.5,0),T.halfWidth.applyMatrix4(h),T.halfHeight.applyMatrix4(h),b++}else if(U.isPointLight){const T=s.point[x];T.position.setFromMatrixPosition(U.matrixWorld),T.position.applyMatrix4(M),x++}else if(U.isHemisphereLight){const T=s.hemi[R];T.direction.setFromMatrixPosition(U.matrixWorld),T.direction.transformDirection(M),R++}}}return{setup:d,setupView:m,state:s}}function Ev(r){const t=new tR(r),i=[],s=[];function l(v){p.camera=v,i.length=0,s.length=0}function c(v){i.push(v)}function h(v){s.push(v)}function d(){t.setup(i)}function m(v){t.setupView(i,v)}const p={lightsArray:i,shadowsArray:s,camera:null,lights:t,transmissionRenderTarget:{}};return{init:l,state:p,setupLights:d,setupLightsView:m,pushLight:c,pushShadow:h}}function eR(r){let t=new WeakMap;function i(l,c=0){const h=t.get(l);let d;return h===void 0?(d=new Ev(r),t.set(l,[d])):c>=h.length?(d=new Ev(r),h.push(d)):d=h[c],d}function s(){t=new WeakMap}return{get:i,dispose:s}}const nR=`void main() {
	gl_Position = vec4( position, 1.0 );
}`,iR=`uniform sampler2D shadow_pass;
uniform vec2 resolution;
uniform float radius;
#include <packing>
void main() {
	const float samples = float( VSM_SAMPLES );
	float mean = 0.0;
	float squared_mean = 0.0;
	float uvStride = samples <= 1.0 ? 0.0 : 2.0 / ( samples - 1.0 );
	float uvStart = samples <= 1.0 ? 0.0 : - 1.0;
	for ( float i = 0.0; i < samples; i ++ ) {
		float uvOffset = uvStart + i * uvStride;
		#ifdef HORIZONTAL_PASS
			vec2 distribution = unpackRGBATo2Half( texture2D( shadow_pass, ( gl_FragCoord.xy + vec2( uvOffset, 0.0 ) * radius ) / resolution ) );
			mean += distribution.x;
			squared_mean += distribution.y * distribution.y + distribution.x * distribution.x;
		#else
			float depth = unpackRGBAToDepth( texture2D( shadow_pass, ( gl_FragCoord.xy + vec2( 0.0, uvOffset ) * radius ) / resolution ) );
			mean += depth;
			squared_mean += depth * depth;
		#endif
	}
	mean = mean / samples;
	squared_mean = squared_mean / samples;
	float std_dev = sqrt( squared_mean - mean * mean );
	gl_FragColor = pack2HalfToRGBA( vec2( mean, std_dev ) );
}`;function aR(r,t,i){let s=new dp;const l=new re,c=new re,h=new Ye,d=new bE({depthPacking:mM}),m=new AE,p={},v=i.maxTextureSize,g={[qa]:Yn,[Yn]:qa,[ua]:ua},x=new Za({defines:{VSM_SAMPLES:8},uniforms:{shadow_pass:{value:null},resolution:{value:new re},radius:{value:4}},vertexShader:nR,fragmentShader:iR}),S=x.clone();S.defines.HORIZONTAL_PASS=1;const b=new ri;b.setAttribute("position",new yi(new Float32Array([-1,-1,.5,3,-1,.5,-1,3,.5]),3));const R=new vi(b,x),M=this;this.enabled=!1,this.autoUpdate=!0,this.needsUpdate=!1,this.type=Ov;let _=this.type;this.render=function(B,F,G){if(M.enabled===!1||M.autoUpdate===!1&&M.needsUpdate===!1||B.length===0)return;const D=r.getRenderTarget(),w=r.getActiveCubeFace(),V=r.getActiveMipmapLevel(),K=r.state;K.setBlending(ja),K.buffers.depth.getReversed()===!0?K.buffers.color.setClear(0,0,0,0):K.buffers.color.setClear(1,1,1,1),K.buffers.depth.setTest(!0),K.setScissorTest(!1);const ot=_!==ca&&this.type===ca,ct=_===ca&&this.type!==ca;for(let ht=0,N=B.length;ht<N;ht++){const J=B[ht],Y=J.shadow;if(Y===void 0){console.warn("THREE.WebGLShadowMap:",J,"has no shadow.");continue}if(Y.autoUpdate===!1&&Y.needsUpdate===!1)continue;l.copy(Y.mapSize);const Tt=Y.getFrameExtents();if(l.multiply(Tt),c.copy(Y.mapSize),(l.x>v||l.y>v)&&(l.x>v&&(c.x=Math.floor(v/Tt.x),l.x=c.x*Tt.x,Y.mapSize.x=c.x),l.y>v&&(c.y=Math.floor(v/Tt.y),l.y=c.y*Tt.y,Y.mapSize.y=c.y)),Y.map===null||ot===!0||ct===!0){const z=this.type!==ca?{minFilter:si,magFilter:si}:{};Y.map!==null&&Y.map.dispose(),Y.map=new Ds(l.x,l.y,z),Y.map.texture.name=J.name+".shadowMap",Y.camera.updateProjectionMatrix()}r.setRenderTarget(Y.map),r.clear();const Ct=Y.getViewportCount();for(let z=0;z<Ct;z++){const at=Y.getViewport(z);h.set(c.x*at.x,c.y*at.y,c.x*at.z,c.y*at.w),K.viewport(h),Y.updateMatrices(J,z),s=Y.getFrustum(),T(F,G,Y.camera,J,this.type)}Y.isPointLightShadow!==!0&&this.type===ca&&L(Y,G),Y.needsUpdate=!1}_=this.type,M.needsUpdate=!1,r.setRenderTarget(D,w,V)};function L(B,F){const G=t.update(R);x.defines.VSM_SAMPLES!==B.blurSamples&&(x.defines.VSM_SAMPLES=B.blurSamples,S.defines.VSM_SAMPLES=B.blurSamples,x.needsUpdate=!0,S.needsUpdate=!0),B.mapPass===null&&(B.mapPass=new Ds(l.x,l.y)),x.uniforms.shadow_pass.value=B.map.texture,x.uniforms.resolution.value=B.mapSize,x.uniforms.radius.value=B.radius,r.setRenderTarget(B.mapPass),r.clear(),r.renderBufferDirect(F,null,G,x,R,null),S.uniforms.shadow_pass.value=B.mapPass.texture,S.uniforms.resolution.value=B.mapSize,S.uniforms.radius.value=B.radius,r.setRenderTarget(B.map),r.clear(),r.renderBufferDirect(F,null,G,S,R,null)}function U(B,F,G,D){let w=null;const V=G.isPointLight===!0?B.customDistanceMaterial:B.customDepthMaterial;if(V!==void 0)w=V;else if(w=G.isPointLight===!0?m:d,r.localClippingEnabled&&F.clipShadows===!0&&Array.isArray(F.clippingPlanes)&&F.clippingPlanes.length!==0||F.displacementMap&&F.displacementScale!==0||F.alphaMap&&F.alphaTest>0||F.map&&F.alphaTest>0||F.alphaToCoverage===!0){const K=w.uuid,ot=F.uuid;let ct=p[K];ct===void 0&&(ct={},p[K]=ct);let ht=ct[ot];ht===void 0&&(ht=w.clone(),ct[ot]=ht,F.addEventListener("dispose",P)),w=ht}if(w.visible=F.visible,w.wireframe=F.wireframe,D===ca?w.side=F.shadowSide!==null?F.shadowSide:F.side:w.side=F.shadowSide!==null?F.shadowSide:g[F.side],w.alphaMap=F.alphaMap,w.alphaTest=F.alphaToCoverage===!0?.5:F.alphaTest,w.map=F.map,w.clipShadows=F.clipShadows,w.clippingPlanes=F.clippingPlanes,w.clipIntersection=F.clipIntersection,w.displacementMap=F.displacementMap,w.displacementScale=F.displacementScale,w.displacementBias=F.displacementBias,w.wireframeLinewidth=F.wireframeLinewidth,w.linewidth=F.linewidth,G.isPointLight===!0&&w.isMeshDistanceMaterial===!0){const K=r.properties.get(w);K.light=G}return w}function T(B,F,G,D,w){if(B.visible===!1)return;if(B.layers.test(F.layers)&&(B.isMesh||B.isLine||B.isPoints)&&(B.castShadow||B.receiveShadow&&w===ca)&&(!B.frustumCulled||s.intersectsObject(B))){B.modelViewMatrix.multiplyMatrices(G.matrixWorldInverse,B.matrixWorld);const ot=t.update(B),ct=B.material;if(Array.isArray(ct)){const ht=ot.groups;for(let N=0,J=ht.length;N<J;N++){const Y=ht[N],Tt=ct[Y.materialIndex];if(Tt&&Tt.visible){const Ct=U(B,Tt,D,w);B.onBeforeShadow(r,B,F,G,ot,Ct,Y),r.renderBufferDirect(G,null,ot,Ct,B,Y),B.onAfterShadow(r,B,F,G,ot,Ct,Y)}}}else if(ct.visible){const ht=U(B,ct,D,w);B.onBeforeShadow(r,B,F,G,ot,ht,null),r.renderBufferDirect(G,null,ot,ht,B,null),B.onAfterShadow(r,B,F,G,ot,ht,null)}}const K=B.children;for(let ot=0,ct=K.length;ot<ct;ot++)T(K[ot],F,G,D,w)}function P(B){B.target.removeEventListener("dispose",P);for(const G in p){const D=p[G],w=B.target.uuid;w in D&&(D[w].dispose(),delete D[w])}}}const sR={[dd]:pd,[md]:vd,[gd]:yd,[Hr]:_d,[pd]:dd,[vd]:md,[yd]:gd,[_d]:Hr};function rR(r,t){function i(){let W=!1;const _t=new Ye;let Rt=null;const Pt=new Ye(0,0,0,0);return{setMask:function(Et){Rt!==Et&&!W&&(r.colorMask(Et,Et,Et,Et),Rt=Et)},setLocked:function(Et){W=Et},setClear:function(Et,pt,zt,ne,_e){_e===!0&&(Et*=ne,pt*=ne,zt*=ne),_t.set(Et,pt,zt,ne),Pt.equals(_t)===!1&&(r.clearColor(Et,pt,zt,ne),Pt.copy(_t))},reset:function(){W=!1,Rt=null,Pt.set(-1,0,0,0)}}}function s(){let W=!1,_t=!1,Rt=null,Pt=null,Et=null;return{setReversed:function(pt){if(_t!==pt){const zt=t.get("EXT_clip_control");pt?zt.clipControlEXT(zt.LOWER_LEFT_EXT,zt.ZERO_TO_ONE_EXT):zt.clipControlEXT(zt.LOWER_LEFT_EXT,zt.NEGATIVE_ONE_TO_ONE_EXT),_t=pt;const ne=Et;Et=null,this.setClear(ne)}},getReversed:function(){return _t},setTest:function(pt){pt?it(r.DEPTH_TEST):xt(r.DEPTH_TEST)},setMask:function(pt){Rt!==pt&&!W&&(r.depthMask(pt),Rt=pt)},setFunc:function(pt){if(_t&&(pt=sR[pt]),Pt!==pt){switch(pt){case dd:r.depthFunc(r.NEVER);break;case pd:r.depthFunc(r.ALWAYS);break;case md:r.depthFunc(r.LESS);break;case Hr:r.depthFunc(r.LEQUAL);break;case gd:r.depthFunc(r.EQUAL);break;case _d:r.depthFunc(r.GEQUAL);break;case vd:r.depthFunc(r.GREATER);break;case yd:r.depthFunc(r.NOTEQUAL);break;default:r.depthFunc(r.LEQUAL)}Pt=pt}},setLocked:function(pt){W=pt},setClear:function(pt){Et!==pt&&(_t&&(pt=1-pt),r.clearDepth(pt),Et=pt)},reset:function(){W=!1,Rt=null,Pt=null,Et=null,_t=!1}}}function l(){let W=!1,_t=null,Rt=null,Pt=null,Et=null,pt=null,zt=null,ne=null,_e=null;return{setTest:function(Re){W||(Re?it(r.STENCIL_TEST):xt(r.STENCIL_TEST))},setMask:function(Re){_t!==Re&&!W&&(r.stencilMask(Re),_t=Re)},setFunc:function(Re,En,oi){(Rt!==Re||Pt!==En||Et!==oi)&&(r.stencilFunc(Re,En,oi),Rt=Re,Pt=En,Et=oi)},setOp:function(Re,En,oi){(pt!==Re||zt!==En||ne!==oi)&&(r.stencilOp(Re,En,oi),pt=Re,zt=En,ne=oi)},setLocked:function(Re){W=Re},setClear:function(Re){_e!==Re&&(r.clearStencil(Re),_e=Re)},reset:function(){W=!1,_t=null,Rt=null,Pt=null,Et=null,pt=null,zt=null,ne=null,_e=null}}}const c=new i,h=new s,d=new l,m=new WeakMap,p=new WeakMap;let v={},g={},x=new WeakMap,S=[],b=null,R=!1,M=null,_=null,L=null,U=null,T=null,P=null,B=null,F=new Me(0,0,0),G=0,D=!1,w=null,V=null,K=null,ot=null,ct=null;const ht=r.getParameter(r.MAX_COMBINED_TEXTURE_IMAGE_UNITS);let N=!1,J=0;const Y=r.getParameter(r.VERSION);Y.indexOf("WebGL")!==-1?(J=parseFloat(/^WebGL (\d)/.exec(Y)[1]),N=J>=1):Y.indexOf("OpenGL ES")!==-1&&(J=parseFloat(/^OpenGL ES (\d)/.exec(Y)[1]),N=J>=2);let Tt=null,Ct={};const z=r.getParameter(r.SCISSOR_BOX),at=r.getParameter(r.VIEWPORT),Mt=new Ye().fromArray(z),wt=new Ye().fromArray(at);function mt(W,_t,Rt,Pt){const Et=new Uint8Array(4),pt=r.createTexture();r.bindTexture(W,pt),r.texParameteri(W,r.TEXTURE_MIN_FILTER,r.NEAREST),r.texParameteri(W,r.TEXTURE_MAG_FILTER,r.NEAREST);for(let zt=0;zt<Rt;zt++)W===r.TEXTURE_3D||W===r.TEXTURE_2D_ARRAY?r.texImage3D(_t,0,r.RGBA,1,1,Pt,0,r.RGBA,r.UNSIGNED_BYTE,Et):r.texImage2D(_t+zt,0,r.RGBA,1,1,0,r.RGBA,r.UNSIGNED_BYTE,Et);return pt}const q={};q[r.TEXTURE_2D]=mt(r.TEXTURE_2D,r.TEXTURE_2D,1),q[r.TEXTURE_CUBE_MAP]=mt(r.TEXTURE_CUBE_MAP,r.TEXTURE_CUBE_MAP_POSITIVE_X,6),q[r.TEXTURE_2D_ARRAY]=mt(r.TEXTURE_2D_ARRAY,r.TEXTURE_2D_ARRAY,1,1),q[r.TEXTURE_3D]=mt(r.TEXTURE_3D,r.TEXTURE_3D,1,1),c.setClear(0,0,0,1),h.setClear(1),d.setClear(0),it(r.DEPTH_TEST),h.setFunc(Hr),Qt(!1),Xt(h0),it(r.CULL_FACE),jt(ja);function it(W){v[W]!==!0&&(r.enable(W),v[W]=!0)}function xt(W){v[W]!==!1&&(r.disable(W),v[W]=!1)}function Lt(W,_t){return g[W]!==_t?(r.bindFramebuffer(W,_t),g[W]=_t,W===r.DRAW_FRAMEBUFFER&&(g[r.FRAMEBUFFER]=_t),W===r.FRAMEBUFFER&&(g[r.DRAW_FRAMEBUFFER]=_t),!0):!1}function Bt(W,_t){let Rt=S,Pt=!1;if(W){Rt=x.get(_t),Rt===void 0&&(Rt=[],x.set(_t,Rt));const Et=W.textures;if(Rt.length!==Et.length||Rt[0]!==r.COLOR_ATTACHMENT0){for(let pt=0,zt=Et.length;pt<zt;pt++)Rt[pt]=r.COLOR_ATTACHMENT0+pt;Rt.length=Et.length,Pt=!0}}else Rt[0]!==r.BACK&&(Rt[0]=r.BACK,Pt=!0);Pt&&r.drawBuffers(Rt)}function le(W){return b!==W?(r.useProgram(W),b=W,!0):!1}const ft={[Ts]:r.FUNC_ADD,[VS]:r.FUNC_SUBTRACT,[kS]:r.FUNC_REVERSE_SUBTRACT};ft[XS]=r.MIN,ft[WS]=r.MAX;const I={[jS]:r.ZERO,[YS]:r.ONE,[qS]:r.SRC_COLOR,[fd]:r.SRC_ALPHA,[tM]:r.SRC_ALPHA_SATURATE,[JS]:r.DST_COLOR,[KS]:r.DST_ALPHA,[ZS]:r.ONE_MINUS_SRC_COLOR,[hd]:r.ONE_MINUS_SRC_ALPHA,[$S]:r.ONE_MINUS_DST_COLOR,[QS]:r.ONE_MINUS_DST_ALPHA,[eM]:r.CONSTANT_COLOR,[nM]:r.ONE_MINUS_CONSTANT_COLOR,[iM]:r.CONSTANT_ALPHA,[aM]:r.ONE_MINUS_CONSTANT_ALPHA};function jt(W,_t,Rt,Pt,Et,pt,zt,ne,_e,Re){if(W===ja){R===!0&&(xt(r.BLEND),R=!1);return}if(R===!1&&(it(r.BLEND),R=!0),W!==GS){if(W!==M||Re!==D){if((_!==Ts||T!==Ts)&&(r.blendEquation(r.FUNC_ADD),_=Ts,T=Ts),Re)switch(W){case Ir:r.blendFuncSeparate(r.ONE,r.ONE_MINUS_SRC_ALPHA,r.ONE,r.ONE_MINUS_SRC_ALPHA);break;case d0:r.blendFunc(r.ONE,r.ONE);break;case p0:r.blendFuncSeparate(r.ZERO,r.ONE_MINUS_SRC_COLOR,r.ZERO,r.ONE);break;case m0:r.blendFuncSeparate(r.DST_COLOR,r.ONE_MINUS_SRC_ALPHA,r.ZERO,r.ONE);break;default:console.error("THREE.WebGLState: Invalid blending: ",W);break}else switch(W){case Ir:r.blendFuncSeparate(r.SRC_ALPHA,r.ONE_MINUS_SRC_ALPHA,r.ONE,r.ONE_MINUS_SRC_ALPHA);break;case d0:r.blendFuncSeparate(r.SRC_ALPHA,r.ONE,r.ONE,r.ONE);break;case p0:console.error("THREE.WebGLState: SubtractiveBlending requires material.premultipliedAlpha = true");break;case m0:console.error("THREE.WebGLState: MultiplyBlending requires material.premultipliedAlpha = true");break;default:console.error("THREE.WebGLState: Invalid blending: ",W);break}L=null,U=null,P=null,B=null,F.set(0,0,0),G=0,M=W,D=Re}return}Et=Et||_t,pt=pt||Rt,zt=zt||Pt,(_t!==_||Et!==T)&&(r.blendEquationSeparate(ft[_t],ft[Et]),_=_t,T=Et),(Rt!==L||Pt!==U||pt!==P||zt!==B)&&(r.blendFuncSeparate(I[Rt],I[Pt],I[pt],I[zt]),L=Rt,U=Pt,P=pt,B=zt),(ne.equals(F)===!1||_e!==G)&&(r.blendColor(ne.r,ne.g,ne.b,_e),F.copy(ne),G=_e),M=W,D=!1}function Kt(W,_t){W.side===ua?xt(r.CULL_FACE):it(r.CULL_FACE);let Rt=W.side===Yn;_t&&(Rt=!Rt),Qt(Rt),W.blending===Ir&&W.transparent===!1?jt(ja):jt(W.blending,W.blendEquation,W.blendSrc,W.blendDst,W.blendEquationAlpha,W.blendSrcAlpha,W.blendDstAlpha,W.blendColor,W.blendAlpha,W.premultipliedAlpha),h.setFunc(W.depthFunc),h.setTest(W.depthTest),h.setMask(W.depthWrite),c.setMask(W.colorWrite);const Pt=W.stencilWrite;d.setTest(Pt),Pt&&(d.setMask(W.stencilWriteMask),d.setFunc(W.stencilFunc,W.stencilRef,W.stencilFuncMask),d.setOp(W.stencilFail,W.stencilZFail,W.stencilZPass)),Dt(W.polygonOffset,W.polygonOffsetFactor,W.polygonOffsetUnits),W.alphaToCoverage===!0?it(r.SAMPLE_ALPHA_TO_COVERAGE):xt(r.SAMPLE_ALPHA_TO_COVERAGE)}function Qt(W){w!==W&&(W?r.frontFace(r.CW):r.frontFace(r.CCW),w=W)}function Xt(W){W!==IS?(it(r.CULL_FACE),W!==V&&(W===h0?r.cullFace(r.BACK):W===FS?r.cullFace(r.FRONT):r.cullFace(r.FRONT_AND_BACK))):xt(r.CULL_FACE),V=W}function Ee(W){W!==K&&(N&&r.lineWidth(W),K=W)}function Dt(W,_t,Rt){W?(it(r.POLYGON_OFFSET_FILL),(ot!==_t||ct!==Rt)&&(r.polygonOffset(_t,Rt),ot=_t,ct=Rt)):xt(r.POLYGON_OFFSET_FILL)}function Ht(W){W?it(r.SCISSOR_TEST):xt(r.SCISSOR_TEST)}function xe(W){W===void 0&&(W=r.TEXTURE0+ht-1),Tt!==W&&(r.activeTexture(W),Tt=W)}function Se(W,_t,Rt){Rt===void 0&&(Tt===null?Rt=r.TEXTURE0+ht-1:Rt=Tt);let Pt=Ct[Rt];Pt===void 0&&(Pt={type:void 0,texture:void 0},Ct[Rt]=Pt),(Pt.type!==W||Pt.texture!==_t)&&(Tt!==Rt&&(r.activeTexture(Rt),Tt=Rt),r.bindTexture(W,_t||q[W]),Pt.type=W,Pt.texture=_t)}function O(){const W=Ct[Tt];W!==void 0&&W.type!==void 0&&(r.bindTexture(W.type,null),W.type=void 0,W.texture=void 0)}function E(){try{r.compressedTexImage2D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function tt(){try{r.compressedTexImage3D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function gt(){try{r.texSubImage2D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function At(){try{r.texSubImage3D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function dt(){try{r.compressedTexSubImage2D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function Jt(){try{r.compressedTexSubImage3D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function Nt(){try{r.texStorage2D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function Wt(){try{r.texStorage3D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function $t(){try{r.texImage2D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function Ut(){try{r.texImage3D(...arguments)}catch(W){console.error("THREE.WebGLState:",W)}}function Ot(W){Mt.equals(W)===!1&&(r.scissor(W.x,W.y,W.z,W.w),Mt.copy(W))}function ee(W){wt.equals(W)===!1&&(r.viewport(W.x,W.y,W.z,W.w),wt.copy(W))}function It(W,_t){let Rt=p.get(_t);Rt===void 0&&(Rt=new WeakMap,p.set(_t,Rt));let Pt=Rt.get(W);Pt===void 0&&(Pt=r.getUniformBlockIndex(_t,W.name),Rt.set(W,Pt))}function Ft(W,_t){const Pt=p.get(_t).get(W);m.get(_t)!==Pt&&(r.uniformBlockBinding(_t,Pt,W.__bindingPointIndex),m.set(_t,Pt))}function fe(){r.disable(r.BLEND),r.disable(r.CULL_FACE),r.disable(r.DEPTH_TEST),r.disable(r.POLYGON_OFFSET_FILL),r.disable(r.SCISSOR_TEST),r.disable(r.STENCIL_TEST),r.disable(r.SAMPLE_ALPHA_TO_COVERAGE),r.blendEquation(r.FUNC_ADD),r.blendFunc(r.ONE,r.ZERO),r.blendFuncSeparate(r.ONE,r.ZERO,r.ONE,r.ZERO),r.blendColor(0,0,0,0),r.colorMask(!0,!0,!0,!0),r.clearColor(0,0,0,0),r.depthMask(!0),r.depthFunc(r.LESS),h.setReversed(!1),r.clearDepth(1),r.stencilMask(4294967295),r.stencilFunc(r.ALWAYS,0,4294967295),r.stencilOp(r.KEEP,r.KEEP,r.KEEP),r.clearStencil(0),r.cullFace(r.BACK),r.frontFace(r.CCW),r.polygonOffset(0,0),r.activeTexture(r.TEXTURE0),r.bindFramebuffer(r.FRAMEBUFFER,null),r.bindFramebuffer(r.DRAW_FRAMEBUFFER,null),r.bindFramebuffer(r.READ_FRAMEBUFFER,null),r.useProgram(null),r.lineWidth(1),r.scissor(0,0,r.canvas.width,r.canvas.height),r.viewport(0,0,r.canvas.width,r.canvas.height),v={},Tt=null,Ct={},g={},x=new WeakMap,S=[],b=null,R=!1,M=null,_=null,L=null,U=null,T=null,P=null,B=null,F=new Me(0,0,0),G=0,D=!1,w=null,V=null,K=null,ot=null,ct=null,Mt.set(0,0,r.canvas.width,r.canvas.height),wt.set(0,0,r.canvas.width,r.canvas.height),c.reset(),h.reset(),d.reset()}return{buffers:{color:c,depth:h,stencil:d},enable:it,disable:xt,bindFramebuffer:Lt,drawBuffers:Bt,useProgram:le,setBlending:jt,setMaterial:Kt,setFlipSided:Qt,setCullFace:Xt,setLineWidth:Ee,setPolygonOffset:Dt,setScissorTest:Ht,activeTexture:xe,bindTexture:Se,unbindTexture:O,compressedTexImage2D:E,compressedTexImage3D:tt,texImage2D:$t,texImage3D:Ut,updateUBOMapping:It,uniformBlockBinding:Ft,texStorage2D:Nt,texStorage3D:Wt,texSubImage2D:gt,texSubImage3D:At,compressedTexSubImage2D:dt,compressedTexSubImage3D:Jt,scissor:Ot,viewport:ee,reset:fe}}function oR(r,t,i,s,l,c,h){const d=t.has("WEBGL_multisampled_render_to_texture")?t.get("WEBGL_multisampled_render_to_texture"):null,m=typeof navigator>"u"?!1:/OculusBrowser/g.test(navigator.userAgent),p=new re,v=new WeakMap;let g;const x=new WeakMap;let S=!1;try{S=typeof OffscreenCanvas<"u"&&new OffscreenCanvas(1,1).getContext("2d")!==null}catch{}function b(O,E){return S?new OffscreenCanvas(O,E):au("canvas")}function R(O,E,tt){let gt=1;const At=Se(O);if((At.width>tt||At.height>tt)&&(gt=tt/Math.max(At.width,At.height)),gt<1)if(typeof HTMLImageElement<"u"&&O instanceof HTMLImageElement||typeof HTMLCanvasElement<"u"&&O instanceof HTMLCanvasElement||typeof ImageBitmap<"u"&&O instanceof ImageBitmap||typeof VideoFrame<"u"&&O instanceof VideoFrame){const dt=Math.floor(gt*At.width),Jt=Math.floor(gt*At.height);g===void 0&&(g=b(dt,Jt));const Nt=E?b(dt,Jt):g;return Nt.width=dt,Nt.height=Jt,Nt.getContext("2d").drawImage(O,0,0,dt,Jt),console.warn("THREE.WebGLRenderer: Texture has been resized from ("+At.width+"x"+At.height+") to ("+dt+"x"+Jt+")."),Nt}else return"data"in O&&console.warn("THREE.WebGLRenderer: Image in DataTexture is too big ("+At.width+"x"+At.height+")."),O;return O}function M(O){return O.generateMipmaps}function _(O){r.generateMipmap(O)}function L(O){return O.isWebGLCubeRenderTarget?r.TEXTURE_CUBE_MAP:O.isWebGL3DRenderTarget?r.TEXTURE_3D:O.isWebGLArrayRenderTarget||O.isCompressedArrayTexture?r.TEXTURE_2D_ARRAY:r.TEXTURE_2D}function U(O,E,tt,gt,At=!1){if(O!==null){if(r[O]!==void 0)return r[O];console.warn("THREE.WebGLRenderer: Attempt to use non-existing WebGL internal format '"+O+"'")}let dt=E;if(E===r.RED&&(tt===r.FLOAT&&(dt=r.R32F),tt===r.HALF_FLOAT&&(dt=r.R16F),tt===r.UNSIGNED_BYTE&&(dt=r.R8)),E===r.RED_INTEGER&&(tt===r.UNSIGNED_BYTE&&(dt=r.R8UI),tt===r.UNSIGNED_SHORT&&(dt=r.R16UI),tt===r.UNSIGNED_INT&&(dt=r.R32UI),tt===r.BYTE&&(dt=r.R8I),tt===r.SHORT&&(dt=r.R16I),tt===r.INT&&(dt=r.R32I)),E===r.RG&&(tt===r.FLOAT&&(dt=r.RG32F),tt===r.HALF_FLOAT&&(dt=r.RG16F),tt===r.UNSIGNED_BYTE&&(dt=r.RG8)),E===r.RG_INTEGER&&(tt===r.UNSIGNED_BYTE&&(dt=r.RG8UI),tt===r.UNSIGNED_SHORT&&(dt=r.RG16UI),tt===r.UNSIGNED_INT&&(dt=r.RG32UI),tt===r.BYTE&&(dt=r.RG8I),tt===r.SHORT&&(dt=r.RG16I),tt===r.INT&&(dt=r.RG32I)),E===r.RGB_INTEGER&&(tt===r.UNSIGNED_BYTE&&(dt=r.RGB8UI),tt===r.UNSIGNED_SHORT&&(dt=r.RGB16UI),tt===r.UNSIGNED_INT&&(dt=r.RGB32UI),tt===r.BYTE&&(dt=r.RGB8I),tt===r.SHORT&&(dt=r.RGB16I),tt===r.INT&&(dt=r.RGB32I)),E===r.RGBA_INTEGER&&(tt===r.UNSIGNED_BYTE&&(dt=r.RGBA8UI),tt===r.UNSIGNED_SHORT&&(dt=r.RGBA16UI),tt===r.UNSIGNED_INT&&(dt=r.RGBA32UI),tt===r.BYTE&&(dt=r.RGBA8I),tt===r.SHORT&&(dt=r.RGBA16I),tt===r.INT&&(dt=r.RGBA32I)),E===r.RGB&&(tt===r.UNSIGNED_INT_5_9_9_9_REV&&(dt=r.RGB9_E5),tt===r.UNSIGNED_INT_10F_11F_11F_REV&&(dt=r.R11F_G11F_B10F)),E===r.RGBA){const Jt=At?nu:Pe.getTransfer(gt);tt===r.FLOAT&&(dt=r.RGBA32F),tt===r.HALF_FLOAT&&(dt=r.RGBA16F),tt===r.UNSIGNED_BYTE&&(dt=Jt===We?r.SRGB8_ALPHA8:r.RGBA8),tt===r.UNSIGNED_SHORT_4_4_4_4&&(dt=r.RGBA4),tt===r.UNSIGNED_SHORT_5_5_5_1&&(dt=r.RGB5_A1)}return(dt===r.R16F||dt===r.R32F||dt===r.RG16F||dt===r.RG32F||dt===r.RGBA16F||dt===r.RGBA32F)&&t.get("EXT_color_buffer_float"),dt}function T(O,E){let tt;return O?E===null||E===Cs||E===al?tt=r.DEPTH24_STENCIL8:E===zi?tt=r.DEPTH32F_STENCIL8:E===il&&(tt=r.DEPTH24_STENCIL8,console.warn("DepthTexture: 16 bit depth attachment is not supported with stencil. Using 24-bit attachment.")):E===null||E===Cs||E===al?tt=r.DEPTH_COMPONENT24:E===zi?tt=r.DEPTH_COMPONENT32F:E===il&&(tt=r.DEPTH_COMPONENT16),tt}function P(O,E){return M(O)===!0||O.isFramebufferTexture&&O.minFilter!==si&&O.minFilter!==Pi?Math.log2(Math.max(E.width,E.height))+1:O.mipmaps!==void 0&&O.mipmaps.length>0?O.mipmaps.length:O.isCompressedTexture&&Array.isArray(O.image)?E.mipmaps.length:1}function B(O){const E=O.target;E.removeEventListener("dispose",B),G(E),E.isVideoTexture&&v.delete(E)}function F(O){const E=O.target;E.removeEventListener("dispose",F),w(E)}function G(O){const E=s.get(O);if(E.__webglInit===void 0)return;const tt=O.source,gt=x.get(tt);if(gt){const At=gt[E.__cacheKey];At.usedTimes--,At.usedTimes===0&&D(O),Object.keys(gt).length===0&&x.delete(tt)}s.remove(O)}function D(O){const E=s.get(O);r.deleteTexture(E.__webglTexture);const tt=O.source,gt=x.get(tt);delete gt[E.__cacheKey],h.memory.textures--}function w(O){const E=s.get(O);if(O.depthTexture&&(O.depthTexture.dispose(),s.remove(O.depthTexture)),O.isWebGLCubeRenderTarget)for(let gt=0;gt<6;gt++){if(Array.isArray(E.__webglFramebuffer[gt]))for(let At=0;At<E.__webglFramebuffer[gt].length;At++)r.deleteFramebuffer(E.__webglFramebuffer[gt][At]);else r.deleteFramebuffer(E.__webglFramebuffer[gt]);E.__webglDepthbuffer&&r.deleteRenderbuffer(E.__webglDepthbuffer[gt])}else{if(Array.isArray(E.__webglFramebuffer))for(let gt=0;gt<E.__webglFramebuffer.length;gt++)r.deleteFramebuffer(E.__webglFramebuffer[gt]);else r.deleteFramebuffer(E.__webglFramebuffer);if(E.__webglDepthbuffer&&r.deleteRenderbuffer(E.__webglDepthbuffer),E.__webglMultisampledFramebuffer&&r.deleteFramebuffer(E.__webglMultisampledFramebuffer),E.__webglColorRenderbuffer)for(let gt=0;gt<E.__webglColorRenderbuffer.length;gt++)E.__webglColorRenderbuffer[gt]&&r.deleteRenderbuffer(E.__webglColorRenderbuffer[gt]);E.__webglDepthRenderbuffer&&r.deleteRenderbuffer(E.__webglDepthRenderbuffer)}const tt=O.textures;for(let gt=0,At=tt.length;gt<At;gt++){const dt=s.get(tt[gt]);dt.__webglTexture&&(r.deleteTexture(dt.__webglTexture),h.memory.textures--),s.remove(tt[gt])}s.remove(O)}let V=0;function K(){V=0}function ot(){const O=V;return O>=l.maxTextures&&console.warn("THREE.WebGLTextures: Trying to use "+O+" texture units while this GPU supports only "+l.maxTextures),V+=1,O}function ct(O){const E=[];return E.push(O.wrapS),E.push(O.wrapT),E.push(O.wrapR||0),E.push(O.magFilter),E.push(O.minFilter),E.push(O.anisotropy),E.push(O.internalFormat),E.push(O.format),E.push(O.type),E.push(O.generateMipmaps),E.push(O.premultiplyAlpha),E.push(O.flipY),E.push(O.unpackAlignment),E.push(O.colorSpace),E.join()}function ht(O,E){const tt=s.get(O);if(O.isVideoTexture&&Ht(O),O.isRenderTargetTexture===!1&&O.isExternalTexture!==!0&&O.version>0&&tt.__version!==O.version){const gt=O.image;if(gt===null)console.warn("THREE.WebGLRenderer: Texture marked for update but no image data found.");else if(gt.complete===!1)console.warn("THREE.WebGLRenderer: Texture marked for update but image is incomplete");else{q(tt,O,E);return}}else O.isExternalTexture&&(tt.__webglTexture=O.sourceTexture?O.sourceTexture:null);i.bindTexture(r.TEXTURE_2D,tt.__webglTexture,r.TEXTURE0+E)}function N(O,E){const tt=s.get(O);if(O.isRenderTargetTexture===!1&&O.version>0&&tt.__version!==O.version){q(tt,O,E);return}i.bindTexture(r.TEXTURE_2D_ARRAY,tt.__webglTexture,r.TEXTURE0+E)}function J(O,E){const tt=s.get(O);if(O.isRenderTargetTexture===!1&&O.version>0&&tt.__version!==O.version){q(tt,O,E);return}i.bindTexture(r.TEXTURE_3D,tt.__webglTexture,r.TEXTURE0+E)}function Y(O,E){const tt=s.get(O);if(O.version>0&&tt.__version!==O.version){it(tt,O,E);return}i.bindTexture(r.TEXTURE_CUBE_MAP,tt.__webglTexture,r.TEXTURE0+E)}const Tt={[Md]:r.REPEAT,[As]:r.CLAMP_TO_EDGE,[Ed]:r.MIRRORED_REPEAT},Ct={[si]:r.NEAREST,[dM]:r.NEAREST_MIPMAP_NEAREST,[xc]:r.NEAREST_MIPMAP_LINEAR,[Pi]:r.LINEAR,[wh]:r.LINEAR_MIPMAP_NEAREST,[Rs]:r.LINEAR_MIPMAP_LINEAR},z={[_M]:r.NEVER,[EM]:r.ALWAYS,[vM]:r.LESS,[jv]:r.LEQUAL,[yM]:r.EQUAL,[MM]:r.GEQUAL,[xM]:r.GREATER,[SM]:r.NOTEQUAL};function at(O,E){if(E.type===zi&&t.has("OES_texture_float_linear")===!1&&(E.magFilter===Pi||E.magFilter===wh||E.magFilter===xc||E.magFilter===Rs||E.minFilter===Pi||E.minFilter===wh||E.minFilter===xc||E.minFilter===Rs)&&console.warn("THREE.WebGLRenderer: Unable to use linear filtering with floating point textures. OES_texture_float_linear not supported on this device."),r.texParameteri(O,r.TEXTURE_WRAP_S,Tt[E.wrapS]),r.texParameteri(O,r.TEXTURE_WRAP_T,Tt[E.wrapT]),(O===r.TEXTURE_3D||O===r.TEXTURE_2D_ARRAY)&&r.texParameteri(O,r.TEXTURE_WRAP_R,Tt[E.wrapR]),r.texParameteri(O,r.TEXTURE_MAG_FILTER,Ct[E.magFilter]),r.texParameteri(O,r.TEXTURE_MIN_FILTER,Ct[E.minFilter]),E.compareFunction&&(r.texParameteri(O,r.TEXTURE_COMPARE_MODE,r.COMPARE_REF_TO_TEXTURE),r.texParameteri(O,r.TEXTURE_COMPARE_FUNC,z[E.compareFunction])),t.has("EXT_texture_filter_anisotropic")===!0){if(E.magFilter===si||E.minFilter!==xc&&E.minFilter!==Rs||E.type===zi&&t.has("OES_texture_float_linear")===!1)return;if(E.anisotropy>1||s.get(E).__currentAnisotropy){const tt=t.get("EXT_texture_filter_anisotropic");r.texParameterf(O,tt.TEXTURE_MAX_ANISOTROPY_EXT,Math.min(E.anisotropy,l.getMaxAnisotropy())),s.get(E).__currentAnisotropy=E.anisotropy}}}function Mt(O,E){let tt=!1;O.__webglInit===void 0&&(O.__webglInit=!0,E.addEventListener("dispose",B));const gt=E.source;let At=x.get(gt);At===void 0&&(At={},x.set(gt,At));const dt=ct(E);if(dt!==O.__cacheKey){At[dt]===void 0&&(At[dt]={texture:r.createTexture(),usedTimes:0},h.memory.textures++,tt=!0),At[dt].usedTimes++;const Jt=At[O.__cacheKey];Jt!==void 0&&(At[O.__cacheKey].usedTimes--,Jt.usedTimes===0&&D(E)),O.__cacheKey=dt,O.__webglTexture=At[dt].texture}return tt}function wt(O,E,tt){return Math.floor(Math.floor(O/tt)/E)}function mt(O,E,tt,gt){const dt=O.updateRanges;if(dt.length===0)i.texSubImage2D(r.TEXTURE_2D,0,0,0,E.width,E.height,tt,gt,E.data);else{dt.sort((Ut,Ot)=>Ut.start-Ot.start);let Jt=0;for(let Ut=1;Ut<dt.length;Ut++){const Ot=dt[Jt],ee=dt[Ut],It=Ot.start+Ot.count,Ft=wt(ee.start,E.width,4),fe=wt(Ot.start,E.width,4);ee.start<=It+1&&Ft===fe&&wt(ee.start+ee.count-1,E.width,4)===Ft?Ot.count=Math.max(Ot.count,ee.start+ee.count-Ot.start):(++Jt,dt[Jt]=ee)}dt.length=Jt+1;const Nt=r.getParameter(r.UNPACK_ROW_LENGTH),Wt=r.getParameter(r.UNPACK_SKIP_PIXELS),$t=r.getParameter(r.UNPACK_SKIP_ROWS);r.pixelStorei(r.UNPACK_ROW_LENGTH,E.width);for(let Ut=0,Ot=dt.length;Ut<Ot;Ut++){const ee=dt[Ut],It=Math.floor(ee.start/4),Ft=Math.ceil(ee.count/4),fe=It%E.width,W=Math.floor(It/E.width),_t=Ft,Rt=1;r.pixelStorei(r.UNPACK_SKIP_PIXELS,fe),r.pixelStorei(r.UNPACK_SKIP_ROWS,W),i.texSubImage2D(r.TEXTURE_2D,0,fe,W,_t,Rt,tt,gt,E.data)}O.clearUpdateRanges(),r.pixelStorei(r.UNPACK_ROW_LENGTH,Nt),r.pixelStorei(r.UNPACK_SKIP_PIXELS,Wt),r.pixelStorei(r.UNPACK_SKIP_ROWS,$t)}}function q(O,E,tt){let gt=r.TEXTURE_2D;(E.isDataArrayTexture||E.isCompressedArrayTexture)&&(gt=r.TEXTURE_2D_ARRAY),E.isData3DTexture&&(gt=r.TEXTURE_3D);const At=Mt(O,E),dt=E.source;i.bindTexture(gt,O.__webglTexture,r.TEXTURE0+tt);const Jt=s.get(dt);if(dt.version!==Jt.__version||At===!0){i.activeTexture(r.TEXTURE0+tt);const Nt=Pe.getPrimaries(Pe.workingColorSpace),Wt=E.colorSpace===Wa?null:Pe.getPrimaries(E.colorSpace),$t=E.colorSpace===Wa||Nt===Wt?r.NONE:r.BROWSER_DEFAULT_WEBGL;r.pixelStorei(r.UNPACK_FLIP_Y_WEBGL,E.flipY),r.pixelStorei(r.UNPACK_PREMULTIPLY_ALPHA_WEBGL,E.premultiplyAlpha),r.pixelStorei(r.UNPACK_ALIGNMENT,E.unpackAlignment),r.pixelStorei(r.UNPACK_COLORSPACE_CONVERSION_WEBGL,$t);let Ut=R(E.image,!1,l.maxTextureSize);Ut=xe(E,Ut);const Ot=c.convert(E.format,E.colorSpace),ee=c.convert(E.type);let It=U(E.internalFormat,Ot,ee,E.colorSpace,E.isVideoTexture);at(gt,E);let Ft;const fe=E.mipmaps,W=E.isVideoTexture!==!0,_t=Jt.__version===void 0||At===!0,Rt=dt.dataReady,Pt=P(E,Ut);if(E.isDepthTexture)It=T(E.format===rl,E.type),_t&&(W?i.texStorage2D(r.TEXTURE_2D,1,It,Ut.width,Ut.height):i.texImage2D(r.TEXTURE_2D,0,It,Ut.width,Ut.height,0,Ot,ee,null));else if(E.isDataTexture)if(fe.length>0){W&&_t&&i.texStorage2D(r.TEXTURE_2D,Pt,It,fe[0].width,fe[0].height);for(let Et=0,pt=fe.length;Et<pt;Et++)Ft=fe[Et],W?Rt&&i.texSubImage2D(r.TEXTURE_2D,Et,0,0,Ft.width,Ft.height,Ot,ee,Ft.data):i.texImage2D(r.TEXTURE_2D,Et,It,Ft.width,Ft.height,0,Ot,ee,Ft.data);E.generateMipmaps=!1}else W?(_t&&i.texStorage2D(r.TEXTURE_2D,Pt,It,Ut.width,Ut.height),Rt&&mt(E,Ut,Ot,ee)):i.texImage2D(r.TEXTURE_2D,0,It,Ut.width,Ut.height,0,Ot,ee,Ut.data);else if(E.isCompressedTexture)if(E.isCompressedArrayTexture){W&&_t&&i.texStorage3D(r.TEXTURE_2D_ARRAY,Pt,It,fe[0].width,fe[0].height,Ut.depth);for(let Et=0,pt=fe.length;Et<pt;Et++)if(Ft=fe[Et],E.format!==Ri)if(Ot!==null)if(W){if(Rt)if(E.layerUpdates.size>0){const zt=$0(Ft.width,Ft.height,E.format,E.type);for(const ne of E.layerUpdates){const _e=Ft.data.subarray(ne*zt/Ft.data.BYTES_PER_ELEMENT,(ne+1)*zt/Ft.data.BYTES_PER_ELEMENT);i.compressedTexSubImage3D(r.TEXTURE_2D_ARRAY,Et,0,0,ne,Ft.width,Ft.height,1,Ot,_e)}E.clearLayerUpdates()}else i.compressedTexSubImage3D(r.TEXTURE_2D_ARRAY,Et,0,0,0,Ft.width,Ft.height,Ut.depth,Ot,Ft.data)}else i.compressedTexImage3D(r.TEXTURE_2D_ARRAY,Et,It,Ft.width,Ft.height,Ut.depth,0,Ft.data,0,0);else console.warn("THREE.WebGLRenderer: Attempt to load unsupported compressed texture format in .uploadTexture()");else W?Rt&&i.texSubImage3D(r.TEXTURE_2D_ARRAY,Et,0,0,0,Ft.width,Ft.height,Ut.depth,Ot,ee,Ft.data):i.texImage3D(r.TEXTURE_2D_ARRAY,Et,It,Ft.width,Ft.height,Ut.depth,0,Ot,ee,Ft.data)}else{W&&_t&&i.texStorage2D(r.TEXTURE_2D,Pt,It,fe[0].width,fe[0].height);for(let Et=0,pt=fe.length;Et<pt;Et++)Ft=fe[Et],E.format!==Ri?Ot!==null?W?Rt&&i.compressedTexSubImage2D(r.TEXTURE_2D,Et,0,0,Ft.width,Ft.height,Ot,Ft.data):i.compressedTexImage2D(r.TEXTURE_2D,Et,It,Ft.width,Ft.height,0,Ft.data):console.warn("THREE.WebGLRenderer: Attempt to load unsupported compressed texture format in .uploadTexture()"):W?Rt&&i.texSubImage2D(r.TEXTURE_2D,Et,0,0,Ft.width,Ft.height,Ot,ee,Ft.data):i.texImage2D(r.TEXTURE_2D,Et,It,Ft.width,Ft.height,0,Ot,ee,Ft.data)}else if(E.isDataArrayTexture)if(W){if(_t&&i.texStorage3D(r.TEXTURE_2D_ARRAY,Pt,It,Ut.width,Ut.height,Ut.depth),Rt)if(E.layerUpdates.size>0){const Et=$0(Ut.width,Ut.height,E.format,E.type);for(const pt of E.layerUpdates){const zt=Ut.data.subarray(pt*Et/Ut.data.BYTES_PER_ELEMENT,(pt+1)*Et/Ut.data.BYTES_PER_ELEMENT);i.texSubImage3D(r.TEXTURE_2D_ARRAY,0,0,0,pt,Ut.width,Ut.height,1,Ot,ee,zt)}E.clearLayerUpdates()}else i.texSubImage3D(r.TEXTURE_2D_ARRAY,0,0,0,0,Ut.width,Ut.height,Ut.depth,Ot,ee,Ut.data)}else i.texImage3D(r.TEXTURE_2D_ARRAY,0,It,Ut.width,Ut.height,Ut.depth,0,Ot,ee,Ut.data);else if(E.isData3DTexture)W?(_t&&i.texStorage3D(r.TEXTURE_3D,Pt,It,Ut.width,Ut.height,Ut.depth),Rt&&i.texSubImage3D(r.TEXTURE_3D,0,0,0,0,Ut.width,Ut.height,Ut.depth,Ot,ee,Ut.data)):i.texImage3D(r.TEXTURE_3D,0,It,Ut.width,Ut.height,Ut.depth,0,Ot,ee,Ut.data);else if(E.isFramebufferTexture){if(_t)if(W)i.texStorage2D(r.TEXTURE_2D,Pt,It,Ut.width,Ut.height);else{let Et=Ut.width,pt=Ut.height;for(let zt=0;zt<Pt;zt++)i.texImage2D(r.TEXTURE_2D,zt,It,Et,pt,0,Ot,ee,null),Et>>=1,pt>>=1}}else if(fe.length>0){if(W&&_t){const Et=Se(fe[0]);i.texStorage2D(r.TEXTURE_2D,Pt,It,Et.width,Et.height)}for(let Et=0,pt=fe.length;Et<pt;Et++)Ft=fe[Et],W?Rt&&i.texSubImage2D(r.TEXTURE_2D,Et,0,0,Ot,ee,Ft):i.texImage2D(r.TEXTURE_2D,Et,It,Ot,ee,Ft);E.generateMipmaps=!1}else if(W){if(_t){const Et=Se(Ut);i.texStorage2D(r.TEXTURE_2D,Pt,It,Et.width,Et.height)}Rt&&i.texSubImage2D(r.TEXTURE_2D,0,0,0,Ot,ee,Ut)}else i.texImage2D(r.TEXTURE_2D,0,It,Ot,ee,Ut);M(E)&&_(gt),Jt.__version=dt.version,E.onUpdate&&E.onUpdate(E)}O.__version=E.version}function it(O,E,tt){if(E.image.length!==6)return;const gt=Mt(O,E),At=E.source;i.bindTexture(r.TEXTURE_CUBE_MAP,O.__webglTexture,r.TEXTURE0+tt);const dt=s.get(At);if(At.version!==dt.__version||gt===!0){i.activeTexture(r.TEXTURE0+tt);const Jt=Pe.getPrimaries(Pe.workingColorSpace),Nt=E.colorSpace===Wa?null:Pe.getPrimaries(E.colorSpace),Wt=E.colorSpace===Wa||Jt===Nt?r.NONE:r.BROWSER_DEFAULT_WEBGL;r.pixelStorei(r.UNPACK_FLIP_Y_WEBGL,E.flipY),r.pixelStorei(r.UNPACK_PREMULTIPLY_ALPHA_WEBGL,E.premultiplyAlpha),r.pixelStorei(r.UNPACK_ALIGNMENT,E.unpackAlignment),r.pixelStorei(r.UNPACK_COLORSPACE_CONVERSION_WEBGL,Wt);const $t=E.isCompressedTexture||E.image[0].isCompressedTexture,Ut=E.image[0]&&E.image[0].isDataTexture,Ot=[];for(let pt=0;pt<6;pt++)!$t&&!Ut?Ot[pt]=R(E.image[pt],!0,l.maxCubemapSize):Ot[pt]=Ut?E.image[pt].image:E.image[pt],Ot[pt]=xe(E,Ot[pt]);const ee=Ot[0],It=c.convert(E.format,E.colorSpace),Ft=c.convert(E.type),fe=U(E.internalFormat,It,Ft,E.colorSpace),W=E.isVideoTexture!==!0,_t=dt.__version===void 0||gt===!0,Rt=At.dataReady;let Pt=P(E,ee);at(r.TEXTURE_CUBE_MAP,E);let Et;if($t){W&&_t&&i.texStorage2D(r.TEXTURE_CUBE_MAP,Pt,fe,ee.width,ee.height);for(let pt=0;pt<6;pt++){Et=Ot[pt].mipmaps;for(let zt=0;zt<Et.length;zt++){const ne=Et[zt];E.format!==Ri?It!==null?W?Rt&&i.compressedTexSubImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,zt,0,0,ne.width,ne.height,It,ne.data):i.compressedTexImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,zt,fe,ne.width,ne.height,0,ne.data):console.warn("THREE.WebGLRenderer: Attempt to load unsupported compressed texture format in .setTextureCube()"):W?Rt&&i.texSubImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,zt,0,0,ne.width,ne.height,It,Ft,ne.data):i.texImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,zt,fe,ne.width,ne.height,0,It,Ft,ne.data)}}}else{if(Et=E.mipmaps,W&&_t){Et.length>0&&Pt++;const pt=Se(Ot[0]);i.texStorage2D(r.TEXTURE_CUBE_MAP,Pt,fe,pt.width,pt.height)}for(let pt=0;pt<6;pt++)if(Ut){W?Rt&&i.texSubImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,0,0,0,Ot[pt].width,Ot[pt].height,It,Ft,Ot[pt].data):i.texImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,0,fe,Ot[pt].width,Ot[pt].height,0,It,Ft,Ot[pt].data);for(let zt=0;zt<Et.length;zt++){const _e=Et[zt].image[pt].image;W?Rt&&i.texSubImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,zt+1,0,0,_e.width,_e.height,It,Ft,_e.data):i.texImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,zt+1,fe,_e.width,_e.height,0,It,Ft,_e.data)}}else{W?Rt&&i.texSubImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,0,0,0,It,Ft,Ot[pt]):i.texImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,0,fe,It,Ft,Ot[pt]);for(let zt=0;zt<Et.length;zt++){const ne=Et[zt];W?Rt&&i.texSubImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,zt+1,0,0,It,Ft,ne.image[pt]):i.texImage2D(r.TEXTURE_CUBE_MAP_POSITIVE_X+pt,zt+1,fe,It,Ft,ne.image[pt])}}}M(E)&&_(r.TEXTURE_CUBE_MAP),dt.__version=At.version,E.onUpdate&&E.onUpdate(E)}O.__version=E.version}function xt(O,E,tt,gt,At,dt){const Jt=c.convert(tt.format,tt.colorSpace),Nt=c.convert(tt.type),Wt=U(tt.internalFormat,Jt,Nt,tt.colorSpace),$t=s.get(E),Ut=s.get(tt);if(Ut.__renderTarget=E,!$t.__hasExternalTextures){const Ot=Math.max(1,E.width>>dt),ee=Math.max(1,E.height>>dt);At===r.TEXTURE_3D||At===r.TEXTURE_2D_ARRAY?i.texImage3D(At,dt,Wt,Ot,ee,E.depth,0,Jt,Nt,null):i.texImage2D(At,dt,Wt,Ot,ee,0,Jt,Nt,null)}i.bindFramebuffer(r.FRAMEBUFFER,O),Dt(E)?d.framebufferTexture2DMultisampleEXT(r.FRAMEBUFFER,gt,At,Ut.__webglTexture,0,Ee(E)):(At===r.TEXTURE_2D||At>=r.TEXTURE_CUBE_MAP_POSITIVE_X&&At<=r.TEXTURE_CUBE_MAP_NEGATIVE_Z)&&r.framebufferTexture2D(r.FRAMEBUFFER,gt,At,Ut.__webglTexture,dt),i.bindFramebuffer(r.FRAMEBUFFER,null)}function Lt(O,E,tt){if(r.bindRenderbuffer(r.RENDERBUFFER,O),E.depthBuffer){const gt=E.depthTexture,At=gt&&gt.isDepthTexture?gt.type:null,dt=T(E.stencilBuffer,At),Jt=E.stencilBuffer?r.DEPTH_STENCIL_ATTACHMENT:r.DEPTH_ATTACHMENT,Nt=Ee(E);Dt(E)?d.renderbufferStorageMultisampleEXT(r.RENDERBUFFER,Nt,dt,E.width,E.height):tt?r.renderbufferStorageMultisample(r.RENDERBUFFER,Nt,dt,E.width,E.height):r.renderbufferStorage(r.RENDERBUFFER,dt,E.width,E.height),r.framebufferRenderbuffer(r.FRAMEBUFFER,Jt,r.RENDERBUFFER,O)}else{const gt=E.textures;for(let At=0;At<gt.length;At++){const dt=gt[At],Jt=c.convert(dt.format,dt.colorSpace),Nt=c.convert(dt.type),Wt=U(dt.internalFormat,Jt,Nt,dt.colorSpace),$t=Ee(E);tt&&Dt(E)===!1?r.renderbufferStorageMultisample(r.RENDERBUFFER,$t,Wt,E.width,E.height):Dt(E)?d.renderbufferStorageMultisampleEXT(r.RENDERBUFFER,$t,Wt,E.width,E.height):r.renderbufferStorage(r.RENDERBUFFER,Wt,E.width,E.height)}}r.bindRenderbuffer(r.RENDERBUFFER,null)}function Bt(O,E){if(E&&E.isWebGLCubeRenderTarget)throw new Error("Depth Texture with cube render targets is not supported");if(i.bindFramebuffer(r.FRAMEBUFFER,O),!(E.depthTexture&&E.depthTexture.isDepthTexture))throw new Error("renderTarget.depthTexture must be an instance of THREE.DepthTexture");const gt=s.get(E.depthTexture);gt.__renderTarget=E,(!gt.__webglTexture||E.depthTexture.image.width!==E.width||E.depthTexture.image.height!==E.height)&&(E.depthTexture.image.width=E.width,E.depthTexture.image.height=E.height,E.depthTexture.needsUpdate=!0),ht(E.depthTexture,0);const At=gt.__webglTexture,dt=Ee(E);if(E.depthTexture.format===sl)Dt(E)?d.framebufferTexture2DMultisampleEXT(r.FRAMEBUFFER,r.DEPTH_ATTACHMENT,r.TEXTURE_2D,At,0,dt):r.framebufferTexture2D(r.FRAMEBUFFER,r.DEPTH_ATTACHMENT,r.TEXTURE_2D,At,0);else if(E.depthTexture.format===rl)Dt(E)?d.framebufferTexture2DMultisampleEXT(r.FRAMEBUFFER,r.DEPTH_STENCIL_ATTACHMENT,r.TEXTURE_2D,At,0,dt):r.framebufferTexture2D(r.FRAMEBUFFER,r.DEPTH_STENCIL_ATTACHMENT,r.TEXTURE_2D,At,0);else throw new Error("Unknown depthTexture format")}function le(O){const E=s.get(O),tt=O.isWebGLCubeRenderTarget===!0;if(E.__boundDepthTexture!==O.depthTexture){const gt=O.depthTexture;if(E.__depthDisposeCallback&&E.__depthDisposeCallback(),gt){const At=()=>{delete E.__boundDepthTexture,delete E.__depthDisposeCallback,gt.removeEventListener("dispose",At)};gt.addEventListener("dispose",At),E.__depthDisposeCallback=At}E.__boundDepthTexture=gt}if(O.depthTexture&&!E.__autoAllocateDepthBuffer){if(tt)throw new Error("target.depthTexture not supported in Cube render targets");const gt=O.texture.mipmaps;gt&&gt.length>0?Bt(E.__webglFramebuffer[0],O):Bt(E.__webglFramebuffer,O)}else if(tt){E.__webglDepthbuffer=[];for(let gt=0;gt<6;gt++)if(i.bindFramebuffer(r.FRAMEBUFFER,E.__webglFramebuffer[gt]),E.__webglDepthbuffer[gt]===void 0)E.__webglDepthbuffer[gt]=r.createRenderbuffer(),Lt(E.__webglDepthbuffer[gt],O,!1);else{const At=O.stencilBuffer?r.DEPTH_STENCIL_ATTACHMENT:r.DEPTH_ATTACHMENT,dt=E.__webglDepthbuffer[gt];r.bindRenderbuffer(r.RENDERBUFFER,dt),r.framebufferRenderbuffer(r.FRAMEBUFFER,At,r.RENDERBUFFER,dt)}}else{const gt=O.texture.mipmaps;if(gt&&gt.length>0?i.bindFramebuffer(r.FRAMEBUFFER,E.__webglFramebuffer[0]):i.bindFramebuffer(r.FRAMEBUFFER,E.__webglFramebuffer),E.__webglDepthbuffer===void 0)E.__webglDepthbuffer=r.createRenderbuffer(),Lt(E.__webglDepthbuffer,O,!1);else{const At=O.stencilBuffer?r.DEPTH_STENCIL_ATTACHMENT:r.DEPTH_ATTACHMENT,dt=E.__webglDepthbuffer;r.bindRenderbuffer(r.RENDERBUFFER,dt),r.framebufferRenderbuffer(r.FRAMEBUFFER,At,r.RENDERBUFFER,dt)}}i.bindFramebuffer(r.FRAMEBUFFER,null)}function ft(O,E,tt){const gt=s.get(O);E!==void 0&&xt(gt.__webglFramebuffer,O,O.texture,r.COLOR_ATTACHMENT0,r.TEXTURE_2D,0),tt!==void 0&&le(O)}function I(O){const E=O.texture,tt=s.get(O),gt=s.get(E);O.addEventListener("dispose",F);const At=O.textures,dt=O.isWebGLCubeRenderTarget===!0,Jt=At.length>1;if(Jt||(gt.__webglTexture===void 0&&(gt.__webglTexture=r.createTexture()),gt.__version=E.version,h.memory.textures++),dt){tt.__webglFramebuffer=[];for(let Nt=0;Nt<6;Nt++)if(E.mipmaps&&E.mipmaps.length>0){tt.__webglFramebuffer[Nt]=[];for(let Wt=0;Wt<E.mipmaps.length;Wt++)tt.__webglFramebuffer[Nt][Wt]=r.createFramebuffer()}else tt.__webglFramebuffer[Nt]=r.createFramebuffer()}else{if(E.mipmaps&&E.mipmaps.length>0){tt.__webglFramebuffer=[];for(let Nt=0;Nt<E.mipmaps.length;Nt++)tt.__webglFramebuffer[Nt]=r.createFramebuffer()}else tt.__webglFramebuffer=r.createFramebuffer();if(Jt)for(let Nt=0,Wt=At.length;Nt<Wt;Nt++){const $t=s.get(At[Nt]);$t.__webglTexture===void 0&&($t.__webglTexture=r.createTexture(),h.memory.textures++)}if(O.samples>0&&Dt(O)===!1){tt.__webglMultisampledFramebuffer=r.createFramebuffer(),tt.__webglColorRenderbuffer=[],i.bindFramebuffer(r.FRAMEBUFFER,tt.__webglMultisampledFramebuffer);for(let Nt=0;Nt<At.length;Nt++){const Wt=At[Nt];tt.__webglColorRenderbuffer[Nt]=r.createRenderbuffer(),r.bindRenderbuffer(r.RENDERBUFFER,tt.__webglColorRenderbuffer[Nt]);const $t=c.convert(Wt.format,Wt.colorSpace),Ut=c.convert(Wt.type),Ot=U(Wt.internalFormat,$t,Ut,Wt.colorSpace,O.isXRRenderTarget===!0),ee=Ee(O);r.renderbufferStorageMultisample(r.RENDERBUFFER,ee,Ot,O.width,O.height),r.framebufferRenderbuffer(r.FRAMEBUFFER,r.COLOR_ATTACHMENT0+Nt,r.RENDERBUFFER,tt.__webglColorRenderbuffer[Nt])}r.bindRenderbuffer(r.RENDERBUFFER,null),O.depthBuffer&&(tt.__webglDepthRenderbuffer=r.createRenderbuffer(),Lt(tt.__webglDepthRenderbuffer,O,!0)),i.bindFramebuffer(r.FRAMEBUFFER,null)}}if(dt){i.bindTexture(r.TEXTURE_CUBE_MAP,gt.__webglTexture),at(r.TEXTURE_CUBE_MAP,E);for(let Nt=0;Nt<6;Nt++)if(E.mipmaps&&E.mipmaps.length>0)for(let Wt=0;Wt<E.mipmaps.length;Wt++)xt(tt.__webglFramebuffer[Nt][Wt],O,E,r.COLOR_ATTACHMENT0,r.TEXTURE_CUBE_MAP_POSITIVE_X+Nt,Wt);else xt(tt.__webglFramebuffer[Nt],O,E,r.COLOR_ATTACHMENT0,r.TEXTURE_CUBE_MAP_POSITIVE_X+Nt,0);M(E)&&_(r.TEXTURE_CUBE_MAP),i.unbindTexture()}else if(Jt){for(let Nt=0,Wt=At.length;Nt<Wt;Nt++){const $t=At[Nt],Ut=s.get($t);let Ot=r.TEXTURE_2D;(O.isWebGL3DRenderTarget||O.isWebGLArrayRenderTarget)&&(Ot=O.isWebGL3DRenderTarget?r.TEXTURE_3D:r.TEXTURE_2D_ARRAY),i.bindTexture(Ot,Ut.__webglTexture),at(Ot,$t),xt(tt.__webglFramebuffer,O,$t,r.COLOR_ATTACHMENT0+Nt,Ot,0),M($t)&&_(Ot)}i.unbindTexture()}else{let Nt=r.TEXTURE_2D;if((O.isWebGL3DRenderTarget||O.isWebGLArrayRenderTarget)&&(Nt=O.isWebGL3DRenderTarget?r.TEXTURE_3D:r.TEXTURE_2D_ARRAY),i.bindTexture(Nt,gt.__webglTexture),at(Nt,E),E.mipmaps&&E.mipmaps.length>0)for(let Wt=0;Wt<E.mipmaps.length;Wt++)xt(tt.__webglFramebuffer[Wt],O,E,r.COLOR_ATTACHMENT0,Nt,Wt);else xt(tt.__webglFramebuffer,O,E,r.COLOR_ATTACHMENT0,Nt,0);M(E)&&_(Nt),i.unbindTexture()}O.depthBuffer&&le(O)}function jt(O){const E=O.textures;for(let tt=0,gt=E.length;tt<gt;tt++){const At=E[tt];if(M(At)){const dt=L(O),Jt=s.get(At).__webglTexture;i.bindTexture(dt,Jt),_(dt),i.unbindTexture()}}}const Kt=[],Qt=[];function Xt(O){if(O.samples>0){if(Dt(O)===!1){const E=O.textures,tt=O.width,gt=O.height;let At=r.COLOR_BUFFER_BIT;const dt=O.stencilBuffer?r.DEPTH_STENCIL_ATTACHMENT:r.DEPTH_ATTACHMENT,Jt=s.get(O),Nt=E.length>1;if(Nt)for(let $t=0;$t<E.length;$t++)i.bindFramebuffer(r.FRAMEBUFFER,Jt.__webglMultisampledFramebuffer),r.framebufferRenderbuffer(r.FRAMEBUFFER,r.COLOR_ATTACHMENT0+$t,r.RENDERBUFFER,null),i.bindFramebuffer(r.FRAMEBUFFER,Jt.__webglFramebuffer),r.framebufferTexture2D(r.DRAW_FRAMEBUFFER,r.COLOR_ATTACHMENT0+$t,r.TEXTURE_2D,null,0);i.bindFramebuffer(r.READ_FRAMEBUFFER,Jt.__webglMultisampledFramebuffer);const Wt=O.texture.mipmaps;Wt&&Wt.length>0?i.bindFramebuffer(r.DRAW_FRAMEBUFFER,Jt.__webglFramebuffer[0]):i.bindFramebuffer(r.DRAW_FRAMEBUFFER,Jt.__webglFramebuffer);for(let $t=0;$t<E.length;$t++){if(O.resolveDepthBuffer&&(O.depthBuffer&&(At|=r.DEPTH_BUFFER_BIT),O.stencilBuffer&&O.resolveStencilBuffer&&(At|=r.STENCIL_BUFFER_BIT)),Nt){r.framebufferRenderbuffer(r.READ_FRAMEBUFFER,r.COLOR_ATTACHMENT0,r.RENDERBUFFER,Jt.__webglColorRenderbuffer[$t]);const Ut=s.get(E[$t]).__webglTexture;r.framebufferTexture2D(r.DRAW_FRAMEBUFFER,r.COLOR_ATTACHMENT0,r.TEXTURE_2D,Ut,0)}r.blitFramebuffer(0,0,tt,gt,0,0,tt,gt,At,r.NEAREST),m===!0&&(Kt.length=0,Qt.length=0,Kt.push(r.COLOR_ATTACHMENT0+$t),O.depthBuffer&&O.resolveDepthBuffer===!1&&(Kt.push(dt),Qt.push(dt),r.invalidateFramebuffer(r.DRAW_FRAMEBUFFER,Qt)),r.invalidateFramebuffer(r.READ_FRAMEBUFFER,Kt))}if(i.bindFramebuffer(r.READ_FRAMEBUFFER,null),i.bindFramebuffer(r.DRAW_FRAMEBUFFER,null),Nt)for(let $t=0;$t<E.length;$t++){i.bindFramebuffer(r.FRAMEBUFFER,Jt.__webglMultisampledFramebuffer),r.framebufferRenderbuffer(r.FRAMEBUFFER,r.COLOR_ATTACHMENT0+$t,r.RENDERBUFFER,Jt.__webglColorRenderbuffer[$t]);const Ut=s.get(E[$t]).__webglTexture;i.bindFramebuffer(r.FRAMEBUFFER,Jt.__webglFramebuffer),r.framebufferTexture2D(r.DRAW_FRAMEBUFFER,r.COLOR_ATTACHMENT0+$t,r.TEXTURE_2D,Ut,0)}i.bindFramebuffer(r.DRAW_FRAMEBUFFER,Jt.__webglMultisampledFramebuffer)}else if(O.depthBuffer&&O.resolveDepthBuffer===!1&&m){const E=O.stencilBuffer?r.DEPTH_STENCIL_ATTACHMENT:r.DEPTH_ATTACHMENT;r.invalidateFramebuffer(r.DRAW_FRAMEBUFFER,[E])}}}function Ee(O){return Math.min(l.maxSamples,O.samples)}function Dt(O){const E=s.get(O);return O.samples>0&&t.has("WEBGL_multisampled_render_to_texture")===!0&&E.__useRenderToTexture!==!1}function Ht(O){const E=h.render.frame;v.get(O)!==E&&(v.set(O,E),O.update())}function xe(O,E){const tt=O.colorSpace,gt=O.format,At=O.type;return O.isCompressedTexture===!0||O.isVideoTexture===!0||tt!==kr&&tt!==Wa&&(Pe.getTransfer(tt)===We?(gt!==Ri||At!==Ii)&&console.warn("THREE.WebGLTextures: sRGB encoded textures have to use RGBAFormat and UnsignedByteType."):console.error("THREE.WebGLTextures: Unsupported texture color space:",tt)),E}function Se(O){return typeof HTMLImageElement<"u"&&O instanceof HTMLImageElement?(p.width=O.naturalWidth||O.width,p.height=O.naturalHeight||O.height):typeof VideoFrame<"u"&&O instanceof VideoFrame?(p.width=O.displayWidth,p.height=O.displayHeight):(p.width=O.width,p.height=O.height),p}this.allocateTextureUnit=ot,this.resetTextureUnits=K,this.setTexture2D=ht,this.setTexture2DArray=N,this.setTexture3D=J,this.setTextureCube=Y,this.rebindTextures=ft,this.setupRenderTarget=I,this.updateRenderTargetMipmap=jt,this.updateMultisampleRenderTarget=Xt,this.setupDepthRenderbuffer=le,this.setupFrameBufferTexture=xt,this.useMultisampledRTT=Dt}function lR(r,t){function i(s,l=Wa){let c;const h=Pe.getTransfer(l);if(s===Ii)return r.UNSIGNED_BYTE;if(s===ap)return r.UNSIGNED_SHORT_4_4_4_4;if(s===sp)return r.UNSIGNED_SHORT_5_5_5_1;if(s===Hv)return r.UNSIGNED_INT_5_9_9_9_REV;if(s===Gv)return r.UNSIGNED_INT_10F_11F_11F_REV;if(s===Iv)return r.BYTE;if(s===Fv)return r.SHORT;if(s===il)return r.UNSIGNED_SHORT;if(s===ip)return r.INT;if(s===Cs)return r.UNSIGNED_INT;if(s===zi)return r.FLOAT;if(s===cl)return r.HALF_FLOAT;if(s===Vv)return r.ALPHA;if(s===kv)return r.RGB;if(s===Ri)return r.RGBA;if(s===sl)return r.DEPTH_COMPONENT;if(s===rl)return r.DEPTH_STENCIL;if(s===rp)return r.RED;if(s===op)return r.RED_INTEGER;if(s===Xv)return r.RG;if(s===lp)return r.RG_INTEGER;if(s===cp)return r.RGBA_INTEGER;if(s===Qc||s===Jc||s===$c||s===tu)if(h===We)if(c=t.get("WEBGL_compressed_texture_s3tc_srgb"),c!==null){if(s===Qc)return c.COMPRESSED_SRGB_S3TC_DXT1_EXT;if(s===Jc)return c.COMPRESSED_SRGB_ALPHA_S3TC_DXT1_EXT;if(s===$c)return c.COMPRESSED_SRGB_ALPHA_S3TC_DXT3_EXT;if(s===tu)return c.COMPRESSED_SRGB_ALPHA_S3TC_DXT5_EXT}else return null;else if(c=t.get("WEBGL_compressed_texture_s3tc"),c!==null){if(s===Qc)return c.COMPRESSED_RGB_S3TC_DXT1_EXT;if(s===Jc)return c.COMPRESSED_RGBA_S3TC_DXT1_EXT;if(s===$c)return c.COMPRESSED_RGBA_S3TC_DXT3_EXT;if(s===tu)return c.COMPRESSED_RGBA_S3TC_DXT5_EXT}else return null;if(s===Td||s===bd||s===Ad||s===Rd)if(c=t.get("WEBGL_compressed_texture_pvrtc"),c!==null){if(s===Td)return c.COMPRESSED_RGB_PVRTC_4BPPV1_IMG;if(s===bd)return c.COMPRESSED_RGB_PVRTC_2BPPV1_IMG;if(s===Ad)return c.COMPRESSED_RGBA_PVRTC_4BPPV1_IMG;if(s===Rd)return c.COMPRESSED_RGBA_PVRTC_2BPPV1_IMG}else return null;if(s===Cd||s===wd||s===Dd)if(c=t.get("WEBGL_compressed_texture_etc"),c!==null){if(s===Cd||s===wd)return h===We?c.COMPRESSED_SRGB8_ETC2:c.COMPRESSED_RGB8_ETC2;if(s===Dd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ETC2_EAC:c.COMPRESSED_RGBA8_ETC2_EAC}else return null;if(s===Ud||s===Ld||s===Nd||s===Od||s===Pd||s===zd||s===Bd||s===Id||s===Fd||s===Hd||s===Gd||s===Vd||s===kd||s===Xd)if(c=t.get("WEBGL_compressed_texture_astc"),c!==null){if(s===Ud)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_4x4_KHR:c.COMPRESSED_RGBA_ASTC_4x4_KHR;if(s===Ld)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_5x4_KHR:c.COMPRESSED_RGBA_ASTC_5x4_KHR;if(s===Nd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_5x5_KHR:c.COMPRESSED_RGBA_ASTC_5x5_KHR;if(s===Od)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_6x5_KHR:c.COMPRESSED_RGBA_ASTC_6x5_KHR;if(s===Pd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_6x6_KHR:c.COMPRESSED_RGBA_ASTC_6x6_KHR;if(s===zd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_8x5_KHR:c.COMPRESSED_RGBA_ASTC_8x5_KHR;if(s===Bd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_8x6_KHR:c.COMPRESSED_RGBA_ASTC_8x6_KHR;if(s===Id)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_8x8_KHR:c.COMPRESSED_RGBA_ASTC_8x8_KHR;if(s===Fd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_10x5_KHR:c.COMPRESSED_RGBA_ASTC_10x5_KHR;if(s===Hd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_10x6_KHR:c.COMPRESSED_RGBA_ASTC_10x6_KHR;if(s===Gd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_10x8_KHR:c.COMPRESSED_RGBA_ASTC_10x8_KHR;if(s===Vd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_10x10_KHR:c.COMPRESSED_RGBA_ASTC_10x10_KHR;if(s===kd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_12x10_KHR:c.COMPRESSED_RGBA_ASTC_12x10_KHR;if(s===Xd)return h===We?c.COMPRESSED_SRGB8_ALPHA8_ASTC_12x12_KHR:c.COMPRESSED_RGBA_ASTC_12x12_KHR}else return null;if(s===Wd||s===jd||s===Yd)if(c=t.get("EXT_texture_compression_bptc"),c!==null){if(s===Wd)return h===We?c.COMPRESSED_SRGB_ALPHA_BPTC_UNORM_EXT:c.COMPRESSED_RGBA_BPTC_UNORM_EXT;if(s===jd)return c.COMPRESSED_RGB_BPTC_SIGNED_FLOAT_EXT;if(s===Yd)return c.COMPRESSED_RGB_BPTC_UNSIGNED_FLOAT_EXT}else return null;if(s===qd||s===Zd||s===Kd||s===Qd)if(c=t.get("EXT_texture_compression_rgtc"),c!==null){if(s===qd)return c.COMPRESSED_RED_RGTC1_EXT;if(s===Zd)return c.COMPRESSED_SIGNED_RED_RGTC1_EXT;if(s===Kd)return c.COMPRESSED_RED_GREEN_RGTC2_EXT;if(s===Qd)return c.COMPRESSED_SIGNED_RED_GREEN_RGTC2_EXT}else return null;return s===al?r.UNSIGNED_INT_24_8:r[s]!==void 0?r[s]:null}return{convert:i}}const cR=`
void main() {

	gl_Position = vec4( position, 1.0 );

}`,uR=`
uniform sampler2DArray depthColor;
uniform float depthWidth;
uniform float depthHeight;

void main() {

	vec2 coord = vec2( gl_FragCoord.x / depthWidth, gl_FragCoord.y / depthHeight );

	if ( coord.x >= 1.0 ) {

		gl_FragDepth = texture( depthColor, vec3( coord.x - 1.0, coord.y, 1 ) ).r;

	} else {

		gl_FragDepth = texture( depthColor, vec3( coord.x, coord.y, 0 ) ).r;

	}

}`;class fR{constructor(){this.texture=null,this.mesh=null,this.depthNear=0,this.depthFar=0}init(t,i){if(this.texture===null){const s=new ry(t.texture);(t.depthNear!==i.depthNear||t.depthFar!==i.depthFar)&&(this.depthNear=t.depthNear,this.depthFar=t.depthFar),this.texture=s}}getMesh(t){if(this.texture!==null&&this.mesh===null){const i=t.cameras[0].viewport,s=new Za({vertexShader:cR,fragmentShader:uR,uniforms:{depthColor:{value:this.texture},depthWidth:{value:i.z},depthHeight:{value:i.w}}});this.mesh=new vi(new du(20,20),s)}return this.mesh}reset(){this.texture=null,this.mesh=null}getDepthTexture(){return this.texture}}class hR extends Us{constructor(t,i){super();const s=this;let l=null,c=1,h=null,d="local-floor",m=1,p=null,v=null,g=null,x=null,S=null,b=null;const R=typeof XRWebGLBinding<"u",M=new fR,_={},L=i.getContextAttributes();let U=null,T=null;const P=[],B=[],F=new re;let G=null;const D=new ai;D.viewport=new Ye;const w=new ai;w.viewport=new Ye;const V=[D,w],K=new UE;let ot=null,ct=null;this.cameraAutoUpdate=!0,this.enabled=!1,this.isPresenting=!1,this.getController=function(q){let it=P[q];return it===void 0&&(it=new Kh,P[q]=it),it.getTargetRaySpace()},this.getControllerGrip=function(q){let it=P[q];return it===void 0&&(it=new Kh,P[q]=it),it.getGripSpace()},this.getHand=function(q){let it=P[q];return it===void 0&&(it=new Kh,P[q]=it),it.getHandSpace()};function ht(q){const it=B.indexOf(q.inputSource);if(it===-1)return;const xt=P[it];xt!==void 0&&(xt.update(q.inputSource,q.frame,p||h),xt.dispatchEvent({type:q.type,data:q.inputSource}))}function N(){l.removeEventListener("select",ht),l.removeEventListener("selectstart",ht),l.removeEventListener("selectend",ht),l.removeEventListener("squeeze",ht),l.removeEventListener("squeezestart",ht),l.removeEventListener("squeezeend",ht),l.removeEventListener("end",N),l.removeEventListener("inputsourceschange",J);for(let q=0;q<P.length;q++){const it=B[q];it!==null&&(B[q]=null,P[q].disconnect(it))}ot=null,ct=null,M.reset();for(const q in _)delete _[q];t.setRenderTarget(U),S=null,x=null,g=null,l=null,T=null,mt.stop(),s.isPresenting=!1,t.setPixelRatio(G),t.setSize(F.width,F.height,!1),s.dispatchEvent({type:"sessionend"})}this.setFramebufferScaleFactor=function(q){c=q,s.isPresenting===!0&&console.warn("THREE.WebXRManager: Cannot change framebuffer scale while presenting.")},this.setReferenceSpaceType=function(q){d=q,s.isPresenting===!0&&console.warn("THREE.WebXRManager: Cannot change reference space type while presenting.")},this.getReferenceSpace=function(){return p||h},this.setReferenceSpace=function(q){p=q},this.getBaseLayer=function(){return x!==null?x:S},this.getBinding=function(){return g===null&&R&&(g=new XRWebGLBinding(l,i)),g},this.getFrame=function(){return b},this.getSession=function(){return l},this.setSession=async function(q){if(l=q,l!==null){if(U=t.getRenderTarget(),l.addEventListener("select",ht),l.addEventListener("selectstart",ht),l.addEventListener("selectend",ht),l.addEventListener("squeeze",ht),l.addEventListener("squeezestart",ht),l.addEventListener("squeezeend",ht),l.addEventListener("end",N),l.addEventListener("inputsourceschange",J),L.xrCompatible!==!0&&await i.makeXRCompatible(),G=t.getPixelRatio(),t.getSize(F),R&&"createProjectionLayer"in XRWebGLBinding.prototype){let xt=null,Lt=null,Bt=null;L.depth&&(Bt=L.stencil?i.DEPTH24_STENCIL8:i.DEPTH_COMPONENT24,xt=L.stencil?rl:sl,Lt=L.stencil?al:Cs);const le={colorFormat:i.RGBA8,depthFormat:Bt,scaleFactor:c};g=this.getBinding(),x=g.createProjectionLayer(le),l.updateRenderState({layers:[x]}),t.setPixelRatio(1),t.setSize(x.textureWidth,x.textureHeight,!1),T=new Ds(x.textureWidth,x.textureHeight,{format:Ri,type:Ii,depthTexture:new sy(x.textureWidth,x.textureHeight,Lt,void 0,void 0,void 0,void 0,void 0,void 0,xt),stencilBuffer:L.stencil,colorSpace:t.outputColorSpace,samples:L.antialias?4:0,resolveDepthBuffer:x.ignoreDepthValues===!1,resolveStencilBuffer:x.ignoreDepthValues===!1})}else{const xt={antialias:L.antialias,alpha:!0,depth:L.depth,stencil:L.stencil,framebufferScaleFactor:c};S=new XRWebGLLayer(l,i,xt),l.updateRenderState({baseLayer:S}),t.setPixelRatio(1),t.setSize(S.framebufferWidth,S.framebufferHeight,!1),T=new Ds(S.framebufferWidth,S.framebufferHeight,{format:Ri,type:Ii,colorSpace:t.outputColorSpace,stencilBuffer:L.stencil,resolveDepthBuffer:S.ignoreDepthValues===!1,resolveStencilBuffer:S.ignoreDepthValues===!1})}T.isXRRenderTarget=!0,this.setFoveation(m),p=null,h=await l.requestReferenceSpace(d),mt.setContext(l),mt.start(),s.isPresenting=!0,s.dispatchEvent({type:"sessionstart"})}},this.getEnvironmentBlendMode=function(){if(l!==null)return l.environmentBlendMode},this.getDepthTexture=function(){return M.getDepthTexture()};function J(q){for(let it=0;it<q.removed.length;it++){const xt=q.removed[it],Lt=B.indexOf(xt);Lt>=0&&(B[Lt]=null,P[Lt].disconnect(xt))}for(let it=0;it<q.added.length;it++){const xt=q.added[it];let Lt=B.indexOf(xt);if(Lt===-1){for(let le=0;le<P.length;le++)if(le>=B.length){B.push(xt),Lt=le;break}else if(B[le]===null){B[le]=xt,Lt=le;break}if(Lt===-1)break}const Bt=P[Lt];Bt&&Bt.connect(xt)}}const Y=new X,Tt=new X;function Ct(q,it,xt){Y.setFromMatrixPosition(it.matrixWorld),Tt.setFromMatrixPosition(xt.matrixWorld);const Lt=Y.distanceTo(Tt),Bt=it.projectionMatrix.elements,le=xt.projectionMatrix.elements,ft=Bt[14]/(Bt[10]-1),I=Bt[14]/(Bt[10]+1),jt=(Bt[9]+1)/Bt[5],Kt=(Bt[9]-1)/Bt[5],Qt=(Bt[8]-1)/Bt[0],Xt=(le[8]+1)/le[0],Ee=ft*Qt,Dt=ft*Xt,Ht=Lt/(-Qt+Xt),xe=Ht*-Qt;if(it.matrixWorld.decompose(q.position,q.quaternion,q.scale),q.translateX(xe),q.translateZ(Ht),q.matrixWorld.compose(q.position,q.quaternion,q.scale),q.matrixWorldInverse.copy(q.matrixWorld).invert(),Bt[10]===-1)q.projectionMatrix.copy(it.projectionMatrix),q.projectionMatrixInverse.copy(it.projectionMatrixInverse);else{const Se=ft+Ht,O=I+Ht,E=Ee-xe,tt=Dt+(Lt-xe),gt=jt*I/O*Se,At=Kt*I/O*Se;q.projectionMatrix.makePerspective(E,tt,gt,At,Se,O),q.projectionMatrixInverse.copy(q.projectionMatrix).invert()}}function z(q,it){it===null?q.matrixWorld.copy(q.matrix):q.matrixWorld.multiplyMatrices(it.matrixWorld,q.matrix),q.matrixWorldInverse.copy(q.matrixWorld).invert()}this.updateCamera=function(q){if(l===null)return;let it=q.near,xt=q.far;M.texture!==null&&(M.depthNear>0&&(it=M.depthNear),M.depthFar>0&&(xt=M.depthFar)),K.near=w.near=D.near=it,K.far=w.far=D.far=xt,(ot!==K.near||ct!==K.far)&&(l.updateRenderState({depthNear:K.near,depthFar:K.far}),ot=K.near,ct=K.far),K.layers.mask=q.layers.mask|6,D.layers.mask=K.layers.mask&3,w.layers.mask=K.layers.mask&5;const Lt=q.parent,Bt=K.cameras;z(K,Lt);for(let le=0;le<Bt.length;le++)z(Bt[le],Lt);Bt.length===2?Ct(K,D,w):K.projectionMatrix.copy(D.projectionMatrix),at(q,K,Lt)};function at(q,it,xt){xt===null?q.matrix.copy(it.matrixWorld):(q.matrix.copy(xt.matrixWorld),q.matrix.invert(),q.matrix.multiply(it.matrixWorld)),q.matrix.decompose(q.position,q.quaternion,q.scale),q.updateMatrixWorld(!0),q.projectionMatrix.copy(it.projectionMatrix),q.projectionMatrixInverse.copy(it.projectionMatrixInverse),q.isPerspectiveCamera&&(q.fov=ol*2*Math.atan(1/q.projectionMatrix.elements[5]),q.zoom=1)}this.getCamera=function(){return K},this.getFoveation=function(){if(!(x===null&&S===null))return m},this.setFoveation=function(q){m=q,x!==null&&(x.fixedFoveation=q),S!==null&&S.fixedFoveation!==void 0&&(S.fixedFoveation=q)},this.hasDepthSensing=function(){return M.texture!==null},this.getDepthSensingMesh=function(){return M.getMesh(K)},this.getCameraTexture=function(q){return _[q]};let Mt=null;function wt(q,it){if(v=it.getViewerPose(p||h),b=it,v!==null){const xt=v.views;S!==null&&(t.setRenderTargetFramebuffer(T,S.framebuffer),t.setRenderTarget(T));let Lt=!1;xt.length!==K.cameras.length&&(K.cameras.length=0,Lt=!0);for(let I=0;I<xt.length;I++){const jt=xt[I];let Kt=null;if(S!==null)Kt=S.getViewport(jt);else{const Xt=g.getViewSubImage(x,jt);Kt=Xt.viewport,I===0&&(t.setRenderTargetTextures(T,Xt.colorTexture,Xt.depthStencilTexture),t.setRenderTarget(T))}let Qt=V[I];Qt===void 0&&(Qt=new ai,Qt.layers.enable(I),Qt.viewport=new Ye,V[I]=Qt),Qt.matrix.fromArray(jt.transform.matrix),Qt.matrix.decompose(Qt.position,Qt.quaternion,Qt.scale),Qt.projectionMatrix.fromArray(jt.projectionMatrix),Qt.projectionMatrixInverse.copy(Qt.projectionMatrix).invert(),Qt.viewport.set(Kt.x,Kt.y,Kt.width,Kt.height),I===0&&(K.matrix.copy(Qt.matrix),K.matrix.decompose(K.position,K.quaternion,K.scale)),Lt===!0&&K.cameras.push(Qt)}const Bt=l.enabledFeatures;if(Bt&&Bt.includes("depth-sensing")&&l.depthUsage=="gpu-optimized"&&R){g=s.getBinding();const I=g.getDepthInformation(xt[0]);I&&I.isValid&&I.texture&&M.init(I,l.renderState)}if(Bt&&Bt.includes("camera-access")&&R){t.state.unbindTexture(),g=s.getBinding();for(let I=0;I<xt.length;I++){const jt=xt[I].camera;if(jt){let Kt=_[jt];Kt||(Kt=new ry,_[jt]=Kt);const Qt=g.getCameraImage(jt);Kt.sourceTexture=Qt}}}}for(let xt=0;xt<P.length;xt++){const Lt=B[xt],Bt=P[xt];Lt!==null&&Bt!==void 0&&Bt.update(Lt,it,p||h)}Mt&&Mt(q,it),it.detectedPlanes&&s.dispatchEvent({type:"planesdetected",data:it}),b=null}const mt=new ly;mt.setAnimationLoop(wt),this.setAnimationLoop=function(q){Mt=q},this.dispose=function(){}}}const xs=new Fi,dR=new qe;function pR(r,t){function i(M,_){M.matrixAutoUpdate===!0&&M.updateMatrix(),_.value.copy(M.matrix)}function s(M,_){_.color.getRGB(M.fogColor.value,$v(r)),_.isFog?(M.fogNear.value=_.near,M.fogFar.value=_.far):_.isFogExp2&&(M.fogDensity.value=_.density)}function l(M,_,L,U,T){_.isMeshBasicMaterial||_.isMeshLambertMaterial?c(M,_):_.isMeshToonMaterial?(c(M,_),g(M,_)):_.isMeshPhongMaterial?(c(M,_),v(M,_)):_.isMeshStandardMaterial?(c(M,_),x(M,_),_.isMeshPhysicalMaterial&&S(M,_,T)):_.isMeshMatcapMaterial?(c(M,_),b(M,_)):_.isMeshDepthMaterial?c(M,_):_.isMeshDistanceMaterial?(c(M,_),R(M,_)):_.isMeshNormalMaterial?c(M,_):_.isLineBasicMaterial?(h(M,_),_.isLineDashedMaterial&&d(M,_)):_.isPointsMaterial?m(M,_,L,U):_.isSpriteMaterial?p(M,_):_.isShadowMaterial?(M.color.value.copy(_.color),M.opacity.value=_.opacity):_.isShaderMaterial&&(_.uniformsNeedUpdate=!1)}function c(M,_){M.opacity.value=_.opacity,_.color&&M.diffuse.value.copy(_.color),_.emissive&&M.emissive.value.copy(_.emissive).multiplyScalar(_.emissiveIntensity),_.map&&(M.map.value=_.map,i(_.map,M.mapTransform)),_.alphaMap&&(M.alphaMap.value=_.alphaMap,i(_.alphaMap,M.alphaMapTransform)),_.bumpMap&&(M.bumpMap.value=_.bumpMap,i(_.bumpMap,M.bumpMapTransform),M.bumpScale.value=_.bumpScale,_.side===Yn&&(M.bumpScale.value*=-1)),_.normalMap&&(M.normalMap.value=_.normalMap,i(_.normalMap,M.normalMapTransform),M.normalScale.value.copy(_.normalScale),_.side===Yn&&M.normalScale.value.negate()),_.displacementMap&&(M.displacementMap.value=_.displacementMap,i(_.displacementMap,M.displacementMapTransform),M.displacementScale.value=_.displacementScale,M.displacementBias.value=_.displacementBias),_.emissiveMap&&(M.emissiveMap.value=_.emissiveMap,i(_.emissiveMap,M.emissiveMapTransform)),_.specularMap&&(M.specularMap.value=_.specularMap,i(_.specularMap,M.specularMapTransform)),_.alphaTest>0&&(M.alphaTest.value=_.alphaTest);const L=t.get(_),U=L.envMap,T=L.envMapRotation;U&&(M.envMap.value=U,xs.copy(T),xs.x*=-1,xs.y*=-1,xs.z*=-1,U.isCubeTexture&&U.isRenderTargetTexture===!1&&(xs.y*=-1,xs.z*=-1),M.envMapRotation.value.setFromMatrix4(dR.makeRotationFromEuler(xs)),M.flipEnvMap.value=U.isCubeTexture&&U.isRenderTargetTexture===!1?-1:1,M.reflectivity.value=_.reflectivity,M.ior.value=_.ior,M.refractionRatio.value=_.refractionRatio),_.lightMap&&(M.lightMap.value=_.lightMap,M.lightMapIntensity.value=_.lightMapIntensity,i(_.lightMap,M.lightMapTransform)),_.aoMap&&(M.aoMap.value=_.aoMap,M.aoMapIntensity.value=_.aoMapIntensity,i(_.aoMap,M.aoMapTransform))}function h(M,_){M.diffuse.value.copy(_.color),M.opacity.value=_.opacity,_.map&&(M.map.value=_.map,i(_.map,M.mapTransform))}function d(M,_){M.dashSize.value=_.dashSize,M.totalSize.value=_.dashSize+_.gapSize,M.scale.value=_.scale}function m(M,_,L,U){M.diffuse.value.copy(_.color),M.opacity.value=_.opacity,M.size.value=_.size*L,M.scale.value=U*.5,_.map&&(M.map.value=_.map,i(_.map,M.uvTransform)),_.alphaMap&&(M.alphaMap.value=_.alphaMap,i(_.alphaMap,M.alphaMapTransform)),_.alphaTest>0&&(M.alphaTest.value=_.alphaTest)}function p(M,_){M.diffuse.value.copy(_.color),M.opacity.value=_.opacity,M.rotation.value=_.rotation,_.map&&(M.map.value=_.map,i(_.map,M.mapTransform)),_.alphaMap&&(M.alphaMap.value=_.alphaMap,i(_.alphaMap,M.alphaMapTransform)),_.alphaTest>0&&(M.alphaTest.value=_.alphaTest)}function v(M,_){M.specular.value.copy(_.specular),M.shininess.value=Math.max(_.shininess,1e-4)}function g(M,_){_.gradientMap&&(M.gradientMap.value=_.gradientMap)}function x(M,_){M.metalness.value=_.metalness,_.metalnessMap&&(M.metalnessMap.value=_.metalnessMap,i(_.metalnessMap,M.metalnessMapTransform)),M.roughness.value=_.roughness,_.roughnessMap&&(M.roughnessMap.value=_.roughnessMap,i(_.roughnessMap,M.roughnessMapTransform)),_.envMap&&(M.envMapIntensity.value=_.envMapIntensity)}function S(M,_,L){M.ior.value=_.ior,_.sheen>0&&(M.sheenColor.value.copy(_.sheenColor).multiplyScalar(_.sheen),M.sheenRoughness.value=_.sheenRoughness,_.sheenColorMap&&(M.sheenColorMap.value=_.sheenColorMap,i(_.sheenColorMap,M.sheenColorMapTransform)),_.sheenRoughnessMap&&(M.sheenRoughnessMap.value=_.sheenRoughnessMap,i(_.sheenRoughnessMap,M.sheenRoughnessMapTransform))),_.clearcoat>0&&(M.clearcoat.value=_.clearcoat,M.clearcoatRoughness.value=_.clearcoatRoughness,_.clearcoatMap&&(M.clearcoatMap.value=_.clearcoatMap,i(_.clearcoatMap,M.clearcoatMapTransform)),_.clearcoatRoughnessMap&&(M.clearcoatRoughnessMap.value=_.clearcoatRoughnessMap,i(_.clearcoatRoughnessMap,M.clearcoatRoughnessMapTransform)),_.clearcoatNormalMap&&(M.clearcoatNormalMap.value=_.clearcoatNormalMap,i(_.clearcoatNormalMap,M.clearcoatNormalMapTransform),M.clearcoatNormalScale.value.copy(_.clearcoatNormalScale),_.side===Yn&&M.clearcoatNormalScale.value.negate())),_.dispersion>0&&(M.dispersion.value=_.dispersion),_.iridescence>0&&(M.iridescence.value=_.iridescence,M.iridescenceIOR.value=_.iridescenceIOR,M.iridescenceThicknessMinimum.value=_.iridescenceThicknessRange[0],M.iridescenceThicknessMaximum.value=_.iridescenceThicknessRange[1],_.iridescenceMap&&(M.iridescenceMap.value=_.iridescenceMap,i(_.iridescenceMap,M.iridescenceMapTransform)),_.iridescenceThicknessMap&&(M.iridescenceThicknessMap.value=_.iridescenceThicknessMap,i(_.iridescenceThicknessMap,M.iridescenceThicknessMapTransform))),_.transmission>0&&(M.transmission.value=_.transmission,M.transmissionSamplerMap.value=L.texture,M.transmissionSamplerSize.value.set(L.width,L.height),_.transmissionMap&&(M.transmissionMap.value=_.transmissionMap,i(_.transmissionMap,M.transmissionMapTransform)),M.thickness.value=_.thickness,_.thicknessMap&&(M.thicknessMap.value=_.thicknessMap,i(_.thicknessMap,M.thicknessMapTransform)),M.attenuationDistance.value=_.attenuationDistance,M.attenuationColor.value.copy(_.attenuationColor)),_.anisotropy>0&&(M.anisotropyVector.value.set(_.anisotropy*Math.cos(_.anisotropyRotation),_.anisotropy*Math.sin(_.anisotropyRotation)),_.anisotropyMap&&(M.anisotropyMap.value=_.anisotropyMap,i(_.anisotropyMap,M.anisotropyMapTransform))),M.specularIntensity.value=_.specularIntensity,M.specularColor.value.copy(_.specularColor),_.specularColorMap&&(M.specularColorMap.value=_.specularColorMap,i(_.specularColorMap,M.specularColorMapTransform)),_.specularIntensityMap&&(M.specularIntensityMap.value=_.specularIntensityMap,i(_.specularIntensityMap,M.specularIntensityMapTransform))}function b(M,_){_.matcap&&(M.matcap.value=_.matcap)}function R(M,_){const L=t.get(_).light;M.referencePosition.value.setFromMatrixPosition(L.matrixWorld),M.nearDistance.value=L.shadow.camera.near,M.farDistance.value=L.shadow.camera.far}return{refreshFogUniforms:s,refreshMaterialUniforms:l}}function mR(r,t,i,s){let l={},c={},h=[];const d=r.getParameter(r.MAX_UNIFORM_BUFFER_BINDINGS);function m(L,U){const T=U.program;s.uniformBlockBinding(L,T)}function p(L,U){let T=l[L.id];T===void 0&&(b(L),T=v(L),l[L.id]=T,L.addEventListener("dispose",M));const P=U.program;s.updateUBOMapping(L,P);const B=t.render.frame;c[L.id]!==B&&(x(L),c[L.id]=B)}function v(L){const U=g();L.__bindingPointIndex=U;const T=r.createBuffer(),P=L.__size,B=L.usage;return r.bindBuffer(r.UNIFORM_BUFFER,T),r.bufferData(r.UNIFORM_BUFFER,P,B),r.bindBuffer(r.UNIFORM_BUFFER,null),r.bindBufferBase(r.UNIFORM_BUFFER,U,T),T}function g(){for(let L=0;L<d;L++)if(h.indexOf(L)===-1)return h.push(L),L;return console.error("THREE.WebGLRenderer: Maximum number of simultaneously usable uniforms groups reached."),0}function x(L){const U=l[L.id],T=L.uniforms,P=L.__cache;r.bindBuffer(r.UNIFORM_BUFFER,U);for(let B=0,F=T.length;B<F;B++){const G=Array.isArray(T[B])?T[B]:[T[B]];for(let D=0,w=G.length;D<w;D++){const V=G[D];if(S(V,B,D,P)===!0){const K=V.__offset,ot=Array.isArray(V.value)?V.value:[V.value];let ct=0;for(let ht=0;ht<ot.length;ht++){const N=ot[ht],J=R(N);typeof N=="number"||typeof N=="boolean"?(V.__data[0]=N,r.bufferSubData(r.UNIFORM_BUFFER,K+ct,V.__data)):N.isMatrix3?(V.__data[0]=N.elements[0],V.__data[1]=N.elements[1],V.__data[2]=N.elements[2],V.__data[3]=0,V.__data[4]=N.elements[3],V.__data[5]=N.elements[4],V.__data[6]=N.elements[5],V.__data[7]=0,V.__data[8]=N.elements[6],V.__data[9]=N.elements[7],V.__data[10]=N.elements[8],V.__data[11]=0):(N.toArray(V.__data,ct),ct+=J.storage/Float32Array.BYTES_PER_ELEMENT)}r.bufferSubData(r.UNIFORM_BUFFER,K,V.__data)}}}r.bindBuffer(r.UNIFORM_BUFFER,null)}function S(L,U,T,P){const B=L.value,F=U+"_"+T;if(P[F]===void 0)return typeof B=="number"||typeof B=="boolean"?P[F]=B:P[F]=B.clone(),!0;{const G=P[F];if(typeof B=="number"||typeof B=="boolean"){if(G!==B)return P[F]=B,!0}else if(G.equals(B)===!1)return G.copy(B),!0}return!1}function b(L){const U=L.uniforms;let T=0;const P=16;for(let F=0,G=U.length;F<G;F++){const D=Array.isArray(U[F])?U[F]:[U[F]];for(let w=0,V=D.length;w<V;w++){const K=D[w],ot=Array.isArray(K.value)?K.value:[K.value];for(let ct=0,ht=ot.length;ct<ht;ct++){const N=ot[ct],J=R(N),Y=T%P,Tt=Y%J.boundary,Ct=Y+Tt;T+=Tt,Ct!==0&&P-Ct<J.storage&&(T+=P-Ct),K.__data=new Float32Array(J.storage/Float32Array.BYTES_PER_ELEMENT),K.__offset=T,T+=J.storage}}}const B=T%P;return B>0&&(T+=P-B),L.__size=T,L.__cache={},this}function R(L){const U={boundary:0,storage:0};return typeof L=="number"||typeof L=="boolean"?(U.boundary=4,U.storage=4):L.isVector2?(U.boundary=8,U.storage=8):L.isVector3||L.isColor?(U.boundary=16,U.storage=12):L.isVector4?(U.boundary=16,U.storage=16):L.isMatrix3?(U.boundary=48,U.storage=48):L.isMatrix4?(U.boundary=64,U.storage=64):L.isTexture?console.warn("THREE.WebGLRenderer: Texture samplers can not be part of an uniforms group."):console.warn("THREE.WebGLRenderer: Unsupported uniform value type.",L),U}function M(L){const U=L.target;U.removeEventListener("dispose",M);const T=h.indexOf(U.__bindingPointIndex);h.splice(T,1),r.deleteBuffer(l[U.id]),delete l[U.id],delete c[U.id]}function _(){for(const L in l)r.deleteBuffer(l[L]);h=[],l={},c={}}return{bind:m,update:p,dispose:_}}class gR{constructor(t={}){const{canvas:i=HM(),context:s=null,depth:l=!0,stencil:c=!1,alpha:h=!1,antialias:d=!1,premultipliedAlpha:m=!0,preserveDrawingBuffer:p=!1,powerPreference:v="default",failIfMajorPerformanceCaveat:g=!1,reversedDepthBuffer:x=!1}=t;this.isWebGLRenderer=!0;let S;if(s!==null){if(typeof WebGLRenderingContext<"u"&&s instanceof WebGLRenderingContext)throw new Error("THREE.WebGLRenderer: WebGL 1 is not supported since r163.");S=s.getContextAttributes().alpha}else S=h;const b=new Uint32Array(4),R=new Int32Array(4);let M=null,_=null;const L=[],U=[];this.domElement=i,this.debug={checkShaderErrors:!0,onShaderError:null},this.autoClear=!0,this.autoClearColor=!0,this.autoClearDepth=!0,this.autoClearStencil=!0,this.sortObjects=!0,this.clippingPlanes=[],this.localClippingEnabled=!1,this.toneMapping=Ya,this.toneMappingExposure=1,this.transmissionResolutionScale=1;const T=this;let P=!1;this._outputColorSpace=jn;let B=0,F=0,G=null,D=-1,w=null;const V=new Ye,K=new Ye;let ot=null;const ct=new Me(0);let ht=0,N=i.width,J=i.height,Y=1,Tt=null,Ct=null;const z=new Ye(0,0,N,J),at=new Ye(0,0,N,J);let Mt=!1;const wt=new dp;let mt=!1,q=!1;const it=new qe,xt=new X,Lt=new Ye,Bt={background:null,fog:null,environment:null,overrideMaterial:null,isScene:!0};let le=!1;function ft(){return G===null?Y:1}let I=s;function jt(C,Z){return i.getContext(C,Z)}try{const C={alpha:!0,depth:l,stencil:c,antialias:d,premultipliedAlpha:m,preserveDrawingBuffer:p,powerPreference:v,failIfMajorPerformanceCaveat:g};if("setAttribute"in i&&i.setAttribute("data-engine",`three.js r${np}`),i.addEventListener("webglcontextlost",Rt,!1),i.addEventListener("webglcontextrestored",Pt,!1),i.addEventListener("webglcontextcreationerror",Et,!1),I===null){const Z="webgl2";if(I=jt(Z,C),I===null)throw jt(Z)?new Error("Error creating WebGL context with your selected attributes."):new Error("Error creating WebGL context.")}}catch(C){throw console.error("THREE.WebGLRenderer: "+C.message),C}let Kt,Qt,Xt,Ee,Dt,Ht,xe,Se,O,E,tt,gt,At,dt,Jt,Nt,Wt,$t,Ut,Ot,ee,It,Ft,fe;function W(){Kt=new AA(I),Kt.init(),It=new lR(I,Kt),Qt=new yA(I,Kt,t,It),Xt=new rR(I,Kt),Qt.reversedDepthBuffer&&x&&Xt.buffers.depth.setReversed(!0),Ee=new wA(I),Dt=new Y1,Ht=new oR(I,Kt,Xt,Dt,Qt,It,Ee),xe=new SA(T),Se=new bA(T),O=new PE(I),Ft=new _A(I,O),E=new RA(I,O,Ee,Ft),tt=new UA(I,E,O,Ee),Ut=new DA(I,Qt,Ht),Nt=new xA(Dt),gt=new j1(T,xe,Se,Kt,Qt,Ft,Nt),At=new pR(T,Dt),dt=new Z1,Jt=new eR(Kt),$t=new gA(T,xe,Se,Xt,tt,S,m),Wt=new aR(T,tt,Qt),fe=new mR(I,Ee,Qt,Xt),Ot=new vA(I,Kt,Ee),ee=new CA(I,Kt,Ee),Ee.programs=gt.programs,T.capabilities=Qt,T.extensions=Kt,T.properties=Dt,T.renderLists=dt,T.shadowMap=Wt,T.state=Xt,T.info=Ee}W();const _t=new hR(T,I);this.xr=_t,this.getContext=function(){return I},this.getContextAttributes=function(){return I.getContextAttributes()},this.forceContextLoss=function(){const C=Kt.get("WEBGL_lose_context");C&&C.loseContext()},this.forceContextRestore=function(){const C=Kt.get("WEBGL_lose_context");C&&C.restoreContext()},this.getPixelRatio=function(){return Y},this.setPixelRatio=function(C){C!==void 0&&(Y=C,this.setSize(N,J,!1))},this.getSize=function(C){return C.set(N,J)},this.setSize=function(C,Z,rt=!0){if(_t.isPresenting){console.warn("THREE.WebGLRenderer: Can't change size while VR device is presenting.");return}N=C,J=Z,i.width=Math.floor(C*Y),i.height=Math.floor(Z*Y),rt===!0&&(i.style.width=C+"px",i.style.height=Z+"px"),this.setViewport(0,0,C,Z)},this.getDrawingBufferSize=function(C){return C.set(N*Y,J*Y).floor()},this.setDrawingBufferSize=function(C,Z,rt){N=C,J=Z,Y=rt,i.width=Math.floor(C*rt),i.height=Math.floor(Z*rt),this.setViewport(0,0,C,Z)},this.getCurrentViewport=function(C){return C.copy(V)},this.getViewport=function(C){return C.copy(z)},this.setViewport=function(C,Z,rt,lt){C.isVector4?z.set(C.x,C.y,C.z,C.w):z.set(C,Z,rt,lt),Xt.viewport(V.copy(z).multiplyScalar(Y).round())},this.getScissor=function(C){return C.copy(at)},this.setScissor=function(C,Z,rt,lt){C.isVector4?at.set(C.x,C.y,C.z,C.w):at.set(C,Z,rt,lt),Xt.scissor(K.copy(at).multiplyScalar(Y).round())},this.getScissorTest=function(){return Mt},this.setScissorTest=function(C){Xt.setScissorTest(Mt=C)},this.setOpaqueSort=function(C){Tt=C},this.setTransparentSort=function(C){Ct=C},this.getClearColor=function(C){return C.copy($t.getClearColor())},this.setClearColor=function(){$t.setClearColor(...arguments)},this.getClearAlpha=function(){return $t.getClearAlpha()},this.setClearAlpha=function(){$t.setClearAlpha(...arguments)},this.clear=function(C=!0,Z=!0,rt=!0){let lt=0;if(C){let $=!1;if(G!==null){const bt=G.texture.format;$=bt===cp||bt===lp||bt===op}if($){const bt=G.texture.type,Gt=bt===Ii||bt===Cs||bt===il||bt===al||bt===ap||bt===sp,qt=$t.getClearColor(),kt=$t.getClearAlpha(),ie=qt.r,ue=qt.g,ae=qt.b;Gt?(b[0]=ie,b[1]=ue,b[2]=ae,b[3]=kt,I.clearBufferuiv(I.COLOR,0,b)):(R[0]=ie,R[1]=ue,R[2]=ae,R[3]=kt,I.clearBufferiv(I.COLOR,0,R))}else lt|=I.COLOR_BUFFER_BIT}Z&&(lt|=I.DEPTH_BUFFER_BIT),rt&&(lt|=I.STENCIL_BUFFER_BIT,this.state.buffers.stencil.setMask(4294967295)),I.clear(lt)},this.clearColor=function(){this.clear(!0,!1,!1)},this.clearDepth=function(){this.clear(!1,!0,!1)},this.clearStencil=function(){this.clear(!1,!1,!0)},this.dispose=function(){i.removeEventListener("webglcontextlost",Rt,!1),i.removeEventListener("webglcontextrestored",Pt,!1),i.removeEventListener("webglcontextcreationerror",Et,!1),$t.dispose(),dt.dispose(),Jt.dispose(),Dt.dispose(),xe.dispose(),Se.dispose(),tt.dispose(),Ft.dispose(),fe.dispose(),gt.dispose(),_t.dispose(),_t.removeEventListener("sessionstart",oi),_t.removeEventListener("sessionend",qr),Ci.stop()};function Rt(C){C.preventDefault(),console.log("THREE.WebGLRenderer: Context Lost."),P=!0}function Pt(){console.log("THREE.WebGLRenderer: Context Restored."),P=!1;const C=Ee.autoReset,Z=Wt.enabled,rt=Wt.autoUpdate,lt=Wt.needsUpdate,$=Wt.type;W(),Ee.autoReset=C,Wt.enabled=Z,Wt.autoUpdate=rt,Wt.needsUpdate=lt,Wt.type=$}function Et(C){console.error("THREE.WebGLRenderer: A WebGL context could not be created. Reason: ",C.statusMessage)}function pt(C){const Z=C.target;Z.removeEventListener("dispose",pt),zt(Z)}function zt(C){ne(C),Dt.remove(C)}function ne(C){const Z=Dt.get(C).programs;Z!==void 0&&(Z.forEach(function(rt){gt.releaseProgram(rt)}),C.isShaderMaterial&&gt.releaseShaderCache(C))}this.renderBufferDirect=function(C,Z,rt,lt,$,bt){Z===null&&(Z=Bt);const Gt=$.isMesh&&$.matrixWorld.determinant()<0,qt=pa(C,Z,rt,lt,$);Xt.setMaterial(lt,Gt);let kt=rt.index,ie=1;if(lt.wireframe===!0){if(kt=E.getWireframeAttribute(rt),kt===void 0)return;ie=2}const ue=rt.drawRange,ae=rt.attributes.position;let ve=ue.start*ie,Be=(ue.start+ue.count)*ie;bt!==null&&(ve=Math.max(ve,bt.start*ie),Be=Math.min(Be,(bt.start+bt.count)*ie)),kt!==null?(ve=Math.max(ve,0),Be=Math.min(Be,kt.count)):ae!=null&&(ve=Math.max(ve,0),Be=Math.min(Be,ae.count));const Ze=Be-ve;if(Ze<0||Ze===1/0)return;Ft.setup($,lt,qt,rt,kt);let Ie,Te=Ot;if(kt!==null&&(Ie=O.get(kt),Te=ee,Te.setIndex(Ie)),$.isMesh)lt.wireframe===!0?(Xt.setLineWidth(lt.wireframeLinewidth*ft()),Te.setMode(I.LINES)):Te.setMode(I.TRIANGLES);else if($.isLine){let Zt=lt.linewidth;Zt===void 0&&(Zt=1),Xt.setLineWidth(Zt*ft()),$.isLineSegments?Te.setMode(I.LINES):$.isLineLoop?Te.setMode(I.LINE_LOOP):Te.setMode(I.LINE_STRIP)}else $.isPoints?Te.setMode(I.POINTS):$.isSprite&&Te.setMode(I.TRIANGLES);if($.isBatchedMesh)if($._multiDrawInstances!==null)ll("THREE.WebGLRenderer: renderMultiDrawInstances has been deprecated and will be removed in r184. Append to renderMultiDraw arguments and use indirection."),Te.renderMultiDrawInstances($._multiDrawStarts,$._multiDrawCounts,$._multiDrawCount,$._multiDrawInstances);else if(Kt.get("WEBGL_multi_draw"))Te.renderMultiDraw($._multiDrawStarts,$._multiDrawCounts,$._multiDrawCount);else{const Zt=$._multiDrawStarts,Qe=$._multiDrawCounts,Le=$._multiDrawCount,bn=kt?O.get(kt).bytesPerElement:1,Hi=Dt.get(lt).currentProgram.getUniforms();for(let yn=0;yn<Le;yn++)Hi.setValue(I,"_gl_DrawID",yn),Te.render(Zt[yn]/bn,Qe[yn])}else if($.isInstancedMesh)Te.renderInstances(ve,Ze,$.count);else if(rt.isInstancedBufferGeometry){const Zt=rt._maxInstanceCount!==void 0?rt._maxInstanceCount:1/0,Qe=Math.min(rt.instanceCount,Zt);Te.renderInstances(ve,Ze,Qe)}else Te.render(ve,Ze)};function _e(C,Z,rt){C.transparent===!0&&C.side===ua&&C.forceSinglePass===!1?(C.side=Yn,C.needsUpdate=!0,qn(C,Z,rt),C.side=qa,C.needsUpdate=!0,qn(C,Z,rt),C.side=ua):qn(C,Z,rt)}this.compile=function(C,Z,rt=null){rt===null&&(rt=C),_=Jt.get(rt),_.init(Z),U.push(_),rt.traverseVisible(function($){$.isLight&&$.layers.test(Z.layers)&&(_.pushLight($),$.castShadow&&_.pushShadow($))}),C!==rt&&C.traverseVisible(function($){$.isLight&&$.layers.test(Z.layers)&&(_.pushLight($),$.castShadow&&_.pushShadow($))}),_.setupLights();const lt=new Set;return C.traverse(function($){if(!($.isMesh||$.isPoints||$.isLine||$.isSprite))return;const bt=$.material;if(bt)if(Array.isArray(bt))for(let Gt=0;Gt<bt.length;Gt++){const qt=bt[Gt];_e(qt,rt,$),lt.add(qt)}else _e(bt,rt,$),lt.add(bt)}),_=U.pop(),lt},this.compileAsync=function(C,Z,rt=null){const lt=this.compile(C,Z,rt);return new Promise($=>{function bt(){if(lt.forEach(function(Gt){Dt.get(Gt).currentProgram.isReady()&&lt.delete(Gt)}),lt.size===0){$(C);return}setTimeout(bt,10)}Kt.get("KHR_parallel_shader_compile")!==null?bt():setTimeout(bt,10)})};let Re=null;function En(C){Re&&Re(C)}function oi(){Ci.stop()}function qr(){Ci.start()}const Ci=new ly;Ci.setAnimationLoop(En),typeof self<"u"&&Ci.setContext(self),this.setAnimationLoop=function(C){Re=C,_t.setAnimationLoop(C),C===null?Ci.stop():Ci.start()},_t.addEventListener("sessionstart",oi),_t.addEventListener("sessionend",qr),this.render=function(C,Z){if(Z!==void 0&&Z.isCamera!==!0){console.error("THREE.WebGLRenderer.render: camera is not an instance of THREE.Camera.");return}if(P===!0)return;if(C.matrixWorldAutoUpdate===!0&&C.updateMatrixWorld(),Z.parent===null&&Z.matrixWorldAutoUpdate===!0&&Z.updateMatrixWorld(),_t.enabled===!0&&_t.isPresenting===!0&&(_t.cameraAutoUpdate===!0&&_t.updateCamera(Z),Z=_t.getCamera()),C.isScene===!0&&C.onBeforeRender(T,C,Z,G),_=Jt.get(C,U.length),_.init(Z),U.push(_),it.multiplyMatrices(Z.projectionMatrix,Z.matrixWorldInverse),wt.setFromProjectionMatrix(it,Bi,Z.reversedDepth),q=this.localClippingEnabled,mt=Nt.init(this.clippingPlanes,q),M=dt.get(C,L.length),M.init(),L.push(M),_t.enabled===!0&&_t.isPresenting===!0){const bt=T.xr.getDepthSensingMesh();bt!==null&&Ns(bt,Z,-1/0,T.sortObjects)}Ns(C,Z,0,T.sortObjects),M.finish(),T.sortObjects===!0&&M.sort(Tt,Ct),le=_t.enabled===!1||_t.isPresenting===!1||_t.hasDepthSensing()===!1,le&&$t.addToRenderList(M,C),this.info.render.frame++,mt===!0&&Nt.beginShadows();const rt=_.state.shadowsArray;Wt.render(rt,C,Z),mt===!0&&Nt.endShadows(),this.info.autoReset===!0&&this.info.reset();const lt=M.opaque,$=M.transmissive;if(_.setupLights(),Z.isArrayCamera){const bt=Z.cameras;if($.length>0)for(let Gt=0,qt=bt.length;Gt<qt;Gt++){const kt=bt[Gt];Ps(lt,$,C,kt)}le&&$t.render(C);for(let Gt=0,qt=bt.length;Gt<qt;Gt++){const kt=bt[Gt];Os(M,C,kt,kt.viewport)}}else $.length>0&&Ps(lt,$,C,Z),le&&$t.render(C),Os(M,C,Z);G!==null&&F===0&&(Ht.updateMultisampleRenderTarget(G),Ht.updateRenderTargetMipmap(G)),C.isScene===!0&&C.onAfterRender(T,C,Z),Ft.resetDefaultState(),D=-1,w=null,U.pop(),U.length>0?(_=U[U.length-1],mt===!0&&Nt.setGlobalState(T.clippingPlanes,_.state.camera)):_=null,L.pop(),L.length>0?M=L[L.length-1]:M=null};function Ns(C,Z,rt,lt){if(C.visible===!1)return;if(C.layers.test(Z.layers)){if(C.isGroup)rt=C.renderOrder;else if(C.isLOD)C.autoUpdate===!0&&C.update(Z);else if(C.isLight)_.pushLight(C),C.castShadow&&_.pushShadow(C);else if(C.isSprite){if(!C.frustumCulled||wt.intersectsSprite(C)){lt&&Lt.setFromMatrixPosition(C.matrixWorld).applyMatrix4(it);const Gt=tt.update(C),qt=C.material;qt.visible&&M.push(C,Gt,qt,rt,Lt.z,null)}}else if((C.isMesh||C.isLine||C.isPoints)&&(!C.frustumCulled||wt.intersectsObject(C))){const Gt=tt.update(C),qt=C.material;if(lt&&(C.boundingSphere!==void 0?(C.boundingSphere===null&&C.computeBoundingSphere(),Lt.copy(C.boundingSphere.center)):(Gt.boundingSphere===null&&Gt.computeBoundingSphere(),Lt.copy(Gt.boundingSphere.center)),Lt.applyMatrix4(C.matrixWorld).applyMatrix4(it)),Array.isArray(qt)){const kt=Gt.groups;for(let ie=0,ue=kt.length;ie<ue;ie++){const ae=kt[ie],ve=qt[ae.materialIndex];ve&&ve.visible&&M.push(C,Gt,ve,rt,Lt.z,ae)}}else qt.visible&&M.push(C,Gt,qt,rt,Lt.z,null)}}const bt=C.children;for(let Gt=0,qt=bt.length;Gt<qt;Gt++)Ns(bt[Gt],Z,rt,lt)}function Os(C,Z,rt,lt){const $=C.opaque,bt=C.transmissive,Gt=C.transparent;_.setupLightsView(rt),mt===!0&&Nt.setGlobalState(T.clippingPlanes,rt),lt&&Xt.viewport(V.copy(lt)),$.length>0&&Ka($,Z,rt),bt.length>0&&Ka(bt,Z,rt),Gt.length>0&&Ka(Gt,Z,rt),Xt.buffers.depth.setTest(!0),Xt.buffers.depth.setMask(!0),Xt.buffers.color.setMask(!0),Xt.setPolygonOffset(!1)}function Ps(C,Z,rt,lt){if((rt.isScene===!0?rt.overrideMaterial:null)!==null)return;_.state.transmissionRenderTarget[lt.id]===void 0&&(_.state.transmissionRenderTarget[lt.id]=new Ds(1,1,{generateMipmaps:!0,type:Kt.has("EXT_color_buffer_half_float")||Kt.has("EXT_color_buffer_float")?cl:Ii,minFilter:Rs,samples:4,stencilBuffer:c,resolveDepthBuffer:!1,resolveStencilBuffer:!1,colorSpace:Pe.workingColorSpace}));const bt=_.state.transmissionRenderTarget[lt.id],Gt=lt.viewport||V;bt.setSize(Gt.z*T.transmissionResolutionScale,Gt.w*T.transmissionResolutionScale);const qt=T.getRenderTarget(),kt=T.getActiveCubeFace(),ie=T.getActiveMipmapLevel();T.setRenderTarget(bt),T.getClearColor(ct),ht=T.getClearAlpha(),ht<1&&T.setClearColor(16777215,.5),T.clear(),le&&$t.render(rt);const ue=T.toneMapping;T.toneMapping=Ya;const ae=lt.viewport;if(lt.viewport!==void 0&&(lt.viewport=void 0),_.setupLightsView(lt),mt===!0&&Nt.setGlobalState(T.clippingPlanes,lt),Ka(C,rt,lt),Ht.updateMultisampleRenderTarget(bt),Ht.updateRenderTargetMipmap(bt),Kt.has("WEBGL_multisampled_render_to_texture")===!1){let ve=!1;for(let Be=0,Ze=Z.length;Be<Ze;Be++){const Ie=Z[Be],Te=Ie.object,Zt=Ie.geometry,Qe=Ie.material,Le=Ie.group;if(Qe.side===ua&&Te.layers.test(lt.layers)){const bn=Qe.side;Qe.side=Yn,Qe.needsUpdate=!0,Zr(Te,rt,lt,Zt,Qe,Le),Qe.side=bn,Qe.needsUpdate=!0,ve=!0}}ve===!0&&(Ht.updateMultisampleRenderTarget(bt),Ht.updateRenderTargetMipmap(bt))}T.setRenderTarget(qt,kt,ie),T.setClearColor(ct,ht),ae!==void 0&&(lt.viewport=ae),T.toneMapping=ue}function Ka(C,Z,rt){const lt=Z.isScene===!0?Z.overrideMaterial:null;for(let $=0,bt=C.length;$<bt;$++){const Gt=C[$],qt=Gt.object,kt=Gt.geometry,ie=Gt.group;let ue=Gt.material;ue.allowOverride===!0&&lt!==null&&(ue=lt),qt.layers.test(rt.layers)&&Zr(qt,Z,rt,kt,ue,ie)}}function Zr(C,Z,rt,lt,$,bt){C.onBeforeRender(T,Z,rt,lt,$,bt),C.modelViewMatrix.multiplyMatrices(rt.matrixWorldInverse,C.matrixWorld),C.normalMatrix.getNormalMatrix(C.modelViewMatrix),$.onBeforeRender(T,Z,rt,lt,C,bt),$.transparent===!0&&$.side===ua&&$.forceSinglePass===!1?($.side=Yn,$.needsUpdate=!0,T.renderBufferDirect(rt,Z,lt,$,C,bt),$.side=qa,$.needsUpdate=!0,T.renderBufferDirect(rt,Z,lt,$,C,bt),$.side=ua):T.renderBufferDirect(rt,Z,lt,$,C,bt),C.onAfterRender(T,Z,rt,lt,$,bt)}function qn(C,Z,rt){Z.isScene!==!0&&(Z=Bt);const lt=Dt.get(C),$=_.state.lights,bt=_.state.shadowsArray,Gt=$.state.version,qt=gt.getParameters(C,$.state,bt,Z,rt),kt=gt.getProgramCacheKey(qt);let ie=lt.programs;lt.environment=C.isMeshStandardMaterial?Z.environment:null,lt.fog=Z.fog,lt.envMap=(C.isMeshStandardMaterial?Se:xe).get(C.envMap||lt.environment),lt.envMapRotation=lt.environment!==null&&C.envMap===null?Z.environmentRotation:C.envMapRotation,ie===void 0&&(C.addEventListener("dispose",pt),ie=new Map,lt.programs=ie);let ue=ie.get(kt);if(ue!==void 0){if(lt.currentProgram===ue&&lt.lightsStateVersion===Gt)return Tn(C,qt),ue}else qt.uniforms=gt.getUniforms(C),C.onBeforeCompile(qt,T),ue=gt.acquireProgram(qt,kt),ie.set(kt,ue),lt.uniforms=qt.uniforms;const ae=lt.uniforms;return(!C.isShaderMaterial&&!C.isRawShaderMaterial||C.clipping===!0)&&(ae.clippingPlanes=Nt.uniform),Tn(C,qt),lt.needsLights=mu(C),lt.lightsStateVersion=Gt,lt.needsLights&&(ae.ambientLightColor.value=$.state.ambient,ae.lightProbe.value=$.state.probe,ae.directionalLights.value=$.state.directional,ae.directionalLightShadows.value=$.state.directionalShadow,ae.spotLights.value=$.state.spot,ae.spotLightShadows.value=$.state.spotShadow,ae.rectAreaLights.value=$.state.rectArea,ae.ltc_1.value=$.state.rectAreaLTC1,ae.ltc_2.value=$.state.rectAreaLTC2,ae.pointLights.value=$.state.point,ae.pointLightShadows.value=$.state.pointShadow,ae.hemisphereLights.value=$.state.hemi,ae.directionalShadowMap.value=$.state.directionalShadowMap,ae.directionalShadowMatrix.value=$.state.directionalShadowMatrix,ae.spotShadowMap.value=$.state.spotShadowMap,ae.spotLightMatrix.value=$.state.spotLightMatrix,ae.spotLightMap.value=$.state.spotLightMap,ae.pointShadowMap.value=$.state.pointShadowMap,ae.pointShadowMatrix.value=$.state.pointShadowMatrix),lt.currentProgram=ue,lt.uniformsList=null,ue}function on(C){if(C.uniformsList===null){const Z=C.currentProgram.getUniforms();C.uniformsList=eu.seqWithValue(Z.seq,C.uniforms)}return C.uniformsList}function Tn(C,Z){const rt=Dt.get(C);rt.outputColorSpace=Z.outputColorSpace,rt.batching=Z.batching,rt.batchingColor=Z.batchingColor,rt.instancing=Z.instancing,rt.instancingColor=Z.instancingColor,rt.instancingMorph=Z.instancingMorph,rt.skinning=Z.skinning,rt.morphTargets=Z.morphTargets,rt.morphNormals=Z.morphNormals,rt.morphColors=Z.morphColors,rt.morphTargetsCount=Z.morphTargetsCount,rt.numClippingPlanes=Z.numClippingPlanes,rt.numIntersection=Z.numClipIntersection,rt.vertexAlphas=Z.vertexAlphas,rt.vertexTangents=Z.vertexTangents,rt.toneMapping=Z.toneMapping}function pa(C,Z,rt,lt,$){Z.isScene!==!0&&(Z=Bt),Ht.resetTextureUnits();const bt=Z.fog,Gt=lt.isMeshStandardMaterial?Z.environment:null,qt=G===null?T.outputColorSpace:G.isXRRenderTarget===!0?G.texture.colorSpace:kr,kt=(lt.isMeshStandardMaterial?Se:xe).get(lt.envMap||Gt),ie=lt.vertexColors===!0&&!!rt.attributes.color&&rt.attributes.color.itemSize===4,ue=!!rt.attributes.tangent&&(!!lt.normalMap||lt.anisotropy>0),ae=!!rt.morphAttributes.position,ve=!!rt.morphAttributes.normal,Be=!!rt.morphAttributes.color;let Ze=Ya;lt.toneMapped&&(G===null||G.isXRRenderTarget===!0)&&(Ze=T.toneMapping);const Ie=rt.morphAttributes.position||rt.morphAttributes.normal||rt.morphAttributes.color,Te=Ie!==void 0?Ie.length:0,Zt=Dt.get(lt),Qe=_.state.lights;if(mt===!0&&(q===!0||C!==w)){const pn=C===w&&lt.id===D;Nt.setState(lt,C,pn)}let Le=!1;lt.version===Zt.__version?(Zt.needsLights&&Zt.lightsStateVersion!==Qe.state.version||Zt.outputColorSpace!==qt||$.isBatchedMesh&&Zt.batching===!1||!$.isBatchedMesh&&Zt.batching===!0||$.isBatchedMesh&&Zt.batchingColor===!0&&$.colorTexture===null||$.isBatchedMesh&&Zt.batchingColor===!1&&$.colorTexture!==null||$.isInstancedMesh&&Zt.instancing===!1||!$.isInstancedMesh&&Zt.instancing===!0||$.isSkinnedMesh&&Zt.skinning===!1||!$.isSkinnedMesh&&Zt.skinning===!0||$.isInstancedMesh&&Zt.instancingColor===!0&&$.instanceColor===null||$.isInstancedMesh&&Zt.instancingColor===!1&&$.instanceColor!==null||$.isInstancedMesh&&Zt.instancingMorph===!0&&$.morphTexture===null||$.isInstancedMesh&&Zt.instancingMorph===!1&&$.morphTexture!==null||Zt.envMap!==kt||lt.fog===!0&&Zt.fog!==bt||Zt.numClippingPlanes!==void 0&&(Zt.numClippingPlanes!==Nt.numPlanes||Zt.numIntersection!==Nt.numIntersection)||Zt.vertexAlphas!==ie||Zt.vertexTangents!==ue||Zt.morphTargets!==ae||Zt.morphNormals!==ve||Zt.morphColors!==Be||Zt.toneMapping!==Ze||Zt.morphTargetsCount!==Te)&&(Le=!0):(Le=!0,Zt.__version=lt.version);let bn=Zt.currentProgram;Le===!0&&(bn=qn(lt,Z,$));let Hi=!1,yn=!1,Ja=!1;const Ce=bn.getUniforms(),Dn=Zt.uniforms;if(Xt.useProgram(bn.program)&&(Hi=!0,yn=!0,Ja=!0),lt.id!==D&&(D=lt.id,yn=!0),Hi||w!==C){Xt.buffers.depth.getReversed()&&C.reversedDepth!==!0&&(C._reversedDepth=!0,C.updateProjectionMatrix()),Ce.setValue(I,"projectionMatrix",C.projectionMatrix),Ce.setValue(I,"viewMatrix",C.matrixWorldInverse);const an=Ce.map.cameraPosition;an!==void 0&&an.setValue(I,xt.setFromMatrixPosition(C.matrixWorld)),Qt.logarithmicDepthBuffer&&Ce.setValue(I,"logDepthBufFC",2/(Math.log(C.far+1)/Math.LN2)),(lt.isMeshPhongMaterial||lt.isMeshToonMaterial||lt.isMeshLambertMaterial||lt.isMeshBasicMaterial||lt.isMeshStandardMaterial||lt.isShaderMaterial)&&Ce.setValue(I,"isOrthographic",C.isOrthographicCamera===!0),w!==C&&(w=C,yn=!0,Ja=!0)}if($.isSkinnedMesh){Ce.setOptional(I,$,"bindMatrix"),Ce.setOptional(I,$,"bindMatrixInverse");const pn=$.skeleton;pn&&(pn.boneTexture===null&&pn.computeBoneTexture(),Ce.setValue(I,"boneTexture",pn.boneTexture,Ht))}$.isBatchedMesh&&(Ce.setOptional(I,$,"batchingTexture"),Ce.setValue(I,"batchingTexture",$._matricesTexture,Ht),Ce.setOptional(I,$,"batchingIdTexture"),Ce.setValue(I,"batchingIdTexture",$._indirectTexture,Ht),Ce.setOptional(I,$,"batchingColorTexture"),$._colorsTexture!==null&&Ce.setValue(I,"batchingColorTexture",$._colorsTexture,Ht));const Pn=rt.morphAttributes;if((Pn.position!==void 0||Pn.normal!==void 0||Pn.color!==void 0)&&Ut.update($,rt,bn),(yn||Zt.receiveShadow!==$.receiveShadow)&&(Zt.receiveShadow=$.receiveShadow,Ce.setValue(I,"receiveShadow",$.receiveShadow)),lt.isMeshGouraudMaterial&&lt.envMap!==null&&(Dn.envMap.value=kt,Dn.flipEnvMap.value=kt.isCubeTexture&&kt.isRenderTargetTexture===!1?-1:1),lt.isMeshStandardMaterial&&lt.envMap===null&&Z.environment!==null&&(Dn.envMapIntensity.value=Z.environmentIntensity),yn&&(Ce.setValue(I,"toneMappingExposure",T.toneMappingExposure),Zt.needsLights&&Kr(Dn,Ja),bt&&lt.fog===!0&&At.refreshFogUniforms(Dn,bt),At.refreshMaterialUniforms(Dn,lt,Y,J,_.state.transmissionRenderTarget[C.id]),eu.upload(I,on(Zt),Dn,Ht)),lt.isShaderMaterial&&lt.uniformsNeedUpdate===!0&&(eu.upload(I,on(Zt),Dn,Ht),lt.uniformsNeedUpdate=!1),lt.isSpriteMaterial&&Ce.setValue(I,"center",$.center),Ce.setValue(I,"modelViewMatrix",$.modelViewMatrix),Ce.setValue(I,"normalMatrix",$.normalMatrix),Ce.setValue(I,"modelMatrix",$.matrixWorld),lt.isShaderMaterial||lt.isRawShaderMaterial){const pn=lt.uniformsGroups;for(let an=0,zs=pn.length;an<zs;an++){const wi=pn[an];fe.update(wi,bn),fe.bind(wi,bn)}}return bn}function Kr(C,Z){C.ambientLightColor.needsUpdate=Z,C.lightProbe.needsUpdate=Z,C.directionalLights.needsUpdate=Z,C.directionalLightShadows.needsUpdate=Z,C.pointLights.needsUpdate=Z,C.pointLightShadows.needsUpdate=Z,C.spotLights.needsUpdate=Z,C.spotLightShadows.needsUpdate=Z,C.rectAreaLights.needsUpdate=Z,C.hemisphereLights.needsUpdate=Z}function mu(C){return C.isMeshLambertMaterial||C.isMeshToonMaterial||C.isMeshPhongMaterial||C.isMeshStandardMaterial||C.isShadowMaterial||C.isShaderMaterial&&C.lights===!0}this.getActiveCubeFace=function(){return B},this.getActiveMipmapLevel=function(){return F},this.getRenderTarget=function(){return G},this.setRenderTargetTextures=function(C,Z,rt){const lt=Dt.get(C);lt.__autoAllocateDepthBuffer=C.resolveDepthBuffer===!1,lt.__autoAllocateDepthBuffer===!1&&(lt.__useRenderToTexture=!1),Dt.get(C.texture).__webglTexture=Z,Dt.get(C.depthTexture).__webglTexture=lt.__autoAllocateDepthBuffer?void 0:rt,lt.__hasExternalTextures=!0},this.setRenderTargetFramebuffer=function(C,Z){const rt=Dt.get(C);rt.__webglFramebuffer=Z,rt.__useDefaultFramebuffer=Z===void 0};const gu=I.createFramebuffer();this.setRenderTarget=function(C,Z=0,rt=0){G=C,B=Z,F=rt;let lt=!0,$=null,bt=!1,Gt=!1;if(C){const kt=Dt.get(C);if(kt.__useDefaultFramebuffer!==void 0)Xt.bindFramebuffer(I.FRAMEBUFFER,null),lt=!1;else if(kt.__webglFramebuffer===void 0)Ht.setupRenderTarget(C);else if(kt.__hasExternalTextures)Ht.rebindTextures(C,Dt.get(C.texture).__webglTexture,Dt.get(C.depthTexture).__webglTexture);else if(C.depthBuffer){const ae=C.depthTexture;if(kt.__boundDepthTexture!==ae){if(ae!==null&&Dt.has(ae)&&(C.width!==ae.image.width||C.height!==ae.image.height))throw new Error("WebGLRenderTarget: Attached DepthTexture is initialized to the incorrect size.");Ht.setupDepthRenderbuffer(C)}}const ie=C.texture;(ie.isData3DTexture||ie.isDataArrayTexture||ie.isCompressedArrayTexture)&&(Gt=!0);const ue=Dt.get(C).__webglFramebuffer;C.isWebGLCubeRenderTarget?(Array.isArray(ue[Z])?$=ue[Z][rt]:$=ue[Z],bt=!0):C.samples>0&&Ht.useMultisampledRTT(C)===!1?$=Dt.get(C).__webglMultisampledFramebuffer:Array.isArray(ue)?$=ue[rt]:$=ue,V.copy(C.viewport),K.copy(C.scissor),ot=C.scissorTest}else V.copy(z).multiplyScalar(Y).floor(),K.copy(at).multiplyScalar(Y).floor(),ot=Mt;if(rt!==0&&($=gu),Xt.bindFramebuffer(I.FRAMEBUFFER,$)&&lt&&Xt.drawBuffers(C,$),Xt.viewport(V),Xt.scissor(K),Xt.setScissorTest(ot),bt){const kt=Dt.get(C.texture);I.framebufferTexture2D(I.FRAMEBUFFER,I.COLOR_ATTACHMENT0,I.TEXTURE_CUBE_MAP_POSITIVE_X+Z,kt.__webglTexture,rt)}else if(Gt){const kt=Z;for(let ie=0;ie<C.textures.length;ie++){const ue=Dt.get(C.textures[ie]);I.framebufferTextureLayer(I.FRAMEBUFFER,I.COLOR_ATTACHMENT0+ie,ue.__webglTexture,rt,kt)}}else if(C!==null&&rt!==0){const kt=Dt.get(C.texture);I.framebufferTexture2D(I.FRAMEBUFFER,I.COLOR_ATTACHMENT0,I.TEXTURE_2D,kt.__webglTexture,rt)}D=-1},this.readRenderTargetPixels=function(C,Z,rt,lt,$,bt,Gt,qt=0){if(!(C&&C.isWebGLRenderTarget)){console.error("THREE.WebGLRenderer.readRenderTargetPixels: renderTarget is not THREE.WebGLRenderTarget.");return}let kt=Dt.get(C).__webglFramebuffer;if(C.isWebGLCubeRenderTarget&&Gt!==void 0&&(kt=kt[Gt]),kt){Xt.bindFramebuffer(I.FRAMEBUFFER,kt);try{const ie=C.textures[qt],ue=ie.format,ae=ie.type;if(!Qt.textureFormatReadable(ue)){console.error("THREE.WebGLRenderer.readRenderTargetPixels: renderTarget is not in RGBA or implementation defined format.");return}if(!Qt.textureTypeReadable(ae)){console.error("THREE.WebGLRenderer.readRenderTargetPixels: renderTarget is not in UnsignedByteType or implementation defined type.");return}Z>=0&&Z<=C.width-lt&&rt>=0&&rt<=C.height-$&&(C.textures.length>1&&I.readBuffer(I.COLOR_ATTACHMENT0+qt),I.readPixels(Z,rt,lt,$,It.convert(ue),It.convert(ae),bt))}finally{const ie=G!==null?Dt.get(G).__webglFramebuffer:null;Xt.bindFramebuffer(I.FRAMEBUFFER,ie)}}},this.readRenderTargetPixelsAsync=async function(C,Z,rt,lt,$,bt,Gt,qt=0){if(!(C&&C.isWebGLRenderTarget))throw new Error("THREE.WebGLRenderer.readRenderTargetPixels: renderTarget is not THREE.WebGLRenderTarget.");let kt=Dt.get(C).__webglFramebuffer;if(C.isWebGLCubeRenderTarget&&Gt!==void 0&&(kt=kt[Gt]),kt)if(Z>=0&&Z<=C.width-lt&&rt>=0&&rt<=C.height-$){Xt.bindFramebuffer(I.FRAMEBUFFER,kt);const ie=C.textures[qt],ue=ie.format,ae=ie.type;if(!Qt.textureFormatReadable(ue))throw new Error("THREE.WebGLRenderer.readRenderTargetPixelsAsync: renderTarget is not in RGBA or implementation defined format.");if(!Qt.textureTypeReadable(ae))throw new Error("THREE.WebGLRenderer.readRenderTargetPixelsAsync: renderTarget is not in UnsignedByteType or implementation defined type.");const ve=I.createBuffer();I.bindBuffer(I.PIXEL_PACK_BUFFER,ve),I.bufferData(I.PIXEL_PACK_BUFFER,bt.byteLength,I.STREAM_READ),C.textures.length>1&&I.readBuffer(I.COLOR_ATTACHMENT0+qt),I.readPixels(Z,rt,lt,$,It.convert(ue),It.convert(ae),0);const Be=G!==null?Dt.get(G).__webglFramebuffer:null;Xt.bindFramebuffer(I.FRAMEBUFFER,Be);const Ze=I.fenceSync(I.SYNC_GPU_COMMANDS_COMPLETE,0);return I.flush(),await GM(I,Ze,4),I.bindBuffer(I.PIXEL_PACK_BUFFER,ve),I.getBufferSubData(I.PIXEL_PACK_BUFFER,0,bt),I.deleteBuffer(ve),I.deleteSync(Ze),bt}else throw new Error("THREE.WebGLRenderer.readRenderTargetPixelsAsync: requested read bounds are out of range.")},this.copyFramebufferToTexture=function(C,Z=null,rt=0){const lt=Math.pow(2,-rt),$=Math.floor(C.image.width*lt),bt=Math.floor(C.image.height*lt),Gt=Z!==null?Z.x:0,qt=Z!==null?Z.y:0;Ht.setTexture2D(C,0),I.copyTexSubImage2D(I.TEXTURE_2D,rt,0,0,Gt,qt,$,bt),Xt.unbindTexture()};const ul=I.createFramebuffer(),Qa=I.createFramebuffer();this.copyTextureToTexture=function(C,Z,rt=null,lt=null,$=0,bt=null){bt===null&&($!==0?(ll("WebGLRenderer: copyTextureToTexture function signature has changed to support src and dst mipmap levels."),bt=$,$=0):bt=0);let Gt,qt,kt,ie,ue,ae,ve,Be,Ze;const Ie=C.isCompressedTexture?C.mipmaps[bt]:C.image;if(rt!==null)Gt=rt.max.x-rt.min.x,qt=rt.max.y-rt.min.y,kt=rt.isBox3?rt.max.z-rt.min.z:1,ie=rt.min.x,ue=rt.min.y,ae=rt.isBox3?rt.min.z:0;else{const Pn=Math.pow(2,-$);Gt=Math.floor(Ie.width*Pn),qt=Math.floor(Ie.height*Pn),C.isDataArrayTexture?kt=Ie.depth:C.isData3DTexture?kt=Math.floor(Ie.depth*Pn):kt=1,ie=0,ue=0,ae=0}lt!==null?(ve=lt.x,Be=lt.y,Ze=lt.z):(ve=0,Be=0,Ze=0);const Te=It.convert(Z.format),Zt=It.convert(Z.type);let Qe;Z.isData3DTexture?(Ht.setTexture3D(Z,0),Qe=I.TEXTURE_3D):Z.isDataArrayTexture||Z.isCompressedArrayTexture?(Ht.setTexture2DArray(Z,0),Qe=I.TEXTURE_2D_ARRAY):(Ht.setTexture2D(Z,0),Qe=I.TEXTURE_2D),I.pixelStorei(I.UNPACK_FLIP_Y_WEBGL,Z.flipY),I.pixelStorei(I.UNPACK_PREMULTIPLY_ALPHA_WEBGL,Z.premultiplyAlpha),I.pixelStorei(I.UNPACK_ALIGNMENT,Z.unpackAlignment);const Le=I.getParameter(I.UNPACK_ROW_LENGTH),bn=I.getParameter(I.UNPACK_IMAGE_HEIGHT),Hi=I.getParameter(I.UNPACK_SKIP_PIXELS),yn=I.getParameter(I.UNPACK_SKIP_ROWS),Ja=I.getParameter(I.UNPACK_SKIP_IMAGES);I.pixelStorei(I.UNPACK_ROW_LENGTH,Ie.width),I.pixelStorei(I.UNPACK_IMAGE_HEIGHT,Ie.height),I.pixelStorei(I.UNPACK_SKIP_PIXELS,ie),I.pixelStorei(I.UNPACK_SKIP_ROWS,ue),I.pixelStorei(I.UNPACK_SKIP_IMAGES,ae);const Ce=C.isDataArrayTexture||C.isData3DTexture,Dn=Z.isDataArrayTexture||Z.isData3DTexture;if(C.isDepthTexture){const Pn=Dt.get(C),pn=Dt.get(Z),an=Dt.get(Pn.__renderTarget),zs=Dt.get(pn.__renderTarget);Xt.bindFramebuffer(I.READ_FRAMEBUFFER,an.__webglFramebuffer),Xt.bindFramebuffer(I.DRAW_FRAMEBUFFER,zs.__webglFramebuffer);for(let wi=0;wi<kt;wi++)Ce&&(I.framebufferTextureLayer(I.READ_FRAMEBUFFER,I.COLOR_ATTACHMENT0,Dt.get(C).__webglTexture,$,ae+wi),I.framebufferTextureLayer(I.DRAW_FRAMEBUFFER,I.COLOR_ATTACHMENT0,Dt.get(Z).__webglTexture,bt,Ze+wi)),I.blitFramebuffer(ie,ue,Gt,qt,ve,Be,Gt,qt,I.DEPTH_BUFFER_BIT,I.NEAREST);Xt.bindFramebuffer(I.READ_FRAMEBUFFER,null),Xt.bindFramebuffer(I.DRAW_FRAMEBUFFER,null)}else if($!==0||C.isRenderTargetTexture||Dt.has(C)){const Pn=Dt.get(C),pn=Dt.get(Z);Xt.bindFramebuffer(I.READ_FRAMEBUFFER,ul),Xt.bindFramebuffer(I.DRAW_FRAMEBUFFER,Qa);for(let an=0;an<kt;an++)Ce?I.framebufferTextureLayer(I.READ_FRAMEBUFFER,I.COLOR_ATTACHMENT0,Pn.__webglTexture,$,ae+an):I.framebufferTexture2D(I.READ_FRAMEBUFFER,I.COLOR_ATTACHMENT0,I.TEXTURE_2D,Pn.__webglTexture,$),Dn?I.framebufferTextureLayer(I.DRAW_FRAMEBUFFER,I.COLOR_ATTACHMENT0,pn.__webglTexture,bt,Ze+an):I.framebufferTexture2D(I.DRAW_FRAMEBUFFER,I.COLOR_ATTACHMENT0,I.TEXTURE_2D,pn.__webglTexture,bt),$!==0?I.blitFramebuffer(ie,ue,Gt,qt,ve,Be,Gt,qt,I.COLOR_BUFFER_BIT,I.NEAREST):Dn?I.copyTexSubImage3D(Qe,bt,ve,Be,Ze+an,ie,ue,Gt,qt):I.copyTexSubImage2D(Qe,bt,ve,Be,ie,ue,Gt,qt);Xt.bindFramebuffer(I.READ_FRAMEBUFFER,null),Xt.bindFramebuffer(I.DRAW_FRAMEBUFFER,null)}else Dn?C.isDataTexture||C.isData3DTexture?I.texSubImage3D(Qe,bt,ve,Be,Ze,Gt,qt,kt,Te,Zt,Ie.data):Z.isCompressedArrayTexture?I.compressedTexSubImage3D(Qe,bt,ve,Be,Ze,Gt,qt,kt,Te,Ie.data):I.texSubImage3D(Qe,bt,ve,Be,Ze,Gt,qt,kt,Te,Zt,Ie):C.isDataTexture?I.texSubImage2D(I.TEXTURE_2D,bt,ve,Be,Gt,qt,Te,Zt,Ie.data):C.isCompressedTexture?I.compressedTexSubImage2D(I.TEXTURE_2D,bt,ve,Be,Ie.width,Ie.height,Te,Ie.data):I.texSubImage2D(I.TEXTURE_2D,bt,ve,Be,Gt,qt,Te,Zt,Ie);I.pixelStorei(I.UNPACK_ROW_LENGTH,Le),I.pixelStorei(I.UNPACK_IMAGE_HEIGHT,bn),I.pixelStorei(I.UNPACK_SKIP_PIXELS,Hi),I.pixelStorei(I.UNPACK_SKIP_ROWS,yn),I.pixelStorei(I.UNPACK_SKIP_IMAGES,Ja),bt===0&&Z.generateMipmaps&&I.generateMipmap(Qe),Xt.unbindTexture()},this.initRenderTarget=function(C){Dt.get(C).__webglFramebuffer===void 0&&Ht.setupRenderTarget(C)},this.initTexture=function(C){C.isCubeTexture?Ht.setTextureCube(C,0):C.isData3DTexture?Ht.setTexture3D(C,0):C.isDataArrayTexture||C.isCompressedArrayTexture?Ht.setTexture2DArray(C,0):Ht.setTexture2D(C,0),Xt.unbindTexture()},this.resetState=function(){B=0,F=0,G=null,Xt.reset(),Ft.reset()},typeof __THREE_DEVTOOLS__<"u"&&__THREE_DEVTOOLS__.dispatchEvent(new CustomEvent("observe",{detail:this}))}get coordinateSystem(){return Bi}get outputColorSpace(){return this._outputColorSpace}set outputColorSpace(t){this._outputColorSpace=t;const i=this.getContext();i.drawingBufferColorSpace=Pe._getDrawingBufferColorSpace(t),i.unpackColorSpace=Pe._getUnpackColorSpace()}}const Tv={type:"change"},_p={type:"start"},dy={type:"end"},qc=new hu,bv=new Xa,_R=Math.cos(70*Yv.DEG2RAD),gn=new X,Wn=2*Math.PI,je={NONE:-1,ROTATE:0,DOLLY:1,PAN:2,TOUCH_ROTATE:3,TOUCH_PAN:4,TOUCH_DOLLY_PAN:5,TOUCH_DOLLY_ROTATE:6},ld=1e-6;class vR extends NE{constructor(t,i=null){super(t,i),this.state=je.NONE,this.target=new X,this.cursor=new X,this.minDistance=0,this.maxDistance=1/0,this.minZoom=0,this.maxZoom=1/0,this.minTargetRadius=0,this.maxTargetRadius=1/0,this.minPolarAngle=0,this.maxPolarAngle=Math.PI,this.minAzimuthAngle=-1/0,this.maxAzimuthAngle=1/0,this.enableDamping=!1,this.dampingFactor=.05,this.enableZoom=!0,this.zoomSpeed=1,this.enableRotate=!0,this.rotateSpeed=1,this.keyRotateSpeed=1,this.enablePan=!0,this.panSpeed=1,this.screenSpacePanning=!0,this.keyPanSpeed=7,this.zoomToCursor=!1,this.autoRotate=!1,this.autoRotateSpeed=2,this.keys={LEFT:"ArrowLeft",UP:"ArrowUp",RIGHT:"ArrowRight",BOTTOM:"ArrowDown"},this.mouseButtons={LEFT:Br.ROTATE,MIDDLE:Br.DOLLY,RIGHT:Br.PAN},this.touches={ONE:Pr.ROTATE,TWO:Pr.DOLLY_PAN},this.target0=this.target.clone(),this.position0=this.object.position.clone(),this.zoom0=this.object.zoom,this._domElementKeyEvents=null,this._lastPosition=new X,this._lastQuaternion=new ws,this._lastTargetPosition=new X,this._quat=new ws().setFromUnitVectors(t.up,new X(0,1,0)),this._quatInverse=this._quat.clone().invert(),this._spherical=new J0,this._sphericalDelta=new J0,this._scale=1,this._panOffset=new X,this._rotateStart=new re,this._rotateEnd=new re,this._rotateDelta=new re,this._panStart=new re,this._panEnd=new re,this._panDelta=new re,this._dollyStart=new re,this._dollyEnd=new re,this._dollyDelta=new re,this._dollyDirection=new X,this._mouse=new re,this._performCursorZoom=!1,this._pointers=[],this._pointerPositions={},this._controlActive=!1,this._onPointerMove=xR.bind(this),this._onPointerDown=yR.bind(this),this._onPointerUp=SR.bind(this),this._onContextMenu=CR.bind(this),this._onMouseWheel=TR.bind(this),this._onKeyDown=bR.bind(this),this._onTouchStart=AR.bind(this),this._onTouchMove=RR.bind(this),this._onMouseDown=MR.bind(this),this._onMouseMove=ER.bind(this),this._interceptControlDown=wR.bind(this),this._interceptControlUp=DR.bind(this),this.domElement!==null&&this.connect(this.domElement),this.update()}connect(t){super.connect(t),this.domElement.addEventListener("pointerdown",this._onPointerDown),this.domElement.addEventListener("pointercancel",this._onPointerUp),this.domElement.addEventListener("contextmenu",this._onContextMenu),this.domElement.addEventListener("wheel",this._onMouseWheel,{passive:!1}),this.domElement.getRootNode().addEventListener("keydown",this._interceptControlDown,{passive:!0,capture:!0}),this.domElement.style.touchAction="none"}disconnect(){this.domElement.removeEventListener("pointerdown",this._onPointerDown),this.domElement.removeEventListener("pointermove",this._onPointerMove),this.domElement.removeEventListener("pointerup",this._onPointerUp),this.domElement.removeEventListener("pointercancel",this._onPointerUp),this.domElement.removeEventListener("wheel",this._onMouseWheel),this.domElement.removeEventListener("contextmenu",this._onContextMenu),this.stopListenToKeyEvents(),this.domElement.getRootNode().removeEventListener("keydown",this._interceptControlDown,{capture:!0}),this.domElement.style.touchAction="auto"}dispose(){this.disconnect()}getPolarAngle(){return this._spherical.phi}getAzimuthalAngle(){return this._spherical.theta}getDistance(){return this.object.position.distanceTo(this.target)}listenToKeyEvents(t){t.addEventListener("keydown",this._onKeyDown),this._domElementKeyEvents=t}stopListenToKeyEvents(){this._domElementKeyEvents!==null&&(this._domElementKeyEvents.removeEventListener("keydown",this._onKeyDown),this._domElementKeyEvents=null)}saveState(){this.target0.copy(this.target),this.position0.copy(this.object.position),this.zoom0=this.object.zoom}reset(){this.target.copy(this.target0),this.object.position.copy(this.position0),this.object.zoom=this.zoom0,this.object.updateProjectionMatrix(),this.dispatchEvent(Tv),this.update(),this.state=je.NONE}update(t=null){const i=this.object.position;gn.copy(i).sub(this.target),gn.applyQuaternion(this._quat),this._spherical.setFromVector3(gn),this.autoRotate&&this.state===je.NONE&&this._rotateLeft(this._getAutoRotationAngle(t)),this.enableDamping?(this._spherical.theta+=this._sphericalDelta.theta*this.dampingFactor,this._spherical.phi+=this._sphericalDelta.phi*this.dampingFactor):(this._spherical.theta+=this._sphericalDelta.theta,this._spherical.phi+=this._sphericalDelta.phi);let s=this.minAzimuthAngle,l=this.maxAzimuthAngle;isFinite(s)&&isFinite(l)&&(s<-Math.PI?s+=Wn:s>Math.PI&&(s-=Wn),l<-Math.PI?l+=Wn:l>Math.PI&&(l-=Wn),s<=l?this._spherical.theta=Math.max(s,Math.min(l,this._spherical.theta)):this._spherical.theta=this._spherical.theta>(s+l)/2?Math.max(s,this._spherical.theta):Math.min(l,this._spherical.theta)),this._spherical.phi=Math.max(this.minPolarAngle,Math.min(this.maxPolarAngle,this._spherical.phi)),this._spherical.makeSafe(),this.enableDamping===!0?this.target.addScaledVector(this._panOffset,this.dampingFactor):this.target.add(this._panOffset),this.target.sub(this.cursor),this.target.clampLength(this.minTargetRadius,this.maxTargetRadius),this.target.add(this.cursor);let c=!1;if(this.zoomToCursor&&this._performCursorZoom||this.object.isOrthographicCamera)this._spherical.radius=this._clampDistance(this._spherical.radius);else{const h=this._spherical.radius;this._spherical.radius=this._clampDistance(this._spherical.radius*this._scale),c=h!=this._spherical.radius}if(gn.setFromSpherical(this._spherical),gn.applyQuaternion(this._quatInverse),i.copy(this.target).add(gn),this.object.lookAt(this.target),this.enableDamping===!0?(this._sphericalDelta.theta*=1-this.dampingFactor,this._sphericalDelta.phi*=1-this.dampingFactor,this._panOffset.multiplyScalar(1-this.dampingFactor)):(this._sphericalDelta.set(0,0,0),this._panOffset.set(0,0,0)),this.zoomToCursor&&this._performCursorZoom){let h=null;if(this.object.isPerspectiveCamera){const d=gn.length();h=this._clampDistance(d*this._scale);const m=d-h;this.object.position.addScaledVector(this._dollyDirection,m),this.object.updateMatrixWorld(),c=!!m}else if(this.object.isOrthographicCamera){const d=new X(this._mouse.x,this._mouse.y,0);d.unproject(this.object);const m=this.object.zoom;this.object.zoom=Math.max(this.minZoom,Math.min(this.maxZoom,this.object.zoom/this._scale)),this.object.updateProjectionMatrix(),c=m!==this.object.zoom;const p=new X(this._mouse.x,this._mouse.y,0);p.unproject(this.object),this.object.position.sub(p).add(d),this.object.updateMatrixWorld(),h=gn.length()}else console.warn("WARNING: OrbitControls.js encountered an unknown camera type - zoom to cursor disabled."),this.zoomToCursor=!1;h!==null&&(this.screenSpacePanning?this.target.set(0,0,-1).transformDirection(this.object.matrix).multiplyScalar(h).add(this.object.position):(qc.origin.copy(this.object.position),qc.direction.set(0,0,-1).transformDirection(this.object.matrix),Math.abs(this.object.up.dot(qc.direction))<_R?this.object.lookAt(this.target):(bv.setFromNormalAndCoplanarPoint(this.object.up,this.target),qc.intersectPlane(bv,this.target))))}else if(this.object.isOrthographicCamera){const h=this.object.zoom;this.object.zoom=Math.max(this.minZoom,Math.min(this.maxZoom,this.object.zoom/this._scale)),h!==this.object.zoom&&(this.object.updateProjectionMatrix(),c=!0)}return this._scale=1,this._performCursorZoom=!1,c||this._lastPosition.distanceToSquared(this.object.position)>ld||8*(1-this._lastQuaternion.dot(this.object.quaternion))>ld||this._lastTargetPosition.distanceToSquared(this.target)>ld?(this.dispatchEvent(Tv),this._lastPosition.copy(this.object.position),this._lastQuaternion.copy(this.object.quaternion),this._lastTargetPosition.copy(this.target),!0):!1}_getAutoRotationAngle(t){return t!==null?Wn/60*this.autoRotateSpeed*t:Wn/60/60*this.autoRotateSpeed}_getZoomScale(t){const i=Math.abs(t*.01);return Math.pow(.95,this.zoomSpeed*i)}_rotateLeft(t){this._sphericalDelta.theta-=t}_rotateUp(t){this._sphericalDelta.phi-=t}_panLeft(t,i){gn.setFromMatrixColumn(i,0),gn.multiplyScalar(-t),this._panOffset.add(gn)}_panUp(t,i){this.screenSpacePanning===!0?gn.setFromMatrixColumn(i,1):(gn.setFromMatrixColumn(i,0),gn.crossVectors(this.object.up,gn)),gn.multiplyScalar(t),this._panOffset.add(gn)}_pan(t,i){const s=this.domElement;if(this.object.isPerspectiveCamera){const l=this.object.position;gn.copy(l).sub(this.target);let c=gn.length();c*=Math.tan(this.object.fov/2*Math.PI/180),this._panLeft(2*t*c/s.clientHeight,this.object.matrix),this._panUp(2*i*c/s.clientHeight,this.object.matrix)}else this.object.isOrthographicCamera?(this._panLeft(t*(this.object.right-this.object.left)/this.object.zoom/s.clientWidth,this.object.matrix),this._panUp(i*(this.object.top-this.object.bottom)/this.object.zoom/s.clientHeight,this.object.matrix)):(console.warn("WARNING: OrbitControls.js encountered an unknown camera type - pan disabled."),this.enablePan=!1)}_dollyOut(t){this.object.isPerspectiveCamera||this.object.isOrthographicCamera?this._scale/=t:(console.warn("WARNING: OrbitControls.js encountered an unknown camera type - dolly/zoom disabled."),this.enableZoom=!1)}_dollyIn(t){this.object.isPerspectiveCamera||this.object.isOrthographicCamera?this._scale*=t:(console.warn("WARNING: OrbitControls.js encountered an unknown camera type - dolly/zoom disabled."),this.enableZoom=!1)}_updateZoomParameters(t,i){if(!this.zoomToCursor)return;this._performCursorZoom=!0;const s=this.domElement.getBoundingClientRect(),l=t-s.left,c=i-s.top,h=s.width,d=s.height;this._mouse.x=l/h*2-1,this._mouse.y=-(c/d)*2+1,this._dollyDirection.set(this._mouse.x,this._mouse.y,1).unproject(this.object).sub(this.object.position).normalize()}_clampDistance(t){return Math.max(this.minDistance,Math.min(this.maxDistance,t))}_handleMouseDownRotate(t){this._rotateStart.set(t.clientX,t.clientY)}_handleMouseDownDolly(t){this._updateZoomParameters(t.clientX,t.clientX),this._dollyStart.set(t.clientX,t.clientY)}_handleMouseDownPan(t){this._panStart.set(t.clientX,t.clientY)}_handleMouseMoveRotate(t){this._rotateEnd.set(t.clientX,t.clientY),this._rotateDelta.subVectors(this._rotateEnd,this._rotateStart).multiplyScalar(this.rotateSpeed);const i=this.domElement;this._rotateLeft(Wn*this._rotateDelta.x/i.clientHeight),this._rotateUp(Wn*this._rotateDelta.y/i.clientHeight),this._rotateStart.copy(this._rotateEnd),this.update()}_handleMouseMoveDolly(t){this._dollyEnd.set(t.clientX,t.clientY),this._dollyDelta.subVectors(this._dollyEnd,this._dollyStart),this._dollyDelta.y>0?this._dollyOut(this._getZoomScale(this._dollyDelta.y)):this._dollyDelta.y<0&&this._dollyIn(this._getZoomScale(this._dollyDelta.y)),this._dollyStart.copy(this._dollyEnd),this.update()}_handleMouseMovePan(t){this._panEnd.set(t.clientX,t.clientY),this._panDelta.subVectors(this._panEnd,this._panStart).multiplyScalar(this.panSpeed),this._pan(this._panDelta.x,this._panDelta.y),this._panStart.copy(this._panEnd),this.update()}_handleMouseWheel(t){this._updateZoomParameters(t.clientX,t.clientY),t.deltaY<0?this._dollyIn(this._getZoomScale(t.deltaY)):t.deltaY>0&&this._dollyOut(this._getZoomScale(t.deltaY)),this.update()}_handleKeyDown(t){let i=!1;switch(t.code){case this.keys.UP:t.ctrlKey||t.metaKey||t.shiftKey?this.enableRotate&&this._rotateUp(Wn*this.keyRotateSpeed/this.domElement.clientHeight):this.enablePan&&this._pan(0,this.keyPanSpeed),i=!0;break;case this.keys.BOTTOM:t.ctrlKey||t.metaKey||t.shiftKey?this.enableRotate&&this._rotateUp(-Wn*this.keyRotateSpeed/this.domElement.clientHeight):this.enablePan&&this._pan(0,-this.keyPanSpeed),i=!0;break;case this.keys.LEFT:t.ctrlKey||t.metaKey||t.shiftKey?this.enableRotate&&this._rotateLeft(Wn*this.keyRotateSpeed/this.domElement.clientHeight):this.enablePan&&this._pan(this.keyPanSpeed,0),i=!0;break;case this.keys.RIGHT:t.ctrlKey||t.metaKey||t.shiftKey?this.enableRotate&&this._rotateLeft(-Wn*this.keyRotateSpeed/this.domElement.clientHeight):this.enablePan&&this._pan(-this.keyPanSpeed,0),i=!0;break}i&&(t.preventDefault(),this.update())}_handleTouchStartRotate(t){if(this._pointers.length===1)this._rotateStart.set(t.pageX,t.pageY);else{const i=this._getSecondPointerPosition(t),s=.5*(t.pageX+i.x),l=.5*(t.pageY+i.y);this._rotateStart.set(s,l)}}_handleTouchStartPan(t){if(this._pointers.length===1)this._panStart.set(t.pageX,t.pageY);else{const i=this._getSecondPointerPosition(t),s=.5*(t.pageX+i.x),l=.5*(t.pageY+i.y);this._panStart.set(s,l)}}_handleTouchStartDolly(t){const i=this._getSecondPointerPosition(t),s=t.pageX-i.x,l=t.pageY-i.y,c=Math.sqrt(s*s+l*l);this._dollyStart.set(0,c)}_handleTouchStartDollyPan(t){this.enableZoom&&this._handleTouchStartDolly(t),this.enablePan&&this._handleTouchStartPan(t)}_handleTouchStartDollyRotate(t){this.enableZoom&&this._handleTouchStartDolly(t),this.enableRotate&&this._handleTouchStartRotate(t)}_handleTouchMoveRotate(t){if(this._pointers.length==1)this._rotateEnd.set(t.pageX,t.pageY);else{const s=this._getSecondPointerPosition(t),l=.5*(t.pageX+s.x),c=.5*(t.pageY+s.y);this._rotateEnd.set(l,c)}this._rotateDelta.subVectors(this._rotateEnd,this._rotateStart).multiplyScalar(this.rotateSpeed);const i=this.domElement;this._rotateLeft(Wn*this._rotateDelta.x/i.clientHeight),this._rotateUp(Wn*this._rotateDelta.y/i.clientHeight),this._rotateStart.copy(this._rotateEnd)}_handleTouchMovePan(t){if(this._pointers.length===1)this._panEnd.set(t.pageX,t.pageY);else{const i=this._getSecondPointerPosition(t),s=.5*(t.pageX+i.x),l=.5*(t.pageY+i.y);this._panEnd.set(s,l)}this._panDelta.subVectors(this._panEnd,this._panStart).multiplyScalar(this.panSpeed),this._pan(this._panDelta.x,this._panDelta.y),this._panStart.copy(this._panEnd)}_handleTouchMoveDolly(t){const i=this._getSecondPointerPosition(t),s=t.pageX-i.x,l=t.pageY-i.y,c=Math.sqrt(s*s+l*l);this._dollyEnd.set(0,c),this._dollyDelta.set(0,Math.pow(this._dollyEnd.y/this._dollyStart.y,this.zoomSpeed)),this._dollyOut(this._dollyDelta.y),this._dollyStart.copy(this._dollyEnd);const h=(t.pageX+i.x)*.5,d=(t.pageY+i.y)*.5;this._updateZoomParameters(h,d)}_handleTouchMoveDollyPan(t){this.enableZoom&&this._handleTouchMoveDolly(t),this.enablePan&&this._handleTouchMovePan(t)}_handleTouchMoveDollyRotate(t){this.enableZoom&&this._handleTouchMoveDolly(t),this.enableRotate&&this._handleTouchMoveRotate(t)}_addPointer(t){this._pointers.push(t.pointerId)}_removePointer(t){delete this._pointerPositions[t.pointerId];for(let i=0;i<this._pointers.length;i++)if(this._pointers[i]==t.pointerId){this._pointers.splice(i,1);return}}_isTrackingPointer(t){for(let i=0;i<this._pointers.length;i++)if(this._pointers[i]==t.pointerId)return!0;return!1}_trackPointer(t){let i=this._pointerPositions[t.pointerId];i===void 0&&(i=new re,this._pointerPositions[t.pointerId]=i),i.set(t.pageX,t.pageY)}_getSecondPointerPosition(t){const i=t.pointerId===this._pointers[0]?this._pointers[1]:this._pointers[0];return this._pointerPositions[i]}_customWheelEvent(t){const i=t.deltaMode,s={clientX:t.clientX,clientY:t.clientY,deltaY:t.deltaY};switch(i){case 1:s.deltaY*=16;break;case 2:s.deltaY*=100;break}return t.ctrlKey&&!this._controlActive&&(s.deltaY*=10),s}}function yR(r){this.enabled!==!1&&(this._pointers.length===0&&(this.domElement.setPointerCapture(r.pointerId),this.domElement.addEventListener("pointermove",this._onPointerMove),this.domElement.addEventListener("pointerup",this._onPointerUp)),!this._isTrackingPointer(r)&&(this._addPointer(r),r.pointerType==="touch"?this._onTouchStart(r):this._onMouseDown(r)))}function xR(r){this.enabled!==!1&&(r.pointerType==="touch"?this._onTouchMove(r):this._onMouseMove(r))}function SR(r){switch(this._removePointer(r),this._pointers.length){case 0:this.domElement.releasePointerCapture(r.pointerId),this.domElement.removeEventListener("pointermove",this._onPointerMove),this.domElement.removeEventListener("pointerup",this._onPointerUp),this.dispatchEvent(dy),this.state=je.NONE;break;case 1:const t=this._pointers[0],i=this._pointerPositions[t];this._onTouchStart({pointerId:t,pageX:i.x,pageY:i.y});break}}function MR(r){let t;switch(r.button){case 0:t=this.mouseButtons.LEFT;break;case 1:t=this.mouseButtons.MIDDLE;break;case 2:t=this.mouseButtons.RIGHT;break;default:t=-1}switch(t){case Br.DOLLY:if(this.enableZoom===!1)return;this._handleMouseDownDolly(r),this.state=je.DOLLY;break;case Br.ROTATE:if(r.ctrlKey||r.metaKey||r.shiftKey){if(this.enablePan===!1)return;this._handleMouseDownPan(r),this.state=je.PAN}else{if(this.enableRotate===!1)return;this._handleMouseDownRotate(r),this.state=je.ROTATE}break;case Br.PAN:if(r.ctrlKey||r.metaKey||r.shiftKey){if(this.enableRotate===!1)return;this._handleMouseDownRotate(r),this.state=je.ROTATE}else{if(this.enablePan===!1)return;this._handleMouseDownPan(r),this.state=je.PAN}break;default:this.state=je.NONE}this.state!==je.NONE&&this.dispatchEvent(_p)}function ER(r){switch(this.state){case je.ROTATE:if(this.enableRotate===!1)return;this._handleMouseMoveRotate(r);break;case je.DOLLY:if(this.enableZoom===!1)return;this._handleMouseMoveDolly(r);break;case je.PAN:if(this.enablePan===!1)return;this._handleMouseMovePan(r);break}}function TR(r){this.enabled===!1||this.enableZoom===!1||this.state!==je.NONE||(r.preventDefault(),this.dispatchEvent(_p),this._handleMouseWheel(this._customWheelEvent(r)),this.dispatchEvent(dy))}function bR(r){this.enabled!==!1&&this._handleKeyDown(r)}function AR(r){switch(this._trackPointer(r),this._pointers.length){case 1:switch(this.touches.ONE){case Pr.ROTATE:if(this.enableRotate===!1)return;this._handleTouchStartRotate(r),this.state=je.TOUCH_ROTATE;break;case Pr.PAN:if(this.enablePan===!1)return;this._handleTouchStartPan(r),this.state=je.TOUCH_PAN;break;default:this.state=je.NONE}break;case 2:switch(this.touches.TWO){case Pr.DOLLY_PAN:if(this.enableZoom===!1&&this.enablePan===!1)return;this._handleTouchStartDollyPan(r),this.state=je.TOUCH_DOLLY_PAN;break;case Pr.DOLLY_ROTATE:if(this.enableZoom===!1&&this.enableRotate===!1)return;this._handleTouchStartDollyRotate(r),this.state=je.TOUCH_DOLLY_ROTATE;break;default:this.state=je.NONE}break;default:this.state=je.NONE}this.state!==je.NONE&&this.dispatchEvent(_p)}function RR(r){switch(this._trackPointer(r),this.state){case je.TOUCH_ROTATE:if(this.enableRotate===!1)return;this._handleTouchMoveRotate(r),this.update();break;case je.TOUCH_PAN:if(this.enablePan===!1)return;this._handleTouchMovePan(r),this.update();break;case je.TOUCH_DOLLY_PAN:if(this.enableZoom===!1&&this.enablePan===!1)return;this._handleTouchMoveDollyPan(r),this.update();break;case je.TOUCH_DOLLY_ROTATE:if(this.enableZoom===!1&&this.enableRotate===!1)return;this._handleTouchMoveDollyRotate(r),this.update();break;default:this.state=je.NONE}}function CR(r){this.enabled!==!1&&r.preventDefault()}function wR(r){r.key==="Control"&&(this._controlActive=!0,this.domElement.getRootNode().addEventListener("keyup",this._interceptControlUp,{passive:!0,capture:!0}))}function DR(r){r.key==="Control"&&(this._controlActive=!1,this.domElement.getRootNode().removeEventListener("keyup",this._interceptControlUp,{passive:!0,capture:!0}))}const UR={entity:"#c1798c",fact:"#eee0e2",claim:"#d9c0c7",source:"#d7c49a"},LR=["entity","fact","claim","source"],Av=r=>(r==null?void 0:r.type)==="entity"?r.label:(r==null?void 0:r.text)||(r==null?void 0:r.label)||(r==null?void 0:r.kind);function Rv(r){const t=String(r||"Memory").slice(0,34),i=document.createElement("canvas");i.width=Math.min(640,Math.max(190,Math.ceil(t.length*31+40))),i.height=76;const s=i.getContext("2d");s.fillStyle="rgba(91,54,67,.97)",s.strokeStyle="rgba(247,213,222,.9)",s.lineWidth=2,s.beginPath(),s.roundRect(2,2,i.width-4,72,15),s.fill(),s.stroke(),s.fillStyle="#fff8f4",s.font="600 31px Georgia, serif",s.textBaseline="middle",s.fillText(t,19,39,i.width-38);const l=new ME(i);l.colorSpace=jn;const c=new ny({map:l,transparent:!0,depthWrite:!1,sizeAttenuation:!0}),h=new pE(c),d=.52;return h.scale.set(Math.max(1.4,Math.min(3.5,i.width/i.height*d)),d,1),h}function NR({nodes:r,edges:t,positions:i,selectedId:s,onSelect:l,onReady:c,onFailure:h,reducedMotion:d}){const m=oe.useRef(null),p=oe.useRef({onSelect:l,onReady:c,onFailure:h});p.current={onSelect:l,onReady:c,onFailure:h};const v=oe.useRef(s);v.current=s;const g=oe.useRef(null),x=oe.useRef(null),S=oe.useRef(null),b=oe.useRef(new Map),R=oe.useRef(r);R.current=r;const M=oe.useRef(null),_=oe.useRef(null),L=oe.useRef(()=>{});return oe.useEffect(()=>{var I,jt,Kt,Qt,Xt,Ee;const U=m.current;if(!U)return;(jt=(I=p.current).onReady)==null||jt.call(I,!1);let T,P,B,F,G=0,D=!1,w=()=>{},V=()=>{},K=()=>{},ot=()=>{},ct=()=>{},ht=()=>{};const N=new hE,J=U.getBoundingClientRect(),Y=r.map(Dt=>i.get(Dt.id)).filter(Boolean),Tt=Math.max(.15,...Y.map(Dt=>Math.abs(Dt[0]*9))),Ct=Math.max(.15,...Y.map(Dt=>Math.abs(Dt[1]*9))),z=J.width/Math.max(1,J.height),at=Math.max(.5,Ct*z/Tt),Mt=new Map(r.map(Dt=>{const Ht=i.get(Dt.id);return[Dt.id,Ht?new X(Ht[0]*9*at,Ht[1]*9,(Ht[2]||0)*9):null]})),wt=[],mt=new Set,q=new Set,it=new Set,xt=new re,Lt=new LE;let Bt=null;const le=()=>{!D&&T&&B&&T.render(N,B)},ft=Dt=>{const Ht=T.domElement.getBoundingClientRect();xt.x=(Dt.clientX-Ht.left)/Ht.width*2-1,xt.y=-((Dt.clientY-Ht.top)/Ht.height*2-1)};ot=Dt=>{Bt={x:Dt.clientX,y:Dt.clientY}},ct=Dt=>{var Se,O,E;if(!Bt||Math.hypot(Dt.clientX-Bt.x,Dt.clientY-Bt.y)>5){Bt=null;return}Bt=null,ft(Dt),Lt.setFromCamera(xt,B);const Ht=Lt.intersectObjects(wt,!1)[0],xe=Ht&&((Se=Ht.object.userData.nodeIds)==null?void 0:Se[Ht.instanceId]);xe&&((E=(O=p.current).onSelect)==null||E.call(O,xe))},ht=Dt=>{var Se,O,E,tt;const Ht=r.map(gt=>gt.id),xe=Ht.indexOf(v.current);if(["ArrowRight","ArrowDown","ArrowLeft","ArrowUp"].includes(Dt.key)){Dt.preventDefault();const gt=Dt.key==="ArrowRight"||Dt.key==="ArrowDown"?1:-1,At=xe<0?0:(xe+gt+Ht.length)%Ht.length;Ht[At]&&((O=(Se=p.current).onSelect)==null||O.call(Se,Ht[At]))}else Dt.key==="Enter"&&v.current?(tt=(E=p.current).onSelect)==null||tt.call(E,v.current):Dt.key==="Home"&&(Dt.preventDefault(),P.reset());le()};try{T=new gR({antialias:!0,alpha:!0,powerPreference:"high-performance"}),T.setPixelRatio(Math.min(window.devicePixelRatio||1,1.65)),T.outputColorSpace=jn,T.toneMapping=zv,T.toneMappingExposure=1.2,T.domElement.className="graph-canvas",T.domElement.setAttribute("role","application"),T.domElement.setAttribute("aria-label",`Interactive three dimensional memory map with ${r.length} loaded notes. Drag to orbit, pinch or scroll to zoom, arrow keys to select, Enter to inspect, and Home to reset.`),T.domElement.setAttribute("aria-describedby","map-keyboard-help"),T.domElement.tabIndex=0,U.replaceChildren(T.domElement),N.add(new RE(16774897,6043719,2.1));const Dt=new Z0(15247801,110,32,1.8);Dt.position.set(-4,5,8),N.add(Dt);const Ht=new Z0(16771807,62,35,1.8);Ht.position.set(7,-5,5),N.add(Ht);const xe=U.clientWidth||1,Se=U.clientHeight||1;B=new ai(39,xe/Se,.1,150),P=new vR(B,T.domElement),P.enableDamping=!d,P.dampingFactor=d?0:.075,P.enablePan=!0,P.screenSpacePanning=!0,P.minDistance=4,P.maxDistance=38,P.rotateSpeed=.78,P.zoomSpeed=.86,P.panSpeed=.72;let O=!1;P.addEventListener("start",()=>{O=!0});const E=[...Mt.values()].filter(Boolean),tt=new da().setFromPoints(E),gt=tt.isEmpty()?new X:tt.getCenter(new X),At=(tt.isEmpty()?new da().setFromCenterAndSize(gt,new X(2,2,1)):tt.clone()).expandByVector(new X(3.1,.72,0)),dt=(_t,Rt)=>{B.aspect=_t/Math.max(1,Rt),B.updateProjectionMatrix();const Pt=At.getCenter(new X),Et=At.getSize(new X),pt=Math.tan(Yv.degToRad(B.fov/2)),zt=pt*B.aspect,ne=Math.max(Et.y/(2*pt),Et.x/(2*zt))/.84+Et.z/2;B.position.set(Pt.x,Pt.y,Pt.z+Math.max(8,ne)),P.target.copy(Pt),P.update(),P.saveState()};dt(xe,Se);const Jt=[],Nt=[];for(const _t of t){const Rt=Mt.get(_t.from),Pt=Mt.get(_t.to);if(!Rt||!Pt)continue;Jt.push(Rt.x,Rt.y,Rt.z,Pt.x,Pt.y,Pt.z);const Et=new Me(_t.kind==="source"?"#bca270":_t.kind==="history"?"#d48ca0":"#bd8e9a"),pt=_t.kind==="source"?.45:.78;Nt.push(Et.r*pt,Et.g*pt,Et.b*pt,Et.r*pt,Et.g*pt,Et.b*pt)}if(Jt.length){const _t=new ri;_t.setAttribute("position",new dn(Jt,3)),_t.setAttribute("color",new dn(Nt,3));const Rt=new ay({vertexColors:!0,transparent:!0,opacity:.52,depthWrite:!1});mt.add(_t),q.add(Rt),N.add(new SE(_t,Rt))}const Wt=new Mn,$t={entity:new cu(1,28,22),fact:new cu(1,22,18),claim:new mp(1,0),source:new jr(1,1,1)};Object.values($t).forEach(_t=>mt.add(_t));let Ut=0;for(const _t of LR)for(const Rt of[!1,!0]){const Pt=r.filter(zt=>zt.type===_t&&!!zt.isContext===Rt&&Mt.get(zt.id));if(!Pt.length)continue;const Et=new TE({color:UR[_t],roughness:.24,metalness:_t==="source"?.17:.035,clearcoat:.8,clearcoatRoughness:.18,emissive:_t==="entity"?"#552837":"#35262b",emissiveIntensity:_t==="entity"?.3:.11,transparent:Rt,opacity:Rt?.52:1});q.add(Et);const pt=new G0($t[_t],Et,Pt.length);if(pt.userData.nodeIds=[],Pt.forEach((zt,ne)=>{Wt.position.copy(Mt.get(zt.id)),Wt.scale.setScalar(_t==="entity"?.43:_t==="source"?.15:.19),Wt.rotation.set(0,0,0),Wt.updateMatrix(),pt.setMatrixAt(ne,Wt.matrix),pt.userData.nodeIds[ne]=zt.id}),pt.instanceMatrix.needsUpdate=!0,pt.computeBoundingSphere(),N.add(pt),wt.push(pt),_t==="entity"){const zt=new uu(.58,.013,5,56);mt.add(zt);const ne=new su({color:"#c68998",transparent:!0,opacity:Rt?.14:.28});q.add(ne);const _e=new G0(zt,ne,Pt.length);Pt.forEach((Re,En)=>{Wt.position.copy(Mt.get(Re.id)),Wt.rotation.set(.7,.2,-.22),Wt.scale.setScalar(1),Wt.updateMatrix(),_e.setMatrixAt(En,Wt.matrix)}),_e.instanceMatrix.needsUpdate=!0,_e.computeBoundingSphere(),N.add(_e)}if(_t==="entity"&&Ut<12)for(const zt of Pt.slice(0,12-Ut)){const ne=Mt.get(zt.id),_e=Rv(Av(zt));_e.position.set(ne.x+.86,ne.y+.28,ne.z+.05),N.add(_e),it.add(_e.material.map),q.add(_e.material),Ut+=1}}const Ot=new uu(.62,.022,8,56),ee=new su({color:"#ed9cb1",transparent:!0,opacity:.95,depthTest:!1});mt.add(Ot),q.add(ee);const It=new vi(Ot,ee);It.visible=!1,It.renderOrder=20,N.add(It),M.current=It,b.current=Mt,R.current=r,g.current=N,x.current=T,S.current=P;const Ft=()=>{const{width:_t,height:Rt}=U.getBoundingClientRect();!_t||!Rt||(T.setSize(_t,Rt,!1),O?(B.aspect=_t/Rt,B.updateProjectionMatrix()):dt(_t,Rt),le())};F=new ResizeObserver(Ft),F.observe(U),Ft(),P.addEventListener("change",le);let fe=()=>{};w=()=>{D||G||(G=requestAnimationFrame(fe))},fe=()=>{if(G=0,D)return;const _t=P.update();T.render(N,B),_t&&!d&&w()},V=()=>{le(),d||w()},P.addEventListener("change",V),K=()=>w(),T.domElement.addEventListener("pointerdown",K),T.domElement.addEventListener("pointermove",K),T.domElement.addEventListener("wheel",K,{passive:!0}),T.domElement.addEventListener("pointerdown",ot),T.domElement.addEventListener("pointerup",ct),T.domElement.addEventListener("keydown",ht),L.current=le;const W=Mt.get(v.current);W&&(It.visible=!0,It.position.copy(W)),le(),(Qt=(Kt=p.current).onReady)==null||Qt.call(Kt,!0)}catch{D=!0,(Ee=(Xt=p.current).onFailure)==null||Ee.call(Xt)}return()=>{var Dt,Ht,xe;D=!0,cancelAnimationFrame(G),F==null||F.disconnect(),P==null||P.removeEventListener("change",le),P==null||P.removeEventListener("change",V),T==null||T.domElement.removeEventListener("pointerdown",K),T==null||T.domElement.removeEventListener("pointermove",K),T==null||T.domElement.removeEventListener("wheel",K),T==null||T.domElement.removeEventListener("pointerdown",ot),T==null||T.domElement.removeEventListener("pointerup",ct),T==null||T.domElement.removeEventListener("keydown",ht);for(const Se of it)Se.dispose();for(const Se of mt)Se.dispose();for(const Se of q)Se.dispose();(Ht=(Dt=_.current)==null?void 0:Dt.material.map)==null||Ht.dispose(),(xe=_.current)==null||xe.material.dispose(),P==null||P.dispose(),T==null||T.dispose(),T==null||T.forceContextLoss(),T==null||T.domElement.remove(),N.clear(),g.current=null,x.current=null,S.current=null,b.current=new Map,M.current=null,_.current=null,L.current=()=>{}}},[r,t,i,d]),oe.useEffect(()=>{var B;const U=g.current,T=b.current.get(s),P=M.current;if(P&&(P.visible=!!T,T&&P.position.copy(T)),_.current&&U&&(U.remove(_.current),(B=_.current.material.map)==null||B.dispose(),_.current.material.dispose(),_.current=null),U&&T){const F=R.current.find(D=>D.id===s),G=Rv(Av(F));G.position.set(T.x+.55,T.y+.3,T.z+.15),U.add(G),_.current=G}L.current()},[s,r]),j.jsx("div",{className:"graph-3d",ref:m})}const OR=["entity","fact","claim","source"],Zc=r=>(r==null?void 0:r.type)==="entity"?r.label||r.text:(r==null?void 0:r.text)||(r==null?void 0:r.label),PR=r=>typeof r=="string"?r:r==null?void 0:r.id,Ss=r=>typeof r=="string"?r:"";function zR({nodes:r=[],edges:t=[],coreIds:i=[],facets:s={},filters:l={scope:"all",year:"all",q:"",type:"all"},onFilter:c=()=>{},onMore:h=()=>{},hasMore:d=!1,loading:m=!1,totalMatching:p=0,selected:v,onSelect:g=()=>{},detail:x,detailLoading:S=!1,error:b,mode:R="private",brainName:M="Your memory",brainId:_,onBrainChange:L,brains:U=[],onRetry:T}){const[P,B]=oe.useState("graph"),[F,G]=oe.useState(!1),[D,w]=oe.useState(!1),V=oe.useRef(null);oe.useEffect(()=>{const ft=I=>{var jt;(I.metaKey||I.ctrlKey)&&I.key.toLowerCase()==="k"&&(I.preventDefault(),(jt=V.current)==null||jt.focus())};return document.addEventListener("keydown",ft),()=>document.removeEventListener("keydown",ft)},[]),oe.useEffect(()=>{var jt;const ft=window.matchMedia("(prefers-reduced-motion: reduce)"),I=()=>w(ft.matches);return I(),(jt=ft.addEventListener)==null||jt.call(ft,"change",I),()=>{var Kt;return(Kt=ft.removeEventListener)==null?void 0:Kt.call(ft,"change",I)}},[]);const K=oe.useMemo(()=>PS(r,i),[r,i]),ot=oe.useMemo(()=>new Set(i),[i]),ct=oe.useMemo(()=>NS(s),[s]),ht=l.year,N=!ht||String(ht).toLowerCase()==="all"?"":String(ht).toLowerCase()==="unknown"?"unknown":String(ht),J=PR(v),Y=K,Tt=oe.useMemo(()=>new Set(Y.map(ft=>ft.id)),[Y]),Ct=oe.useMemo(()=>t.map(Nv).filter(ft=>Tt.has(ft.from)&&Tt.has(ft.to)),[t,Tt]),{positions:z}=oe.useMemo(()=>BS(Y,Ct),[Y,Ct]),at=Y.filter(ft=>ot.has(ft.id)),Mt=Y.length-at.length,wt=K.find(ft=>ft.id===J)||(v&&typeof v=="object"?v:null),mt=ft=>c({...l,...ft}),q=ft=>{const I=OS(ct,N||null,ft);I&&mt({year:I})},it=ct.map(ft=>({year:ft,count:Ch(s,ft)})),xt=it.reduce((ft,I)=>ft+I.count,Ch(s,"Unknown")),Lt=(x==null?void 0:x.node)||x||null,Bt=((x==null?void 0:x.relatedNodes)||[]).filter(ft=>ft.type==="source"),le=((Lt==null?void 0:Lt.sourceIds)||(Lt==null?void 0:Lt.sources)||[]).map(ft=>{const I=typeof ft=="object"&&(ft!=null&&ft.node)?ft.node:ft,jt=typeof I=="string"?I:I==null?void 0:I.id;return Bt.find(Kt=>Kt.id===jt)||I});return j.jsxs("main",{className:`explorer ${R==="demo"?"explorer-demo":"explorer-private"}`,children:[j.jsxs("header",{className:"explorer-topbar",children:[j.jsxs("a",{className:"explorer-brand",href:R==="demo"?"/":"/dashboard","aria-label":"Serenity memory explorer",children:[j.jsx("img",{className:"brand-mark",src:"/assets/brand.svg",alt:""}),j.jsxs("span",{children:[j.jsx("strong",{children:"Serenity"}),j.jsx("small",{children:"MEMORY EXPLORER"})]})]}),j.jsx("div",{className:"topbar-center",children:R==="demo"?j.jsx("span",{className:"demo-label",children:"SYNTHETIC DEMO"}):j.jsxs(j.Fragment,{children:[j.jsx("span",{className:"private-dot","aria-hidden":"true"})," PRIVATE SPACE"]})}),j.jsxs("div",{className:"brain-picker",children:[L&&U.length>1?j.jsx("label",{className:"sr-only",htmlFor:"explorer-brain",children:"Choose memory space"}):null,L&&U.length>1?j.jsx("select",{id:"explorer-brain",value:_||BR(U,M),onChange:ft=>L(ft.target.value),children:U.map(ft=>j.jsx("option",{value:ft.id||ft.name,children:ft.name||ft.label||ft.id},ft.id||ft.name))}):j.jsx("span",{children:M}),j.jsx("span",{className:"avatar","aria-hidden":"true",children:(M||"M").slice(0,1).toUpperCase()})]})]}),j.jsxs("section",{className:"explorer-intro",children:[j.jsxs("div",{className:"intro-copy",children:[j.jsxs("div",{className:"eyebrow",children:[j.jsx("span",{})," A PERSONAL LIBRARY OF IDEAS"]}),j.jsxs("h1",{children:["Memory, ",j.jsx("em",{children:"in its place."})]}),j.jsxs("p",{children:["Explore the people, moments and ideas connected across ",R==="demo"?"this fictional collection":"your memory","."]})]}),j.jsxs("div",{className:"intro-stats",children:[j.jsx("strong",{children:Ms(p)}),j.jsx("span",{children:"memories in this view"}),j.jsx("div",{className:"stat-rule"}),j.jsx("span",{className:"stat-note",children:R==="demo"?j.jsxs(j.Fragment,{children:[j.jsx("span",{className:"demo-stats-dot"})," Entirely fictional"]}):j.jsxs(j.Fragment,{children:[j.jsx("span",{className:"private-dot"})," Private to you"]})})]})]}),j.jsxs("section",{className:"filter-panel","aria-label":"Memory filters",children:[j.jsxs("label",{className:"search-box",children:[j.jsx("span",{"aria-hidden":"true",className:"search-icon"}),j.jsx("span",{className:"sr-only",children:"Search memories"}),j.jsx("input",{ref:V,value:l.q||"",onChange:ft=>mt({q:ft.target.value}),placeholder:"Search your memories…"}),l.q?j.jsx("button",{type:"button",className:"clear-search",onClick:()=>mt({q:""}),"aria-label":"Clear search",children:"×"}):j.jsx("kbd",{children:"⌘ K"})]}),j.jsxs("label",{className:"filter-select",children:[j.jsx("span",{className:"sr-only",children:"Memory scope"}),j.jsxs("select",{"aria-label":"Memory scope",value:l.scope||"all",onChange:ft=>mt({scope:ft.target.value}),children:[j.jsx("option",{value:"all",children:"All scopes"}),j.jsx("option",{value:"private",children:"Private"}),j.jsx("option",{value:"world",children:"World"})]})]}),j.jsxs("label",{className:"filter-select type-select",children:[j.jsx("span",{className:"sr-only",children:"Memory type"}),j.jsxs("select",{"aria-label":"Memory type",value:l.type||"all",onChange:ft=>mt({type:ft.target.value}),children:[j.jsx("option",{value:"all",children:"All types"}),OR.map(ft=>j.jsx("option",{value:ft,children:IR(ft)},ft))]})]}),j.jsxs("div",{className:"view-switch",role:"group","aria-label":"Presentation",children:[j.jsxs("button",{type:"button","aria-pressed":P==="graph",onClick:()=>B("graph"),children:[j.jsx("span",{"aria-hidden":"true",children:"◌"})," Map"]}),j.jsxs("button",{type:"button","aria-pressed":P==="list",onClick:()=>{G(!1),B("list")},children:[j.jsx("span",{"aria-hidden":"true",children:"☷"})," List"]})]})]}),j.jsxs("section",{className:"year-rail","aria-label":"Browse by year",children:[j.jsxs("div",{className:"rail-heading",children:[j.jsxs("div",{children:[j.jsx("span",{className:"rail-overline",children:"TIME ATLAS"}),j.jsx("strong",{children:"Added to memory"})]}),j.jsxs("span",{className:"rail-total",children:[Ms(xt)," RECORDS"]})]}),j.jsxs("div",{className:"rail-track",children:[j.jsxs("button",{className:`year-all ${N===""?"active":""}`,"aria-pressed":N==="",type:"button",onClick:()=>mt({year:""}),children:["All time ",j.jsx("small",{children:Ms(xt)})]}),j.jsx("button",{className:"year-arrow","aria-label":"Previous year",type:"button",onClick:()=>q(-1),disabled:!ct.length||N===ct[0]||N==="unknown",children:"‹"}),j.jsx("div",{className:"year-items",children:it.map(({year:ft,count:I})=>j.jsxs("button",{className:`year-item ${N===ft?"active":""}`,type:"button","aria-pressed":N===ft,onClick:()=>mt({year:ft}),children:[j.jsx("span",{className:"year-dot"}),ft,j.jsx("small",{children:Ms(I)})]},ft))}),j.jsx("button",{className:"year-arrow","aria-label":"Next year",type:"button",onClick:()=>q(1),disabled:!ct.length||N===ct.at(-1)||N==="unknown",children:"›"}),j.jsxs("button",{className:`year-item unknown-year ${N==="unknown"?"active":""}`,type:"button","aria-pressed":N==="unknown",onClick:()=>mt({year:"unknown"}),children:[j.jsx("span",{className:"year-dot"}),"Unknown",j.jsx("small",{children:Ms(Ch(s,"unknown"))})]})]})]}),j.jsxs("div",{className:"workspace-heading",children:[j.jsxs("div",{children:[j.jsx("span",{className:"workspace-kicker",children:"YOUR COLLECTION"}),j.jsx("h2",{children:N===""?"A connected library":N==="unknown"?"Without a date":`The ${N} collection`})]}),j.jsxs("div",{className:"workspace-count",children:[j.jsx("strong",{children:Ms(p)}),j.jsx("span",{children:"matching memories"})]})]}),b?j.jsxs("div",{className:"explorer-state error-state",role:"alert",children:[j.jsx("span",{className:"state-seal",children:"!"}),j.jsx("h3",{children:"We couldn’t open this collection"}),j.jsx("p",{children:Ss((b==null?void 0:b.message)||b)||"Your memories are still private. Try again when the connection is ready."}),T&&j.jsx("button",{type:"button",className:"rose-button",onClick:T,children:"Try again"})]}):m&&!K.length?j.jsxs("div",{className:"explorer-state",role:"status",children:[j.jsx("span",{className:"loader-orbit"}),j.jsx("p",{children:"Opening your library…"})]}):p?j.jsxs("section",{className:`collection ${P==="list"?"list-mode":"graph-mode"}`,"aria-label":"Memory collection",children:[j.jsxs("div",{className:"collection-main",children:[P==="graph"?j.jsxs("div",{className:"map-stage",children:[j.jsxs("div",{className:"map-caption",children:[j.jsx("span",{className:"caption-pip"})," INTERCONNECTED IDEAS ",j.jsx("span",{className:"caption-divider",children:"·"})," SELECT A NODE TO INSPECT"]}),j.jsx("span",{id:"map-keyboard-help",className:"sr-only",children:"Drag to rotate the map; scroll or pinch to zoom. Use the arrow keys to move the selection, Enter to inspect a memory, and Home to reset the view."}),j.jsx(NR,{nodes:Y,edges:Ct,positions:z,selectedId:J,onSelect:g,reducedMotion:D,onReady:G,onFailure:()=>{G(!1),B("list")}}),!F&&j.jsxs("svg",{className:"constellation",viewBox:"0 0 1000 580",role:"group","aria-label":`Memory constellation with ${Y.length} loaded records`,children:[j.jsxs("defs",{children:[j.jsxs("radialGradient",{id:"rose-halo",children:[j.jsx("stop",{offset:"0",stopColor:"#f1c4cf",stopOpacity:".19"}),j.jsx("stop",{offset:"1",stopColor:"#f1c4cf",stopOpacity:"0"})]}),j.jsx("filter",{id:"soft-shadow",x:"-80%",y:"-80%",width:"260%",height:"260%",children:j.jsx("feDropShadow",{dx:"0",dy:"7",stdDeviation:"7",floodColor:"#442630",floodOpacity:".14"})})]}),j.jsx("ellipse",{cx:"505",cy:"300",rx:"485",ry:"255",fill:"url(#rose-halo)"}),[0,1,2].map(ft=>j.jsx("ellipse",{className:"atlas-ring",cx:"500",cy:"296",rx:145+ft*112,ry:79+ft*61,transform:`rotate(${-13+ft*8} 500 296)`},ft)),Ct.map(ft=>{const I=z.get(ft.from),jt=z.get(ft.to);if(!I||!jt)return null;const Kt=500+I[0]*940,Qt=290+I[1]*520,Xt=500+jt[0]*940,Ee=290+jt[1]*520;return j.jsx("path",{className:`memory-edge edge-${Kc(ft.kind)}`,d:`M ${Kt} ${Qt} Q ${(Kt+Xt)/2+(Ee-Qt)*.07} ${(Qt+Ee)/2-(Xt-Kt)*.06} ${Xt} ${Ee}`},ft.id||`${ft.from}:${ft.to}`)}),Y.map((ft,I)=>{const jt=z.get(ft.id)||[0,0],Kt=500+jt[0]*940,Qt=290+jt[1]*520,Xt=!ot.has(ft.id),Ee=ft.id===J,Dt=Zc(ft)||ft.kind||ft.type,Ht=ft.type==="entity"?16:ft.type==="claim"?8:7;return j.jsxs("g",{className:`map-node node-${Kc(ft.type)} ${Xt?"context-node":""} ${Ee?"selected-node":""}`,transform:`translate(${Kt} ${Qt})`,role:"button",tabIndex:"0","aria-label":`${Dt}${Xt?", related context":""}; ${ft.type}`,"aria-pressed":Ee,onClick:()=>g(ft.id),onKeyDown:xe=>{(xe.key==="Enter"||xe.key===" ")&&(xe.preventDefault(),g(ft.id))},children:[j.jsx("circle",{className:"node-halo",r:Ht*2.5}),j.jsx("circle",{className:"node-core",r:Ht,filter:"url(#soft-shadow)"}),ft.type==="entity"&&j.jsx("circle",{className:"node-rim",r:Ht+4}),I<14&&j.jsx("text",{className:"node-label",x:Ht+9,y:"4",children:FR(Dt,28)}),Xt&&j.jsxs("title",{children:["Related context: ",Dt]})]},ft.id)})]}),F&&j.jsxs("div",{className:"orbit-hint","aria-hidden":"true",children:["DRAG TO ORBIT ",j.jsx("span",{children:"·"})," PINCH TO ZOOM"]}),j.jsxs("div",{className:"map-legend",children:[j.jsxs("span",{children:[j.jsx("i",{className:"legend-core"})," Matching memory"]}),j.jsxs("span",{children:[j.jsx("i",{className:"legend-context"})," Related context"]}),j.jsxs("span",{children:[j.jsx("i",{className:"legend-link"})," Source connection"]})]})]}):j.jsx("ul",{className:"memory-list","aria-label":"Loaded memories",children:Y.map(ft=>j.jsx("li",{className:"memory-list-item",children:j.jsxs("button",{type:"button",className:`memory-row ${ft.id===J?"selected":""}`,"aria-pressed":ft.id===J,onClick:()=>g(ft.id),children:[j.jsx("span",{className:`type-mark type-${Kc(ft.type)}`,"aria-hidden":"true",children:wv(ft.type)}),j.jsxs("span",{className:"row-copy",children:[j.jsx("span",{className:"row-title",children:Zc(ft)||"Untitled memory"}),j.jsxs("span",{className:"row-meta",children:[ft.kind||ft.type," · ",ft.scope||"scope unavailable"," · ",Cv(ft)]})]}),j.jsx("span",{className:`context-tag ${ot.has(ft.id)?"":"visible"}`,children:ot.has(ft.id)?"MATCH":"RELATED"}),j.jsx("span",{className:"row-chevron","aria-hidden":"true",children:"›"})]})},ft.id))}),j.jsxs("div",{className:"collection-footer",children:[j.jsxs("span",{children:[Ms(at.length)," matching loaded ",j.jsx("span",{className:"footer-sep",children:"·"})," ",Mt," related context"]}),d&&j.jsxs("button",{type:"button",className:"load-more",onClick:h,disabled:m,children:[m?"Loading…":"Load more memories",j.jsx("span",{"aria-hidden":"true",children:" ↓"})]})]})]}),j.jsxs("aside",{className:"detail-card","aria-label":"Selected memory","aria-live":"polite",children:[j.jsxs("div",{className:"detail-topline",children:[j.jsx("span",{children:"MEMORY NOTE"}),j.jsx("span",{className:"detail-index",children:wt?String(Math.max(1,K.findIndex(ft=>ft.id===J)+1)).padStart(2,"0"):"—"})]}),S?j.jsxs("div",{className:"detail-placeholder",children:[j.jsx("span",{className:"loader-orbit small"}),j.jsx("span",{children:"Opening note…"})]}):wt&&Lt?j.jsxs(j.Fragment,{children:[j.jsxs("div",{className:"detail-kind",children:[j.jsx("span",{className:`type-mark type-${Kc(wt.type)}`,children:wv(wt.type)}),wt.type," ",j.jsx("span",{className:"detail-status",children:Ss(Lt.status||wt.status||"available")})]}),j.jsx("h3",{children:Zc(Lt)||Zc(wt)||"Untitled memory"}),j.jsx("p",{className:"detail-body",children:Ss(Lt.text)||"No text is available for this memory."}),j.jsx("div",{className:"detail-rule"}),j.jsxs("dl",{className:"detail-meta",children:[j.jsxs("div",{children:[j.jsx("dt",{children:"ADDED"}),j.jsx("dd",{children:Cv(Lt)})]}),j.jsxs("div",{children:[j.jsx("dt",{children:"SCOPE"}),j.jsx("dd",{children:Ss(Lt.scope||wt.scope)||"Unknown"})]}),Lt.confidence!=null&&j.jsxs("div",{children:[j.jsx("dt",{children:"CONFIDENCE"}),j.jsxs("dd",{children:[Math.round(Number(Lt.confidence)*100),"%"]})]})]}),le.length>0&&j.jsxs("div",{className:"source-block",children:[j.jsxs("span",{className:"source-heading",children:["SOURCES ",j.jsx("span",{children:le.length})]}),le.slice(0,4).map((ft,I)=>{const jt=typeof ft=="object"&&(ft!=null&&ft.node)?ft.node:ft,Kt=typeof jt=="string"?jt:(jt==null?void 0:jt.id)||I,Qt=typeof jt=="string"?jt:(jt==null?void 0:jt.label)||(jt==null?void 0:jt.text)||(jt==null?void 0:jt.id);return j.jsxs("button",{type:"button",className:"source-item",onClick:()=>g(jt),children:[j.jsx("span",{className:"source-icon",children:"↗"}),Ss(Qt)]},Kt)})]}),Lt.supersedes&&j.jsxs("div",{className:"source-block",children:[j.jsx("span",{className:"source-heading",children:"SUPERSEDES"}),j.jsx("div",{className:"source-item",children:Ss(Lt.supersedes)})]}),Lt.supersededBy&&j.jsxs("div",{className:"source-block",children:[j.jsx("span",{className:"source-heading",children:"SUPERSEDED BY"}),j.jsx("div",{className:"source-item",children:Ss(Lt.supersededBy)})]}),!ot.has(J)&&j.jsx("span",{className:"related-note",children:"RELATED CONTEXT · OUTSIDE MATCH COUNT"})]}):j.jsxs("div",{className:"detail-empty",children:[j.jsx("span",{className:"detail-book",children:"⌑"}),j.jsx("h3",{children:"Select a memory"}),j.jsx("p",{children:"Choose a point in the map or a note in the list to see its details and sources."})]})]})]}):j.jsxs("div",{className:"explorer-state empty-state",children:[j.jsx("span",{className:"state-seal",children:"✧"}),j.jsx("h3",{children:"No matching memories"}),j.jsxs("p",{children:["Try another year or clear a filter to explore more of your library.",R==="private"?" Remote-private memories remain excluded from this view.":""]}),j.jsx("button",{type:"button",className:"quiet-button",onClick:()=>c({...l,scope:"all",type:"all",q:"",year:""}),children:"Clear filters"})]}),j.jsxs("footer",{className:"explorer-foot",children:[j.jsx("span",{children:"Serenity keeps memory in context."}),j.jsx("span",{children:"BUILT FOR CAREFUL RECOLLECTION"})]})]})}function BR(r,t){var s;const i=r.find(l=>l.name===t||l.label===t||l.id===t);return(i==null?void 0:i.id)||(i==null?void 0:i.name)||((s=r[0])==null?void 0:s.id)||""}function IR(r){return{entity:"Entities",fact:"Facts",claim:"Claims",source:"Sources"}[r]||r}function Ms(r){return new Intl.NumberFormat().format(Number(r)||0)}function Kc(r){return String(r||"memory").toLowerCase().replace(/[^a-z0-9_-]/g,"-")}function FR(r,t){const i=String(r);return i.length>t?`${i.slice(0,t-1)}…`:i}function Cv(r){const t=Lv(r);return t?String(t).slice(0,10):"Unknown date"}function wv(r){return{entity:"✧",fact:"•",claim:"◇",source:"▧"}[r]||"•"}class py extends Error{}const cd={scope:"all",year:"",q:"",type:"all"};function ud(r,t){const i=new URLSearchParams;for(const s of["scope","year","q","type"])r[s]&&i.set(s,r[s]);return i.set("limit","100"),t&&i.set("cursor",t),i.toString()}async function Jo(r,{signal:t,fetcher:i=fetch}={}){const s=await i(r,{credentials:"same-origin",cache:"no-store",signal:t,headers:{Accept:"application/json"}});if(s.status===401)throw new py("Your session has ended. Sign in to continue.");if(!s.ok)throw new Error(s.status===503?"Memories are temporarily unavailable. Please retry.":"Unable to load memories. Please retry.");return s.json()}function Dv(r,t){const i=new Map(r.core.map(d=>[d.id,d]));for(const d of t.nodes||[])i.set(d.id,d);const s=new Map(r.context.map(d=>[d.id,d]));for(const d of t.contextNodes||[])s.set(d.id,d);for(const d of i.keys())s.delete(d);const l=[...i.values(),...s.values()].slice(0,500),c=new Set(l.map(d=>d.id)),h=new Map(r.edges.map(d=>[d.id,d]));for(const d of t.edges||[])h.set(d.id,d);return{core:[...i.values()].filter(d=>c.has(d.id)),context:[...s.values()].filter(d=>c.has(d.id)),edges:[...h.values()].filter(d=>c.has(d.source)&&c.has(d.target)),cursor:t.nextCursor||"",total:t.totalMatching||0,truncated:!!t.contextTruncated}}class HR{constructor(){this.value=0,this.controller=new AbortController}reset(){return this.controller.abort(),this.controller=new AbortController,++this.value}current(t){return t===this.value&&!this.controller.signal.aborted}}function Uv(r){const t=r.capturedAt||(r.type==="entity"&&r.dateKind==="earliest-linked-memory"?r.createdAt:null);return t?String(new Date(t).getUTCFullYear()):"unknown"}function GR(r,t,i){const s=i.q.trim().toLowerCase(),l=r.filter(m=>(i.scope==="all"||m.scope===i.scope)&&(i.type==="all"||m.type===i.type)&&(!s||[m.label,m.text,m.id].filter(Boolean).join(" ").toLowerCase().includes(s))),c=new Map;for(const m of l){const p=Uv(m);c.set(p,(c.get(p)||0)+1)}const h=l.filter(m=>!i.year||Uv(m)===i.year),d=new Set(h.map(m=>m.id));return{nodes:h,edges:t.filter(m=>d.has(m.source)&&d.has(m.target)),totalMatching:h.length,facets:{years:[...c].map(([m,p])=>({year:m,count:p})).sort((m,p)=>m.year.localeCompare(p.year)),totalMatching:l.length}}}const Or={core:[],context:[],edges:[],cursor:"",total:0,truncated:!1},$o=location.pathname.startsWith("/dashboard/");function VR(){var wt;const[r,t]=oe.useState([]),[i,s]=oe.useState(""),[l,c]=oe.useState(cd),[h,d]=oe.useState(""),[m,p]=oe.useState(Or),[v,g]=oe.useState({years:[]}),[x,S]=oe.useState(!0),[b,R]=oe.useState(""),[M,_]=oe.useState(!1),[L,U]=oe.useState(null),[T,P]=oe.useState(null),[B,F]=oe.useState(!1),[G,D]=oe.useState(null),[w,V]=oe.useState(0),K=oe.useRef(new HR),ot=oe.useRef(!1),ct=oe.useRef(null),ht=oe.useMemo(()=>"/api/inspector/v1/brains/"+encodeURIComponent(i),[i]),N=oe.useCallback(mt=>{var q;mt.name!=="AbortError"&&(mt instanceof py&&(K.current.reset(),(q=ct.current)==null||q.abort(),p(Or),g({years:[]}),F(!1),P(null),U(null),t([]),s(""),_(!0)),R(mt.message),S(!1),ot.current=!1)},[]);oe.useEffect(()=>{const mt=setTimeout(()=>d(l.q),180);return()=>clearTimeout(mt)},[l.q]),oe.useEffect(()=>{let mt=!0;const q=new AbortController;return $o?(R(""),S(!0),Jo("/api/inspector/v1/brains",{signal:q.signal}).then(it=>{var xt;mt&&(t(it.brains),s(((xt=it.brains[0])==null?void 0:xt.id)||""),it.brains.length||S(!1))}).catch(it=>{mt&&N(it)})):MS(()=>import("./demo-DcN2nZ57.js"),[]).then(it=>{mt&&(D({nodes:it.demoNodes,edges:it.demoEdges}),S(!1))}).catch(N),()=>{mt=!1,q.abort()}},[N,w]),oe.useEffect(()=>{const mt=q=>{var it,xt;(it=ct.current)==null||it.abort(),F(!1),U(null),P(null),c({...cd,...((xt=q.state)==null?void 0:xt.explorerFilters)||{}})};return addEventListener("popstate",mt),()=>removeEventListener("popstate",mt)},[]);const J=oe.useMemo(()=>({...l,q:h}),[l.scope,l.year,l.type,h]),Y=oe.useCallback(mt=>{var q;(q=ct.current)==null||q.abort(),F(!1),U(null),P(null),c(it=>{const xt={...it,...mt};return xt.year==="all"&&(xt.year=""),xt.year==="Unknown"&&(xt.year="unknown"),["scope","year","type"].some(Lt=>xt[Lt]!==it[Lt])&&history.pushState({explorerFilters:{scope:xt.scope,year:xt.year,type:xt.type}},"",location.pathname),xt})},[]);oe.useEffect(()=>{var it;(it=ct.current)==null||it.abort(),F(!1),U(null),P(null),R(""),p(Or),g({years:[]}),ot.current=!1;const mt=K.current.reset();if(!$o){if(G){const xt=GR(G.nodes,G.edges,J);p({...Or,core:xt.nodes,edges:xt.edges,total:xt.totalMatching}),g(xt.facets)}return}if(!i||M)return;S(!0),ot.current=!0;const q=K.current.controller.signal;return Promise.all([Jo(ht+"/graph?"+ud(J),{signal:q}),Jo(ht+"/facets?"+ud(J),{signal:q})]).then(([xt,Lt])=>{K.current.current(mt)&&(p(Dv(Or,xt)),g(Lt),S(!1),ot.current=!1)}).catch(xt=>{K.current.current(mt)&&N(xt)}),()=>K.current.controller.abort()},[i,J,ht,G,w,M,N]);const Tt=async()=>{if(ot.current||!m.cursor||m.core.length+m.context.length>=500)return;ot.current=!0,S(!0);const mt=K.current.value;try{const q=await Jo(ht+"/graph?"+ud(J,m.cursor),{signal:K.current.controller.signal});K.current.current(mt)&&(p(it=>Dv(it,q)),S(!1),ot.current=!1)}catch(q){K.current.current(mt)&&N(q)}},Ct=async mt=>{var it;if(typeof mt=="string"&&(mt=[...m.core,...m.context].find(xt=>xt.id===mt)||null),(it=ct.current)==null||it.abort(),F(!1),U(mt),P(null),!mt)return;if(!$o){const xt=((G==null?void 0:G.edges)||[]).filter(Bt=>Bt.source===mt.id||Bt.target===mt.id),Lt=new Set(xt.flatMap(Bt=>[Bt.source,Bt.target]));P({node:mt,relatedNodes:((G==null?void 0:G.nodes)||[]).filter(Bt=>Bt.id!==mt.id&&Lt.has(Bt.id)),edges:xt,truncated:!1});return}const q=new AbortController;ct.current=q,F(!0);try{const xt=await Jo(ht+"/nodes/"+encodeURIComponent(mt.id),{signal:q.signal});q.signal.aborted||(P(xt),F(!1))}catch(xt){q.signal.aborted||(F(!1),N(xt))}},z=mt=>{var q;K.current.reset(),(q=ct.current)==null||q.abort(),F(!1),p(Or),g({years:[]}),P(null),U(null),c(cd),s(mt)},at=oe.useMemo(()=>[...m.core,...m.context],[m.core,m.context]),Mt=oe.useMemo(()=>new Set(m.core.map(mt=>mt.id)),[m.core]);return M?j.jsxs("main",{className:"session-message",children:[j.jsx("h1",{children:"Your session has ended"}),j.jsx("p",{children:"Sign in to explore your memories."}),j.jsx("a",{href:"/login",children:"Sign in"})]}):j.jsx(zR,{nodes:at,edges:m.edges,coreIds:Mt,facets:v,filters:l,onFilter:Y,onMore:Tt,hasMore:!!m.cursor&&m.core.length+m.context.length<500,loading:x,totalMatching:m.total,selected:L,onSelect:Ct,detail:T,detailLoading:B,error:b,mode:$o?"private":"demo",brainId:i,brainName:$o?((wt=r.find(mt=>mt.id===i))==null?void 0:wt.name)||"Your memories":"Synthetic demo",onBrainChange:z,brains:r,onRetry:()=>V(mt=>mt+1)})}LS.createRoot(document.getElementById("root")).render(j.jsx(VR,{}));export{kR as c};
