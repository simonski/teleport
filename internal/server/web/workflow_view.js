"use strict";
let workflowViewEpoch=0,workflowLoading=false,workflowMutation=null,workflowPreferred='',workflowFeedback=null;
function resetWorkflowView(){if(typeof cancelWorkflowLaneDrag==='function')cancelWorkflowLaneDrag();workflowViewEpoch++;workflowLoading=false;workflowMutation=null;workflowPreferred='';workflowFeedback=null;}
function workflowNavigationBlocked(){return view==='Workflows'&&(flowDirty||!!flowDrag||!!workflowMutation||!!workflowPublicationPending);}
function workflowViewBusy(){return view==='Workflows'&&(workflowNavigationBlocked()||workflowLoading||!!flowDrag||!!workflowPublicationRecovery);}
function workflowsView(){return '<section id="workflow-editor" class="workflow-view"><p id="workflow-load" role="status">Loading workflows…</p></section>';}
workflowEditor=async function(){navigateView('Workflows');};
flowMessage=function(text){const status=byId('flow-message')||byId('workflow-load');if(status)status.textContent=text;if(flow&&view==='Workflows')workflowFeedback={key:flowKey(flow),revision:flow.draftRevision,text};};

async function mountWorkflows(preferred=workflowPreferred){
  const actor=session?.account.id,generation=loadGeneration,epoch=++workflowViewEpoch,source=flow;workflowLoading=true;
  try{
    const next=await pages('listWorkflows',projectId?{projectId}:{});
    if(session?.account.id!==actor||generation!==loadGeneration||epoch!==workflowViewEpoch||view!=='Workflows'||flow!==source||flowDirty||workflowMutation)return;
    flows=next;flow=next.find(row=>flowKey(row)===preferred)||next.find(row=>flowKey(row)===currentProject()?.agentWorkflow)||next[0];flowDirty=false;
    if(!flow){byId('workflow-editor').textContent='No accessible workflows.';return;}
    workflowPreferred=flowKey(flow);drawWorkflow();
  }catch(error){if(session?.account.id===actor&&generation===loadGeneration&&epoch===workflowViewEpoch)flowMessage(error.message);}
  finally{if(epoch===workflowViewEpoch)workflowLoading=false;}
}

const drawWorkflowBeforeView=drawWorkflow;
drawWorkflow=function(){
  const host=byId('workflow-editor');if(!host||!flow)return;
  drawWorkflowBeforeView();if(view!=='Workflows')return;
  enhanceReadyReviewFields(host);
  if(workflowFocusPhase){const index=flow.phases.findIndex(phase=>phase.id===workflowFocusPhase),row=host.querySelector(`[data-phase="${index}"]`);row?.scrollIntoView({block:'center',behavior:'instant'});(row?.querySelector('input:not(:disabled)')||byId('flow-draft'))?.focus({preventScroll:true});workflowFocusPhase='';}
  if(typeof bindWorkflowLaneOrder==='function')bindWorkflowLaneOrder();
  workflowPreferred=flowKey(flow);
  byId('flow-close').hidden=true;
  if(workflowFeedback?.key===flowKey(flow)&&workflowFeedback.revision===flow.draftRevision)byId('flow-message').textContent=workflowFeedback.text;
  const toolbar=host.querySelector('.flow-toolbar'),discard=document.createElement('button'),remove=document.createElement('button');
  discard.type=remove.type='button';discard.id='flow-discard';remove.id='flow-delete';discard.className=remove.className='secondary';discard.textContent='Discard changes';remove.textContent='Delete latest';discard.disabled=remove.disabled=!flowWritable(flow);toolbar.append(discard,remove);
  discard.onclick=()=>{flowDirty=false;workflowFeedback=null;workflowPublicationRecovery=null;mountWorkflows(flowKey(flow));};remove.onclick=deleteFlowForm;
  byId('flow-select').onchange=e=>{if(flowDirty){e.target.value=flowKey(flow);flowMessage('Save latest or discard changes before switching.');return;}changeWorkflow('getWorkflow',{workflowKey:e.target.value},'');};
  byId('flow-draft').onclick=()=>changeWorkflow('draftWorkflow',{workflowKey:flowKey(flow)},'Latest draft opened.');
  byId('flow-copy').disabled=!projects.some(project=>roleAllowsMaintain(project.id));
  host.oninput=()=>{if(flowWritable(flow)&&flowDirty)flowMessage('Unsaved latest changes');};
};

async function changeWorkflow(operation,input,message){
  if(workflowMutation||workflowPublicationPending)return;
  const source=flow,actor=session?.account.id,generation=loadGeneration,host=byId('workflow-editor'),selector=byId('flow-select'),token={};workflowMutation=token;
  const controls=[...host.querySelectorAll('input,textarea,select,button')],disabled=controls.map(control=>control.disabled);controls.forEach(control=>control.disabled=true);
  const current=()=>workflowMutation===token&&session?.account.id===actor&&loadGeneration===generation&&view==='Workflows'&&host.isConnected&&byId('flow-select')===selector&&flow===source;
  try{
    const result=await api(operation,input);if(!current())return;
    const next=await pages('listWorkflows',projectId?{projectId}:{});if(!current())return;
    workflowPublicationRecovery=null;flow=result;flows=next;flowDirty=false;workflowPreferred=flowKey(result);drawWorkflow();flowMessage(message);
    const focus=operation==='getWorkflow'?byId('flow-select'):operation==='draftWorkflow'?host.querySelector('[data-phase] [data-field=title]'):byId('flow-save');
    queueMicrotask(()=>{if(session?.account.id===actor&&loadGeneration===generation&&flow===result&&document.activeElement===document.body)focus?.focus({preventScroll:true});});
  }catch(error){if(current()){selector.value=flowKey(source);flowMessage(error.message);}}
  finally{if(workflowMutation===token)workflowMutation=null;controls.forEach((control,index)=>{if(control.isConnected)control.disabled=disabled[index];});}
}
saveFlow=async function(){
  if(!flowWritable(flow))return;
  const invalid=byId('workflow-editor')?.querySelector('input:invalid,textarea:invalid');
  if(invalid){flowMessage('Workers must be a whole number from 0 to 20.');invalid.focus();return;}
  await changeWorkflow('saveWorkflow',{definition:structuredClone(flow),expectedRevision:flow.draftRevision},'Draft saved. Existing work remains pinned.');
};

copyFlowForm=function(){
  if(flowDirty){flowMessage('Save latest or discard changes before copying.');return;}
  const sourceKey=flowKey(flow),requestId=crypto.randomUUID(),dialog=openDialog('New workflow',`<label>Title<input name="title" value="${escapeHTML(flow.title)} copy" required maxlength="200"></label><label>Project<select name="projectId">${projects.filter(project=>roleAllowsMaintain(project.id)).map(project=>`<option value="${project.id}" ${project.id===projectId?'selected':''}>${escapeHTML(project.name)}</option>`).join('')}</select></label>`,'Create draft'),guard=captureDialogCompletion(dialog),form=byId('editor-form');let busy=false;
  form.onsubmit=async e=>{
    e.preventDefault();if(busy)return;busy=true;const button=form.querySelector('[type=submit]');button.disabled=true;
    try{const values=Object.fromEntries(new FormData(form)),created=await api('copyWorkflow',{sourceKey,title:values.title,projectId:Number(values.projectId),requestId});if(!guard.current())return;workflowPreferred=flowKey(created);workflowFeedback=null;workflowPublicationRecovery=null;projectId=created.projectId;dialog.close();await workspace();}
    catch(error){if(guard.current())showError(error);}
    finally{busy=false;if(button.isConnected)button.disabled=false;}
  };
};
activateFlow=function(){
  if(flowDirty||!flow.published){flowMessage('Save and publish latest before selecting a version.');return;}
  const chosen=flow,project=currentProject();if(!project||!roleAllowsMaintain(project.id))return;
  const dialog=openDialog('Activate workflow',`<p><strong>${escapeHTML(chosen.title)} · v-${chosen.version}</strong> will be used for new ${escapeHTML(chosen.family)} tickets in ${escapeHTML(project.name)}.</p><p>Existing tickets retain their exact pin.</p>`,'Activate for new work'),guard=captureDialogCompletion(dialog),form=byId('editor-form');let busy=false;
  form.onsubmit=async e=>{
    e.preventDefault();if(busy)return;busy=true;const button=form.querySelector('[type=submit]');button.disabled=true;
    try{const result=await api('activateWorkflowVersion',{workflowKey:flowKey(chosen),expectedRevision:chosen.draftRevision,projectId:project.id,expectedProjectVersion:project.version});if(!guard.current())return;projects=projects.map(row=>row.id===result.id?result:row);workflowPreferred=flowKey(chosen);workflowFeedback=null;workflowPublicationRecovery=null;dialog.close();await workspace();notice('Workflow selected for new work');}
    catch(error){if(guard.current())showError(error);}
    finally{busy=false;if(button.isConnected)button.disabled=false;}
  };
};
function deleteFlowForm(){
  const chosen=flow;if(!flowWritable(chosen))return;
  const dialog=openDialog('Delete latest draft',`<p>${escapeHTML(chosen.title)} · latest revision ${chosen.draftRevision}</p><p>Delete this saved draft and discard its unsaved edits? Published versions and ticket pins remain retained.</p>`,'Delete latest'),guard=captureDialogCompletion(dialog),form=byId('editor-form');let busy=false;
  form.onsubmit=async e=>{
    e.preventDefault();if(busy)return;busy=true;const button=form.querySelector('[type=submit]');button.disabled=true;
    try{await api('deleteWorkflow',{workflowKey:flowKey(chosen),expectedRevision:chosen.draftRevision});if(!guard.current())return;flowDirty=false;workflowPreferred='';workflowFeedback=null;workflowPublicationRecovery=null;dialog.close();await workspace();}
    catch(error){if(guard.current())showError(error);}
    finally{busy=false;if(button.isConnected)button.disabled=false;}
  };
}
