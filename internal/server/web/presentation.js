"use strict";
let preferences = null, preferencesActor = '', initialNavigation = true, appearanceDirty = false, navigationDirty = false;
let preferenceDraft = {}, preferenceSaving = false, preferenceTimer = null, preferenceError = '', preferenceConflict = false;
function presentationBusy() { return view === 'Settings' && (appearanceDirty || navigationDirty); }
function applyAppearance(p) {
  const draft = preferencesActor === session?.account.id ? preferenceDraft : {};
  document.documentElement.dataset.style = draft.style ?? p.style;
  document.documentElement.dataset.mode = draft.mode ?? p.mode;
  document.dispatchEvent(new Event('teleport-appearance'));
  if(typeof applyNavigation==='function')applyNavigation();
}
function clearPresentation() {
  if(typeof ticketEditorPending!=='undefined')ticketEditorPending=null;
  if(typeof cancelHumanTicketDrag==='function')cancelHumanTicketDrag();
  if (typeof dismissOverlays === 'function') dismissOverlays();
  if (typeof resetWelcome === 'function') resetWelcome();
  if (typeof resetTerminals === 'function') resetTerminals();
  if (typeof clearNativeQuestions === 'function') clearNativeQuestions();
  if (typeof resetScriptConsole === 'function') resetScriptConsole();
  if (typeof resetNavigation === 'function') resetNavigation();
  if (typeof resetProjectDirectory === 'function') resetProjectDirectory();
  if (typeof dismissTicketMenu === 'function') dismissTicketMenu();
  if (typeof resetWorkflowPublication === 'function') resetWorkflowPublication();
  if (typeof resetWorkflowView === 'function') resetWorkflowView();
  leaveGraph(); graphCameras.clear(); graphData=null;graphPending={};graphQuery='';graphKind='';calendarState={date:'',period:'default',duePeriod:'',query:''};calendarDirty=false;calendarEpoch++;selected=null;workflowMap={};flow=null;flows=[];flowDirty=false;flowDrag=null;resetShift();projects=[];tickets=[];ideas=[];root.replaceChildren();notice('');session=null;preferences=null;preferencesActor='';initialNavigation=true;appearanceDirty=false;navigationDirty=false;
  clearTimeout(preferenceTimer);preferenceDraft={};preferenceError='';preferenceConflict=false;preferenceSaving=false;
  document.documentElement.removeAttribute('data-style');document.documentElement.removeAttribute('data-mode');
}
function appearanceHTML() {
  const p = {...preferences,...preferenceDraft};
  return `<section><h2>Appearance</h2><div class="form-grid"><label>Visual style<select id="appearance-style"><option value="paper" ${p.style==='paper'?'selected':''}>Paper</option><option value="instrument" ${p.style==='instrument'?'selected':''}>Instrument</option></select></label><label>Mode<select id="appearance-mode"><option value="light" ${p.mode!=='dark'?'selected':''}>Light</option><option value="dark" ${p.mode==='dark'?'selected':''}>Dark</option></select></label></div><h3>Navigation</h3><label>Position<select id="navigation-position">${['top','left','right'].map(position=>`<option value="${position}" ${(p.navigationPosition||'top')===position?'selected':''}>${position[0].toUpperCase()+position.slice(1)}</option>`).join('')}</select></label><p class="navigation-help">Drag tabs to reorder, or focus a tab and press Alt+Shift+Arrow.</p><button type="button" class="secondary" id="navigation-reset">Reset navigation</button><h3>Sign-in defaults</h3><div class="form-grid"><label>Default view<select id="preference-view">${['Home','Board','Todos','Goals','Epics','Calendar','Graph','Ideas','Cost','Projects'].map(name=>`<option ${p.defaultView===name?'selected':''}>${name}</option>`).join('')}</select></label><label>Default project<select id="preference-project"><option value="0">All projects</option>${projects.map(project=>`<option value="${project.id}" ${p.defaultProject===project.id?'selected':''}>${escapeHTML(project.name)}</option>`).join('')}</select></label></div><p id="preference-status" class="save-status" role="status"></p><button type="button" class="secondary" id="preference-retry" hidden>Retry changes</button></section>`;
}
function preferenceStatus() {
  const status=byId('preference-status'),retry=byId('preference-retry');
  if(status){status.textContent=preferenceError || (preferenceSaving?'Saving…':'');status.classList.toggle('error',!!preferenceError);}
  if(retry)retry.hidden=!preferenceError;
  const navStatus=byId('nav-save-status'),navRetry=byId('nav-retry');
  if(navStatus){navStatus.textContent=preferenceError||(preferenceSaving?'Saving…':'');navStatus.classList.toggle('error',!!preferenceError);}
  if(navRetry)navRetry.hidden=!preferenceError;
}
function queuePreferenceSave(field,value) {
  preferenceDraft[field]=value;
  appearanceDirty='style' in preferenceDraft || 'mode' in preferenceDraft;
  navigationDirty=['defaultView','defaultProject','navigationOrder','navigationPosition','navigationCollapsed'].some(field=>field in preferenceDraft);
  if(field==='style'||field==='mode')applyAppearance(preferences);
  if(field.startsWith('navigation')&&typeof applyNavigation==='function')applyNavigation();
  clearTimeout(preferenceTimer);
  if(!preferenceConflict){preferenceError='';preferenceTimer=setTimeout(flushPreferenceSave,120);}
  preferenceStatus();
}
async function flushPreferenceSave() {
  if(preferenceSaving||!session||!Object.keys(preferenceDraft).length||preferenceConflict)return;
  const actor=session.account.id,draft=preferenceDraft;
  preferenceSaving=true;preferenceStatus();
  try {
    for(const [operation,fields] of [['setAppearance',['style','mode']],['saveViewPreferences',['defaultView','defaultProject','navigationOrder','navigationPosition','navigationCollapsed']]]){
      if(session?.account.id!==actor||preferenceDraft!==draft)return;
      const changed=Object.fromEntries(fields.filter(field=>field in draft).map(field=>[field,draft[field]]));
      if(!Object.keys(changed).length)continue;
      const values={...preferences,...draft},body={expectedVersion:preferences.version};
      if(operation==='setAppearance')Object.assign(body,{style:values.style,mode:values.mode});
      else Object.assign(body,{defaultView:values.defaultView,defaultProject:values.defaultProject,recent:preferences.recent||[],navigationOrder:values.navigationOrder,navigationPosition:values.navigationPosition,navigationCollapsed:values.navigationCollapsed});
      const result=await api(operation,body);
      if(session?.account.id!==actor||preferenceDraft!==draft)return;
      if(result.version>=(preferences?.version||0))preferences=result;
      for(const [field,value] of Object.entries(changed))if(draft[field]===value)delete draft[field];
      applyAppearance(preferences);
    }
  }catch(error){
    if(session?.account.id!==actor||preferenceDraft!==draft)return;
    preferenceConflict=error.status===409;
    preferenceError=preferenceConflict?'Preferences changed elsewhere. Retry to apply these changes to the latest version.':error.message||'Preferences could not be saved. Retry changes.';
  }finally{
    if(session?.account.id===actor&&preferenceDraft===draft){preferenceSaving=false;appearanceDirty='style' in draft||'mode' in draft;navigationDirty=['defaultView','defaultProject','navigationOrder','navigationPosition','navigationCollapsed'].some(field=>field in draft);preferenceStatus();if(!preferenceError&&Object.keys(draft).length)preferenceTimer=setTimeout(flushPreferenceSave,120);}
  }
}
function bindPresentation() {
  const defaults=byId('preference-view');if(defaults&&typeof supportedNavigation!=='undefined')defaults.innerHTML=supportedNavigation.map(name=>`<option ${(preferenceDraft.defaultView??preferences?.defaultView)===name?'selected':''}>${name}</option>`).join('');
  for(const [id,field] of [['appearance-style','style'],['appearance-mode','mode'],['preference-view','defaultView'],['preference-project','defaultProject'],['navigation-position','navigationPosition']])on(id,'change',e=>queuePreferenceSave(field,field==='defaultProject'?Number(e.target.value):e.target.value));
  on('preference-retry','click',retryPreferenceSave);
  on('navigation-reset','click',()=>{queuePreferenceSave('navigationOrder',[...canonicalNavigation]);queuePreferenceSave('navigationPosition','top');queuePreferenceSave('navigationCollapsed',false);if(byId('navigation-position'))byId('navigation-position').value='top';});
  preferenceStatus();
}
async function retryPreferenceSave(){
  const actor=session?.account.id,draft=preferenceDraft;
  try{const latest=await api('getViewPreferences');if(session?.account.id!==actor||preferenceDraft!==draft)return;preferences=latest;preferenceConflict=false;preferenceError='';await flushPreferenceSave();}catch(error){if(session?.account.id===actor&&preferenceDraft===draft){preferenceError=error.message;preferenceStatus();}}
}
