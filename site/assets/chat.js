/* Bubble layout and interaction adapted from sire-chat; no persistent transcripts. */
(function(){
'use strict';
const form=document.getElementById('askForm');if(!form)return;
const input=document.getElementById('q'),thread=document.getElementById('thread'),voice=document.getElementById('voiceBtn');
const history=[];let asking=false;
// Deployment writes the public API URL here; no credential belongs in this file.
const endpoint=window.SERENITY_CHAT_ENDPOINT;
function render(text){
 const fragment=document.createDocumentFragment();
 const pattern=/```(?:[a-z]+\n)?([\s\S]*?)```|`([^`\n]+)`|\[([^\]]+)\]\(([^)\s]+)\)/g;
 let offset=0,match;
 while((match=pattern.exec(text))){fragment.append(document.createTextNode(text.slice(offset,match.index)));
  if(match[1]!==undefined){const pre=document.createElement('pre'),code=document.createElement('code');code.textContent=match[1].trim();pre.append(code);fragment.append(pre)}
  else if(match[2]!==undefined){const code=document.createElement('code');code.textContent=match[2];fragment.append(code)}
  else {let url;try{url=new URL(match[4])}catch{}
   if(url&&url.protocol==='https:'&&['serenity.sire.run','ndungu.dev','github.com'].includes(url.hostname)){
    const link=document.createElement('a');link.href=url.href;link.textContent=match[3];link.target='_blank';link.rel='noopener noreferrer';fragment.append(link);
   }else fragment.append(document.createTextNode(match[3]));
  }offset=pattern.lastIndex;
 }fragment.append(document.createTextNode(text.slice(offset)));return fragment;
}
function bubble(role,text){const row=document.createElement('div');row.className='msg '+role;const body=document.createElement('div');body.className='bubble';body.append(render(text));row.append(body);thread.append(row);thread.scrollTop=thread.scrollHeight;return body}
function error(body,message,q){body.textContent=message+' ';const retry=document.createElement('button');retry.type='button';retry.className='retry';retry.textContent='Try again';retry.onclick=()=>{if(!asking){body.parentElement.remove();ask(q,false)}};body.append(retry)}
async function ask(q,addUser=true){
 if(asking)return;asking=true;input.value='';input.disabled=true;voice.disabled=true;form.setAttribute('aria-busy','true');
 if(addUser)bubble('user',q);const pending=bubble('ai','···');pending.classList.add('pending-dots');
 const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),32000);
 try{
  if(!endpoint)throw new Error('The assistant is being configured. Please use the documentation below.');
  const response=await fetch(endpoint,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message:q,history:history.slice(-8)}),signal:controller.signal});
  if(!response.ok){const messages={429:'The chat has reached its hourly limit. Please try later or browse the documentation.',503:'The assistant is unavailable right now. You can browse the documentation below.',502:'The AI service could not answer just now.'};throw new Error(messages[response.status]||'Your message could not be sent.')}
  const data=await response.json();if(typeof data.answer!=='string'||!data.answer.trim())throw new Error('The assistant returned an empty answer.');
  pending.replaceChildren(render(data.answer));if(data.mode==='search'){const label=document.createElement('span');label.className='mode-note';label.textContent='Documentation search · a generated answer is unavailable';pending.append(label)}history.push({role:'user',content:q},{role:'assistant',content:data.answer.slice(0,3000)});if(history.length>8)history.splice(0,history.length-8);
 }catch(err){error(pending,err.name==='AbortError'?'The assistant took too long to respond.':err instanceof TypeError?'The chat could not be reached.':err.message,q)}
 finally{clearTimeout(timeout);pending.classList.remove('pending-dots');input.disabled=false;voice.disabled=false;asking=false;form.removeAttribute('aria-busy');thread.scrollTop=thread.scrollHeight;input.focus({preventScroll:true})}
}
document.querySelectorAll('[data-question]').forEach(b=>b.addEventListener('click',()=>ask(b.dataset.question)));
form.addEventListener('submit',e=>{e.preventDefault();const q=input.value.trim();if(q&&q.length<=1000)ask(q)});
const Recognition=window.SpeechRecognition||window.webkitSpeechRecognition;
if(Recognition){const recognition=new Recognition();recognition.lang='en-US';recognition.interimResults=false;voice.hidden=false;
 voice.onclick=()=>{if(voice.getAttribute('aria-pressed')==='true'){recognition.stop();return}try{recognition.start()}catch{}};
 recognition.onstart=()=>{voice.classList.add('listening');voice.setAttribute('aria-pressed','true')};
 recognition.onend=()=>{voice.classList.remove('listening');voice.setAttribute('aria-pressed','false')};
 recognition.onresult=e=>{input.value=e.results[0][0].transcript.slice(0,1000);input.focus()};
 recognition.onerror=()=>{document.getElementById('chat-status').textContent='Voice input is unavailable. Please type your message. Messages go to OpenRouter and its model provider.'};
}
})();
