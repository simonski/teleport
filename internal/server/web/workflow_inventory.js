"use strict";
let workflowInventoryDispose=null;
const workflowStepInventory=[
  {title:'Todo',role:'Todo',agentic:false},
  {title:'Refinement',role:'Refine',agentic:true},
  {title:'Agent work',role:'Doing',agentic:true},
  {title:'Human work',role:'Doing',agentic:false},
  {title:'Review',role:'Review',agentic:true},
  {title:'Ready',role:'Ready',agentic:false},
  {title:'Question',role:'Clarify',agentic:false},
  {title:'Approval',role:'Approval',agentic:false},
  {title:'Done',role:'Done',agentic:false},
  {title:'Decomposition',role:'Decompose',agentic:true},
  {title:'Rebase',role:'Rebase',agentic:true},
  {title:'Cancelled',role:'Cancelled',agentic:false},
];
function workflowInventoryHTML(){return `<aside class="workflow-inventory" aria-label="Step inventory"><h3>Steps</h3><p>Drag onto the map, or click to add beside the selected step.</p>${workflowStepInventory.map((step,index)=>({step,index})).filter(({step})=>flow.family!=='human'||['Todo','Doing','Done','Clarify'].includes(step.role)&&!step.agentic).map(({step,index})=>`<button type="button" class="secondary inventory-step" data-inventory-step="${index}" ${!flowWritable(flow)?'disabled':''}><span aria-hidden="true">${step.agentic?'↗':['Ready','Approval','Clarify'].includes(step.role)?'◇':'□'}</span><span>${step.title}<small>${step.agentic?'Agent':'Human'}</small></span><span aria-hidden="true">⠿</span></button>`).join('')}</aside>`;}
function workflowMapPoint(definition,id){const index=definition.phases.findIndex(phase=>phase.id===id);return definition.layout?.[id]||{x:35+(index%4)*320,y:40+Math.floor(index/4)*175};}
function workflowConnectionOutcome(definition,source,target){
  if(['Done','Cancelled'].includes(source.role)||target.role==='Todo')return '';
  if(definition.family==='agent'){
    if(target.role==='Done')return source.role==='Approval'?'approve':'';
    if(source.role==='Approval')return '';
    if(source.role==='Todo')return target.role==='Refine'?'requestRefinement':'';
    if(source.role==='Refine')return target.role==='Ready'?'acceptScope':target.role==='Decompose'?'acceptPlan':'';
    if(source.role==='Ready')return target.role==='Doing'?'start':'';
  }
  if(source.role==='Clarify')return 'answered';
  return source.agentic?'success':'complete';
}
function workflowNearbyConnection(definition,id){
  const phase=definition.phases.find(row=>row.id===id);if(!phase)return null;const point=workflowMapPoint(definition,id),candidates=[];
  for(const other of definition.phases){if(other.id===id)continue;const at=workflowMapPoint(definition,other.id),dx=Math.abs(point.x-at.x),dy=Math.abs(point.y-at.y),gap=dx-248,verticalGap=dy-72,horizontal=gap>=-35&&gap<=96&&dy<=70,vertical=verticalGap>=-20&&verticalGap<=80&&dx<=80;if(!horizontal&&!vertical)continue;
    const follows=horizontal?point.x>at.x:point.y>at.y,[source,target]=follows?[other,phase]:[phase,other],outcome=workflowConnectionOutcome(definition,source,target);
    if(!outcome||definition.rules.some(rule=>rule.source===source.id&&rule.outcome===outcome))continue;
    candidates.push({source:source.id,target:target.id,outcome,distance:horizontal?Math.hypot(Math.max(0,gap),dy):Math.hypot(dx,Math.max(0,verticalGap))});
  }
  candidates.sort((a,b)=>a.distance-b.distance);if(candidates.length>1&&candidates[1].distance-candidates[0].distance<8)return null;return candidates[0]||null;
}
function attachNearbyWorkflowStep(id){
  if(!workflowLaneCanEdit())return false;const candidate=workflowNearbyConnection(flow,id);if(!candidate)return false;
  flow.rules.push({id:'rule-'+crypto.randomUUID().slice(0,8),source:candidate.source,target:candidate.target,outcome:candidate.outcome,priority:0});flowDirty=true;return true;
}
function addInventoryStep(index,point){
  if(!workflowLaneCanEdit())return;const template=workflowStepInventory[index];if(!template||flow.family==='human'&&(template.agentic||!['Todo','Doing','Done','Clarify'].includes(template.role)))return;
  const selected=workflowMapRuntime?.scene.snapshot().selected,anchor=selected&&workflowMapPoint(flow,selected),position=point||{x:(anchor?.x??35)+320,y:anchor?.y??40},id='lane:phase-'+crypto.randomUUID().slice(0,8);
  const phase={...template,id,instructions:`Describe ${template.title.toLowerCase()} instructions.`,description:''};
  const after=flow.phases.findIndex(row=>row.id===selected);if(after>=0)flow.phases.splice(after+1,0,phase);else flow.phases.push(phase);
  flow.layout??={};flow.layout[id]={x:Math.max(-10000,Math.min(10000,position.x)),y:Math.max(-10000,Math.min(10000,position.y))};flowDirty=true;
  const attached=attachNearbyWorkflowStep(id);if(attached)workflowTrace=true;drawWorkflow();flowMessage(attached?'Step added with a nearby outcome route. Review and save latest.':'Step added. Move near another step to connect, then review and save latest.');
}
function bindWorkflowInventory(){
  const inventory=byId('workflow-editor')?.querySelector('.workflow-inventory'),host=byId('workflow-map-stage');if(!inventory||!host)return;
  const source=flow,actor=session?.account.id;let drag=null,suppress=false;
  function cancel(){if(!drag)return;drag.ghost?.remove();if(drag.button.hasPointerCapture(drag.id))drag.button.releasePointerCapture(drag.id);drag=null;if(flowDrag?.kind==='inventory')flowDrag=null;host.classList.remove('inventory-drop-active');}
  function down(event){const button=event.target.closest('[data-inventory-step]');if(!button||event.button!==0||!workflowLaneCanEdit())return;event.preventDefault();drag={button,id:event.pointerId,x:event.clientX,y:event.clientY,index:Number(button.dataset.inventoryStep)};button.setPointerCapture(event.pointerId);flowDrag={kind:'inventory'};}
  function move(event){if(!drag||event.pointerId!==drag.id)return;if(!drag.ghost&&Math.hypot(event.clientX-drag.x,event.clientY-drag.y)<5)return;if(!drag.ghost){drag.ghost=drag.button.cloneNode(true);drag.ghost.classList.add('inventory-ghost');drag.ghost.removeAttribute('data-inventory-step');drag.ghost.setAttribute('aria-hidden','true');document.body.append(drag.ghost);}drag.ghost.style.left=event.clientX+12+'px';drag.ghost.style.top=event.clientY-20+'px';host.classList.add('inventory-drop-active');}
  function up(event){if(!drag||event.pointerId!==drag.id)return;const previous=drag,rect=host.getBoundingClientRect(),inside=event.clientX>=rect.left&&event.clientX<=rect.right&&event.clientY>=rect.top&&event.clientY<=rect.bottom,world=workflowMapRuntime?.scene.clientToWorld(event);cancel();suppress=!!previous.ghost;
    if(flow!==source||session?.account.id!==actor||view!=='Workflows')return;
    if(previous.ghost&&inside&&world)addInventoryStep(previous.index,{x:world.x-115,y:-world.y-35});else if(!previous.ghost)addInventoryStep(previous.index);
  }
  function key(event){if(event.key==='Escape'&&drag){event.preventDefault();event.stopPropagation();cancel();}}
  function click(event){if(!event.target.closest('[data-inventory-step]'))return;if(suppress){suppress=false;return;}if(event.detail===0)addInventoryStep(Number(event.target.closest('[data-inventory-step]').dataset.inventoryStep));}
  inventory.addEventListener('pointerdown',down);inventory.addEventListener('pointermove',move);inventory.addEventListener('pointerup',up);inventory.addEventListener('pointercancel',cancel);inventory.addEventListener('click',click);document.addEventListener('keydown',key,true);window.addEventListener('blur',cancel);
  workflowInventoryDispose=()=>{cancel();inventory.removeEventListener('pointerdown',down);inventory.removeEventListener('pointermove',move);inventory.removeEventListener('pointerup',up);inventory.removeEventListener('pointercancel',cancel);inventory.removeEventListener('click',click);document.removeEventListener('keydown',key,true);window.removeEventListener('blur',cancel);};
}
