"use strict";
let workflowPresentation='editor',workflowMapRuntime=null,workflowMapEpoch=0,workflowTrace=false;
const workflowMapCameras=new Map();
function leaveWorkflowMap(){workflowMapEpoch++;if(typeof workflowInventoryDispose==='function'){workflowInventoryDispose();workflowInventoryDispose=null;}if(workflowMapRuntime){workflowMapCameras.set(workflowMapRuntime.scope,workflowMapRuntime.scene.snapshot());workflowMapRuntime.scene.dispose();workflowMapRuntime=null;}}
const resetWorkflowViewBeforeMap=resetWorkflowView;
resetWorkflowView=function(){leaveWorkflowMap();workflowMapCameras.clear();workflowPresentation='editor';workflowTrace=false;resetWorkflowViewBeforeMap();};

function workflowMapScope(){return `${session?.account.id}:${projectId}:${flowKey(flow)}`;}
function selectWorkflowMapPhase(id){
  const panel=byId('workflow-map-inspector'),phase=flow?.phases.find(row=>row.id===id);if(!panel)return;
  if(!phase){panel.innerHTML='<p>Select a phase to inspect its outcome routes.</p>';return;}
  const rules=flow.rules.filter(rule=>rule.source===id||rule.target===id),title=key=>flow.phases.find(row=>row.id===key)?.title||key;
  panel.innerHTML=`<p class="eyebrow">${escapeHTML(phase.agentic?'Agent phase':'Human phase')}</p><h3>${escapeHTML(phase.title)}</h3><p>${escapeHTML(phase.description||phase.instructions||phase.role)}</p><button type="button" class="secondary" id="workflow-map-edit">${flowWritable(flow)?'Edit phase':'Inspect phase'}</button><h3>Outcome routes</h3><div class="workflow-routes-scroll"><table><thead><tr><th>From</th><th>Outcome</th><th>To</th></tr></thead><tbody>${rules.map(rule=>`<tr><td>${escapeHTML(title(rule.source))}</td><td>${escapeHTML(rule.outcome)}</td><td>${escapeHTML(rule.target?title(rule.target):rule.action||'Wait')}</td></tr>`).join('')}</tbody></table></div>`;
  byId('workflow-map-edit').onclick=()=>openWorkflowMapPhase(id);
}
function openWorkflowMapPhase(id){workflowPresentation='editor';workflowFocusPhase=id;drawWorkflow();}
async function mountWorkflowMap(){
  leaveWorkflowMap();const epoch=workflowMapEpoch,source=flow,actor=session?.account.id,scope=workflowMapScope(),host=byId('workflow-map-stage');
  if(!host)return;
  try{
    const module=await import('/graph_2d_scene.js'),model=await import('/graph_2d_model.js');
    if(epoch!==workflowMapEpoch||source!==flow||session?.account.id!==actor||view!=='Workflows'||workflowPresentation!=='map'||!host.isConnected)return;
    const graph=model.workflowGraph2D(flow,flowWritable(flow),workflowTrace),snapshot=workflowMapCameras.get(scope)||{};
    const scene=module.createGraphScene(host,graph,{
      select:selectWorkflowMapPhase,open:openWorkflowMapPhase,
      constrain:point=>({x:Math.max(-9885,Math.min(10115,point.x)),y:Math.max(-10035,Math.min(9965,point.y)),z:0}),
      move:(id,point)=>{if(!flowWritable(source)||flow!==source)return;flow.layout??={};flow.layout[id]={x:point.x-115,y:-point.y-35};flowDirty=true;const candidate=workflowNearbyConnection(flow,id);host.querySelectorAll('.workflow-nearby').forEach(node=>node.classList.remove('workflow-nearby'));if(candidate)for(const key of [candidate.source,candidate.target])host.querySelector(`[data-graph-node="${CSS.escape(key)}"]`)?.classList.add('workflow-nearby');flowMessage(candidate?`Release to connect ${candidate.outcome} to ${flow.phases.find(phase=>phase.id===candidate.target).title}`:'Unsaved phase arrangement');},
      drop:id=>{if(!flowWritable(source)||flow!==source)return;if(attachNearbyWorkflowStep(id)){workflowTrace=true;mountWorkflowMap();flowMessage('Nearby outcome route added to the draft. Review and save latest.');}},
      interacting:active=>{if(active)flowDrag={kind:'map'};else if(flowDrag?.kind==='map')flowDrag=null;},
    },snapshot);
    workflowMapRuntime={scope,scene};selectWorkflowMapPhase(snapshot.selected||flow.phases[0]?.id);bindWorkflowInventory();
  }catch(error){if(epoch===workflowMapEpoch&&host.isConnected)host.innerHTML='<div class="graph-fallback"><h3>Map unavailable</h3><p>The editor remains available for all phases and outcome rules.</p></div>';}
}
const drawWorkflowBeforeMap=drawWorkflow;
drawWorkflow=function(){
  leaveWorkflowMap();drawWorkflowBeforeMap();const host=byId('workflow-editor');if(!host||view!=='Workflows')return;
  const switcher=document.createElement('div');switcher.className='graph-projection workflow-presentation';switcher.setAttribute('role','group');switcher.setAttribute('aria-label','Workflow view');
  switcher.innerHTML=`<button type="button" class="secondary" id="workflow-view-editor" aria-pressed="${workflowPresentation==='editor'}">Editor</button><button type="button" class="secondary" id="workflow-view-map" aria-pressed="${workflowPresentation==='map'}">Map</button>`;
  host.querySelector('.flow-heading').append(switcher);
  for(const mode of ['editor','map'])byId('workflow-view-'+mode).onclick=()=>{if(workflowMutation||workflowPublicationPending||flowDrag)return;workflowPresentation=mode;drawWorkflow();byId('workflow-view-'+mode)?.focus({preventScroll:true});};
  const canvas=host.querySelector('.flow-canvas-wrap'),lists=host.querySelector('.flow-lists');canvas.hidden=lists.hidden=workflowPresentation==='map';
  if(workflowPresentation!=='map')return;
  const map=document.createElement('section');map.className='workflow-map';
  map.innerHTML=`<div class="workflow-map-toolbar"><button type="button" class="secondary" id="workflow-map-fit">Fit</button><button type="button" class="secondary" id="workflow-map-zoom-in" aria-label="Zoom workflow in">＋</button><button type="button" class="secondary" id="workflow-map-zoom-out" aria-label="Zoom workflow out">−</button><button type="button" class="secondary" id="workflow-map-trace" aria-pressed="${workflowTrace}">Trace routes</button><span>Definition preview · ${flow.published?'published':'save latest to retain layout'}</span></div><div class="workflow-map-workspace">${workflowInventoryHTML()}<div id="workflow-map-stage" aria-label="Workflow phase map"></div><aside id="workflow-map-inspector" class="graph-inspector"></aside></div>`;
  canvas.before(map);
  byId('workflow-map-fit').onclick=()=>workflowMapRuntime?.scene.fit();byId('workflow-map-zoom-in').onclick=()=>workflowMapRuntime?.scene.zoom(.8);byId('workflow-map-zoom-out').onclick=()=>workflowMapRuntime?.scene.zoom(1.25);
  byId('workflow-map-trace').onclick=()=>{workflowTrace=!workflowTrace;byId('workflow-map-trace').setAttribute('aria-pressed',String(workflowTrace));mountWorkflowMap();};
  mountWorkflowMap();
};
