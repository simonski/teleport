"use strict";
let localSessions=[],terminalView=null,terminalEpoch=0,rosterLoading=false;
const runningSession=s=>!['exited','failed'].includes(s.state);
function robotMark(){return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v3M10 3h4M5 8h14v12H5zM2 12v5M22 12v5M8 16h8" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><circle cx="9" cy="12" r="1" fill="currentColor"/><circle cx="15" cy="12" r="1" fill="currentColor"/></svg>';}
function robotButton(row,extra=''){const attention=row.attention||row.errorCode;return `<button type="button" class="robot-session secondary ${attention?'needs-attention':''} ${extra}" data-session="${escapeHTML(row.id)}" aria-label="${escapeHTML(row.id+' · '+(attention||row.state))}" title="${escapeHTML(row.id+' · '+(attention||row.state))}">${robotMark()}<span>${escapeHTML(row.id)}</span></button>`;}
function sessionIcons(){const active=localSessions.filter(runningSession);return active.slice(0,3).map(row=>robotButton(row)).join('')+(active.length>1?`<button type="button" class="secondary session-more" data-session-switch aria-label="${active.length} harness sessions">${active.length}</button>`:'');}
function ticketRobot(t){return localSessions.filter(row=>row.ticketId===t.id&&runningSession(row)).map(row=>robotButton(row,'ticket-robot')).join('');}
function nativeSessionHTML(t){const rows=localSessions.filter(row=>row.ticketId===t.id);return `<section class="native-session-panel"><h3>Harness</h3>${rows.map(row=>`<div class="session-row">${robotButton(row)}<span>${escapeHTML(row.harness)} · ${escapeHTML(row.attention||row.state)}</span></div>`).join('')||'<p>No harness attached.</p>'}${t.agent&&t.status==='Doing'&&t.delivery?.worktree.path&&!rows.some(runningSession)&&roleAllowsWrite(t.projectId)?`<button type="button" class="secondary" id="start-ticket-harness">Start local harness</button>`:''}${rows.length?'<details><summary>Direct terminal interactions</summary><div id="session-interventions"></div></details>':''}</section>`;}
function bindTerminalTicket(t,dialog){
  on('start-ticket-harness','click',()=>chooseHarness(t).catch(showError));
  const holder=byId('session-interventions');if(!holder)return;
  const guard=captureDialogCompletion(dialog);
  Promise.all(localSessions.filter(row=>row.ticketId===t.id).map(row=>pages('listSessionInterventions',{id:row.id}))).then(groups=>{if(!guard.current())return;const items=groups.flat().sort((a,b)=>a.at.localeCompare(b.at));holder.innerHTML=items.map(item=>`<p class="intervention">${escapeHTML(item.sessionId)} · ${escapeHTML(item.actor)} · ${escapeHTML(new Date(item.at).toLocaleString())} · ${escapeHTML(item.state)} (${item.written} bytes)</p>`).join('')||'<p>No direct terminal input recorded.</p>';}).catch(error=>{if(guard.current())holder.textContent=error.message;});
}
function disconnectTerminal(){terminalEpoch++;if(!terminalView)return;const viewer=terminalView;terminalView=null;clearTimeout(viewer.timer);viewer.resize.disconnect();viewer.terminal.dispose();}
function resetTerminals(){disconnectTerminal();localSessions=[];}
document.addEventListener('close',e=>{if(e.target.tagName==='TK-DIALOG')terminalEpoch++;},true);
async function refreshSessionRoster(){
  if(!session||rosterLoading)return;rosterLoading=true;const actor=session.account.id;
  try{const rows=await pages('listSessions');if(session?.account.id!==actor)return;localSessions=rows;const holder=byId('session-icons');if(holder){const html=sessionIcons();if(holder.innerHTML!==html){const focused=document.activeElement?.dataset.session;holder.innerHTML=html;if(focused)holder.querySelector(`[data-session="${CSS.escape(focused)}"]`)?.focus();}}
    document.querySelectorAll('[data-ticket-card]').forEach(card=>{const t=tickets.find(t=>t.id===Number(card.dataset.ticketCard)),holder=card.querySelector('.ticket-session');if(t&&holder){const html=ticketRobot(t);if(holder.innerHTML!==html)holder.innerHTML=html;}});
  }catch(error){if(error.status===401&&session?.account.id===actor)resetTerminals();}finally{rosterLoading=false;}
}
async function chooseHarness(ticket=null){
  const actor=session?.account.id,epoch=detailEpoch,requestEpoch=terminalEpoch;
  const [choices,rows]=await Promise.all([api('getLocalHarnesses'),pages('listSessions')]);
  if(session?.account.id!==actor||detailEpoch!==epoch||terminalEpoch!==requestEpoch)return;
  localSessions=rows;
  const existing=rows.find(row=>runningSession(row)&&(ticket?row.ticketId===ticket.id:!row.ticketId));
  if(existing){await openTerminal(existing.id);return;}
  const requestId=crypto.randomUUID(),available=projects.filter(p=>roleAllowsWrite(p.id));
  const dialog=openDialog(ticket?'Ticket harness':'Agent console',`<label>Project<select id="console-project" ${ticket?'disabled':''}>${available.map(p=>`<option value="${p.id}" ${p.id===(ticket?.projectId||projectId)?'selected':''}>${escapeHTML(p.name)}</option>`).join('')}</select></label><label>Harness<select id="console-harness">${choices.available.map(h=>`<option value="${escapeHTML(h.id)}">${escapeHTML(h.title)}</option>`).join('')}</select></label>${choices.available.length?'':'<p>No verified local harness is available.</p>'}<p id="console-status" class="save-status" role="status"></p>`,'Start harness');
  const form=byId('editor-form');form.querySelector('button[type=submit]').disabled=!choices.available.length||!available.length;
  form.onsubmit=async e=>{e.preventDefault();const button=form.querySelector('button[type=submit]');button.disabled=true;try{const row=await api(ticket?'startTicketSession':'openConsoleSession',{projectId:ticket?.projectId||Number(byId('console-project').value),...(ticket?{ticketId:ticket.id,expectedVersion:ticket.version}:{}),harness:byId('console-harness').value,requestId});dialog.close();await refreshSessionRoster();await openTerminal(row.id);}catch(error){if(dialog.open&&dialog.querySelector('form')===form&&!error.staleInteraction)byId('console-status').textContent=error.message;}finally{if(button.isConnected)button.disabled=false;}};
}
async function sessionSwitcher(){
  const actor=session?.account.id,epoch=detailEpoch;await refreshSessionRoster();if(!session||session.account.id!==actor||detailEpoch!==epoch||foregroundOpen())return;const dialog=openDialog('Harness sessions',`<div class="session-list">${localSessions.map(row=>`<div class="session-row">${robotButton(row)}<span>${escapeHTML(row.harness)} · ${escapeHTML(row.attention||row.state)}${row.ticketId?' · #'+row.ticketId:' · free console'}</span></div>`).join('')||'<p>No sessions yet.</p>'}</div><button type="button" class="secondary" id="new-console">New free console</button>`,'Close');
  byId('cancel').hidden=true;byId('editor-form').onsubmit=e=>{e.preventDefault();dialog.close();};on('new-console','click',()=>{dialog.close();chooseHarness().catch(showError);});
}
function terminalBytes(data){const bytes=new Uint8Array(data.length);for(let i=0;i<data.length;i++)bytes[i]=data.charCodeAt(i);return bytes;}
function base64Bytes(bytes){let text='';for(const byte of bytes)text+=String.fromCharCode(byte);return btoa(text);}
async function openTerminal(id){
  disconnectTerminal();const epoch=terminalEpoch,actor=session?.account.id,foregroundEpoch=detailEpoch;
  const first=await api('attachSession',{id,cursor:0});
  if(epoch!==terminalEpoch||session?.account.id!==actor||foregroundEpoch!==detailEpoch)return;
  const dialog=openDialog(id+' · '+first.session.harness,`<div class="terminal-toolbar"><span id="terminal-state" role="status">${escapeHTML(first.session.state)}</span><button type="button" class="secondary" id="terminal-escape">Send Esc</button><button type="button" class="secondary" id="terminal-interrupt">Ctrl+C</button><button type="button" class="secondary" id="terminal-stop">Stop</button><button type="button" class="secondary" id="terminal-switch">Sessions</button></div>${first.session.ticketId?'<p class="terminal-intervention-note">Direct input is recorded outside the ticket workflow.</p>':''}<div id="terminal-output" class="terminal-surface" aria-label="Harness terminal"></div><p id="terminal-error" class="save-status" role="status"></p>`,'Detach');
  dialog.classList.add('terminal-dialog');byId('cancel').hidden=true;
  const nonce=document.querySelector('meta[name=teleport-style-nonce]')?.content;const terminalDocument=new Proxy(document,{get(target,key){if(key==='createElement')return (name,...args)=>{const element=target.createElement(name,...args);if(name.toLowerCase()==='style'&&nonce)element.nonce=nonce;return element;};const value=Reflect.get(target,key,target);return typeof value==='function'?value.bind(target):value;}});
  const terminal=new Terminal({documentOverride:terminalDocument,cols:first.cols||100,rows:first.rows||28,scrollback:5000,fontFamily:'Menlo, Consolas, monospace',fontSize:13,allowProposedApi:false,screenReaderMode:true,theme:{background:'#151a17',foreground:'#e9eee5',cursor:'#f18a62'}}),fit=new FitAddon.FitAddon();
  terminal.loadAddon(fit);terminal.open(byId('terminal-output'));
  const viewer={id,terminal,fit,cursor:0,timer:null,resize:null,stopped:false,queue:[],sending:false,epoch,actor};terminalView=viewer;
  const current=()=>terminalView===viewer&&session?.account.id===actor&&epoch===terminalEpoch&&dialog.open;
  const error=message=>{if(current())byId('terminal-error').textContent=message;};
  function write(out){if(!current())return;if(out.gap&&(!viewer.gapAnnounced||viewer.cursor!==out.cursor)){terminal.reset();terminal.writeln('[Retained output is incomplete]');viewer.gapAnnounced=true;}terminal.options.disableStdin=out.session.state!=='running';viewer.cursor=out.next;if(out.data)terminal.write(terminalBytes(atob(out.data)));byId('terminal-state').textContent=out.session.attention||out.session.state;}
  async function read(){if(!current())return;try{const out=await api('attachSession',{id,cursor:viewer.cursor});if(!current())return;write(out);viewer.timer=setTimeout(read,out.data&&out.data.length>60000?10:180);}catch(e){if(current()){error(e.message);viewer.timer=setTimeout(read,1500);}}}
  async function send(){if(viewer.sending||viewer.stopped||!current()||!viewer.queue.length)return;viewer.sending=true;
    try{while(viewer.queue.length&&current()){const item=viewer.queue[0],result=await api('sendSessionInput',{id,requestId:item.id,data:base64Bytes(item.bytes)});if(result.state!=='delivered'){viewer.stopped=true;error('Input delivery '+result.state+'; further input is paused. Operation '+item.id);break;}viewer.queue.shift();}}
    catch(e){viewer.stopped=true;error(e.message+' · Input paused; operation '+(viewer.queue[0]?.id||''));}
    finally{viewer.sending=false;}
  }
  function enqueue(bytes){if(!current()||viewer.stopped)return;if(bytes.length>16384||viewer.queue.reduce((n,item)=>n+item.bytes.length,0)+bytes.length>65536){error('Input exceeds the terminal queue limit; nothing was sent for this entry.');return;}viewer.queue.push({id:crypto.randomUUID(),bytes});send();}
  terminal.onData(data=>enqueue(new TextEncoder().encode(data)));terminal.onBinary(data=>enqueue(terminalBytes(data)));
  terminal.attachCustomKeyEventHandler(e=>{if(e.key==='Escape')return false;if((e.ctrlKey||e.metaKey)&&!e.shiftKey&&!e.altKey&&e.key.toLowerCase()==='c'){if(e.type!=='keydown'||e.repeat)return false;if(terminal.hasSelection()){if(navigator.clipboard?.writeText)navigator.clipboard.writeText(terminal.getSelection()).catch(()=>error('Clipboard access is unavailable.'));else error('Clipboard access is unavailable.');}else confirmTerminalAction(id,'Interrupt harness','Send Ctrl+C to the existing harness?',new Uint8Array([3]));return false;}return true;});
  let resizeTimer;viewer.resize=new ResizeObserver(()=>{clearTimeout(resizeTimer);resizeTimer=setTimeout(()=>{if(!current())return;fit.fit();const cols=Math.max(20,Math.min(400,terminal.cols)),rows=Math.max(5,Math.min(200,terminal.rows));api('resizeSession',{id,cols,rows}).catch(e=>{if(current()&&!e.staleInteraction)error(e.message);});},100);});viewer.resize.observe(byId('terminal-output'));
  byId('editor-form').onsubmit=e=>{e.preventDefault();dialog.close();};
  on('terminal-escape','click',()=>enqueue(new Uint8Array([27])));
  on('terminal-interrupt','click',()=>confirmTerminalAction(id,'Interrupt harness','Send Ctrl+C to the existing harness?',new Uint8Array([3])));
  on('terminal-stop','click',()=>confirmTerminalAction(id,'Stop harness','Stop this owned harness? Retained work and history stay available.'));
  on('terminal-switch','click',()=>{disconnectTerminal();dialog.close();sessionSwitcher().catch(showError);});
  dialog.addEventListener('close',()=>{clearTimeout(resizeTimer);if(terminalView===viewer)disconnectTerminal();dialog.classList.remove('terminal-dialog');},{once:true});
  write(first);terminal.focus();read();
}
function confirmTerminalAction(id,title,text,bytes=null){
  disconnectTerminal();const dialog=openDialog(title,`<p>${text}</p>`,bytes?'Send Ctrl+C':'Stop harness'),form=byId('editor-form'),requestId=crypto.randomUUID();
  dialog.classList.remove('terminal-dialog');form.onsubmit=async e=>{e.preventDefault();const guard=captureDialogCompletion(dialog);try{if(bytes)await api('sendSessionInput',{id,requestId,data:base64Bytes(bytes)});else await api('stopSession',{id,confirm:true});if(!guard.current())return;dialog.close();await openTerminal(id);}catch(error){showError(error);}};
}
document.addEventListener('click',e=>{const target=e.target.closest('[data-session],[data-session-switch]');if(!target)return;e.preventDefault();e.stopPropagation();if(target.hasAttribute('data-session-switch'))sessionSwitcher().catch(showError);else openTerminal(target.dataset.session).catch(showError);});
document.addEventListener('keydown',e=>{if(!session||e.ctrlKey||e.metaKey||e.altKey||e.isComposing||foregroundOpen()||e.target.isContentEditable||['INPUT','TEXTAREA','SELECT'].includes(e.target.tagName))return;if(e.key==='`'){e.preventDefault();chooseHarness().catch(showError);}});
setInterval(refreshSessionRoster,1000);
