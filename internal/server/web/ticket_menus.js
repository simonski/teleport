"use strict";
let ticketMenu=null,ticketMenuEpoch=0,ticketMenuPending=false;
function ticketMenuBusy(){return !!ticketMenu||ticketMenuPending;}
function dismissTicketMenu(restore=false){
  ticketMenuEpoch++;ticketMenuPending=false;
  const previous=ticketMenu;ticketMenu=null;previous?.element.remove();
  if(restore&&previous?.source.isConnected)previous.source.focus({preventScroll:true});
}
function menuForeground(text=true){return document.querySelector('tk-dialog[open],.account-menu:not([hidden]),.choice-list')||text&&document.activeElement?.closest('input,textarea,select,[contenteditable]:not([contenteditable="false"])');}
function menuTicketSource(target){
  if(view==='Backlog'){
    const source=target.closest?.('[data-backlog-open]');if(!source)return null;
    const key=source.dataset.backlogOpen;return key.startsWith('ticket:')?{id:Number(key.slice(7)),source}:{ideaId:key,source};
  }
  if(!['Board','Todos','Graph'].includes(view))return null;
  const card=target.closest?.('.board-workspace [data-ticket-card]');
  if(card)return {id:Number(card.dataset.ticketCard),source:card.querySelector('.ticket-open')};
  const entry=target.closest?.('#graph-list [data-node]'),node=graphData?.nodes.find(row=>row.id===entry?.dataset.node);
  return node?.ticketId?{id:node.ticketId,source:entry}:null;
}
function deleteBacklogRecord(record,idea=false){
  const title=idea?'Delete idea':'Delete ticket',dialog=openDialog(title,`<p>${escapeHTML(record.title)}</p><p>This hides the selected record and retains its history.</p>`,title),completion=captureDialogCompletion(dialog);let pending=false;
  byId('editor-form').onsubmit=async event=>{event.preventDefault();if(pending)return;pending=true;try{await api(idea?'setIdeaVisibility':'setTicketVisibility',{id:record.id,expectedVersion:record.version,deleted:true});if(!completion.current())return;dialog.close();await workspace();if(completion.follow())notice(title+' saved');}catch(error){if(completion.current())showError(error);}finally{pending=false;}};
}
function showBacklogIdeaMenu(id,source,point=null){
  if(!session||!source?.isConnected||menuForeground())return;
  const idea=ideas.find(item=>item.id===id);if(!idea)return;
  dismissTicketMenu();const element=document.createElement('div');element.className='ticket-context-menu';element.setAttribute('role','menu');element.setAttribute('aria-label','Idea actions');
  const canDraft=ideaCanWrite(idea)&&!idea.archived&&!idea.deleted;
  element.innerHTML=`<div class="ticket-context-title">${escapeHTML(idea.id)} · Idea</div><button type="button" role="menuitem" data-idea-menu="open">Open idea</button>${canDraft?'<button type="button" role="menuitem" data-idea-menu="propose">Draft work proposal</button>':''}<button type="button" role="menuitem" disabled>Refine</button><button type="button" role="menuitem" disabled>Breakdown</button><button type="button" role="menuitem" disabled>Spellcheck</button><button type="button" role="menuitem" disabled>Assign to epic</button><button type="button" role="menuitem" data-idea-menu="delete" ${ideaCanControl(idea)&&!idea.deleted?'':'disabled'}>Delete</button><p class="ticket-context-note">Agent actions are not available yet. Ideas can link work through a proposal.</p>`;
  document.body.append(element);ticketMenu={element,source,anchor:source.getBoundingClientRect(),width:innerWidth,height:innerHeight};
  const anchor=source.getBoundingClientRect();element.style.left=Math.max(8,Math.min(point?.x??anchor.left,innerWidth-element.offsetWidth-8))+'px';element.style.top=Math.max(8,Math.min(point?.y??anchor.bottom,innerHeight-element.offsetHeight-8))+'px';
  const buttons=[...element.querySelectorAll('button:not(:disabled)')];buttons[0].focus({preventScroll:true});
  element.addEventListener('keydown',event=>{if(['ArrowDown','ArrowUp','Home','End'].includes(event.key)){event.preventDefault();const index=buttons.indexOf(document.activeElement),next=event.key==='Home'?0:event.key==='End'?buttons.length-1:(index+(event.key==='ArrowDown'?1:-1)+buttons.length)%buttons.length;buttons[next].focus();}else if(event.key==='Tab')dismissTicketMenu(true);});
  element.addEventListener('click',event=>{const action=event.target.closest('[data-idea-menu]')?.dataset.ideaMenu;if(!action)return;dismissTicketMenu(true);if(action==='open')openIdea(id);else if(action==='propose')ideaProposalForm(idea);else if(action==='delete')deleteBacklogRecord(idea,true);});
}
function attachedRunningSession(ticket,rows){return rows.find(row=>row.ticketId===ticket.id&&row.projectId===ticket.projectId&&row.state==='running'&&ticket.execution==='local:'+row.id);}
function ticketMenuEntries(ticket,rows,nativePending=false,projectAgents=[]){
  if(view==='Todos'&&!ticket.agent){
    const canWrite=typeof roleAllowsWrite==='function'&&roleAllowsWrite(ticket.projectId),moves=canWrite?humanMoves(ticket):[],roles=['Todo','Doing','Done'],at=roles.indexOf(ticket.status);
    const relative=offset=>moves.find(move=>roles.indexOf(move.role)===at+offset);
    const ahead=relative(1),behind=relative(-1);
    return [
      {id:'ahead',label:'Move to '+(roles[at+1]||'next lane'),target:ahead?.target,disabled:!ahead},
      {id:'behind',label:'Move to '+(roles[at-1]||'previous lane'),target:behind?.target,disabled:!behind},
      {id:'status',label:'Status',submenu:true,disabled:!moves.length},
      {id:'priority',label:'Priority',submenu:true,disabled:!canWrite},
      {id:'prompt',label:'Prompt'},
      {id:'agents',label:'Agents',submenu:true},
      {id:'open',label:'Open'},
      {id:'delete',label:'Delete',disabled:ticket.protected||!roleAllowsMaintain(ticket.projectId)||ticket.archived||ticket.deleted||ticket.active||!!ticket.claim||!!ticket.execution},
    ];
  }
  const entries=[{id:'open',label:'Open ticket'}],attached=attachedRunningSession(ticket,rows);
  if(ticket.agent&&view!=='Todos')entries.push({id:'prompt',label:'Prompt',sessionId:attached?.id});
  if(typeof roleAllowsWrite==='function'&&roleAllowsWrite(ticket.projectId)){
    const epicRefine=view==='Backlog'&&ticket.kind==='epic'&&ticket.agent&&!ticket.protected&&!ticket.archived&&!ticket.deleted&&!ticket.active&&!ticket.claim&&!ticket.execution&&!ticket.delivery&&!(ticket.question&&!ticket.question.resolved&&!ticket.question.obsolete);
    if((canQueueRefinement(ticket)||epicRefine)&&ticket.status==='Todo')entries.push({id:'refine',label:view==='Backlog'?'Refine':'Send to refine'});
    if(ticket.agent&&ticket.status==='Refine'&&canQueueRefinement(ticket))entries.push({id:'return-todo',label:'Send back to todo'});
  }
  if(view==='Backlog'){
    if(!entries.some(entry=>entry.id==='refine'))entries.push({id:'refine-unavailable',label:'Refine',disabled:true});
    entries.push({id:'breakdown',label:'Breakdown',disabled:true},{id:'spellcheck',label:'Spellcheck',disabled:true});
    entries.push({id:'assign-epic',label:'Assign to epic',submenu:true,disabled:ticket.kind!=='task'||!backlogCanMove(ticket)||!backlogLoaded||backlogPending});
    entries.push({id:'delete',label:'Delete',disabled:ticket.protected||!roleAllowsMaintain(ticket.projectId)||ticket.archived||ticket.deleted||ticket.active||!!ticket.claim||!!ticket.execution});
  }
  if(projectAgents.length&&typeof roleAllowsWrite==='function'&&roleAllowsWrite(ticket.projectId))entries.push({id:'agents',label:'Agents',submenu:true});
  if(ticket.question&&!ticket.question.resolved&&!ticket.question.obsolete||nativePending)entries.push({id:'questions',label:'Questions'});
  if(attached&&view!=='Todos')entries.push({id:'teleport',label:'Teleport · '+attached.id,sessionId:attached.id});
  return entries;
}
async function setTicketPriority(ticket,priority){
  try{await api('updateTicket',{id:ticket.id,expectedVersion:ticket.version,priority});await workspace();}
  catch(error){showError(error);}
}
async function showTicketMenu(id,source,point=null){
  if(!session||!source?.isConnected||menuForeground())return;
  dismissTicketMenu();const epoch=++ticketMenuEpoch,actor=session.account.id,generation=loadGeneration,detail=++detailEpoch;ticketMenuPending=true;
  const current=()=>ticketMenuEpoch===epoch&&session?.account.id===actor&&loadGeneration===generation&&detailEpoch===detail&&source.isConnected&&!menuForeground();
  try{
    const [ticket,rows]=await Promise.all([api('getTicket',{id}),view==='Todos'?Promise.resolve([]):pages('listSessions')]);
    if(!current())return;
    const [projectAgents,projectUsages,marks]=roleAllowsWrite(ticket.projectId)?await Promise.all([api('listProjectAgents',{projectId:ticket.projectId}),view==='Todos'?api('listAgentUsages',{projectId:ticket.projectId}):Promise.resolve({items:[]}),view==='Todos'?api('getTicketSpellchecks',{id:ticket.id}):Promise.resolve({items:[]})]):[{items:[]},{items:[]},{items:[]}];
    if(!current())return;
    const attached=attachedRunningSession(ticket,rows),requests=attached?.harness==='codex'?[await api('getSessionRequests',{id:attached.id})]:[];
    if(!current())return;
    const element=document.createElement('div');element.className='ticket-context-menu';element.setAttribute('role','menu');element.setAttribute('aria-label','Ticket #'+id+' actions');
    const entries=ticketMenuEntries(ticket,rows,requests.some(group=>(group.items||[]).some(request=>request.state==='pending')),projectAgents.items);
    const epics=view==='Backlog'?tickets.filter(epic=>epic.kind==='epic'&&epic.projectId===ticket.projectId&&!epic.archived&&!epic.deleted).sort((a,b)=>a.title.localeCompare(b.title)||a.id-b.id):[];
    const moves=view==='Todos'?humanMoves(ticket):[];
    function submenuContents(entry){
      if(entry.id==='assign-epic')return epics.map(epic=>`<button type="button" role="menuitem" data-ticket-epic="${epic.id}" ${epic.id===ticket.parentId?'disabled':''}>${escapeHTML(epic.title)}${epic.id===ticket.parentId?' · current':''}</button>`).join('');
      if(entry.id==='status')return ['Todo','Doing','Done'].map(role=>{const move=moves.find(item=>item.role===role);return `<button type="button" role="menuitem" data-ticket-status="${escapeHTML(move?.target||'')}" ${!move?'disabled':''}>${role}${role===ticket.status?' · current':''}</button>`;}).join('');
      if(entry.id==='agents')return view==='Todos'?`${projectUsages.items.filter(item=>item.enabled).map(item=>{const active=projectAgents.items.some(agent=>agent.id===item.agentId),visible=ticket.kind==='task'&&!ticket.agent&&!ticket.protected&&!ticket.archived&&!ticket.deleted;if(item.kind==='todo-conversation')return `<button type="button" role="menuitem" data-agent-usage="${escapeHTML(item.id)}" ${active&&visible?'':'disabled'}>${escapeHTML(item.name)}</button>`;if(item.kind!=='todo-spellcheck')return '';const mark=marks.items.find(row=>row.usageId===item.id),eligible=active&&visible&&ticket.status==='Todo'&&!ticket.active&&!ticket.claim&&!ticket.execution;return `<button type="button" role="menuitem" data-spellcheck-usage="${escapeHTML(item.id)}" ${eligible?'':'disabled'}>${mark?.current?'✓ ':''}${escapeHTML(item.name)}${mark&&!mark.current?(mark.reason==='configuration_changed'?' · checked before configuration changed':' · checked before text changed'):''}</button>`;}).join('')}<button type="button" role="menuitem" data-ticket-agent="refine" ${roleAllowsWrite(ticket.projectId)&&ticket.status==='Todo'&&canQueueRefinement(ticket)?'':'disabled'}>Refine</button>`:projectAgents.items.map(agent=>`<button type="button" role="menuitem" data-ticket-persona="${escapeHTML(agent.id)}">${escapeHTML(agent.name)} · conversation</button>`).join('');
      return ['critical','high','medium','low'].map(priority=>`<button type="button" role="menuitem" data-ticket-priority="${priority}" ${priority===ticket.priority?'disabled':''}>${priority[0].toUpperCase()+priority.slice(1)}${priority===ticket.priority?' · current':''}</button>`).join('');
    }
    element.innerHTML=`<div class="ticket-context-title">#${id} · ${escapeHTML(phaseTitle(ticket))}</div>${entries.map(entry=>entry.submenu?`<button type="button" role="menuitem" data-ticket-menu="${entry.id}" aria-haspopup="menu" aria-expanded="false" ${entry.disabled||entry.id==='assign-epic'&&!epics.length?'disabled':''}>${escapeHTML(entry.label)} <span aria-hidden="true">›</span></button><div class="ticket-context-submenu" role="menu" aria-label="${escapeHTML(entry.label)}" hidden>${submenuContents(entry)}</div>`:`<button type="button" role="menuitem" data-ticket-menu="${entry.id}" ${entry.disabled?'disabled':''}>${escapeHTML(entry.label)}</button>`).join('')}${view==='Backlog'?'<p class="ticket-context-note">Breakdown and spellcheck are not available yet.</p>':''}`;
    document.body.append(element);ticketMenu={element,source,anchor:source.getBoundingClientRect(),width:innerWidth,height:innerHeight};ticketMenuPending=false;
    const anchor=source.getBoundingClientRect(),x=point?.x??anchor.left,y=point?.y??anchor.bottom;
    element.style.left=Math.max(8,Math.min(x,innerWidth-element.offsetWidth-8))+'px';element.style.top=Math.max(8,Math.min(y,innerHeight-element.offsetHeight-8))+'px';
    const buttons=[...element.querySelectorAll(':scope > button:not(:disabled)')];let submenu=null,trigger=null;buttons[0].focus({preventScroll:true});
    function openSubmenu(button,focus=false){const next=button?.nextElementSibling;if(!next?.classList.contains('ticket-context-submenu')||button.disabled)return;if(submenu&&submenu!==next){submenu.hidden=true;trigger.setAttribute('aria-expanded','false');}submenu=next;trigger=button;submenu.hidden=false;trigger.setAttribute('aria-expanded','true');const box=trigger.getBoundingClientRect();submenu.style.left=(box.right+submenu.offsetWidth+8>innerWidth?Math.max(8,box.left-submenu.offsetWidth):box.right)+'px';submenu.style.top=Math.max(8,Math.min(box.top,innerHeight-submenu.offsetHeight-8))+'px';if(focus)submenu.querySelector('button:not(:disabled)')?.focus();}
    element.querySelectorAll('[aria-haspopup=menu]').forEach(button=>button.addEventListener('pointerenter',()=>openSubmenu(button)));
    element.addEventListener('keydown',e=>{
      const subButtons=submenu&&!submenu.hidden?[...submenu.querySelectorAll('button:not(:disabled)')]:[],inSub=submenu?.contains(document.activeElement),items=inSub?subButtons:buttons;
      if(e.key==='ArrowRight'&&document.activeElement?.getAttribute('aria-haspopup')==='menu'){e.preventDefault();openSubmenu(document.activeElement,true);}
      else if(e.key==='ArrowLeft'&&inSub){e.preventDefault();submenu.hidden=true;trigger.setAttribute('aria-expanded','false');trigger.focus();}
      else if(['ArrowDown','ArrowUp','Home','End'].includes(e.key)&&items.length){e.preventDefault();const index=items.indexOf(document.activeElement),next=e.key==='Home'?0:e.key==='End'?items.length-1:(index+(e.key==='ArrowDown'?1:-1)+items.length)%items.length;items[next].focus();}
      else if(e.key==='Tab')dismissTicketMenu(true);
    });
    element.addEventListener('click',e=>{const epic=e.target.closest('[data-ticket-epic]'),status=e.target.closest('[data-ticket-status]'),priority=e.target.closest('[data-ticket-priority]'),agent=e.target.closest('[data-ticket-agent]'),persona=e.target.closest('[data-ticket-persona]'),spell=e.target.closest('[data-spellcheck-usage]'),action=e.target.closest('[data-agent-usage]');if(action&&!action.disabled){const usage=projectUsages.items.find(item=>item.id===action.dataset.agentUsage),definition=projectAgents.items.find(item=>item.id===usage?.agentId);dismissTicketMenu(true);startAgentConversation(definition,ticket.projectId,ticket.id,usage).catch(showError);return;}if(spell&&!spell.disabled){const usage=projectUsages.items.find(item=>item.id===spell.dataset.spellcheckUsage);dismissTicketMenu(true);startTicketSpellcheck(ticket,usage).catch(showError);return;}if(epic){const parentId=Number(epic.dataset.ticketEpic);dismissTicketMenu(true);backlogMoveTo(ticket,parentId,backlogRows(tickets,ideas,backlogOrder.keys).map(row=>row.key));return;}if(status&&!status.disabled){const target=status.dataset.ticketStatus;dismissTicketMenu(true);moveHumanTicket(ticket,target);return;}if(priority&&!priority.disabled){const value=priority.dataset.ticketPriority;dismissTicketMenu(true);setTicketPriority(ticket,value);return;}if(agent&&!agent.disabled&&agent.dataset.ticketAgent==='refine'){dismissTicketMenu(true);changeRefinementQueue(ticket,'requestRefinement');return;}if(persona){const definition=projectAgents.items.find(item=>item.id===persona.dataset.ticketPersona);dismissTicketMenu(true);startAgentConversation(definition,ticket.projectId,ticket.id).catch(showError);return;}const button=e.target.closest('[data-ticket-menu]'),entry=entries.find(item=>item.id===button?.dataset.ticketMenu);if(!entry||entry.disabled)return;if(entry.submenu){openSubmenu(button,true);return;}dismissTicketMenu(true);if(entry.target)moveHumanTicket(ticket,entry.target);else if(entry.id==='refine'||entry.id==='return-todo')changeRefinementQueue(ticket,entry.id==='refine'?'requestRefinement':'returnRefinementToTodo');else if(entry.id==='delete')deleteBacklogRecord(ticket);else if(entry.id==='teleport')teleportTicketSession(id,entry.sessionId).catch(showError);else if(entry.id==='prompt')showTicketPrompt(id,entry.sessionId).catch(showError);else editTicket(id);});
  }catch(error){if(current())showError(error);}
  finally{if(ticketMenuEpoch===epoch)ticketMenuPending=false;}
}
const spellcheckRequests=new Map();
async function startTicketSpellcheck(ticket,usage){
  const actor=session.account.id,key=[actor,ticket.id,ticket.version,usage.id,usage.version].join(':'),requestId=spellcheckRequests.get(key)||crypto.randomUUID();spellcheckRequests.set(key,requestId);
  let run;try{run=await api('startSpellcheck',{ticketId:ticket.id,expectedVersion:ticket.version,usageId:usage.id,usageVersion:usage.version,requestId});spellcheckRequests.delete(key);}catch(error){if(error.status&&error.status<500)spellcheckRequests.delete(key);throw error;}
  notice(usage.name+' started');
  async function poll(){if(session?.account.id!==actor)return;try{const status=await api('getSpellcheckRun',{id:run.id});if(session?.account.id!==actor)return;if(status.state==='running'){setTimeout(poll,1000);return;}notice(status.state==='completed'?usage.name+' completed':usage.name+' failed: '+(status.error||'inspect session'));if(status.state==='completed'){const fresh=await api('getTicket',{id:ticket.id});if(session?.account.id!==actor)return;tickets=tickets.map(row=>row.id===fresh.id?fresh:row);const title=document.querySelector(`.ticket-open[data-ticket="${fresh.id}"] h3`);if(title)title.textContent=fresh.title;}if(!foregroundOpen())await workspace(true);}catch{setTimeout(poll,3000);}}
  poll();
}
async function showTicketPrompt(id,sessionId){
  const actor=session?.account.id,generation=loadGeneration,detail=++detailEpoch;
  const input={id};if(sessionId)input.sessionId=sessionId;
  let result;
  try{result=await api('getPrompt',input);}catch(error){if(!sessionId)throw error;result=await api('getPrompt',{id});}
  if(session?.account.id!==actor||loadGeneration!==generation||detailEpoch!==detail||menuForeground())return;
  const dialog=openDialog('Prompt · #'+id,`<p>${result.saved?'Saved launch · '+escapeHTML(result.sessionId):result.hypothetical?'Refinement preview · this human ticket has not been sent to an agent':'Current composition · preview'}</p><details open><summary>Complete prompt</summary><pre class="criteria-box">${escapeHTML(result.prompt.text)}</pre></details>${result.prompt.contributions.map(part=>`<details><summary>${escapeHTML(part.source)} · ${escapeHTML(part.version)}</summary><pre class="criteria-box">${escapeHTML(part.text)}</pre></details>`).join('')}`,'Close');
  byId('cancel').hidden=true;byId('editor-form').onsubmit=e=>{e.preventDefault();dialog.close();};
}
async function teleportTicketSession(id,sessionId){
  const actor=session?.account.id,generation=loadGeneration,detail=++detailEpoch;
  const [ticket,rows]=await Promise.all([api('getTicket',{id}),pages('listSessions')]);
  if(session?.account.id!==actor||loadGeneration!==generation||detailEpoch!==detail||menuForeground())return;
  if(attachedRunningSession(ticket,rows)?.id!==sessionId){notice('The attached harness is no longer running. Open the ticket to inspect its current state.');return;}
  await openTerminal(sessionId);
}
document.addEventListener('contextmenu',e=>{
  const target=menuTicketSource(e.target);if(!target||menuForeground(false))return;
  e.preventDefault();target.source.focus({preventScroll:true});if(target.ideaId)showBacklogIdeaMenu(target.ideaId,target.source,{x:e.clientX,y:e.clientY});else showTicketMenu(target.id,target.source,{x:e.clientX,y:e.clientY});
});
document.addEventListener('keydown',e=>{
  if(e.key==='Escape'&&ticketMenuBusy()){e.preventDefault();e.stopImmediatePropagation();dismissTicketMenu(true);return;}
  if(e.key==='F10'&&e.shiftKey&&!e.ctrlKey&&!e.metaKey&&!e.altKey&&!e.isComposing){const target=menuTicketSource(e.target);if(target&&!menuForeground()){e.preventDefault();if(target.ideaId)showBacklogIdeaMenu(target.ideaId,target.source);else showTicketMenu(target.id,target.source);}}
},true);
document.addEventListener('pointerdown',e=>{if(ticketMenuPending||ticketMenu&&!ticketMenu.element.contains(e.target))dismissTicketMenu();},true);
window.addEventListener('resize',()=>{if(ticketMenu&&(ticketMenu.width!==innerWidth||ticketMenu.height!==innerHeight))dismissTicketMenu();});
window.addEventListener('scroll',e=>{if(ticketMenu&&!ticketMenu.element.contains(e.target)){const rect=ticketMenu.source.getBoundingClientRect(),anchor=ticketMenu.anchor;if(Math.abs(rect.left-anchor.left)>.5||Math.abs(rect.top-anchor.top)>.5)dismissTicketMenu();}},true);
