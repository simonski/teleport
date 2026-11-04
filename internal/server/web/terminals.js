"use strict";
let localSessions=[],terminalView=null,terminalEpoch=0,rosterLoading=false,sessionAttention=new Map(),harnessStart=null,shellStart=null,directCodexStart=null,directHarnessStarts=new Map();
const proposalRefreshPending=new Set();
const runningSession=s=>!['exited','failed'].includes(s.state);
function robotMark(){return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v3M10 3h4M5 8h14v12H5zM2 12v5M22 12v5M8 16h8" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><circle cx="9" cy="12" r="1" fill="currentColor"/><circle cx="15" cy="12" r="1" fill="currentColor"/></svg>';}
function robotButton(row,extra=''){const attention=row.attention||row.errorCode,label=row.personaName||(row.harness==='shell'?'Shell':({'codex-pty':'Codex','claude-pty':'Claude','copilot-pty':'Copilot'}[row.harness]|| (row.ticketId?'#'+row.ticketId:row.activity==='idle'?'Idle':'Free'))),state=row.proposalRun&&row.state==='running'?'running':row.activity||row.state,mark=row.harness==='shell'?'<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="5" width="18" height="15" rx="2" fill="none" stroke="currentColor"/><path d="m7 10 3 3-3 3M12 16h5" fill="none" stroke="currentColor"/></svg>':robotMark();return `<button type="button" class="robot-session secondary ${attention?'needs-attention':''} ${row.activity==='active'?'is-active':''} ${row.proposalRun&&row.state==='running'?'is-running':''} ${extra}" data-session="${escapeHTML(row.id)}" aria-label="${escapeHTML(row.id+' · '+label+' · '+(attention||state))}" title="${escapeHTML(row.id+' · '+label+' · '+(attention||state))}">${mark}<span class="robot-owner">${escapeHTML(label)}</span><span class="robot-id">${escapeHTML(row.id)}${row.proposalRun&&row.state==='running'?' · running':''}</span></button>`;}
function sessionProcess(row){return row.processId?' · session PID '+Number(row.processId):'';}
function sessionIcons(){return localSessions.filter(runningSession).sort((a,b)=>Number(!!(b.attention||b.errorCode))-Number(!!(a.attention||a.errorCode))||a.id.localeCompare(b.id)).map(row=>robotButton(row)).join('');}
function ticketRobot(t){return localSessions.filter(row=>row.projectId===t.projectId&&(row.ticketId===t.id||row.contextTicketId===t.id)&&runningSession(row)).map(row=>robotButton(row,'ticket-robot')).join('');}
function ticketIndicators(t){return (view==='Todos'&&t.proposalAttention?`<button type="button" class="proposal-attention secondary" data-proposal-review="${t.id}" aria-label="Review recommendations for ticket #${t.id}" title="Recommendations need review"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3 21 20H3L12 3Z" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="M12 9v5m0 3h.01" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg></button>`:'')+ticketRobot(t);}
function nativeSessionHTML(t){if(!t.agent)return '';const rows=localSessions.filter(row=>row.ticketId===t.id);return `<section class="native-session-panel"><h3>Harness</h3><div id="native-ticket-questions"></div>${rows.map(row=>`<div class="session-row" data-session-summary="${escapeHTML(row.id)}"><span data-session-robot>${robotButton(row)}</span><span data-session-label>${escapeHTML(row.harness)} · ${escapeHTML(row.attention||row.state)}</span></div>`).join('')||'<p>No harness attached.</p>'}${t.status==='Doing'&&t.delivery?.worktree.path&&!rows.some(runningSession)&&roleAllowsWrite(t.projectId)?`<button type="button" class="secondary" id="start-ticket-harness">Start local harness</button>`:''}${rows.length?'<details><summary>Direct terminal interactions</summary><div id="session-interventions"></div></details>':''}</section>`;}
function bindTerminalTicket(t,dialog){
  if(!t.agent)return;
  on('start-ticket-harness','click',()=>chooseHarness(t).catch(showError));watchNativeQuestions(t,dialog);
  const holder=byId('session-interventions');if(!holder)return;
  const guard=captureDialogCompletion(dialog);
  Promise.all(localSessions.filter(row=>row.ticketId===t.id).map(row=>pages('listSessionInterventions',{id:row.id}))).then(groups=>{if(!guard.current())return;const items=groups.flat().sort((a,b)=>a.at.localeCompare(b.at));holder.innerHTML=items.map(item=>`<p class="intervention">${escapeHTML(item.sessionId)} · ${escapeHTML(item.actor)} · ${escapeHTML(new Date(item.at).toLocaleString())} · ${escapeHTML(item.state)} (${item.written} bytes)</p>`).join('')||'<p>No direct terminal input recorded.</p>';}).catch(error=>{if(guard.current())holder.textContent=error.message;});
}
function disconnectTerminal(){terminalEpoch++;if(!terminalView)return;const viewer=terminalView;terminalView=null;clearTimeout(viewer.timer);clearInterval(viewer.clock);viewer.resize.disconnect();viewer.terminal.dispose();}
function resetTerminals(){disconnectTerminal();localSessions=[];sessionAttention.clear();proposalRefreshPending.clear();}
document.addEventListener('close',e=>{if(e.target.tagName==='TK-DIALOG')terminalEpoch++;},true);
async function refreshSessionRoster(){
  if(!session||rosterLoading)return;rosterLoading=true;const actor=session.account.id;
  try{const rows=await pages('listSessions');if(session?.account.id!==actor)return;const previous=new Map(localSessions.map(row=>[row.id,row])),preference=session.welcomeSetup?.attention||'questions';for(const row of rows){const attention=row.attention||row.errorCode||'';if(attention&&sessionAttention.get(row.id)!==attention&&preference!=='quiet'&&(preference==='all'||/question|input|permission|error|unconfirmed/i.test(attention)))notice(row.id+' · '+attention);sessionAttention.set(row.id,attention);if(row.proposalRun&&row.contextTicketId&&!runningSession(row)&&previous.has(row.id)&&runningSession(previous.get(row.id)))proposalRefreshPending.add(row.contextTicketId);}localSessions=rows;const generation=loadGeneration;await Promise.all([...proposalRefreshPending].map(async id=>{try{const fresh=await api('getTicket',{id});if(session?.account.id!==actor||generation!==loadGeneration)return;const ticket=tickets.find(item=>item.id===id&&item.projectId===fresh.projectId);if(ticket)ticket.proposalAttention=fresh.proposalAttention;proposalRefreshPending.delete(id);}catch{}}));if(session?.account.id!==actor)return;const holder=byId('session-icons');if(holder){const html=sessionIcons();if(holder.innerHTML!==html){const focused=document.activeElement?.dataset.session;holder.innerHTML=html;if(focused)holder.querySelector(`[data-session="${CSS.escape(focused)}"]`)?.focus();}}
    document.querySelectorAll('[data-ticket-card]').forEach(card=>{const t=tickets.find(t=>t.id===Number(card.dataset.ticketCard)),holder=card.querySelector('.ticket-session');if(t&&holder){const html=ticketIndicators(t);if(holder.innerHTML!==html)holder.innerHTML=html;}});
    document.querySelectorAll('[data-session-summary]').forEach(summary=>{const row=rows.find(row=>row.id===summary.dataset.sessionSummary);if(!row)return;const robot=summary.querySelector('[data-session-robot]'),label=summary.querySelector('[data-session-label]'),html=robotButton(row);if(robot.innerHTML!==html)robot.innerHTML=html;label.textContent=row.harness+' · '+(row.attention||row.state);});
    if(typeof syncConsoleAttention==='function')syncConsoleAttention();
  }catch(error){if(error.status===401&&session?.account.id===actor)resetTerminals();}finally{rosterLoading=false;}
}
async function chooseHarness(ticket=null){
  if(harnessStart)return harnessStart;
  const pending=startOrAttachHarness(ticket);harnessStart=pending;
  try{return await pending;}finally{if(harnessStart===pending)harnessStart=null;}
}
async function startOrAttachHarness(ticket=null){
  const actor=session?.account.id,epoch=detailEpoch,requestEpoch=terminalEpoch;
  if(!ticket){
    const kind=session?.welcomeSetup?.harness??'codex',operations={codex:'openCodexTerminal',claude:'openClaudeTerminal',copilot:'openCopilotTerminal'},operation=operations[kind];
    if(!operation)throw new Error('Select a default harness in Settings before starting an agent terminal.');
    const available=projects.filter(p=>roleAllowsWrite(p.id)),selected=projectId||(available.length===1?available[0].id:0);
    if(!selected||!available.some(p=>p.id===selected))throw new Error('Select a project with permission to start a harness.');
    const rows=await pages('listSessions');
    if(session?.account.id!==actor||detailEpoch!==epoch||terminalEpoch!==requestEpoch||(session?.welcomeSetup?.harness??'codex')!==kind)return;
    localSessions=rows;
    const existing=rows.find(row=>runningSession(row)&&!row.capacityReleased&&!row.ticketId&&!row.personaId&&row.projectId===selected&&(row.harness===kind+'-pty'||kind==='codex'&&row.harness==='codex'));
    if(existing){await openTerminal(existing.id);return;}
    const row=await api(operation,{projectId:selected,requestId:crypto.randomUUID()});
    await refreshSessionRoster();
    if(session?.account.id===actor&&detailEpoch===epoch&&terminalEpoch===requestEpoch&&(session?.welcomeSetup?.harness??'codex')===kind)await openTerminal(row.id);
    return;
  }
  const [choices,rows]=await Promise.all([api('getLocalHarnesses'),pages('listSessions')]);
  if(session?.account.id!==actor||detailEpoch!==epoch||terminalEpoch!==requestEpoch)return;
  localSessions=rows;
  const existing=rows.find(row=>runningSession(row)&&!['shell','codex-pty','claude-pty','copilot-pty'].includes(row.harness)&&!row.capacityReleased&&(ticket?row.ticketId===ticket.id:!row.ticketId));
  if(existing){await openTerminal(existing.id);return;}
  if(!choices.available.some(h=>h.id==='codex'&&!h.directOnly))throw new Error('No verified structured Codex harness is available. Use the direct Codex terminal if installed.');
  const available=projects.filter(p=>roleAllowsWrite(p.id)),selected=ticket?.projectId||projectId|| (available.length===1?available[0].id:0);
  if(!selected||!available.some(p=>p.id===selected))throw new Error('Select a project with permission to start a harness.');
  const row=await api(ticket?'startTicketSession':'openConsoleSession',{projectId:selected,...(ticket?{ticketId:ticket.id,expectedVersion:ticket.version}:{}),harness:'codex',requestId:crypto.randomUUID()});
  await refreshSessionRoster();
  if(session?.account.id===actor&&detailEpoch===epoch&&terminalEpoch===requestEpoch)await openTerminal(row.id);
}
async function startShell(){
  if(shellStart)return shellStart;
  const actor=session?.account.id,epoch=detailEpoch,requestEpoch=terminalEpoch,available=projects.filter(p=>roleAllowsWrite(p.id)),selected=projectId||(available.length===1?available[0].id:0);
  if(!selected||!available.some(p=>p.id===selected))throw new Error('Select a project with permission to start a shell.');
  const pending=(async()=>{const row=await api('openShellSession',{projectId:selected,requestId:crypto.randomUUID()});await refreshSessionRoster();if(session?.account.id===actor&&detailEpoch===epoch&&terminalEpoch===requestEpoch)await openTerminal(row.id);})();shellStart=pending;
  try{return await pending;}finally{if(shellStart===pending)shellStart=null;}
}
async function startDirectCodex(){
  if(directCodexStart)return directCodexStart;
  const actor=session?.account.id,epoch=detailEpoch,requestEpoch=terminalEpoch,available=projects.filter(p=>roleAllowsWrite(p.id)),selected=projectId||(available.length===1?available[0].id:0);
  if(!selected||!available.some(p=>p.id===selected))throw new Error('Select a project with permission to start Codex.');
  const pending=(async()=>{const row=await api('openCodexTerminal',{projectId:selected,requestId:crypto.randomUUID()});await refreshSessionRoster();if(session?.account.id===actor&&detailEpoch===epoch&&terminalEpoch===requestEpoch)await openTerminal(row.id);})();directCodexStart=pending;
  try{return await pending;}finally{if(directCodexStart===pending)directCodexStart=null;}
}
async function startDirectHarness(kind){
  const operations={claude:'openClaudeTerminal',copilot:'openCopilotTerminal'},operation=operations[kind];if(!operation)throw new Error('Unsupported terminal.');
  if(directHarnessStarts.has(kind))return directHarnessStarts.get(kind);
  const actor=session?.account.id,epoch=detailEpoch,requestEpoch=terminalEpoch,available=projects.filter(p=>roleAllowsWrite(p.id)),selected=projectId||(available.length===1?available[0].id:0);
  if(!selected||!available.some(p=>p.id===selected))throw new Error('Select a project with permission to start '+kind+'.');
  const pending=(async()=>{const row=await api(operation,{projectId:selected,requestId:crypto.randomUUID()});await refreshSessionRoster();if(session?.account.id===actor&&detailEpoch===epoch&&terminalEpoch===requestEpoch)await openTerminal(row.id);})();directHarnessStarts.set(kind,pending);
  try{return await pending;}finally{if(directHarnessStarts.get(kind)===pending)directHarnessStarts.delete(kind);}
}
async function sessionSwitcher(){
  const actor=session?.account.id,epoch=detailEpoch;await refreshSessionRoster();if(!session||session.account.id!==actor||detailEpoch!==epoch||foregroundOpen())return;const recoveryRows=await pages('listSessions',{includeHidden:true});if(!session||session.account.id!==actor||detailEpoch!==epoch||foregroundOpen())return;
  const dialog=openDialog('Harness sessions',`<section><h3>Teleport sessions</h3><div class="session-list">${localSessions.filter(runningSession).map(row=>`<div class="session-row">${robotButton(row)}<span>${escapeHTML(row.harness)} · ${escapeHTML(row.state)}${sessionProcess(row)}${row.attention?' · '+escapeHTML(row.attention):''}${row.ticketId?' · #'+row.ticketId:' · free console'}</span>${row.state==='unknown'?`<span class="muted">Ownership unconfirmed${row.stopRequested?' · stop recorded':''}</span>${row.stopRequested?'':`<button type="button" class="secondary" data-stop-managed="${escapeHTML(row.id)}" data-unknown="true">Record stop</button>`}${row.reference?`<button type="button" class="secondary" data-remove-reference="${escapeHTML(row.id)}">Remove reference</button>`:''}`:`<button type="button" class="secondary" data-stop-managed="${escapeHTML(row.id)}">Stop</button>`}</div>`).join('')||'<p>No active or unresolved Teleport sessions.</p>'}</div>${localSessions.some(row=>!runningSession(row))?`<details><summary>Ended sessions (${localSessions.filter(row=>!runningSession(row)).length})</summary>${localSessions.filter(row=>!runningSession(row)).map(row=>`<div class="session-row">${robotButton(row)}<span>${escapeHTML(row.harness)} · ${escapeHTML(row.state)}${sessionProcess(row)}</span></div>`).join('')}</details>`:''}<button type="button" class="secondary" id="new-console">New free console</button></section>${session.account.administrator?'<section class="host-harnesses"><h3>Non-Teleport harness processes</h3><p class="muted">Recognized processes owned by this server user. These are not associated with a Teleport session; their terminals cannot be attached here. Some harness launchers may not be detected.</p><button type="button" class="secondary" id="refresh-host-harnesses">Refresh processes</button><p id="host-harness-status" role="status">Loading…</p><div id="host-harness-list"></div></section>':''}`,'Close');
  const guard=captureDialogCompletion(dialog);byId('cancel').hidden=true;byId('editor-form').onsubmit=e=>{e.preventDefault();dialog.close();};on('new-console','click',()=>{dialog.close();chooseHarness().catch(showError);});
  const hidden=recoveryRows.filter(row=>row.referenceRemoved&&row.state==='unknown');
  const recovery=document.createElement('section');recovery.className='session-recovery';recovery.innerHTML=`<h3>Unresolved removed sessions</h3><p class="muted">Teleport checks ownership in the background. Releasing a reservation does not confirm that its process stopped.</p>${hidden.map(row=>`<div class="session-row"><span>${escapeHTML(row.id)} · ${escapeHTML(row.harness)} · ${escapeHTML(row.attention||row.errorCode||'Ownership unconfirmed')} · project ${Number(row.projectId)} · ${row.capacityReleased?'Capacity released':'Capacity reserved'}</span>${!row.capacityReleased&&row.reference?`<button type="button" class="secondary" data-release-capacity="${escapeHTML(row.id)}">Release reservation</button>`:''}</div>`).join('')||'<p>No removed unresolved sessions.</p>'}<button type="button" class="secondary" id="refresh-session-recovery">Refresh recovery status</button>`;byId('editor-form').append(recovery);
  recovery.querySelectorAll('[data-release-capacity]').forEach(button=>button.onclick=()=>{const row=hidden.find(item=>item.id===button.dataset.releaseCapacity);if(row){dialog.close();confirmReleaseSession(row);}});
  on('refresh-session-recovery','click',()=>{dialog.close();sessionSwitcher().catch(showError);});
  dialog.querySelectorAll('[data-stop-managed]').forEach(button=>button.onclick=()=>{dialog.close();confirmTerminalAction(button.dataset.stopManaged,'Stop harness',button.dataset.unknown==='true'?'Ownership is unconfirmed. Record a stop request; no signal can be sent until ownership is reconciled.':'Stop this Teleport session? Retained work and history stay available.',null,true);});
  dialog.querySelectorAll('[data-remove-reference]').forEach(button=>button.onclick=()=>{const row=localSessions.find(item=>item.id===button.dataset.removeReference);if(!row?.reference)return;dialog.close();confirmRemoveSessionReference(row);});
  let hostRows=[],loading=false,hostSignature='';
  async function loadHost(quiet=false){if(loading)return;loading=true;const button=byId('refresh-host-harnesses'),status=byId('host-harness-status');if(!quiet){button.disabled=true;status.textContent='Loading…';}try{const result=await api('listHostHarnesses',{});if(!guard.current())return;hostRows=result.items.filter(row=>!row.managed);const holder=byId('host-harness-list'),signature=JSON.stringify(hostRows.map(row=>[row.pid,row.identity,row.harness,row.state]));if(!quiet||signature!==hostSignature){holder.innerHTML=hostRows.map(row=>`<div class="session-row host-harness-row"><span><strong>${escapeHTML(row.harness)}</strong> · PID ${Number(row.pid)} · ${escapeHTML(row.state)} · ${escapeHTML(row.elapsed)}${row.managed?' · Teleport-managed':' · Non-Teleport'}</span><button type="button" class="secondary" data-host-inspect="${Number(row.pid)}">Inspect</button>${row.managed?'':`<button type="button" class="secondary" data-host-stop="${Number(row.pid)}">Stop</button><button type="button" class="secondary" data-host-stop="${Number(row.pid)}" data-force="true">Force stop</button>`}</div>`).join('');hostSignature=signature;}status.textContent=hostRows.length?'':'No recognized local harness processes.';}catch(error){if(guard.current())status.textContent=error.message||'Process discovery unavailable.';}finally{loading=false;if(guard.current()&&!quiet)button.disabled=false;}}
  if(session.account.administrator){
    on('refresh-host-harnesses','click',()=>loadHost());
    byId('host-harness-list').addEventListener('click',event=>{const inspect=event.target.closest('[data-host-inspect]'),stop=event.target.closest('[data-host-stop]');if(!inspect&&!stop)return;const row=hostRows.find(item=>item.pid===Number((inspect||stop).dataset[inspect?'hostInspect':'hostStop']));if(!row)return;if(inspect){showHostHarnessDetails(row);return;}confirmHostHarnessStop(row,stop.dataset.force==='true',true);});
    loadHost();
  }
  const timer=setInterval(async()=>{if(!guard.current()){clearInterval(timer);return;}await refreshSessionRoster();if(!guard.current())return;const list=dialog.querySelector('.session-list');list.querySelectorAll('.session-row').forEach(element=>{const id=element.querySelector('[data-session]')?.dataset.session,row=localSessions.find(item=>item.id===id);if(!row||!runningSession(row))element.remove();});if(!list.querySelector('.session-row')&&!list.querySelector('p'))list.innerHTML='<p>No active or unresolved Teleport sessions.</p>';if(session.account.administrator)loadHost(true);},1000);
  dialog.addEventListener('close',()=>clearInterval(timer),{once:true});
}
function showHostHarnessDetails(row){
  const detail=openDialog('Harness process',`<dl><dt>Harness</dt><dd>${escapeHTML(row.harness)}</dd><dt>Process ID</dt><dd>${Number(row.pid)}</dd><dt>Parent process ID</dt><dd>${Number(row.parentPid)}</dd><dt>State</dt><dd>${escapeHTML(row.state)}</dd><dt>Elapsed</dt><dd>${escapeHTML(row.elapsed)}</dd><dt>Ownership</dt><dd>${row.managed?'Teleport-managed':'Non-Teleport process'}</dd></dl><p>Non-Teleport terminal output cannot be attached here.</p>`,'OK'),guard=captureDialogCompletion(detail);
  byId('cancel').hidden=true;byId('editor-form').onsubmit=e=>{e.preventDefault();detail.close();};
  detail.addEventListener('close',()=>{if(guard.follow())sessionSwitcher().catch(showError);},{once:true});
}
function confirmReleaseSession(row){
  const dialog=openDialog('Release session reservation',`<p>Release ${escapeHTML(row.id)} from the project agent limit?</p><p>Teleport cannot confirm that this process or its commands stopped. They may continue outside Teleport. This withdraws the session’s Teleport credential and allows another console to start; it does not stop any process.</p>`,'Release reservation');let pending=false;
  byId('editor-form').onsubmit=async event=>{event.preventDefault();if(pending)return;pending=true;try{await api('removeSessionReference',{id:row.id,expectedReference:row.reference,confirm:true,releaseCapacity:true});dialog.close();await refreshSessionRoster();notice('Reservation released; process exit remains unverified.');}catch(error){if(dialog.open)showError(error);}finally{pending=false;}};
}
function confirmRemoveSessionReference(row){
  const dialog=openDialog('Remove session reference',`<p>Remove ${escapeHTML(row.id)} from the session list?</p><p>No process will be stopped, and its exit is not verified. The unresolved session remains quarantined and may still prevent a new free console until ownership is reconciled.</p>`,'Remove reference');let pending=false;
  byId('editor-form').onsubmit=async event=>{event.preventDefault();if(pending)return;pending=true;const guard=captureDialogCompletion(dialog);dialog.close();try{await api('removeSessionReference',{id:row.id,expectedReference:row.reference,confirm:true});if(guard.follow()){localSessions=localSessions.filter(item=>item.id!==row.id);await refreshSessionRoster();if(guard.follow())notice('Session reference removed; no process was stopped.');}}catch(error){if(guard.follow())showError(error);}};
}
function confirmHostHarnessStop(row,force=false,returnToSessions=false){
  const dialog=openDialog(force?'Force stop non-Teleport harness':'Stop non-Teleport harness',`<p>${force?'Force stop':'Request a stop for'} ${escapeHTML(row.harness)} process ${Number(row.pid)}?</p><p>This is not associated with a Teleport session. Stopping it may interrupt work in another application or an unresolved session. Only the selected process is signalled; child processes may continue. A stop request does not confirm exit.</p>`,force?'Force stop':'Request stop');let pending=false;
  byId('editor-form').onsubmit=async event=>{event.preventDefault();if(pending)return;pending=true;const guard=captureDialogCompletion(dialog);dialog.close();try{await api('stopHostHarness',{pid:row.pid,identity:row.identity,confirm:true,force});if(guard.follow()){notice('Stop requested; exit has not yet been confirmed.');if(returnToSessions)await sessionSwitcher();}}catch(error){if(guard.follow()){showError(error);if(returnToSessions)await sessionSwitcher();}}};
}

function terminalBytes(data){const bytes=new Uint8Array(data.length);for(let i=0;i<data.length;i++)bytes[i]=data.charCodeAt(i);return bytes;}
function base64Bytes(bytes){let text='';for(const byte of bytes)text+=String.fromCharCode(byte);return btoa(text);}
function terminalElapsed(row,now=Date.now()){
  const start=Date.parse(row.createdAt),finished=['exited','failed'].includes(row.state),end=finished?Date.parse(row.exitedAt):now;
  if(!Number.isFinite(start)||!Number.isFinite(end))return '—';
  const seconds=Math.max(0,Math.floor((end-start)/1000)),hours=Math.floor(seconds/3600),minutes=Math.floor(seconds%3600/60),remaining=seconds%60;
  return (hours?String(hours)+':':'')+String(minutes).padStart(2,'0')+':'+String(remaining).padStart(2,'0');
}
function terminalStartTime(row){const timestamp=Date.parse(row.createdAt);return Number.isFinite(timestamp)?new Date(timestamp).toLocaleString(undefined,{dateStyle:'medium',timeStyle:'medium'}):'Unavailable';}
function closeDirectTerminalAfterExit(initial,current){return !initial.proposalRun&&['shell','codex-pty','claude-pty','copilot-pty'].includes(initial.harness)&&initial.state!=='exited'&&current.state==='exited';}
async function openTerminal(id,options={}){
  disconnectTerminal();const epoch=terminalEpoch,actor=session?.account.id,foregroundEpoch=detailEpoch;
  const first=await api('attachSession',{id,cursor:0});
  if(epoch!==terminalEpoch||session?.account.id!==actor||foregroundEpoch!==detailEpoch)return;
  const dialog=openDialog(escapeHTML(id+' · '+(first.session.personaName||first.session.harness)),`<div class="terminal-toolbar"><span id="terminal-state" role="status">${escapeHTML(first.session.state)}</span><button type="button" class="secondary" id="terminal-escape">Send Esc</button><button type="button" class="secondary" id="terminal-interrupt">Ctrl+C</button><button type="button" class="secondary" id="terminal-stop">Stop</button><button type="button" class="secondary" id="terminal-switch">Sessions</button></div><div class="terminal-timing"><span>Session started <time id="terminal-started" datetime="${escapeHTML(first.session.createdAt||'')}">${escapeHTML(terminalStartTime(first.session))}</time></span><span>Elapsed <time id="terminal-elapsed">${terminalElapsed(first.session)}</time></span></div>${first.session.ticketId?'<p class="terminal-intervention-note">Direct input is recorded outside the ticket workflow.</p>':''}<div id="terminal-output" class="terminal-surface" aria-label="Harness terminal"></div><p id="terminal-error" class="save-status" role="status"></p>`,'Detach');
  dialog.classList.add('terminal-dialog');byId('cancel').hidden=true;
  if(first.session.proposalRun){byId('terminal-escape').hidden=true;byId('terminal-interrupt').hidden=true;const note=document.createElement('p');note.className='terminal-intervention-note';note.textContent='Planning output is live. Add feedback to the proposal after this run finishes.';byId('terminal-output').before(note);}
  const nonce=document.querySelector('meta[name=teleport-style-nonce]')?.content;const terminalDocument=new Proxy(document,{get(target,key){if(key==='createElement')return (name,...args)=>{const element=target.createElement(name,...args);if(name.toLowerCase()==='style'&&nonce)element.nonce=nonce;return element;};const value=Reflect.get(target,key,target);return typeof value==='function'?value.bind(target):value;}});
  const terminal=new Terminal({documentOverride:terminalDocument,cols:first.cols||100,rows:first.rows||28,scrollback:5000,fontFamily:'Menlo, Consolas, monospace',fontSize:13,allowProposedApi:false,screenReaderMode:true,theme:{background:'#151a17',foreground:'#e9eee5',cursor:'#f18a62'}}),fit=new FitAddon.FitAddon();
  terminal.loadAddon(fit);terminal.open(byId('terminal-output'));
  const viewer={id,terminal,fit,cursor:0,timer:null,clock:null,resize:null,stopped:false,queue:[],sending:false,epoch,actor,session:first.session};terminalView=viewer;
  const current=()=>terminalView===viewer&&session?.account.id===actor&&epoch===terminalEpoch&&dialog.open;
  const error=message=>{if(current())byId('terminal-error').textContent=message;};
  const tick=()=>{if(current())byId('terminal-elapsed').textContent=terminalElapsed(viewer.session);};viewer.clock=setInterval(tick,1000);
  function write(out){if(!current())return;viewer.session=out.session;tick();if(out.gap&&(!viewer.gapAnnounced||viewer.cursor!==out.cursor)){terminal.reset();terminal.writeln('[Retained output is incomplete]');viewer.gapAnnounced=true;}terminal.options.disableStdin=first.session.proposalRun||out.session.state!=='running';viewer.cursor=out.next;if(out.data)terminal.write(terminalBytes(atob(out.data)));byId('terminal-state').textContent=out.session.attention||out.session.state;if(first.session.proposalRun&&['exited','failed'].includes(out.session.state)){byId('terminal-stop').disabled=true;if(out.session.state==='exited'&&out.session.exitCode===0){if(!byId('terminal-review')){const button=document.createElement('button');button.type='button';button.id='terminal-review';button.textContent='Review proposal';button.onclick=()=>openTicketProposalReview(first.session.contextTicketId,dialog).catch(e=>error(e.message));byId('terminal-switch').before(button);}if(options.autoReview&&!viewer.reviewOpened){viewer.reviewOpened=true;openTicketProposalReview(first.session.contextTicketId,dialog).catch(e=>error(e.message||'Could not open the proposal; use Review proposal to try again.'));}}else{error('Planning run failed or stopped. Inspect the retained output, then retry.');if(options.retry&&!byId('terminal-retry')){const button=document.createElement('button');button.type='button';button.id='terminal-retry';button.textContent='Retry';button.onclick=()=>{dialog.close();options.retry().catch(showError);};byId('terminal-switch').before(button);}}}if(closeDirectTerminalAfterExit(first.session,out.session)){dialog.close();refreshSessionRoster().catch(()=>{});}}
  async function read(){if(!current())return;try{const out=await api('attachSession',{id,cursor:viewer.cursor});if(!current())return;write(out);if(current())viewer.timer=setTimeout(read,out.data&&out.data.length>60000?10:180);}catch(e){if(current()){error(e.message);viewer.timer=setTimeout(read,1500);}}}
  async function send(){if(viewer.sending||viewer.stopped||!current()||!viewer.queue.length)return;viewer.sending=true;
    try{while(viewer.queue.length&&current()){const item=viewer.queue[0],result=await api('sendSessionInput',{id,requestId:item.id,data:base64Bytes(item.bytes)});if(result.state!=='delivered'){viewer.stopped=true;error('Input delivery '+result.state+'; further input is paused. Operation '+item.id);break;}viewer.queue.shift();}}
    catch(e){viewer.stopped=true;error(e.message+' · Input paused; operation '+(viewer.queue[0]?.id||''));}
    finally{viewer.sending=false;}
  }
  function enqueue(bytes){if(first.session.proposalRun||!current()||viewer.stopped)return;if(bytes.length>16384||viewer.queue.reduce((n,item)=>n+item.bytes.length,0)+bytes.length>65536){error('Input exceeds the terminal queue limit; nothing was sent for this entry.');return;}viewer.queue.push({id:crypto.randomUUID(),bytes});send();}
  terminal.onData(data=>enqueue(new TextEncoder().encode(data)));terminal.onBinary(data=>enqueue(terminalBytes(data)));
  terminal.attachCustomKeyEventHandler(e=>{if(e.key==='Escape')return false;if((e.ctrlKey||e.metaKey)&&!e.shiftKey&&!e.altKey&&e.key.toLowerCase()==='c'){if(e.type!=='keydown'||e.repeat)return false;if(terminal.hasSelection()){if(navigator.clipboard?.writeText)navigator.clipboard.writeText(terminal.getSelection()).catch(()=>error('Clipboard access is unavailable.'));else error('Clipboard access is unavailable.');}else if(!first.session.proposalRun)confirmTerminalAction(id,'Interrupt harness','Send Ctrl+C to the existing harness?',new Uint8Array([3]));return false;}return true;});
  let resizeTimer;viewer.resize=new ResizeObserver(()=>{clearTimeout(resizeTimer);resizeTimer=setTimeout(()=>{if(!current())return;fit.fit();const cols=Math.max(20,Math.min(400,terminal.cols)),rows=Math.max(5,Math.min(200,terminal.rows));api('resizeSession',{id,cols,rows}).catch(e=>{if(current()&&!e.staleInteraction)error(e.message);});},100);});viewer.resize.observe(byId('terminal-output'));
  byId('editor-form').onsubmit=e=>{e.preventDefault();dialog.close();};
  on('terminal-escape','click',()=>enqueue(new Uint8Array([27])));
  on('terminal-interrupt','click',()=>confirmTerminalAction(id,'Interrupt harness','Send Ctrl+C to the existing harness?',new Uint8Array([3])));
  on('terminal-stop','click',()=>confirmTerminalAction(id,'Stop harness','Stop this owned harness? Retained work and history stay available.'));
  on('terminal-switch','click',()=>{disconnectTerminal();dialog.close();sessionSwitcher().catch(showError);});
  dialog.addEventListener('close',()=>{clearTimeout(resizeTimer);if(terminalView===viewer)disconnectTerminal();dialog.classList.remove('terminal-dialog');},{once:true});
  write(first);terminal.focus();read();
}
function confirmTerminalAction(id,title,text,bytes=null,returnToSessions=false){
  disconnectTerminal();const dialog=openDialog(title,`<p>${text}</p><p id="terminal-action-status" class="save-status" role="status"></p>`,bytes?'Send Ctrl+C':'Stop harness'),form=byId('editor-form'),requestId=crypto.randomUUID();let pending=false;
  dialog.classList.remove('terminal-dialog');form.onsubmit=async e=>{e.preventDefault();if(pending)return;pending=true;const guard=captureDialogCompletion(dialog);
    if(!bytes){dialog.close();try{await api('stopSession',{id,confirm:true});if(guard.follow()){notice('Stop requested; exit has not yet been confirmed.');if(returnToSessions)await sessionSwitcher();}}catch(error){if(guard.follow()){showError(error);if(returnToSessions)await sessionSwitcher();}}return;}
    try{await api('sendSessionInput',{id,requestId,data:base64Bytes(bytes)});if(!guard.current())return;dialog.close();await openTerminal(id);}catch(error){if(guard.current())byId('terminal-action-status').textContent=error.message||'Action failed.';}finally{pending=false;}
  };
}

document.addEventListener('click',e=>{const target=e.target.closest('[data-session],[data-session-switch]');if(!target)return;e.preventDefault();e.stopPropagation();if(target.hasAttribute('data-session-switch'))sessionSwitcher().catch(showError);else openTerminal(target.dataset.session).catch(showError);});
document.addEventListener('click',e=>{const target=e.target.closest('[data-proposal-review]');if(!target)return;e.preventDefault();e.stopPropagation();openTicketProposalReview(Number(target.dataset.proposalReview)).catch(showError);});
document.addEventListener('keydown',e=>{if(!session||e.ctrlKey||e.metaKey||e.altKey||e.isComposing||foregroundOpen()||e.target.isContentEditable||['INPUT','TEXTAREA','SELECT'].includes(e.target.tagName))return;if(e.key==='`'){e.preventDefault();chooseHarness().catch(showError);}});
setInterval(refreshSessionRoster,1000);
