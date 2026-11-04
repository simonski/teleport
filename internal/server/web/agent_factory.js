"use strict";
let agentFactoryEpoch=0,agentLaunch=null;
function agentFactoryHTML(){return '<div class="agent-factory"><header><div><p class="eyebrow">SYSTEM / AGENTS</p><h1>Agent factory</h1><p>Define reusable personas for project conversations and ticket assistance.</p></div><button type="button" id="new-agent">New agent</button></header><p id="agent-factory-status" role="status">Loading agents…</p><div id="agent-factory-list" class="agent-factory-list"></div></div>';}
async function mountAgentFactory(){
  const epoch=++agentFactoryEpoch,actor=session.account.id;
  on('new-agent','click',()=>agentDefinitionForm());
  try{
    const items=await pages('listAgentDefinitions',{includeHidden:true});if(epoch!==agentFactoryEpoch||view!=='Agents'||session?.account.id!==actor||!session.account.administrator)return;
    byId('agent-factory-status').textContent=items.length?'':'No agents yet. Create one to define its purpose and harness.';
    byId('agent-factory-list').innerHTML=items.map(agent=>`<article class="agent-factory-card"><div><p class="eyebrow">${escapeHTML(agent.harness)} · v${agent.version}${agent.archived?' · Archived':''}</p><h2>${escapeHTML(agent.name)}</h2><p>${escapeHTML(agent.goal)}</p><small>${escapeHTML(agent.model||'Default model')} · ${escapeHTML(agent.thinking||'Default thinking')} · ${agent.skills.length} skills</small></div><div class="agent-factory-actions"><button type="button" class="secondary" data-agent-edit="${escapeHTML(agent.id)}">Edit</button><button type="button" data-agent-start="${escapeHTML(agent.id)}" ${agent.archived?'disabled':''}>Start conversation</button></div></article>`).join('');
    byId('agent-factory-list').querySelectorAll('[data-agent-edit]').forEach(button=>button.onclick=()=>agentDefinitionForm(items.find(item=>item.id===button.dataset.agentEdit)));
    byId('agent-factory-list').querySelectorAll('[data-agent-start]').forEach(button=>button.onclick=()=>startAgentConversation(items.find(item=>item.id===button.dataset.agentStart),projectId).catch(showError));
  }catch(error){if(epoch===agentFactoryEpoch&&view==='Agents'&&byId('agent-factory-status'))byId('agent-factory-status').textContent=error.message;}
}
function agentDefinitionForm(agent=null){
  const requestId=crypto.randomUUID(),skills=agent?.skills||[];
  const dialog=openDialog(agent?'Edit agent':'New agent',`<label>Name<input name="name" required maxlength="120" value="${escapeHTML(agent?.name||'')}"></label><label>Goal<textarea name="goal" rows="2" required maxlength="2000">${escapeHTML(agent?.goal||'')}</textarea></label><label>Persona instructions<textarea name="prompt" rows="5" maxlength="12000">${escapeHTML(agent?.prompt||'')}</textarea></label><div class="form-grid"><label>Harness<select name="harness"><option value="codex" ${agent?.harness==='codex'?'selected':''}>Codex</option><option value="claude" ${agent?.harness==='claude'?'selected':''}>Claude</option><option value="copilot" disabled>Copilot · persona support unavailable</option></select></label><label>Model<input name="model" maxlength="100" value="${escapeHTML(agent?.model||'')}" placeholder="Harness default"></label></div><label>Thinking<select name="thinking">${[['','Harness default'],['low','Low'],['medium','Medium'],['high','High']].map(([value,label])=>`<option value="${value}" ${(agent?.thinking||'')===value?'selected':''}>${label}</option>`).join('')}</select></label><fieldset><legend>Installed skills</legend><div id="agent-skills">Loading skills…</div></fieldset>${agent?`<label class="checkbox-label"><input name="archived" type="checkbox" ${agent.archived?'checked':''}>Archive agent</label>`:''}<p id="agent-definition-status" role="status" class="save-status"></p>`,'Save agent');
  const guard=captureDialogCompletion(dialog),form=byId('editor-form'),status=byId('agent-definition-status');let loaded=false,busy=false;
  api('listInstalledAgentSkills').then(result=>{if(!guard.current())return;const names=[...new Set([...result.items,...skills])];byId('agent-skills').innerHTML=names.map(name=>`<label class="checkbox-label"><input type="checkbox" name="skill" value="${escapeHTML(name)}" ${skills.includes(name)?'checked':''}>${escapeHTML(name)}${result.items.includes(name)?'':' · no longer installed'}</label>`).join('')||'<p>No installed skills found on this server.</p>';loaded=true;}).catch(error=>{if(guard.current())status.textContent=error.message;});
  form.onsubmit=async event=>{event.preventDefault();if(busy||!loaded)return;busy=true;const data=new FormData(form),button=form.querySelector('button[type=submit]');button.disabled=true;status.textContent='Saving…';
    try{await api('saveAgentDefinition',{...(agent?{id:agent.id,expectedVersion:agent.version}:{}),requestId,name:data.get('name'),goal:data.get('goal'),prompt:data.get('prompt'),harness:data.get('harness'),model:data.get('model'),thinking:data.get('thinking'),skills:data.getAll('skill'),archived:data.has('archived')});if(!guard.current())return;dialog.close();if(view==='Agents')mountAgentFactory();}
    catch(error){if(guard.current())status.textContent=error.message;}finally{busy=false;if(guard.current())button.disabled=false;}
  };
}
async function startAgentConversation(agent,project,ticket=0){
  if(!agent||!project)throw new Error('Select a project before starting an agent conversation.');
  const actor=session?.account.id,key=[actor,agent.id,agent.version,project,ticket].join(':'),epoch=detailEpoch;
  if(agentLaunch?.pending)return;
  if(!agentLaunch||agentLaunch.key!==key)agentLaunch={key,requestId:crypto.randomUUID(),pending:false};
  const launch=agentLaunch;launch.pending=true;
  try{const row=await api('openAgentSession',{projectId:project,agentId:agent.id,expectedVersion:agent.version,...(ticket?{ticketId:ticket}:{}),requestId:launch.requestId});if(agentLaunch===launch)agentLaunch=null;await refreshSessionRoster();if(row.state==='failed')throw new Error('Agent session could not start: '+(row.errorCode||'launch failed'));if(session?.account.id===actor&&detailEpoch===epoch)await openTerminal(row.id);}
  catch(error){if(error.status&&error.status<500&&agentLaunch===launch)agentLaunch=null;throw error;}
  finally{launch.pending=false;}
}

async function agentUsageSettings(project){
  const actor=session?.account.id,epoch=++detailEpoch;
  const [usages,agents]=await Promise.all([api('listAgentUsages',{projectId:project}),api('listProjectAgents',{projectId:project})]);
  if(session?.account.id!==actor||detailEpoch!==epoch||projectId!==project||foregroundOpen())return;
  const dialog=closeOnlyDialog('Agent usage',`<p>Project actions bind a reusable agent to a specific interaction.</p><p class="form-help">Spellcheck execution is not available yet. Saved actions will not run until a scoped execution adapter is available.</p><div id="project-usages">${usages.items.map(item=>`<div class="session-row"><strong>${escapeHTML(item.name)}</strong><span>${escapeHTML(agents.items.find(agent=>agent.id===item.agentId)?.name||'Agent unavailable')} · Todos · ${item.enabled?'Enabled':'Disabled'}</span><button type="button" class="secondary" data-edit-usage="${escapeHTML(item.id)}">Edit</button></div>`).join('')||'<p>No actions configured.</p>'}</div><button type="button" class="secondary" id="new-agent-usage" ${agents.items.length?'':'disabled'}>Add spellcheck action</button>${agents.items.length?'':'<p class="form-help">An administrator must first create an agent.</p>'}`);
  dialog.querySelectorAll('[data-edit-usage]').forEach(button=>button.onclick=()=>{dialog.close();agentUsageForm(project,agents.items,usages.items.find(item=>item.id===button.dataset.editUsage));});
  on('new-agent-usage','click',()=>{dialog.close();agentUsageForm(project,agents.items);});
}
function agentUsageForm(project,agents,usage=null){
  const available=[...agents];if(usage&&!available.some(a=>a.id===usage.agentId))available.push({id:usage.agentId,name:'Agent unavailable'});
  const dialog=openDialog(usage?'Edit agent usage':'Add agent usage',`<label>Action name<input name="name" required maxlength="120" value="${escapeHTML(usage?.name||'Spellcheck')}"></label><label>Agent<select name="agentId">${available.map(agent=>`<option value="${escapeHTML(agent.id)}" ${agent.id===usage?.agentId?'selected':''}>${escapeHTML(agent.name)}</option>`).join('')}</select></label><p>Human action · Todos → ticket context menu → Agents</p><p>Applies to editable Todo-stage human tickets. Successful corrections will retain the original text.</p><label class="checkbox-label"><input name="enabled" type="checkbox" ${!usage||usage.enabled?'checked':''}>Enable action when execution becomes available</label><p class="form-help">Execution is not available yet.</p><p id="usage-status" class="save-status" role="status"></p>`,'Save usage');
  const guard=captureDialogCompletion(dialog),form=byId('editor-form'),requestId=crypto.randomUUID();let busy=false;
  form.onsubmit=async event=>{event.preventDefault();if(busy)return;busy=true;const data=new FormData(form);try{await api('saveAgentUsage',{projectId:project,...(usage?{id:usage.id,expectedVersion:usage.version}:{}),requestId,name:data.get('name'),agentId:data.get('agentId'),enabled:data.has('enabled')});if(!guard.current())return;dialog.close();await agentUsageSettings(project);}catch(error){if(guard.current())byId('usage-status').textContent=error.message;}finally{busy=false;}};
}
