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
  let setup={...options.setup},timer=null,availabilityTimer=null,availabilityEpoch=0,keyAutomatic=!options.setup.projectKey,chain=Promise.resolve(),closed=false,pending=0,error='',stepping=false,finishing=false;
  if(setup.step===2)setup.step=3;
  setup.repository='';setup.storageRoot='';
  const fields=['step','projectId','projectName','projectKey','repository','storageRoot','harness','attention'];
  const steps=[0,1,3,4,5],titles=['Welcome to teleport','Project','','Agent harness','Attention','Review setup'];
  const attention={questions:'Questions and approvals',all:'All workflow updates',quiet:'Badges only'};
  function current(){return !closed&&session?.account.id===actor&&epoch===welcomeEpoch;}
  function status(){const el=byId('welcome-status');if(current()&&el){el.textContent=error||(pending?'Saving…':'');el.classList.toggle('error',!!error);}}
  function read(){
    const form=byId('editor-form');if(!current()||!form)return;
    for(const field of fields){const input=form.elements.namedItem(field);if(input&&!input.disabled)setup[field]=['step','projectId'].includes(field)?Number(input.value):input.value;}
  }
  function save(){
    if(finishing)return Promise.resolve();
    clearTimeout(timer);read();pending++;status();
    const next=chain.catch(()=>{}).then(async()=>{
      if(!current()||finishing)return;
      const body=Object.fromEntries(fields.map(field=>[field,setup[field]]));body.expectedVersion=setup.version;
      const result=await api('saveWelcomeSetup',body);
      if(!current())return;
      setup.version=result.version;setup.id=result.id;error='';
    }).catch(e=>{if(current()){error=e.status===409?'Setup changed in another window. Close and reopen to load saved progress.':e.message;status();}throw e;}).finally(()=>{pending--;status();});
    chain=next;next.catch(()=>{});return next;
  }
  async function checkAvailability(){
    clearTimeout(availabilityTimer);
    const form=byId('editor-form'),name=form?.elements.namedItem('projectName'),key=form?.elements.namedItem('projectKey');
    if(!current()||setup.step!==1||Number(form.elements.namedItem('projectId').value)||!name||!key)return true;
    const request=++availabilityEpoch,nameValue=name.value,keyValue=key.value;
    const status=byId('welcome-project-status');status.textContent='Checking…';
    try{
      const result=await api('checkProjectAvailability',{name:nameValue,key:keyValue});
      if(!current()||request!==availabilityEpoch||!name.isConnected||name.value!==nameValue||key.value!==keyValue)return false;
      if(keyAutomatic){key.value=nameValue.trim()?result.suggestedKey:'';setup.projectKey=key.value;}
      const message=!nameValue.trim()?'':!result.nameAvailable?'Project name already exists':!keyAutomatic&&!result.keyAvailable?'Project prefix already exists':'';
      name.setCustomValidity(!result.nameAvailable&&nameValue.trim()?'Project name already exists':'');
      key.setCustomValidity(!keyAutomatic&&!result.keyAvailable&&keyValue?'Project prefix already exists':'');
      status.textContent=message;status.classList.toggle('error',!!message);
      return result.nameAvailable&&(keyAutomatic||result.keyAvailable);
    }catch(e){if(current()&&request===availabilityEpoch&&name.isConnected){status.textContent=e.message;status.classList.add('error');}return false;}
  }
  function content(){
    const project=projects.find(p=>p.id===setup.projectId);
    if(setup.step===0)return '<p>Set up a project, local harness and attention preferences.</p>';
    if(setup.step===1)return `<label>Project<select name="projectId"><option value="0">Create a project</option>${projects.map(p=>`<option value="${p.id}" ${p.id===setup.projectId?'selected':''}>${escapeHTML(p.name)}</option>`).join('')}</select></label><div id="welcome-new-project" ${setup.projectId?'hidden':''}><label>Project name<input name="projectName" maxlength="200" aria-describedby="welcome-project-status" value="${escapeHTML(setup.projectName)}"></label><label>Project prefix<input name="projectKey" maxlength="80" pattern="[a-z0-9][a-z0-9_-]{0,79}" aria-describedby="welcome-project-status" value="${escapeHTML(setup.projectKey)}"></label><p id="welcome-project-status" class="save-status" role="status"></p></div>`;
    if(setup.step===3)return `<label>Harness<select name="harness"><option value="">Configure later</option>${options.harnesses.map(h=>`<option value="${escapeHTML(h.id)}" ${setup.harness===h.id?'selected':''}>${escapeHTML(h.title)}${h.available?'':' · not installed'}</option>`).join('')}</select></label><p>Local mode uses the harness’s native permissions. Setup starts no agent.</p>`;
    if(setup.step===4)return `<label>Attention notifications<select name="attention">${Object.entries(attention).map(([value,label])=>`<option value="${value}" ${setup.attention===value?'selected':''}>${label}</option>`).join('')}</select></label><p>Ticket and harness attention badges stay visible in every mode.</p>`;
    return `<dl class="setup-summary"><dt>Project</dt><dd>${escapeHTML(project?.name||setup.projectName)}</dd><dt>Prefix</dt><dd>${escapeHTML(project?.key||setup.projectKey)}</dd><dt>Harness</dt><dd>${escapeHTML(options.harnesses.find(h=>h.id===setup.harness)?.title||'Configure later')}</dd><dt>Attention</dt><dd>${attention[setup.attention]}</dd></dl>`;
  }
  function syncForm(){
    const form=byId('editor-form');if(!form)return;
    const creating=setup.step===1&&!Number(form.elements.namedItem('projectId')?.value);
    if(byId('welcome-new-project'))byId('welcome-new-project').hidden=!creating;
    for(const field of ['projectName','projectKey']){const input=form.elements.namedItem(field);if(input){input.disabled=!creating;input.required=creating;}}
    const submit=form.querySelector('button[type=submit]');submit.textContent=setup.step===5?'Finish setup':'Continue';
  }
  function draw(){
    const dialog=openDialog(titles[setup.step],`<p class="eyebrow">SETUP ${steps.indexOf(setup.step)+1} / 5</p>${content()}<p id="welcome-status" class="save-status" role="status"></p>`,'Continue');
    byId('cancel').textContent='Later';
    if(setup.step){const back=document.createElement('button');back.type='button';back.className='secondary';back.id='welcome-back';back.textContent='Back';byId('cancel').before(back);back.onclick=async()=>{if(stepping)return;finishing=false;read();stepping=true;try{await save();if(!current())return;setup.step=steps[steps.indexOf(setup.step)-1];await save();if(current())draw();}catch{}finally{stepping=false;}};}
    const form=byId('editor-form');syncForm();status();
    function changed(e){
      if(stepping||finishing||form!==byId('editor-form'))return;
      availabilityEpoch++;if(e.target.name==='projectKey')keyAutomatic=!e.target.value;
      for(const field of ['projectName','projectKey'])form.elements.namedItem(field)?.setCustomValidity('');
      read();syncForm();clearTimeout(availabilityTimer);availabilityTimer=setTimeout(()=>checkAvailability().then(()=>{if(current()&&!stepping)save();}),200);
      clearTimeout(timer);timer=setTimeout(save,250);
    }
    form.addEventListener('input',changed);form.addEventListener('change',changed);
    if(setup.step===1)checkAvailability();
    form.onsubmit=async e=>{
      e.preventDefault();if(stepping)return;read();clearTimeout(timer);stepping=true;
      const disabled=new Map([...form.querySelectorAll('input,select,textarea,button')].filter(el=>!['cancel','dismiss'].includes(el.id)).map(el=>[el,el.disabled]));for(const el of disabled.keys())el.disabled=true;
      try{
        if(setup.step===1&&!await checkAvailability())return;
        if(setup.step===5){
          finishing=true;availabilityEpoch++;clearTimeout(timer);clearTimeout(availabilityTimer);
          await chain.catch(()=>{});if(!current())return;
          const result=await api('completeWelcomeSetup',{expectedVersion:setup.version,confirm:true});if(!current())return;
          projectId=result.projectId;dialog.close();await workspace();return;
        }
        await save();if(!current())return;
        setup.step=steps[steps.indexOf(setup.step)+1];await save();if(current())draw();
      }catch(e){if(current()&&!e.staleInteraction){error=e.message;status();}}
      finally{stepping=false;for(const [el,value] of disabled)if(el.isConnected)el.disabled=value;}
    };
    dialog.addEventListener('close',()=>{closed=true;availabilityEpoch++;clearTimeout(timer);clearTimeout(availabilityTimer);},{once:true});
  }
  draw();
}
