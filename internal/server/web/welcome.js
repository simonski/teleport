"use strict";
let welcomeSeenActor = '', welcomeEpoch = 0;
function resetWelcome() {welcomeSeenActor='';welcomeEpoch++;}
function maybeWelcome() {
  if(!session?.welcomeSetup||session.welcomeSetup.completed||welcomeSeenActor===session.account.id||foregroundOpen())return;
  welcomeSeenActor=session.account.id;openWelcome().catch(showError);
}
async function openWelcome() {
  const actor=session?.account.id,epoch=++welcomeEpoch;
  const [options,availableProjects]=await Promise.all([api('getWelcomeSetup'),pages('listProjects')]);
  if(session?.account.id!==actor||epoch!==welcomeEpoch||foregroundOpen())return;
  if(options.setup.completed)return;
  projects=availableProjects;
  welcomeSeenActor=actor;
  let setup={...options.setup},timer=null,chain=Promise.resolve(),closed=false,pending=0,error='',stepping=false;
  const fields=['step','projectId','projectName','projectKey','repository','storageRoot','harness','attention'];
  const titles=['Welcome to teleport','Project','Local repository','Agent harness','Attention','Review setup'];
  const attention={questions:'Questions and approvals',all:'All workflow updates',quiet:'Badges only'};
  function current(){return !closed&&session?.account.id===actor&&epoch===welcomeEpoch;}
  function status(){const el=byId('welcome-status');if(current()&&el){el.textContent=error||(pending?'Saving…':'');el.classList.toggle('error',!!error);}}
  function read(){
    const form=byId('editor-form');if(!current()||!form)return;
    for(const field of fields){const input=form.elements.namedItem(field);if(input&&!input.disabled)setup[field]=['step','projectId'].includes(field)?Number(input.value):input.value;}
  }
  function save(){
    clearTimeout(timer);read();pending++;status();
    const next=chain.catch(()=>{}).then(async()=>{
      if(!current())return;
      const body=Object.fromEntries(fields.map(field=>[field,setup[field]]));body.expectedVersion=setup.version;
      const result=await api('saveWelcomeSetup',body);
      if(!current())return;
      setup.version=result.version;setup.id=result.id;error='';
    }).catch(e=>{if(current()){error=e.status===409?'Setup changed in another window. Close and reopen to load saved progress.':e.message;status();}throw e;}).finally(()=>{pending--;status();});
    chain=next;next.catch(()=>{});return next;
  }
  function content(){
    const project=projects.find(p=>p.id===setup.projectId);
    if(setup.step===0)return '<p>Set up a project, local harness and attention preferences.</p>';
    if(setup.step===1)return `<label>Project<select name="projectId"><option value="0">Create a project</option>${projects.map(p=>`<option value="${p.id}" ${p.id===setup.projectId?'selected':''}>${escapeHTML(p.name)}</option>`).join('')}</select></label><div id="welcome-new-project" ${setup.projectId?'hidden':''}><label>Project name<input name="projectName" maxlength="200" value="${escapeHTML(setup.projectName)}"></label><label>Project key<input name="projectKey" maxlength="80" pattern="[a-z0-9][a-z0-9_-]{0,79}" placeholder="lowercase-key" value="${escapeHTML(setup.projectKey)}"></label></div>`;
    if(setup.step===2){
      if(!options.canBindRepository)return '<p>A repository can be granted by an installation administrator in Settings.</p>';
      return `<label>Repository root (optional)<input name="repository" maxlength="4096" placeholder="/absolute/path/to/repository" value="${escapeHTML(setup.repository)}"></label><label>Separate worktree storage root<input name="storageRoot" maxlength="4096" placeholder="/absolute/path/to/worktrees" value="${escapeHTML(setup.storageRoot)}"></label><p>Use a committed repository with main checked out.</p><label class="checkbox-label"><input id="welcome-grant" type="checkbox">Grant teleport access to these exact local directories</label>`;
    }
    if(setup.step===3)return `<label>Harness<select name="harness"><option value="">Configure later</option>${options.harnesses.map(h=>`<option value="${escapeHTML(h.id)}" ${setup.harness===h.id?'selected':''}>${escapeHTML(h.title)}${h.available?'':' · not installed'}</option>`).join('')}</select></label><p>Local mode uses the harness’s native permissions. Setup starts no agent.</p>`;
    if(setup.step===4)return `<label>Attention notifications<select name="attention">${Object.entries(attention).map(([value,label])=>`<option value="${value}" ${setup.attention===value?'selected':''}>${label}</option>`).join('')}</select></label><p>Ticket and harness attention badges stay visible in every mode.</p>`;
    return `<dl class="setup-summary"><dt>Project</dt><dd>${escapeHTML(project?.name||'Unavailable')}</dd><dt>Repository</dt><dd>${escapeHTML(setup.repository||'Configure later')}</dd><dt>Harness</dt><dd>${escapeHTML(options.harnesses.find(h=>h.id===setup.harness)?.title||'Configure later')}</dd><dt>Attention</dt><dd>${attention[setup.attention]}</dd></dl><p>Finish saves setup without starting a harness.</p>`;
  }
  function syncForm(){
    const form=byId('editor-form');if(!form)return;
    const creating=setup.step===1&&!Number(form.elements.namedItem('projectId')?.value);
    if(byId('welcome-new-project'))byId('welcome-new-project').hidden=!creating;
    for(const field of ['projectName','projectKey']){const input=form.elements.namedItem(field);if(input){input.disabled=!creating;input.required=creating&&field==='projectName';}}
    const grant=byId('welcome-grant');if(grant)grant.required=!!(form.elements.namedItem('repository').value||form.elements.namedItem('storageRoot').value);
    const submit=form.querySelector('button[type=submit]');submit.textContent=setup.step===5?'Finish setup':creating?'Create project and continue':grant?.required?'Grant and continue':'Continue';
  }
  function draw(){
    const dialog=openDialog(titles[setup.step],`<p class="eyebrow">SETUP ${setup.step+1} / 6</p>${content()}<p id="welcome-status" class="save-status" role="status"></p>`,'Continue');
    byId('cancel').textContent='Later';
    if(setup.step){const back=document.createElement('button');back.type='button';back.className='secondary';back.id='welcome-back';back.textContent='Back';byId('cancel').before(back);back.onclick=async()=>{if(stepping)return;read();stepping=true;try{await save();if(!current())return;setup.step--;await save();if(current())draw();}catch{}finally{stepping=false;}};}
    const form=byId('editor-form');syncForm();status();
    form.addEventListener('input',()=>{read();syncForm();clearTimeout(timer);timer=setTimeout(save,250);});
    form.addEventListener('change',()=>{read();syncForm();clearTimeout(timer);timer=setTimeout(save,250);});
    form.onsubmit=async e=>{
      e.preventDefault();if(stepping)return;read();clearTimeout(timer);stepping=true;
      const disabled=new Map([...form.querySelectorAll('input,select,textarea,button')].filter(el=>!['cancel','dismiss'].includes(el.id)).map(el=>[el,el.disabled]));for(const el of disabled.keys())el.disabled=true;
      try{
        await save();if(!current())return;
        if(setup.step===1){
          let project=projects.find(p=>p.id===setup.projectId);
          if(!project)project=await api('createProject',{name:setup.projectName,key:setup.projectKey,requestId:'welcome-project-'+setup.id});
          if(!current())return;
          setup.projectId=project.id;setup.projectName=project.name;setup.projectKey=project.key;
          if(!projects.some(p=>p.id===project.id))projects.push(project);
          if(project.repository){setup.repository=project.repository.repository;setup.storageRoot=project.repository.storageRoot;}
          // Disable stale project fields before persisting the selected result.
          form.elements.namedItem('projectId').value=String(project.id);
          if(!form.elements.namedItem('projectId').selectedOptions.length){const option=document.createElement('option');option.value=String(project.id);option.textContent=project.name;form.elements.namedItem('projectId').append(option);form.elements.namedItem('projectId').value=String(project.id);}
        }else if(setup.step===2&&options.canBindRepository&&(setup.repository||setup.storageRoot)){
          if(!byId('welcome-grant').checked)throw new Error('Confirm access to these directories or clear both paths to configure later.');
          const project=projects.find(p=>p.id===setup.projectId);
          const result=await api('configureLocalRepository',{projectId:project.id,expectedVersion:project.version,repository:setup.repository,storageRoot:setup.storageRoot});
          if(!current())return;projects=projects.map(p=>p.id===result.id?result:p);setup.repository=result.repository.repository;setup.storageRoot=result.repository.storageRoot;
          form.elements.namedItem('repository').value=setup.repository;form.elements.namedItem('storageRoot').value=setup.storageRoot;
        }else if(setup.step===5){
          await api('completeWelcomeSetup',{expectedVersion:setup.version,confirm:true});if(!current())return;
          projectId=setup.projectId;dialog.close();await workspace();return;
        }
        setup.step++;await save();if(current())draw();
      }catch(e){if(current()&&!e.staleInteraction){error=e.message;status();}}
      finally{stepping=false;for(const [el,value] of disabled)if(el.isConnected)el.disabled=value;}
    };
    dialog.addEventListener('close',()=>{closed=true;clearTimeout(timer);},{once:true});
  }
  draw();
}
