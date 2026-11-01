"use strict";
const publicationRequests=new Map();
function resetWorkflowPublication(){publicationRequests.clear();}
function workflowMaintainer(w){return w.projectId===0?session.account.administrator:roleAllowsMaintain(w.projectId);}
const drawWorkflowBeforePublication=drawWorkflow;
drawWorkflow=function(){
  drawWorkflowBeforePublication();const toolbar=byId('workflow-editor')?.querySelector('.flow-toolbar');if(!toolbar)return;
  const editable=flowWritable(flow),publish=document.createElement('button');publish.type='button';publish.id='flow-publish';publish.textContent='Publish latest';publish.disabled=!editable;
  toolbar.insertBefore(publish,byId('flow-activate'));publish.onclick=publishFlow;
  byId('flow-save').textContent='Save latest';byId('flow-draft').disabled=!flow.published||!workflowMaintainer(flow);
  byId('flow-activate').disabled=!flow.published||!currentProject()||!roleAllowsMaintain(projectId);
  byId('flow-activate').textContent='Select for new work';
  const label=byId('workflow-editor').querySelector('.flow-heading .eyebrow');if(label)label.textContent=flow.published?'v-'+flow.version:'latest · revision '+flow.draftRevision;
  for(const option of byId('flow-select').options){const definition=flows.find(row=>flowKey(row)===option.value);if(definition)option.textContent=definition.title+' · '+(definition.published?'v-'+definition.version:'latest')+' · '+definition.family;}
};
const activatePublishedWorkflow=activateFlow;
activateFlow=function(){if(!flow.published){flowMessage('Save and publish latest before selecting a version.');return;}activatePublishedWorkflow();};

async function publishFlow(){
  if(flowDirty){flowMessage('Save latest before publication.');return;}
  const source=flow,dialog=byId('workflow-editor'),selector=byId('flow-select'),actor=session.account.id,generation=loadGeneration;
  if(!flowWritable(source))return;
  const identity=actor+':'+flowKey(source)+':'+source.draftRevision;
  if(!publicationRequests.has(identity))publicationRequests.set(identity,crypto.randomUUID());
  const controls=[...dialog.querySelectorAll('input,textarea,select,button')].filter(element=>element.id!=='flow-close'),states=controls.map(element=>element.disabled);
  controls.forEach(element=>element.disabled=true);
  const current=()=>session?.account.id===actor&&loadGeneration===generation&&dialog.open&&byId('flow-select')===selector&&flow===source;
  flowMessage('Publishing…');
  try{
    const result=await api('publishWorkflow',{workflowKey:flowKey(source),expectedRevision:source.draftRevision,requestId:publicationRequests.get(identity)});
    if(!current())return;
    const next=await pages('listWorkflows',projectId?{projectId}:{});
    if(!current())return;
    flow=result;flows=next;flowDirty=false;drawWorkflow();flowMessage('Published v-'+result.version+'. Select it separately for new work.');
  }catch(error){if(current())flowMessage(error.message+' Retry publication with the same request.');}
  finally{controls.forEach((element,index)=>{if(element.isConnected)element.disabled=states[index];});}
}
