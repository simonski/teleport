"use strict";
let projectDirectory=null,projectPresentation='cards',projectDirectoryEpoch=0;

function resetProjectDirectory(){projectDirectory=null;projectPresentation='cards';projectDirectoryEpoch++;}
function projectDirectoryHTML(){
  return `<div class="project-directory-actions"><div class="project-presentation" role="group" aria-label="Project presentation"><button type="button" class="secondary" data-project-presentation="cards" aria-pressed="${projectPresentation==='cards'}">Card view</button><button type="button" class="secondary" data-project-presentation="list" aria-pressed="${projectPresentation==='list'}">List view</button></div><span class="header-space"></span><button type="button" class="secondary" id="project-directory-refresh">Refresh</button><button type="button" id="new-project">＋ New project</button></div><p id="project-directory-status" class="save-status" role="status"></p><div id="project-directory-content">${projectDirectoryContent()}</div>`;
}
function projectDirectoryContent(){
  const rows=(projectDirectory||projects).filter(row=>projects.some(project=>project.id===row.id));
  if(!rows.length)return '<div class="quiet-state">No accessible projects.</div>';
  const edit=p=>roleAllowsMaintain(p.id)?`<button type="button" class="secondary project-edit" data-project-edit="${p.id}" aria-label="Edit ${escapeHTML(p.name)}">Edit</button>`:'';
  const count=(p,name)=>p[name]===undefined?'…':p[name]===null?'Unavailable':Number(p[name]).toLocaleString();
  if(projectPresentation==='cards')return `<div class="project-grid">${rows.map(p=>`<article class="directory-card"><button class="project-card" data-project="${p.id}"><span class="eyebrow">${escapeHTML(p.key)} / ${String(p.id).padStart(2,'0')}</span><h2>${escapeHTML(p.name)}</h2>${p.description?`<p>${escapeHTML(p.description)}</p>`:''}<div><span>${count(p,'tickets')} tickets</span><span>Open workspace ↗</span></div></button>${edit(p)}</article>`).join('')}</div>`;
  return `<div class="project-table-scroll" role="region" aria-label="Project list" tabindex="0"><table class="project-table"><caption class="sr-only">All accessible projects. Tokens are unavailable until metered. Agents counts own authorized sessions; last active uses retained ticket and session start/exit events.</caption><thead><tr>${['Name','Description','Tickets','Epics','Goals','Tokens','Last active','Agents'].map(name=>`<th scope="col">${name}</th>`).join('')}</tr></thead><tbody>${rows.map(p=>`<tr data-project-row="${p.id}"><th scope="row"><button class="project-link secondary" data-project="${p.id}">${escapeHTML(p.name)}</button>${edit(p)}</th><td class="project-description">${escapeHTML(p.description||'Not set')}</td><td data-metric="tickets">${count(p,'tickets')}</td><td data-metric="epics">${count(p,'epics')}</td><td data-metric="goals">${count(p,'goals')}</td><td data-metric="tokens" title="Provider usage is not yet collected">${count(p,'tokens')}</td><td title="Retained ticket changes and authorized session start/exit events">${p.lastActive?`<time datetime="${escapeHTML(p.lastActive)}">${escapeHTML(new Date(p.lastActive).toLocaleString())}</time>`:p.lastActive===null?'No recorded work':'…'}</td><td data-metric="agents" title="Verified running sessions accessible to this account">${count(p,'agents')}</td></tr>`).join('')}</tbody></table></div>`;
}
function drawProjectDirectory(){
  const content=byId('project-directory-content');if(!content)return;
  const focused=document.activeElement,project=focused?.dataset.project,edit=focused?.dataset.projectEdit;
  content.innerHTML=projectDirectoryContent();
  if(document.activeElement===document.body){if(project)content.querySelector(`[data-project="${CSS.escape(project)}"]`)?.focus({preventScroll:true});else if(edit)content.querySelector(`[data-project-edit="${CSS.escape(edit)}"]`)?.focus({preventScroll:true});}
  root.querySelectorAll('[data-project-presentation]').forEach(button=>button.setAttribute('aria-pressed',String(button.dataset.projectPresentation===projectPresentation)));
}
function bindProjectDirectory(){
  if(view!=='Projects')return;
  root.querySelectorAll('[data-project-presentation]').forEach(button=>button.onclick=()=>{projectPresentation=button.dataset.projectPresentation;drawProjectDirectory();});
  byId('project-directory-content').onclick=e=>{
    const open=e.target.closest('[data-project]'),edit=e.target.closest('[data-project-edit]');
    if(open){projectId=Number(open.dataset.project);view='Board';workspace().catch(showError);}
    else if(edit)projectMetadataForm(Number(edit.dataset.projectEdit));
  };
  on('project-directory-refresh','click',()=>loadProjectDirectory());
  loadProjectDirectory();
}
async function loadProjectDirectory(){
  const actor=session?.account.id,generation=loadGeneration,epoch=++projectDirectoryEpoch,status=byId('project-directory-status');
  if(status){status.textContent='Loading…';status.classList.remove('error');}
  try{
    const result=await pages('listProjectSummaries');
    if(session?.account.id!==actor||loadGeneration!==generation||epoch!==projectDirectoryEpoch||view!=='Projects')return;
    projectDirectory=result;drawProjectDirectory();if(status?.isConnected)status.textContent='';
  }catch(error){if(session?.account.id===actor&&loadGeneration===generation&&epoch===projectDirectoryEpoch&&status?.isConnected){status.textContent=error.message||'Project totals unavailable. Refresh to retry.';status.classList.add('error');}}
}
function projectMetadataForm(id){
  const p=[projectDirectory?.find(row=>row.id===id),projects.find(row=>row.id===id)].filter(Boolean).sort((a,b)=>b.version-a.version)[0];if(!p||!roleAllowsMaintain(id))return;
  const dialog=openDialog('Edit project',`<label>Name<input name="name" required maxlength="200" value="${escapeHTML(p.name)}"></label><label>Description<textarea name="description" rows="4" maxlength="20000">${escapeHTML(p.description)}</textarea></label>`,'Save project'),guard=captureDialogCompletion(dialog),form=byId('editor-form');let busy=false;
  form.onsubmit=async e=>{
    e.preventDefault();if(busy)return;busy=true;const button=form.querySelector('[type=submit]');button.disabled=true;
    try{const result=await api('saveProject',{id,expectedVersion:p.version,...Object.fromEntries(new FormData(form))});if(!guard.current())return;projects=projects.map(row=>row.id===id?result:row);dialog.close();await workspace();}
    catch(error){if(guard.current())showError(error);}
    finally{busy=false;if(button.isConnected)button.disabled=false;}
  };
}
