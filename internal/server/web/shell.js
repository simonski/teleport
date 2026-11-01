"use strict";

function closeAccountMenu() {
  const menu=byId('account-menu');if(menu)menu.hidden=true;
  byId('account')?.setAttribute('aria-expanded','false');
}

function closeOnlyDialog(title,body) {
  const dialog=openDialog(title,body,'Close');
  dialog.opener=byId('account');
  byId('cancel').hidden=true;
  byId('editor-form').onsubmit=e=>{e.preventDefault();dialog.close();};
  return dialog;
}

function openPersonalSettings() {
  closeOnlyDialog('Settings',`<div class="settings-list">${appearanceHTML()}<section><h2>Account</h2><p>${escapeHTML(session.account.username)}</p><button type="button" class="secondary" id="change-password">Change password</button></section></div>`);
  bindPresentation();on('change-password','click',passwordForm);
}

function openSystemSettings() {
  const settings=session.settings,admin=session.account.administrator;
  const modes=[['off','Off'],['demo','Demo · simulated outcomes'],['local','Local · native Codex workflows']];
  closeOnlyDialog('System',`<div class="settings-list"><section><h2>Execution</h2>${admin?`<label>Mode<select id="execution-mode">${modes.map(([value,label])=>`<option value="${value}" ${settings.executionMode===value?'selected':''}>${label}</option>`).join('')}</select></label><p id="execution-save-status" class="save-status" role="status"></p><p>Local mode runs queued agent work using native Codex permissions and model credits.</p><button type="button" class="secondary" id="system-brake">${settings.handbrake?'Release system handbrake':'Pause system admissions'}</button>`:`<p>${runtimeModeLabel()}</p><p>${settings.handbrake?'System admissions paused':'System handbrake released'}</p>`}</section><section><h2>Registration</h2><p>${settings.registrations?'Enabled':'Disabled'}</p></section></div>`);
  on('execution-mode','change',async()=>{await setMode();syncRuntimeLabel();});
  on('system-brake','click',systemBrakeForm);
}

function openProjectSettings() {
  const project=currentProject();if(!project)return;
  const dialog=closeOnlyDialog('Project · '+escapeHTML(project.name),`<div class="settings-list">${repositoryHTML()}<section><h2>Execution</h2><button type="button" class="secondary" id="inspect-native">Inspect native execution readiness</button>${roleAllowsMaintain(project.id)?`<button type="button" class="secondary" id="project-brake">${project.handbrake?'Release project handbrake':'Pause project admissions'}</button><button type="button" class="secondary" id="edit-workflows">Workflow editor</button>`:''}</section><section><button type="button" class="secondary" id="hidden-tickets">Hidden tickets</button></section></div>`);
  on('configure-repository','click',bindingForm);on('inspect-native','click',inspectNativeCapabilities);
  const guard=captureDialogCompletion(dialog);let brakeBasis=project,busy=false;
  on('project-brake','click',async()=>{
    if(busy)return;busy=true;const button=byId('project-brake');button.disabled=true;
    try{
      const result=await api('setHandbrake',{projectId:brakeBasis.id,expectedVersion:brakeBasis.version,engaged:!brakeBasis.handbrake});
      if(!guard.current())return;brakeBasis=result;projects=projects.map(p=>p.id===result.id?result:p);
      button.textContent=result.handbrake?'Release project handbrake':'Pause project admissions';
    }catch(error){if(guard.current())showError(error);}
    finally{busy=false;if(button.isConnected)button.disabled=false;}
  });
  on('edit-workflows','click',()=>{byId('editor').close();workflowEditor();});
  on('hidden-tickets','click',hiddenTickets);
}

function systemBrakeForm() {
  const actor=session.account.id,basis={...session.settings},engaged=!basis.handbrake;
  const dialog=openDialog(engaged?'Pause system admissions':'Release system handbrake','<p>Apply this handbrake to installation-wide agent admissions?</p>',engaged?'Pause admissions':'Release handbrake');
  const guard=captureDialogCompletion(dialog),form=byId('editor-form');let busy=false;
  form.onsubmit=async e=>{
    e.preventDefault();if(busy)return;busy=true;form.querySelector('button[type=submit]').disabled=true;
    try{
      const result=await api('setHandbrake',{projectId:0,expectedVersion:basis.version,engaged});
      if(session?.account.id!==actor||!guard.current())return;
      session.settings=result;dialog.close();await workspace();
    }catch(error){if(guard.current())showError(error);}
    finally{busy=false;if(form.isConnected)form.querySelector('button[type=submit]').disabled=false;}
  };
}

function syncRuntimeLabel() {
  const label=document.querySelector('.runtime .muted');
  if(label)label.textContent='/ '+runtimeModeLabel();
}
function runtimeModeLabel(){return session.settings.executionMode==='local'?'native workflows':session.settings.executionMode==='demo'?'DEMO · simulated':'agents off';}

function syncConsoleAttention(){
  const button=byId('harness-console');if(!button)return;
  const count=localSessions.filter(row=>runningSession(row)&&(row.attention||row.errorCode)).length;
  button.classList.toggle('needs-attention',count>0);
  button.setAttribute('aria-label',count?`Harness sessions · ${count} require attention`:'Harness sessions');
}

function captureShellFocus(){
  const element=document.activeElement,actor=session?.account.id,generation=loadGeneration;
  let selector='';
  if(element?.closest('#nav-views')&&element.dataset.view)selector=`#nav-views [data-view="${CSS.escape(element.dataset.view)}"]`;
  else if(element?.closest('.topbar')){
    if(element.id)selector='#'+CSS.escape(element.id);
    else if(element.dataset.session)selector=`#session-icons [data-session="${CSS.escape(element.dataset.session)}"]`;
    else if(element.classList.contains('choice-button')&&element.previousElementSibling?.id==='project')selector='#project + .choice-button';
  }
  return ()=>{
    if(!selector)return;
    const restore=()=>{if(session?.account.id===actor&&loadGeneration===generation&&document.activeElement===document.body)root.querySelector(selector)?.focus({preventScroll:true});};
    restore();queueMicrotask(restore);
  };
}

function bindShell() {
  const menu=byId('account-menu'),settings=byId('account-settings');
  if(menu&&settings){
    const system=document.createElement('button');system.type='button';system.id='account-system';system.setAttribute('role','menuitem');system.textContent='System';settings.after(system);
    on('account-system','click',()=>{closeAccountMenu();openSystemSettings();});
    if(currentProject()){
      const project=document.createElement('button');project.type='button';project.id='account-project-settings';project.setAttribute('role','menuitem');project.textContent='Project settings';system.after(project);
      on('account-project-settings','click',()=>{closeAccountMenu();openProjectSettings();});
    }
  }
  const sessions=byId('session-icons');
  if(sessions){
    const consoles=document.createElement('div');consoles.className='header-consoles';
    consoles.innerHTML='<button type="button" class="secondary console-trigger" id="javascript-console" aria-label="JavaScript console" title="JavaScript console · /"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 7 5 5-5 5M13 17h6"/></svg></button><button type="button" class="secondary console-trigger" id="harness-console" aria-label="Harness sessions" title="Harness sessions · `"><svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="6" width="18" height="14" rx="2"/><path d="M12 3v3M7 11h2M15 11h2M8 16h8"/></svg></button>';
    sessions.before(consoles);
    on('javascript-console','click',()=>openScriptConsole().catch(showError));
    on('harness-console','click',()=>sessionSwitcher().catch(showError));
    syncConsoleAttention();
  }
  const main=root.querySelector('main');main?.setAttribute('aria-label',view);
  for(const row of root.querySelectorAll('.page-heading,.board-heading')){
    row.querySelectorAll('h1,.eyebrow,.date').forEach(node=>node.remove());
    for(const child of [...row.children])if(!child.textContent.trim()&&!child.querySelector('button,input,select,textarea'))child.remove();
    if(!row.children.length)row.remove();else row.classList.add('view-actions');
  }
}

document.addEventListener('DOMContentLoaded',()=>{
  const footer=byId('build-version'),version=document.querySelector('meta[name=teleport-build-version]')?.content;
  if(footer)footer.textContent=version||'Version unavailable';
});
