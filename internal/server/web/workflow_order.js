"use strict";
let workflowLaneDrag=null;

function moveWorkflowLane(order,source,target,after=false){
  const next=[...order];if(source===target||!next.includes(source)||!next.includes(target))return next;
  next.splice(next.indexOf(source),1);next.splice(next.indexOf(target)+(after?1:0),0,source);return next;
}
function workflowLaneOrderControls(index,editable){
  const title=escapeHTML(flow.phases[index].title);
  return `<div class="lane-order-controls"><button type="button" class="secondary lane-grip" data-lane-grip="${index}" aria-label="Reorder ${title}" title="Drag to reorder · Alt+ArrowUp/Down" aria-keyshortcuts="Alt+ArrowUp Alt+ArrowDown" ${!editable?'disabled':''}>⠿</button><button type="button" class="secondary" data-lane-up="${index}" aria-label="Move ${title} up" ${!editable||index===0?'disabled':''}>↑</button><button type="button" class="secondary" data-lane-down="${index}" aria-label="Move ${title} down" ${!editable||index===flow.phases.length-1?'disabled':''}>↓</button></div>`;
}
function workflowLaneCanEdit(){
  return !!flow&&view==='Workflows'&&flowWritable(flow)&&!workflowMutation&&!workflowPublicationPending&&!workflowLoading&&!document.querySelector('tk-dialog[open],.choice-list');
}
function commitWorkflowLaneOrder(order,id){
  if(!workflowLaneCanEdit()||order.join('|')===flow.phases.map(phase=>phase.id).join('|'))return;
  const phases=new Map(flow.phases.map(phase=>[phase.id,phase]));
  if(order.length!==phases.size||new Set(order).size!==phases.size||order.some(key=>!phases.has(key)))return;
  flow.phases=order.map(key=>phases.get(key));flowDirty=true;drawWorkflow();
  flowMessage('Unsaved lane order');
  byId('workflow-editor')?.querySelector(`[data-lane-grip="${order.indexOf(id)}"]`)?.focus({preventScroll:true});
}
function moveWorkflowLaneBy(index,direction){
  if(!workflowLaneCanEdit()||workflowLaneDrag)return;
  const order=flow.phases.map(phase=>phase.id),id=order[index],target=order[index+direction];
  if(id&&target)commitWorkflowLaneOrder(moveWorkflowLane(order,id,target,direction>0),id);
}
function previewWorkflowLaneOrder(drag,order,animate=true){
  const before=new Map(drag.rows.map(row=>[row,row.getBoundingClientRect()]));
  for(const row of drag.rows)row.getAnimations?.().forEach(animation=>animation.cancel());
  for(const id of order)drag.list.append(drag.rows[drag.original.indexOf(id)]);
  if(animate&&!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches){
    for(const row of drag.rows){const previous=before.get(row),next=row.getBoundingClientRect();if(row.animate&&previous.y!==next.y)row.animate([{transform:`translateY(${previous.y-next.y}px)`},{transform:'none'}],{duration:170,easing:'cubic-bezier(.2,.8,.2,1)'});}
  }
}
function finishWorkflowLaneDrag(commit=false){
  const drag=workflowLaneDrag;if(!drag)return;
  workflowLaneDrag=null;if(flowDrag===drag)flowDrag=null;
  drag.ghost?.remove();drag.row.classList.remove('lane-drag-source');
  const current=flow===drag.source&&session?.account.id===drag.actor&&loadGeneration===drag.generation&&drag.list.isConnected&&view==='Workflows';
  if(current){previewWorkflowLaneOrder(drag,drag.original,false);if(commit&&drag.dragging)commitWorkflowLaneOrder(drag.order,drag.id);else drag.grip.focus({preventScroll:true});}
  if(document.body.hasPointerCapture?.(drag.pointerId))document.body.releasePointerCapture(drag.pointerId);
}
function cancelWorkflowLaneDrag(){finishWorkflowLaneDrag(false);}
function bindWorkflowLaneOrder(){
  const host=byId('workflow-editor');if(!host)return;
  host.querySelectorAll('[data-lane-up]').forEach(button=>button.onclick=()=>moveWorkflowLaneBy(Number(button.dataset.laneUp),-1));
  host.querySelectorAll('[data-lane-down]').forEach(button=>button.onclick=()=>moveWorkflowLaneBy(Number(button.dataset.laneDown),1));
  host.querySelectorAll('[data-lane-grip]').forEach(grip=>{
    grip.onkeydown=e=>{if(!e.altKey||e.ctrlKey||e.metaKey||e.shiftKey||!['ArrowUp','ArrowDown'].includes(e.key))return;e.preventDefault();e.stopPropagation();moveWorkflowLaneBy(Number(grip.dataset.laneGrip),e.key==='ArrowUp'?-1:1);};
    grip.onpointerdown=e=>{
      if(e.button!==0||e.isPrimary===false||flowDrag||!workflowLaneCanEdit())return;
      e.preventDefault();grip.focus({preventScroll:true});
      const original=flow.phases.map(phase=>phase.id),index=Number(grip.dataset.laneGrip),list=byId('phase-rows');
      workflowLaneDrag={kind:'lane',pointerId:e.pointerId,startX:e.clientX,startY:e.clientY,id:original[index],source:flow,actor:session.account.id,generation:loadGeneration,grip,row:grip.closest('[data-phase]'),list,rows:[...list.children],original,order:original,dragging:false};
      flowDrag=workflowLaneDrag;document.body.setPointerCapture(e.pointerId);
    };
  });
}
document.addEventListener('pointermove',e=>{
  const drag=workflowLaneDrag;if(!drag||drag.pointerId!==e.pointerId)return;
  if(flow!==drag.source||session?.account.id!==drag.actor||loadGeneration!==drag.generation||!drag.list.isConnected||!workflowLaneCanEdit()){cancelWorkflowLaneDrag();return;}
  if(!drag.dragging){
    if(Math.hypot(e.clientX-drag.startX,e.clientY-drag.startY)<6)return;
    drag.dragging=true;drag.row.classList.add('lane-drag-source');
    drag.ghost=document.createElement('div');drag.ghost.className='lane-drag-ghost';drag.ghost.textContent=flow.phases.find(phase=>phase.id===drag.id).title;drag.ghost.setAttribute('aria-hidden','true');document.body.append(drag.ghost);
  }
  e.preventDefault();drag.ghost.style.left=(e.clientX+12)+'px';drag.ghost.style.top=(e.clientY+10)+'px';
  const target=document.elementFromPoint(e.clientX,e.clientY)?.closest('#phase-rows [data-phase]');if(!target||target===drag.row)return;
  const rect=target.getBoundingClientRect(),targetID=drag.original[drag.rows.indexOf(target)];
  const next=moveWorkflowLane(drag.order,drag.id,targetID,e.clientY>rect.y+rect.height/2);
  if(next.join('|')!==drag.order.join('|')){drag.order=next;previewWorkflowLaneOrder(drag,next);}
},{passive:false});
document.addEventListener('pointerup',e=>{if(workflowLaneDrag?.pointerId===e.pointerId)finishWorkflowLaneDrag(true);});
document.addEventListener('pointercancel',e=>{if(workflowLaneDrag?.pointerId===e.pointerId)cancelWorkflowLaneDrag();});
document.addEventListener('lostpointercapture',e=>{if(workflowLaneDrag?.pointerId===e.pointerId)cancelWorkflowLaneDrag();});
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&workflowLaneDrag){e.preventDefault();e.stopImmediatePropagation();cancelWorkflowLaneDrag();}},true);
window.addEventListener('blur',cancelWorkflowLaneDrag);
window.addEventListener('resize',cancelWorkflowLaneDrag);
