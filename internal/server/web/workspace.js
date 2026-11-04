"use strict";
let session=null,projects=[],tickets=[],view='Home',projectId=0,selected=null,refreshing=false,lens='agent',loadGeneration=0,workflowMap={};
let ticketEditorPending=null;
const escapeHTML=value=>String(value??'').replace(/[&<>"']/g,char=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
const byId=id=>document.getElementById(id);
async function pages(operation,data={}){const items=[];let cursor='';do{const page=await api(operation,{...data,cursor,limit:200});items.push(...page.items);cursor=page.nextCursor;}while(cursor);return items;}
function on(id,event,handler){const element=byId(id);if(element)element.addEventListener(event,handler);}
function showError(error){if(error.staleInteraction)return;notice(error.message||String(error));}
async function workspace(background=false){
  const generation=++loadGeneration,nextSession=await api('getSession');if(generation!==loadGeneration)return;
  const changedAccount=preferencesActor!==nextSession.account.id;
  if(changedAccount){clearPresentation();preferencesActor=nextSession.account.id;initialNavigation=false;}
  if(!preferences||nextSession.preferences.version>=preferences.version)preferences=nextSession.preferences;applyAppearance(preferences);
  const nextProjects=await pages('listProjects');if(generation!==loadGeneration)return;
  if(changedAccount){const location=chooseInitialLocation(nextSession,nextProjects);view=location.view;projectId=location.projectId;}
  if(projectId&&!nextProjects.some(project=>project.id===projectId))projectId=0;
  const scope=projectId;let nextTickets,nextFlows,nextIdeas;
  try{[nextTickets,nextFlows,nextIdeas]=await Promise.all([pages('listTickets',scope?{projectId:scope}:{}),pages('listWorkflows',scope?{projectId:scope}:{}),pages('listIdeas',{...(scope?{projectId:scope}:{}),includeHidden:ideaHidden})]);}
  catch(error){if(generation===loadGeneration&&scope&&error.status===403){projectId=0;return workspace(background);}throw error;}
  const nextMap=Object.fromEntries(nextFlows.map(w=>[flowKey(w),w]));for(const ticket of nextTickets)if(!nextMap[ticket.workflow])nextMap[ticket.workflow]=await api('getWorkflow',{workflowKey:ticket.workflow,ticketId:ticket.id});
  if(generation!==loadGeneration||foregroundOpen()||background&&root.querySelector('.board-workspace .ticket:hover,.ticket-list-card:hover,.backlog-row:hover'))return;session=nextSession;projects=nextProjects;tickets=nextTickets;workflowMap=nextMap;ideas=nextIdeas;render();maybeWelcome();
}
function projectName(id){return projects.find(p=>p.id===id)?.name||'Project';}
function currentProject(){return projects.find(p=>p.id===projectId);}
function roleAllowsWrite(id){const project=projects.find(p=>p.id===id);return session.account.administrator||['Contributor','Maintainer'].includes(project?.members[session.account.id]);}
function header(){
  const options=[...projects].sort((a,b)=>a.name.localeCompare(b.name)||a.id-b.id).map(p=>`<option value="${p.id}" ${projectId===p.id?'selected':''}>${escapeHTML(p.name)}</option>`).join('');
  return `<div class="workspace-header"><header class="topbar"><button class="wordmark" id="home" aria-label="teleport Home">${rocketMark()}<span>teleport</span></button><div class="divider"></div><label class="project-switch"><span class="sr-only">Project</span><select id="project"><option value="0">All projects</option>${options}<optgroup label="Project directory"><option value="manage-projects">Manage projects…</option></optgroup><optgroup label="New project"><option value="new-project">Create project…</option></optgroup></select></label><div class="header-space"></div><span class="runtime"><i></i> LOCAL <span class="muted">/ ${runtimeModeLabel()}</span></span><div id="session-icons" class="session-icons" aria-label="Harness sessions">${sessionIcons()}</div><button class="avatar secondary" id="account" aria-controls="account-menu" aria-expanded="false" aria-haspopup="menu" aria-label="Account and attention">${escapeHTML(session.account.username.slice(0,2).toUpperCase())}${tickets.some(t=>t.attention&&!['Done','Cancelled'].includes(t.status))?'<span class="attention-dot" aria-label="Attention required"></span>':''}</button>${accountMenuHTML()}</header>${navigationHTML()}</div>`;
}
function plainCard(t){return `<article class="ticket" data-ticket-card="${t.id}"><button class="ticket-open" data-ticket="${t.id}"><div class="ticket-meta"><span class="number">#${t.id}</span><span class="priority ${escapeHTML(t.priority)}">${escapeHTML(t.priority)}</span>${t.agent?'<span class="agent-mark" title="Agent ticket">↗</span>':''}</div>${t.attention?`<span class="card-attention">${escapeHTML(t.attention)}</span>`:''}<h3>${escapeHTML(t.title)}</h3><div class="ticket-footer"><span>${escapeHTML(projectName(t.projectId))}</span><span>${t.agent?'Agent':escapeHTML(t.assignee||'Unassigned')}</span></div>${t.progress>0?`<progress class="progress" value="${t.progress}" max="100" aria-label="${t.progress}% complete"></progress>`:''}</button><div class="ticket-session">${ticketIndicators(t)}</div></article>`;}
function homeView(){return projectHomeHTML();}
function empty(title,description,id,label){return `<div class="empty"><div class="empty-symbol" aria-hidden="true">↗</div><h2>${title}</h2><p>${description}</p>${id?`<button id="${id}">${label}</button>`:''}</div>`;}
function phaseTitle(t){return workflowMap[t.workflow]?.phases.find(p=>p.id===t.phase)?.title||(t.agent&&t.status==='Doing'?'Implementation':t.status);}
function boardView(){
  const human=view==='Todos',leaf=tickets.filter(t=>(t.kind==='task'||!human&&t.kind==='epic'&&t.planning)&&(human?!t.agent:lens==='both'||t.agent===(lens==='agent'))&&t.status!=='Cancelled');
  if(!human)return '<div class="board-heading view-actions"><label class="compact">Intent <select id="lens">'+['agent','human','both'].map(value=>'<option value="'+value+'" '+(value===lens?'selected':'')+'>'+value[0].toUpperCase()+value.slice(1)+'</option>').join('')+'</select></label></div>'+agentBoardView(leaf);
  return `<div class="board human-board">${['Todo','Doing','Done'].map(role=>`<section class="lane"><header><h2>${role}</h2><span class="lane-count">${leaf.filter(t=>t.status===role).length}</span><span class="lane-line"></span></header><div class="lane-body">${leaf.filter(t=>t.status===role).sort((a,b)=>Number(!!b.attention)-Number(!!a.attention)||a.id-b.id).map(card).join('')}</div></section>`).join('')}</div>`;
}
function projectsView(){return projectDirectoryHTML();}
function navigateView(name){name=canonicalView(name);if(!availableNavigation(name))return;if(typeof backlogBusy==='function'&&backlogBusy()){notice('Finish the backlog move before switching views.');return;}if(typeof workflowNavigationBlocked==='function'&&workflowNavigationBlocked()){notice('Save latest or discard changes, and wait for pending workflow changes before switching.');return;}loadGeneration++;if(view!==name){calendarDirty=false;}detailEpoch++;view=name;render();root.querySelector(`#nav-views [data-view="${CSS.escape(name)}"]`)?.focus({preventScroll:true});window.scrollTo({top:0,behavior:'instant'});}
function render(){
  const backlogFocus=document.activeElement?.closest('[data-backlog-grip],[data-backlog-open]');
  const backlogFocusKey=backlogFocus?{kind:backlogFocus.dataset.backlogGrip?'grip':'open',key:backlogFocus.dataset.backlogGrip||backlogFocus.dataset.backlogOpen}:null;
  if(typeof leaveBacklog==='function')leaveBacklog();
  const ticketListFocus=document.activeElement?.dataset.listTicket;
  if(typeof leaveTicketList==='function')leaveTicketList();
  if(typeof leaveWorkflowMap==='function')leaveWorkflowMap();
  const restoreFocus=captureShellFocus(),restoreBoardFocus=captureBoardFocus();
  leaveGraph();calendarEpoch++;
  const content=view==='Home'?homeView():view==='Backlog'?backlogHTML():view==='Graph'?graphView():view==='Workflows'?workflowsView():view==='Agents'&&session.account.administrator?agentFactoryHTML():view==='Cost'?costView():view==='Calendar'?calendarView():boardView();
  root.innerHTML=header()+`<main class="workspace ${view==='Board'||view==='Todos'?'board-workspace':''}">${content}</main><tk-dialog id="editor"></tk-dialog>`;
  root.querySelectorAll('[data-view]').forEach(button=>button.onclick=()=>navigateView(button.dataset.view));
  root.querySelectorAll('[data-ticket]').forEach(button=>button.onclick=()=>editTicket(Number(button.dataset.ticket)));
  on('project','change',e=>{
    if(presentationBusy()||calendarBusy()||graphBusy()||(typeof backlogBusy==='function'&&backlogBusy())||(typeof workflowNavigationBlocked==='function'&&workflowNavigationBlocked())){e.target.value=String(projectId);notice('Save, apply or reload pending view changes before switching project.');return;}
    if(e.target.value==='new-project'){e.target.value=String(projectId);projectForm();return;}
    if(e.target.value==='manage-projects'){e.target.value=String(projectId);openProjectDirectory();return;}
    detailEpoch++;graphPending={};leaveGraph();projectId=Number(e.target.value);workspace().catch(showError);
  });
  on('home','click',()=>navigateView('Home'));
  for(const id of ['new-project','first-project'])on(id,'click',projectForm);
  for(const id of ['new-hierarchy','first-hierarchy'])on(id,'click',()=>newTicketForm(view==='Goals'?'goal':'epic'));
  bindAccountMenu();bindIdeas();bindPresentation();bindShell();bindNavigation();bindProjectDirectory();
  bindHumanTodos();
  mountBoardControls();
  rememberCurrentLocation();
  restoreFocus();restoreBoardFocus();
  if(view==='Graph')mountGraph();if(view==='Calendar')mountCalendar();
  if(view==='Workflows')mountWorkflows();if(view==='Agents'&&session.account.administrator)mountAgentFactory();
  if(view==='Home')mountHome();if(view==='Cost')mountCost();if(view==='Backlog')mountBacklog(backlogFocusKey);
  on('lens','change',e=>{lens=e.target.value;render();});
}
async function signOut(){try{await flushCurrentLocation();await api('logout');loadGeneration++;detailEpoch++;clearPresentation();session=null;projects=[];tickets=[];ideas=[];projectId=0;view='Home';login();}catch(error){showError(error);}}
function openDialog(title,body,submit){detailEpoch++;const dialog=byId('editor');dialog.classList.remove('finder-dialog');delete dialog.dataset.dirty;dialog.innerHTML=`<form method="dialog" id="editor-form"><header class="dialog-heading"><h2>${title}</h2><button type="button" id="dismiss" class="icon-button secondary" aria-label="Close">×</button></header>${body}<div class="dialog-actions"><button type="button" class="secondary" id="cancel">Cancel</button><button type="submit">${submit||'Save'}</button></div></form>`;dialog.showModal();byId('dismiss').onclick=()=>dialog.close();byId('cancel').onclick=()=>dialog.close();dialog.querySelector('input:not(:disabled):not([type=checkbox]):not([type=radio]):not([type=hidden]),textarea:not(:disabled)')?.focus();return dialog;}
function projectForm(){const requestId=crypto.randomUUID();const dialog=openDialog('New project','<label>Name<input name="name" required maxlength="200" placeholder="A name for the work"></label><label>Project key<input name="key" placeholder="lowercase-key" pattern="[a-z0-9][a-z0-9_-]{0,79}"></label><label>Project root<input name="root" value="~/code/teleport" maxlength="4096" required></label>','Create project');byId('editor-form').onsubmit=async e=>{e.preventDefault();try{const p=await api('createProject',{...Object.fromEntries(new FormData(e.currentTarget)),requestId});projectId=p.id;view='Board';dialog.close();await workspace();notice('Project created');}catch(error){showError(error);}};}
function newTicketForm(kind=ticketCreationContext(view,lens,view==='Tickets'?workflowMap[ticketListWorkflow]?.family:undefined).kind){
  if(!projects.length){projectForm();return;}
  const selectedProject=projectId,writable=projects.filter(p=>roleAllowsWrite(p.id));
  if(selectedProject&&!writable.some(p=>p.id===selectedProject)){notice('The selected project does not permit ticket creation.');return;}
  if(!writable.length){notice('No accessible project permits ticket creation.');return;}
  const requestId=crypto.randomUUID(),human=ticketCreationContext(view,lens,view==='Tickets'?workflowMap[ticketListWorkflow]?.family:undefined).human;
  const projectField=selectedProject?`<p class="form-help">Project · ${escapeHTML(projectName(selectedProject))}</p>`:`<label>Project<select name="projectId" required><option value="" disabled selected>Choose a project</option>${writable.map(p=>`<option value="${p.id}">${escapeHTML(p.name)}</option>`).join('')}</select></label>`;
  const dialog=openDialog('New ticket',`<label>Title<input name="title" required maxlength="20000"></label>${projectField}<div class="form-grid"><label>Type<select name="kind"><option value="task" ${kind==='task'?'selected':''}>Task</option><option value="epic" ${kind==='epic'?'selected':''}>Epic</option><option value="goal" ${kind==='goal'?'selected':''}>Goal</option></select></label><label>Intent<select name="intent"><option value="agent" ${human?'':'selected'}>Agent</option><option value="human" ${human?'selected':''}>Human</option></select></label></div><label>Description<textarea name="description" rows="3" maxlength="20000"></textarea></label><label>Success criteria<textarea name="criteria" rows="2" maxlength="20000"></textarea></label>`,'Create ticket');
  byId('editor-form').onsubmit=async e=>{e.preventDefault();const form=Object.fromEntries(new FormData(e.currentTarget));form.projectId=selectedProject||Number(form.projectId);form.agent=form.intent==='agent';delete form.intent;try{await api('createTicket',{...form,requestId});dialog.close();await workspace();notice('Ticket created');}catch(error){showError(error);}};
}
function ticketHistoryHTML(history){
  return history.map((h,index)=>{const before=history[index-1]?.snapshot,after=h.snapshot,changed=before&&(before.title!==after.title||before.description!==after.description);return `<div class="revision"><span>v${h.version}</span><strong>${escapeHTML(h.action)}</strong><span>${escapeHTML(after.status)} · ${escapeHTML(h.actor)}</span>${changed?`<details><summary>Compare text</summary><p><strong>Before</strong></p><pre class="criteria-box">${escapeHTML(before.title)}\n\n${escapeHTML(before.description||'')}</pre><p><strong>After</strong></p><pre class="criteria-box">${escapeHTML(after.title)}\n\n${escapeHTML(after.description||'')}</pre></details>`:''}</div>`;}).join('');
}
async function editTicket(id){
  const epoch=++detailEpoch,account=session?.account.id,scope=projectId,token={};ticketEditorPending=token;
  try{
    const t=await api('getTicket',{id});if(t.agent)t.agreementBasis=await api('getTicketAgreements',{id});
    const [history,sourceIdeas]=await Promise.all([pages('listTicketHistory',{id}),pages('getTicketIdeas',{id})]);
    if(epoch!==detailEpoch||session?.account.id!==account||scope!==projectId||ticketEditorPending!==token)return;
    selected=t;const writable=roleAllowsWrite(t.projectId),contentLocked=t.status!=='Todo'||!writable;
    const dialog=openDialog(`<span class="number">#${t.id}</span> Ticket`,`
      <div class="editor-meta"><span class="status-chip">${escapeHTML(phaseTitle(t))}</span><span>${escapeHTML(projectName(t.projectId))} / ${escapeHTML(t.kind)}</span><span>v${t.version}</span></div>
      <label>Title<input name="title" value="${escapeHTML(t.title)}" ${contentLocked?'disabled':''} required></label>
      <label>Description<textarea name="description" rows="4" ${contentLocked?'disabled':''}>${escapeHTML(t.description)}</textarea></label>
      <label>Success criteria<textarea name="criteria" rows="3" ${contentLocked?'disabled':''}>${escapeHTML(t.criteria)}</textarea></label>
      <div class="form-grid"><label>Priority<select name="priority" ${!writable?'disabled':''}>${['critical','high','medium','low'].map(p=>`<option ${t.priority===p?'selected':''}>${p}</option>`).join('')}</select></label><label>Due<input name="due" type="date" value="${escapeHTML(t.due||'')}" ${!writable?'disabled':''}></label></div>
      <div class="form-grid"><label>Start<input name="start" type="date" value="${escapeHTML(t.start||'')}" ${!writable?'disabled':''}></label><label>ETA<input name="eta" type="date" value="${escapeHTML(t.eta||'')}" ${!writable?'disabled':''}></label></div>
      ${!t.agent&&writable?`<div class="status-actions">${humanMoves(t).map(move=>`<button type="button" class="secondary" data-transition="${escapeHTML(move.target)}">Move to ${move.role}</button>`).join('')}</div>`:''}
      ${agentHTML(t)}${nativeSessionHTML(t)}${readyReviewHTML(t)}${deliveryHTML(t)}${goalHTML(t)}${controlsHTML(t)}
      ${sourceIdeas.length?'<section><h3>Source Ideas</h3><div class="status-actions">'+sourceIdeas.map(link=>'<button type="button" class="secondary" data-source-idea="'+escapeHTML(link.ideaId)+'">'+escapeHTML(link.ideaId)+' · accepted v'+link.ideaVersion+'</button>').join('')+'</div></section>':''}
      <details class="history"><summary>History · ${history.length} revisions</summary>${ticketHistoryHTML(history)}</details>`,'Save changes');
    const guard=captureDialogCompletion(dialog),form=byId('editor-form');let saving=false;
    form.addEventListener('input',()=>dialog.dataset.dirty='true');
    dialog.querySelectorAll('[data-source-idea]').forEach(button=>button.onclick=()=>{dialog.close();openIdea(button.dataset.sourceIdea);});
    bindDeliveryControls(t,dialog);bindAgentControls(t,dialog);bindTerminalTicket(t,dialog);bindGoals(t,dialog);bindControls(t,dialog);
    if(!writable)form.querySelector('button[type=submit]').hidden=true;
    form.onsubmit=async e=>{e.preventDefault();if(saving)return;saving=true;const values=Object.fromEntries(new FormData(form));try{await api('updateTicket',{id:t.id,expectedVersion:t.version,...values});if(!guard.current())return;dialog.close();await workspace();notice('Saved');}catch(error){if(guard.current())showError(error);}finally{saving=false;}};
    dialog.querySelectorAll('[data-transition]').forEach(button=>button.onclick=()=>{if(dialog.dataset.dirty==='true'){notice('Save changes before changing status.');return;}moveHumanTicket(t,button.dataset.transition);});
  }catch(error){showError(error);}finally{if(ticketEditorPending===token)ticketEditorPending=null;}
}
function completionForm(t){if(byId('editor').dataset.dirty==='true'){notice('Save changes before changing status.');return;}byId('editor').close();const dialog=openDialog('Confirm completion',`<p>${escapeHTML(t.title)}</p><div class="criteria-box">${escapeHTML(t.criteria||'Complete the described work.')}</div><label class="checkbox-label"><input id="agreement" type="checkbox" required>Accept these completion criteria</label>`,'Complete ticket');byId('editor-form').onsubmit=e=>{e.preventDefault();if(byId('agreement').checked)transition(t,'Done',true);};}
async function transition(t,target,confirm){if(byId('editor').dataset.dirty==='true'){notice('Save changes before changing workflow.');return;}try{await api('transitionTicket',{id:t.id,expectedVersion:t.version,target,confirm});byId('editor').close();await workspace();notice(`Moved to ${target}`);}catch(error){showError(error);}}
document.addEventListener('keydown',e=>{if(!session||e.defaultPrevented||e.ctrlKey||e.metaKey||e.altKey||e.isComposing||foregroundOpen()||e.target.closest?.('input,textarea,select,[contenteditable]:not([contenteditable="false"])'))return;if(e.key.toLowerCase()==='n'){e.preventDefault();if(view==='Agents'&&session.account.administrator)agentDefinitionForm();else if(view==='Backlog'&&backlogKind==='idea')ideaForm();else newTicketForm(view==='Backlog'&&['goal','epic'].includes(backlogKind)?backlogKind:undefined);}if(e.key.toLowerCase()==='q'){e.preventDefault();const names=navigationOrder().filter(availableNavigation);navigateView(names[(names.indexOf(view)+(e.shiftKey?-1:1)+names.length)%names.length]);}});
setInterval(async()=>{if(!session||refreshing||graphInteracting||typeof connectionInterrupted!=='undefined'&&connectionInterrupted)return;refreshing=true;const actorAtStart=session.account.id,generation=loadGeneration;try{const current=await api('getSession');if(!session||generation!==loadGeneration)return;if(current.account.id!==actorAtStart){loadGeneration++;clearPresentation();session=null;login();return;}if(!preferences||current.preferences.version>preferences.version){preferences=current.preferences;applyAppearance(preferences);}if((current.contentRevision??current.revision)!==(session.contentRevision??session.revision)&&!foregroundOpen()&&!root.querySelector('.board-workspace .ticket:hover,.ticket-list-card:hover,.backlog-row:hover'))await workspace(true);}catch(error){if(generation===loadGeneration&&(error.status===401||error.status===403)){loadGeneration++;clearPresentation();session=null;login();}}finally{refreshing=false;}},1000);

function foregroundOpen(){return ticketEditorPending||(typeof agentLaunchBusy==='function'&&agentLaunchBusy())||document.querySelector('tk-dialog[open]')||document.querySelector('.account-menu:not([hidden])')||document.querySelector('.choice-list')||document.activeElement?.matches('main input,main textarea,main select,main .choice-button,main [data-graph-node]')||graphBusy()||(typeof backlogBusy==='function'&&backlogBusy())||presentationBusy()||calendarBusy()||(typeof humanTicketDrag!=='undefined'&&humanTicketDrag)||(typeof boardTicketDrag!=='undefined'&&boardTicketDrag)||(typeof navigationBusy==='function'&&navigationBusy())||(typeof ticketMenuBusy==='function'&&ticketMenuBusy())||(typeof workflowViewBusy==='function'&&workflowViewBusy());}
