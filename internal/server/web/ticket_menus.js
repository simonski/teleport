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
  if(!['Board','Todos','Graph'].includes(view))return null;
  const card=target.closest?.('.board-workspace [data-ticket-card]');
  if(card)return {id:Number(card.dataset.ticketCard),source:card.querySelector('.ticket-open')};
  const entry=target.closest?.('#graph-list [data-node]'),node=graphData?.nodes.find(row=>row.id===entry?.dataset.node);
  return node?.ticketId?{id:node.ticketId,source:entry}:null;
}
function attachedRunningSession(ticket,rows){return rows.find(row=>row.ticketId===ticket.id&&row.projectId===ticket.projectId&&row.state==='running'&&ticket.execution==='local:'+row.id);}
function ticketMenuEntries(ticket,rows,nativePending=false){
  const entries=[{id:'open',label:'Open ticket'}],attached=attachedRunningSession(ticket,rows);
  if(ticket.agent&&view!=='Todos')entries.push({id:'prompt',label:'Prompt',sessionId:attached?.id});
  if(typeof roleAllowsWrite==='function'&&roleAllowsWrite(ticket.projectId)){
    if(view==='Todos'&&!ticket.agent)for(const move of humanMoves(ticket))entries.push({id:'move-'+move.target,label:'Move to '+move.role,target:move.target});
    if(canQueueRefinement(ticket)&&ticket.status==='Todo')entries.push({id:'refine',label:'Send to refine'});
    if(ticket.agent&&ticket.status==='Refine'&&canQueueRefinement(ticket))entries.push({id:'return-todo',label:'Send back to Todos'});
  }
  if(ticket.question&&!ticket.question.resolved&&!ticket.question.obsolete||nativePending)entries.push({id:'questions',label:'Questions'});
  if(attached&&view!=='Todos')entries.push({id:'teleport',label:'Teleport · '+attached.id,sessionId:attached.id});
  return entries;
}
async function showTicketMenu(id,source,point=null){
  if(!session||!source?.isConnected||menuForeground())return;
  dismissTicketMenu();const epoch=++ticketMenuEpoch,actor=session.account.id,generation=loadGeneration,detail=++detailEpoch;ticketMenuPending=true;
  const current=()=>ticketMenuEpoch===epoch&&session?.account.id===actor&&loadGeneration===generation&&detailEpoch===detail&&source.isConnected&&!menuForeground();
  try{
    const [ticket,rows]=await Promise.all([api('getTicket',{id}),view==='Todos'?Promise.resolve([]):pages('listSessions')]);
    if(!current())return;
    const attached=attachedRunningSession(ticket,rows),requests=attached?.harness==='codex'?[await api('getSessionRequests',{id:attached.id})]:[];
    if(!current())return;
    const element=document.createElement('div');element.className='ticket-context-menu';element.setAttribute('role','menu');element.setAttribute('aria-label','Ticket #'+id+' actions');
    const entries=ticketMenuEntries(ticket,rows,requests.some(group=>(group.items||[]).some(request=>request.state==='pending')));
    element.innerHTML=`<div class="ticket-context-title">#${id} · ${escapeHTML(phaseTitle(ticket))}</div>${entries.map(entry=>`<button type="button" role="menuitem" data-ticket-menu="${entry.id}">${escapeHTML(entry.label)}</button>`).join('')}`;
    document.body.append(element);ticketMenu={element,source};ticketMenuPending=false;
    const anchor=source.getBoundingClientRect(),x=point?.x??anchor.left,y=point?.y??anchor.bottom;
    element.style.left=Math.max(8,Math.min(x,innerWidth-element.offsetWidth-8))+'px';element.style.top=Math.max(8,Math.min(y,innerHeight-element.offsetHeight-8))+'px';
    const buttons=[...element.querySelectorAll('button')];buttons[0].focus();
    element.addEventListener('keydown',e=>{
      if(['ArrowDown','ArrowUp','Home','End'].includes(e.key)){e.preventDefault();const index=buttons.indexOf(document.activeElement),next=e.key==='Home'?0:e.key==='End'?buttons.length-1:(index+(e.key==='ArrowDown'?1:-1)+buttons.length)%buttons.length;buttons[next].focus();}
      else if(e.key==='Tab')dismissTicketMenu(true);
    });
    element.addEventListener('click',e=>{const button=e.target.closest('[data-ticket-menu]'),entry=entries.find(item=>item.id===button?.dataset.ticketMenu);if(!entry)return;dismissTicketMenu(true);if(entry.target)moveHumanTicket(ticket,entry.target);else if(entry.id==='refine'||entry.id==='return-todo')changeRefinementQueue(ticket,entry.id==='refine'?'requestRefinement':'returnRefinementToTodo');else if(entry.id==='teleport')teleportTicketSession(id,entry.sessionId).catch(showError);else if(entry.id==='prompt')showTicketPrompt(id,entry.sessionId).catch(showError);else editTicket(id);});
  }catch(error){if(current())showError(error);}
  finally{if(ticketMenuEpoch===epoch)ticketMenuPending=false;}
}
async function showTicketPrompt(id,sessionId){
  const actor=session?.account.id,generation=loadGeneration,detail=++detailEpoch;
  const input={id};if(sessionId)input.sessionId=sessionId;
  let result;
  try{result=await api('getPrompt',input);}catch(error){if(!sessionId)throw error;result=await api('getPrompt',{id});}
  if(session?.account.id!==actor||loadGeneration!==generation||detailEpoch!==detail||menuForeground())return;
  const dialog=openDialog('Prompt · #'+id,`<p>${result.saved?'Saved launch · '+escapeHTML(result.sessionId):'Current composition · preview'}</p><details open><summary>Complete prompt</summary><pre class="criteria-box">${escapeHTML(result.prompt.text)}</pre></details>${result.prompt.contributions.map(part=>`<details><summary>${escapeHTML(part.source)} · ${escapeHTML(part.version)}</summary><pre class="criteria-box">${escapeHTML(part.text)}</pre></details>`).join('')}`,'Close');
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
  e.preventDefault();target.source.focus({preventScroll:true});showTicketMenu(target.id,target.source,{x:e.clientX,y:e.clientY});
});
document.addEventListener('keydown',e=>{
  if(e.key==='Escape'&&ticketMenuBusy()){e.preventDefault();e.stopImmediatePropagation();dismissTicketMenu(true);return;}
  if(e.key==='F10'&&e.shiftKey&&!e.ctrlKey&&!e.metaKey&&!e.altKey&&!e.isComposing){const target=menuTicketSource(e.target);if(target&&!menuForeground()){e.preventDefault();showTicketMenu(target.id,target.source);}}
},true);
document.addEventListener('pointerdown',e=>{if(ticketMenuPending||ticketMenu&&!ticketMenu.element.contains(e.target))dismissTicketMenu();},true);
window.addEventListener('resize',()=>dismissTicketMenu());
window.addEventListener('scroll',e=>{if(ticketMenu&&!ticketMenu.element.contains(e.target))dismissTicketMenu();},true);
