"use strict";
let ticketListWorkflow='',ticketListEpoch=0,ticketListDispose=null;
function leaveTicketList(){ticketListEpoch++;ticketListDispose?.();ticketListDispose=null;}
function resetTicketList(){leaveTicketList();ticketListWorkflow='';}
function ticketListDefinitions(){return Object.values(workflowMap).filter(definition=>definition.published).sort((a,b)=>a.title.localeCompare(b.title)||b.version-a.version);}
function ticketsListHTML(){
  const definitions=ticketListDefinitions();if(!definitions.some(definition=>flowKey(definition)===ticketListWorkflow))ticketListWorkflow=currentProject()?.agentWorkflow||flowKey(definitions[0]||{id:'',version:0,published:true});
  return `<section class="tickets-list"><header class="summary-heading"><div><h2>Tickets</h2><p>Epics and stories · workflow list</p></div><label class="compact">Workflow<select id="tickets-workflow">${definitions.map(definition=>`<option value="${escapeHTML(flowKey(definition))}" ${flowKey(definition)===ticketListWorkflow?'selected':''}>${escapeHTML(definition.title)} · v${definition.version}</option>`).join('')}</select></label><span class="shortcut-note"><kbd>N</kbd> New ticket</span></header><p id="tickets-list-status" class="save-status" role="status"></p><div class="tickets-list-scroll" tabindex="0" role="region" aria-label="Epic and story workflow board"><div id="tickets-list-content"></div></div></section>`;
}
function ticketListGroups(rows,workflow){
  const groups=new Map(),epics=new Map(rows.filter(ticket=>ticket.kind==='epic').map(ticket=>[ticket.id,ticket]));
  for(const ticket of rows)if(ticket.kind==='task'&&!ticket.protected&&!ticket.archived&&!ticket.deleted&&ticket.workflow===workflow&&ticket.status!=='Cancelled'){
    const key=`${ticket.projectId}:${ticket.parentId}`;if(!groups.has(key))groups.set(key,{epic:epics.get(ticket.parentId),parentId:ticket.parentId,projectId:ticket.projectId,tasks:[]});groups.get(key).tasks.push(ticket);
  }
  return [...groups.values()].sort((a,b)=>a.projectId-b.projectId||a.parentId-b.parentId);
}
async function mountTicketList(focusId){
  leaveTicketList();if(view!=='Tickets')return;
  const epoch=ticketListEpoch,actor=session.account.id,scope=projectId,host=byId('tickets-list-content'),definition=workflowMap[ticketListWorkflow];
  on('tickets-workflow','change',event=>{ticketListWorkflow=event.target.value;render();});if(!definition){host.innerHTML='<p class="quiet-state">No workflow available.</p>';return;}
  try{const model=await import('/graph_2d_model.js');if(epoch!==ticketListEpoch||session?.account.id!==actor||scope!==projectId||view!=='Tickets'||!host.isConnected)return;
    const groups=ticketListGroups(tickets,ticketListWorkflow),phases=definition.phases.filter(phase=>phase.role!=='Cancelled');
    const card=(ticket,kind='task')=>`<button type="button" class="graph2d-card ticket-list-card" data-list-ticket="${ticket.id}" data-ticket="${ticket.id}" data-kind="${kind}" data-status="${escapeHTML(ticket.status)}"><span class="graph2d-icon" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="${model.graphNodeIcons[kind]}"/></svg></span><span class="graph2d-copy"><strong>${escapeHTML(ticket.title)}</strong><span class="graph2d-meta">#${ticket.id} · ${escapeHTML(phaseTitle(ticket))}${ticket.attention?' · Attention':''}</span></span><span class="graph2d-status" aria-hidden="true">${ticket.status==='Done'?'✓':ticket.status==='Doing'?'↻':'·'}</span></button>`;
    host.style.setProperty('--ticket-lanes',String(phases.length));
    host.innerHTML=`<div class="ticket-list-head"><span>Epic / stories</span>${phases.map(phase=>`<span>${escapeHTML(phase.title)}<small>${groups.reduce((count,group)=>count+group.tasks.filter(ticket=>ticket.phase===phase.id).length,0)}</small></span>`).join('')}</div>${groups.map(group=>`<section class="ticket-list-group" aria-label="${escapeHTML(group.epic?.title||'Unassigned epic')}"><svg class="ticket-list-links" aria-hidden="true"></svg><div class="ticket-list-epic">${group.epic?card(group.epic,'epic'):'<div class="unassigned-epic">Unassigned epic</div>'}<small>${escapeHTML(projectName(group.projectId))} · ${group.tasks.length} stories</small></div>${phases.map(phase=>`<div class="ticket-list-lane" data-list-phase="${escapeHTML(phase.id)}">${group.tasks.filter(ticket=>ticket.phase===phase.id).sort((a,b)=>Number(!!b.attention)-Number(!!a.attention)||a.id-b.id).map(ticket=>card(ticket)).join('')}</div>`).join('')}</section>`).join('')||'<div class="quiet-state">No stories use this workflow in the selected project.</div>'}`;
    host.querySelectorAll('[data-ticket]').forEach(button=>button.onclick=()=>editTicket(Number(button.dataset.ticket)));
    function connect(){if(!host.isConnected)return;for(const group of host.querySelectorAll('.ticket-list-group')){
      const svg=group.querySelector('svg.ticket-list-links'),base=group.getBoundingClientRect(),parent=group.querySelector('.ticket-list-epic .ticket-list-card,.unassigned-epic'),rect=parent.getBoundingClientRect();
      svg.setAttribute('viewBox',`0 0 ${base.width} ${base.height}`);svg.innerHTML='';
      for(const child of group.querySelectorAll('.ticket-list-lane .ticket-list-card')){const target=child.getBoundingClientRect(),x=rect.right-base.left,y=rect.top+rect.height/2-base.top,tx=target.left-base.left,ty=target.top+target.height/2-base.top,bend=Math.max(28,(tx-x)*.45),path=document.createElementNS(svg.namespaceURI,'path');path.setAttribute('d',`M${x} ${y}C${x+bend} ${y} ${tx-bend} ${ty} ${tx} ${ty}`);path.dataset.linkTicket=child.dataset.ticket;svg.append(path);}
    }}
    function emphasize(event){const card=event.target.closest?.('.ticket-list-card'),group=card?.closest('.ticket-list-group');for(const row of host.querySelectorAll('.ticket-list-card'))row.classList.toggle('graph2d-muted',!!card&&row!==card&&row!==group.querySelector('.ticket-list-epic .ticket-list-card')&&!(card.dataset.kind==='epic'&&group.contains(row)));for(const path of host.querySelectorAll('[data-link-ticket]'))path.classList.toggle('muted',!!card&&!(card.dataset.kind==='epic'?group.contains(path):path.dataset.linkTicket===card.dataset.ticket));}
    function clear(){host.querySelectorAll('.graph2d-muted,.ticket-list-links .muted').forEach(node=>node.classList.remove('graph2d-muted','muted'));}
    host.addEventListener('pointerover',emphasize);host.addEventListener('pointerleave',clear);host.addEventListener('focusin',emphasize);host.addEventListener('focusout',clear);
    const observer=new ResizeObserver(connect);observer.observe(host);connect();
    if(focusId&&document.activeElement===document.body)host.querySelector(`[data-list-ticket="${CSS.escape(focusId)}"]`)?.focus({preventScroll:true});
    ticketListDispose=()=>{observer.disconnect();host.removeEventListener('pointerover',emphasize);host.removeEventListener('pointerleave',clear);host.removeEventListener('focusin',emphasize);host.removeEventListener('focusout',clear);};
  }catch(error){if(epoch===ticketListEpoch&&host.isConnected)byId('tickets-list-status').textContent=error.message||'Tickets unavailable';}
}
