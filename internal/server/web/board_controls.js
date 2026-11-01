"use strict";
let boardObservationEpoch=0,boardObservationTimer=null,boardActiveTickets=new Set(),boardActiveGeneration=-1,boardObservedAt=0,boardBusy=false,workflowFocusPhase='';
function resetBoardObservation(){boardObservationEpoch++;clearTimeout(boardObservationTimer);boardActiveTickets.clear();boardBusy=false;workflowFocusPhase='';}
function boardCardState(t){
  if(t.status==='Done')return {key:'complete',label:'Done'};
  if(boardActiveGeneration===loadGeneration&&Date.now()-boardObservedAt<3500&&boardActiveTickets.has(t.id))return {key:'active',label:'Agent working'};
  if(t.attention||t.question&&!t.question.resolved&&!t.question.obsolete)return {key:'attention',label:'Needs attention'};
  const project=projects.find(p=>p.id===t.projectId),workflow=workflowMap[t.workflow];
  if(t.resumeHold||t.stopRequested||t.routingHold||t.agent&&(session.settings.handbrake||project?.handbrake||project?.laneLocks?.[workflow?.id+'#*']||project?.laneLocks?.[workflow?.id+'#'+t.phase])||(t.dependencies||[]).some(id=>tickets.find(ticket=>ticket.id===id)?.status!=='Done'))return {key:'blocked',label:'Blocked / paused'};
  if(t.active||t.claim||t.execution)return {key:'reserved',label:'Reserved'};
  if(t.status==='Ready')return {key:'ready',label:'Ready'};
  if(t.status==='Todo')return {key:'draft',label:'Draft'};
  return {key:'queued',label:t.agent?'Queued':'In progress'};
}
function card(t){
  let html=plainCard(t);if(view!=='Board')return html;const state=boardCardState(t);
  html=html.replace('class="ticket"',`class="ticket readiness-${state.key}${state.key==='active'?' is-working':''}" data-readiness="${state.key}"`);
  html=html.replace('</h3>',`</h3><p class="ticket-description" id="ticket-description-${t.id}">${escapeHTML(t.description||'No description yet.')}</p><span class="readiness-label">${state.label}</span>`);
  html=html.replace('class="ticket-open"',`class="ticket-open" aria-describedby="ticket-description-${t.id}"`);
  return html.replace('</article>','<svg class="agent-trace" aria-hidden="true"><rect x="2" y="2" rx="7" pathLength="100"/></svg></article>');
}
const boardEditIcon='<svg viewBox="0 0 20 20" aria-hidden="true"><path d="m4 13 8-8 3 3-8 8-4 1zM11 6l3 3"/></svg>';
const boardBrakeIcon='<svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="5"/><path d="M3 4a9 9 0 0 0 0 12M17 4a9 9 0 0 1 0 12M8 7v6m4-6v6"/></svg>';
function boardLaneControls(p,key,phase){
  const w=workflowMap[key],locked=!!p.laneLocks?.[w.id+'#'+phase.id],maintain=roleAllowsMaintain(p.id);
  return `<div class="board-lane-controls"><button type="button" class="secondary board-icon" data-board-action="edit" data-board-phase="${escapeHTML(phase.id)}" aria-label="Edit ${escapeHTML(phase.title)} workflow" title="Edit workflow">${boardEditIcon}</button>${phase.agentic?`<span class="board-workers" data-phase-workers="${escapeHTML(phase.id)}" title="Observed active / effective configured agents">…/…</span><button type="button" class="secondary board-icon ${locked?'engaged':''}" data-board-action="lane-brake" data-board-phase="${escapeHTML(phase.id)}" aria-label="${locked?'Release':'Engage'} ${escapeHTML(phase.title)} handbrake" aria-pressed="${locked}" ${maintain?'':'disabled'}>${boardBrakeIcon}</button>`:''}${phase.role==='Ready'?`<button type="button" class="secondary ready-mode" data-board-action="edit" data-board-phase="${escapeHTML(phase.id)}">${phase.requireReadyReview?'Human review required':'Automatic implementation'}</button>`:''}</div>`;
}
function agentBoardView(leaf){
  const groups=new Map();for(const t of leaf){const id=t.projectId+':'+t.workflow;if(!groups.has(id))groups.set(id,{project:projects.find(p=>p.id===t.projectId),key:t.workflow,items:[]});groups.get(id).items.push(t);}
  const p=currentProject();if(p)for(const key of lens==='both'?[p.agentWorkflow,p.humanWorkflow]:[lens==='human'?p.humanWorkflow:p.agentWorkflow]){const id=p.id+':'+key;if(!groups.has(id))groups.set(id,{project:p,key,items:[]});}
  return [...groups.values()].map(({project:p,key,items})=>{const w=workflowMap[key];if(!p||!w)return '';const locked=!!p.laneLocks?.[w.id+'#*'];return `<section class="board-group" data-board-project="${p.id}" data-board-workflow="${escapeHTML(key)}"><header class="board-group-heading"><span>${escapeHTML(p.name)}${p.simulation?' · Simulation':''} / ${escapeHTML(w.title)} · v${w.version}</span><span class="header-space"></span><span class="board-scope-status" role="status"></span><button type="button" class="secondary board-brake ${locked?'engaged':''}" data-board-action="board-brake" aria-pressed="${locked}" ${roleAllowsMaintain(p.id)?'':'disabled'}>${boardBrakeIcon}${locked?'Resume board':'Pause board'}</button></header><div class="board">${w.phases.filter(phase=>phase.role!=='Cancelled').map(phase=>`<section class="lane" data-phase-id="${escapeHTML(phase.id)}"><header><h2>${escapeHTML(phase.title)}</h2><span class="lane-count">${items.filter(t=>t.phase===phase.id).length}</span><span class="lane-line"></span></header>${boardLaneControls(p,key,phase)}<div class="lane-body">${items.filter(t=>t.phase===phase.id).sort((a,b)=>Number(!!b.attention)-Number(!!a.attention)||a.id-b.id).map(card).join('')}</div></section>`).join('')}</div></section>`;}).join('')||'<div class="quiet-state">No board in this scope.</div>';
}
function mountBoardControls(){
  clearTimeout(boardObservationTimer);const epoch=++boardObservationEpoch;if(view!=='Board')return;
  const actor=session.account.id,generation=loadGeneration;const current=()=>epoch===boardObservationEpoch&&actor===session?.account.id&&generation===loadGeneration&&view==='Board';
  for(const group of root.querySelectorAll('.board-group'))group.onclick=async e=>{
    const button=e.target.closest('[data-board-action]');if(!button||boardBusy)return;const p=projects.find(row=>row.id===Number(group.dataset.boardProject)),key=group.dataset.boardWorkflow,w=workflowMap[key];
    if(button.dataset.boardAction==='edit'){workflowPreferred=key;workflowFocusPhase=button.dataset.boardPhase;projectId=p.id;view='Workflows';detailEpoch++;workspace().catch(showError);return;}
    const phase=button.dataset.boardAction==='lane-brake'?button.dataset.boardPhase:'*',locked=!!p.laneLocks?.[w.id+'#'+phase];boardBusy=true;button.disabled=true;
    try{await api('setHandbrake',{projectId:p.id,expectedVersion:p.version,workflowKey:key,...(phase==='*'?{}:{phaseId:phase}),engaged:!locked});if(current())await workspace();}catch(error){if(current())showError(error);}finally{boardBusy=false;if(button.isConnected)button.disabled=!roleAllowsMaintain(p.id);}
  };
  async function poll(){if(!current())return;const active=new Set();try{
    for(const group of root.querySelectorAll('.board-group')){
      if(!current())return;const data=await api('getBoardExecution',{projectId:Number(group.dataset.boardProject),workflowKey:group.dataset.boardWorkflow});if(!current())return;
      group.querySelector('.board-scope-status').textContent=data.systemBraked?'System paused':data.projectBraked?'Project paused':'';
      for(const phase of data.phases){for(const id of phase.activeTicketIds)active.add(id);const label=group.querySelector(`[data-phase-workers="${CSS.escape(phase.phaseId)}"]`);if(label){label.textContent=phase.active+'/'+phase.capacity;label.title=`${phase.active} active / ${phase.capacity} configured · ${phase.reserved} reserved · ${phase.unresolved} unresolved`;}}
    }
    boardActiveTickets=active;boardActiveGeneration=generation;boardObservedAt=Date.now();for(const node of root.querySelectorAll('[data-ticket-card]')){const t=tickets.find(t=>t.id===Number(node.dataset.ticketCard));if(!t)continue;const state=boardCardState(t);for(const name of [...node.classList])if(name.startsWith('readiness-'))node.classList.remove(name);node.classList.add('readiness-'+state.key);node.classList.toggle('is-working',state.key==='active');node.dataset.readiness=state.key;const label=node.querySelector('.readiness-label');if(label)label.textContent=state.label;}
  }catch(error){if(current()&&!error.staleInteraction){boardActiveTickets.clear();root.querySelectorAll('.is-working').forEach(node=>{node.classList.remove('is-working','readiness-active');node.querySelector('.readiness-label').textContent='Observation unavailable';});root.querySelectorAll('[data-phase-workers]').forEach(node=>{node.textContent='?/…';node.title='Worker observation unavailable';});}}
  finally{if(current())boardObservationTimer=setTimeout(poll,1200);}}
  poll();
}
function readyReviewHTML(t){if(!workflowMap[t.workflow]?.phases.find(p=>p.id===t.phase)?.requireReadyReview||t.status!=='Ready')return '';return `<section class="ready-review-panel"><h3>Final human review</h3>${t.agreementBasis?.readyReviewed?'<p>Reviewed for the current readiness basis.</p>':`<p class="attention-reason">Review required before implementation.</p>${roleAllowsWrite(t.projectId)?'<button type="button" data-delivery="ready">Review readiness</button>':''}`}</section>`;}
function enhanceReadyReviewFields(host){
  for(const row of host.querySelectorAll('[data-phase]')){const phase=flow.phases[Number(row.dataset.phase)],label=document.createElement('label');label.textContent='Ready admission';label.dataset.readyAdmission='true';const select=document.createElement('select');select.innerHTML='<option value="auto">Automatically send to implementation</option><option value="human">Final review by human required</option>';select.value=phase.requireReadyReview?'human':'auto';select.disabled=!flowWritable(flow);label.append(select);row.append(label);
    const sync=()=>{label.hidden=phase.role!=='Ready'||flow.family!=='agent';if(label.hidden)delete phase.requireReadyReview;};sync();row.addEventListener('input',sync);select.onchange=()=>{if(!flowWritable(flow))return;phase.requireReadyReview=select.value==='human';flowDirty=true;flowMessage('Unsaved latest changes');};
  }
}
