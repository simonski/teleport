"use strict";
let scriptConsoleEpoch=0,scriptConsoleTimer=null,scriptDraft=null,scriptPendingStart=null,scriptSelection=null;
let scriptEntries=[],scriptCommands=[],scriptRunLines=new Map(),scriptClearedSignature='';
function resetScriptConsole(){scriptConsoleEpoch++;clearTimeout(scriptConsoleTimer);scriptDraft=null;scriptPendingStart=null;scriptSelection=null;scriptEntries=[];scriptCommands=[];scriptRunLines.clear();scriptClearedSignature='';}
function closeScriptViewer(){scriptConsoleEpoch++;clearTimeout(scriptConsoleTimer);}
function scriptLive(state){return ['starting','running','waiting','stopping','unknown'].includes(state);}
function parseScriptCommand(line,bindings){
  const text=line.trim();
  if(!text)return null;
  if(['help','.help','tk','tk help','tk --help'].includes(text))return {local:'help'};
  if(['clear','.clear'].includes(text))return {local:'clear'};
  if(text==='tk tools')return {local:'tools'};
  if(text==='tk version')return {local:'version'};
  if(!/^tk\s/.test(text))return {source:line,mode:'console'};
  const match=text.match(/^tk\s+([A-Za-z][A-Za-z0-9]*)\s*([\s\S]*)$/);
  if(!match||!bindings.some(binding=>binding.operation===match[1]))throw new Error('Unknown or unavailable tk command. Type help for available bindings.');
  const supplied=match[2].trim();let data=supplied.replace(/^--?data(?:\s+|$)/,'').trim(),args={};
  if(/^--?data$/.test(supplied))throw new Error('The data flag requires a JSON object.');
  if(data){
    if((data[0]==="'"&&data.at(-1)==="'")||(data[0]==='"'&&data.at(-1)==='"'))data=data.slice(1,-1);
    try{args=JSON.parse(data);}catch{throw new Error('Use one JSON object: tk getTicket --data \'{"id":3}\'');}
    if(!args||typeof args!=='object'||Array.isArray(args))throw new Error('Command arguments must be a JSON object.');
  }
  // JSON is data even when it contains quotes, source text or __proto__ keys.
  const source='return await tk('+JSON.stringify(match[1])+',JSON.parse('+JSON.stringify(JSON.stringify(args))+'));';
  if(new TextEncoder().encode(source).length>65536)throw new Error('Command exceeds the 64 KiB source limit.');
  return {source,mode:'script'};
}
function rememberScriptCommand(line){
  if(scriptCommands.at(-1)!==line)scriptCommands.push(line);
  while(scriptCommands.length>50||scriptCommands.join('').length>65536)scriptCommands.shift();
}
function retainScriptEntry(entry){
  const index=scriptEntries.findIndex(row=>row.key===entry.key);
  if(index<0)scriptEntries.push(entry);else scriptEntries[index]=entry;
  while(scriptEntries.length>32||scriptEntries.length>1&&scriptEntries.reduce((size,row)=>size+row.input.length+row.output.length,0)>262144)scriptEntries.shift();
}
async function openScriptConsole(preferred=''){
  const actor=session?.account.id,initialDetail=detailEpoch,epoch=++scriptConsoleEpoch;
  const [bindings,all]=await Promise.all([api('getScriptBindings'),pages('listScriptRuns')]);
  if(session?.account.id!==actor||epoch!==scriptConsoleEpoch||detailEpoch!==initialDetail||foregroundOpen())return;
  const rows=all.filter(row=>row.creator===actor).sort((a,b)=>b.createdAt.localeCompare(a.createdAt));
  if(scriptPendingStart){const existing=rows.find(row=>row.requestId===scriptPendingStart.requestId);if(existing){preferred=existing.id;scriptRunLines.set(existing.id,scriptPendingStart.command);if(scriptDraft?.source===scriptPendingStart.command)scriptDraft={source:'',mode:'console'};scriptEntries=scriptEntries.filter(entry=>entry.key!==scriptPendingStart.requestId);scriptPendingStart=null;}}
  let selected=preferred||(scriptSelection===null?rows[0]?.id||'':scriptSelection),busy=false,currentRun=null,sourceRevision=0,historyIndex=scriptCommands.length,historyDraft='';
  if(selected&&!rows.some(row=>row.id===selected))selected='';
  const dialog=openDialog('JavaScript console',`<div class="script-toolbar"><label>Runs<select id="script-history"><option value="">Prompt</option>${rows.map(row=>`<option value="${escapeHTML(row.id)}" ${row.id===selected?'selected':''}>${escapeHTML(row.id+' · '+row.state)}</option>`).join('')}</select></label><button type="button" class="secondary" id="script-stop" disabled>Stop</button><button type="button" class="secondary" id="script-retry" hidden>Retry pending</button></div><p id="script-status" role="status"></p><div id="script-result" tabindex="0" role="log" aria-label="Console output" aria-live="polite"></div><section id="script-confirmation"></section><div class="script-prompt"><span class="script-prompt-marker" aria-hidden="true">tk ›</span><textarea id="script-source" rows="1" spellcheck="false" autocomplete="off" autocapitalize="off" maxlength="65536" aria-label="JavaScript command" placeholder="JavaScript or tk command"></textarea></div><details class="script-audit"><summary>Call receipts</summary><div id="script-calls"></div></details>`,'↵');
  dialog.classList.add('script-console');byId('cancel').hidden=true;
  const viewerDetail=detailEpoch,source=byId('script-source'),form=byId('editor-form'),runButton=form.querySelector('button[type=submit]'),transcript=byId('script-result');
  runButton.id='script-submit';runButton.setAttribute('aria-label','Run command');runButton.title='Run command · Enter';byId('script-source').parentElement.append(runButton);
  const current=()=>epoch===scriptConsoleEpoch&&session?.account.id===actor&&detailEpoch===viewerDetail&&dialog.open&&dialog.querySelector('form')===form;
  const status=message=>{if(current())byId('script-status').textContent=message;};
  function controls(){if(!current())return;runButton.disabled=busy||!!currentRun&&scriptLive(currentRun.run.state);byId('script-retry').hidden=!scriptPendingStart;byId('script-retry').disabled=busy;}
  function inputChanged(){sourceRevision++;scriptDraft={source:source.value,mode:'console'};source.style.height='auto';source.style.height=Math.min(200,Math.max(24,source.scrollHeight))+'px';}
  source.value=scriptDraft?.source||'';source.oninput=()=>{inputChanged();historyIndex=scriptCommands.length;};
  function drawTranscript(){
    if(!current())return;
    const atBottom=transcript.scrollHeight-transcript.scrollTop-transcript.clientHeight<40,keep=new Set(scriptEntries.map(entry=>entry.key));
    for(const node of [...transcript.children])if(!keep.has(node.dataset.entry))node.remove();
    for(const entry of scriptEntries){
      let node=[...transcript.children].find(item=>item.dataset.entry===entry.key);
      if(!node){node=document.createElement('article');node.className='script-entry';node.dataset.entry=entry.key;node.innerHTML='<pre class="script-command"></pre><small class="script-entry-state"></small><pre class="script-entry-output"></pre>';transcript.append(node);}
      node.classList.toggle('script-error',!!entry.error);
      for(const [selector,value] of [['.script-command','tk › '+entry.input],['.script-entry-state',entry.state],['.script-entry-output',entry.output]]){const child=node.querySelector(selector);if(child.textContent!==value)child.textContent=value;}
    }
    if(atBottom)transcript.scrollTop=transcript.scrollHeight;
  }
  function localResult(line,value){retainScriptEntry({key:'local-'+crypto.randomUUID(),input:line,state:'',output:value,error:false});drawTranscript();}
  function clearTranscript(){scriptEntries=[];scriptClearedSignature=currentRun?JSON.stringify(currentRun):'';drawTranscript();}
  function render(data){
    if(!current())return;currentRun=data;const row=data.run;status(row.id+' · '+row.state+(row.errorCode?' · '+row.errorCode:''));
    const option=[...byId('script-history').options].find(option=>option.value===row.id);if(option)option.textContent=row.id+' · '+row.state;
    byId('script-stop').disabled=!scriptLive(row.state)||row.stopRequested;controls();
    const output=data.output,value=output?[...(output.logs||[]),...(output.success?[JSON.stringify(output.value,null,2)]:[output.error||'Script did not complete'])].join('\n'):'';
    if(scriptClearedSignature!==JSON.stringify(data))retainScriptEntry({key:row.id,input:scriptRunLines.get(row.id)||data.source,state:row.id+' · '+row.state,output:value,error:!!output&&!output.success});
    drawTranscript();
    const calls=byId('script-calls');calls.replaceChildren();for(const call of data.calls){const item=document.createElement('p');item.textContent=call.id+' · '+call.operation+' · '+call.state+(call.errorCode?' · '+call.errorCode:'');calls.append(item);}
    const pending=data.calls.find(call=>call.state==='waiting'),holder=byId('script-confirmation');
    if(!pending){holder.replaceChildren();delete holder.dataset.call;return;}
    const confirmationKey=row.id+':'+pending.id+':'+pending.fingerprint+':'+(data.pendingTicket?.version||'stale');if(holder.dataset.call===confirmationKey)return;holder.dataset.call=confirmationKey;
    const ticket=data.pendingTicket,preview=ticket?.delivery?.candidate;
    holder.innerHTML=`<div class="script-confirm-body"><h3>Confirm exact action</h3><p>${escapeHTML(pending.operation)}${ticket?' · #'+ticket.id+' '+escapeHTML(ticket.title)+' · v'+ticket.version:''}</p><pre>${escapeHTML(JSON.stringify(pending.arguments,null,2))}</pre>${ticket&&pending.operation==='transitionTicket'?`<h4>Completion criteria</h4><pre>${escapeHTML(ticket.criteria||'No written criteria.')}</pre>`:''}${ticket?.refinement&&pending.operation==='acceptRefinement'?`<pre>${escapeHTML(ticket.refinement.text+'\n'+ticket.refinement.criteria)}</pre>`:''}${preview&&pending.operation==='approveTicket'?`<p>Commit ${escapeHTML(preview.commit)} · ${preview.checksPassed?'checks passed':'checks unavailable'}</p><pre>${escapeHTML(preview.diff)}</pre>`:''}${ticket?'':'<p>The decision basis changed. Decline this call or stop the run.</p>'}</div><div class="script-confirm-buttons"><button type="button" id="script-approve" ${ticket?'':'disabled'}>Confirm action</button><button type="button" class="secondary" id="script-decline">Decline action</button></div>`;
    const decisionCurrent=()=>current()&&selected===row.id&&holder.dataset.call===confirmationKey;
    async function decide(decision){if(busy)return;busy=true;holder.querySelectorAll('button').forEach(button=>button.disabled=true);try{await api('confirmScriptCall',{id:row.id,callId:pending.id,fingerprint:pending.fingerprint,decision,confirm:true});if(decisionCurrent()){holder.replaceChildren();delete holder.dataset.call;await poll();}}catch(error){if(decisionCurrent()&&!error.staleInteraction)status(error.message);}finally{busy=false;controls();if(decisionCurrent())holder.querySelectorAll('button').forEach(button=>button.disabled=false);}}
    on('script-approve','click',()=>decide('approve'));on('script-decline','click',()=>decide('decline'));
  }
  async function load(id,restoreSource){
    const editRevision=sourceRevision;
    if(id!==selected){byId('script-confirmation').replaceChildren();delete byId('script-confirmation').dataset.call;}
    selected=id;scriptSelection=id;
    if(!id){currentRun=null;byId('script-confirmation').replaceChildren();byId('script-calls').replaceChildren();byId('script-stop').disabled=true;status('Ready');controls();return;}
    const data=await api('getScriptRun',{id});if(!current()||selected!==id)return;
    if(restoreSource&&sourceRevision===editRevision){source.value=scriptRunLines.get(id)||data.source;inputChanged();}
    render(data);
  }
  async function poll(){clearTimeout(scriptConsoleTimer);if(!current())return;try{if(selected)await load(selected,false);}catch(error){if(current()&&!error.staleInteraction)status(error.message);}finally{if(current())scriptConsoleTimer=setTimeout(poll,700);}}
  byId('script-history').onchange=async()=>{try{scriptClearedSignature='';await load(byId('script-history').value,true);await poll();}catch(error){if(current()&&!error.staleInteraction)status(error.message);}};
  async function submit(retry=false){
    if(busy){status('Finishing the submission. Command retained; press Enter again when ready.');return;}
    if(currentRun&&scriptLive(currentRun.run.state)){status('Current command is '+currentRun.run.state+'. Stop it or select another run.');return;}
    const line=source.value,editRevision=sourceRevision,selectionAtSubmit=selected;let intent=null;
    try{
      if(scriptPendingStart){if(!retry&&line!==scriptPendingStart.command)throw new Error('Retry the pending submission before starting another command.');intent=scriptPendingStart;}
      else {
        const command=parseScriptCommand(line,bindings.bindings);if(!command)return;
        rememberScriptCommand(line);historyIndex=scriptCommands.length;
        if(command.local){
          if(command.local==='clear')clearTranscript();
          else if(command.local==='version')localResult(line,(document.querySelector('meta[name=teleport-build-version]')?.content||'Version unavailable')+' · bindings v'+bindings.version);
          else localResult(line,(command.local==='help'?'Enter runs · Shift+Enter newline · ↑/↓ history · Ctrl+L clear\nJavaScript: 1 + 2, await tk("listProjects", {})\ntk listProjects\ntk getTicket --data \'{"id":3}\'\nEach command has its own isolated run.\n\n':'')+bindings.bindings.map(binding=>binding.operation+(binding.humanConfirmation?' · confirmation':'')).join('\n'));
          source.value='';inputChanged();return;
        }
        intent={...command,command:line,requestId:crypto.randomUUID()};scriptPendingStart=intent;
      }
      busy=true;controls();status('Submitting…');
      const row=await api('startScriptRun',{source:intent.source,mode:intent.mode,requestId:intent.requestId});
      if(session?.account.id===actor){scriptRunLines.set(row.id,intent.command);while(scriptRunLines.size>256)scriptRunLines.delete(scriptRunLines.keys().next().value);if(scriptPendingStart===intent)scriptPendingStart=null;scriptEntries=scriptEntries.filter(entry=>entry.key!==intent.requestId);}
      if(!current())return;
      if(sourceRevision===editRevision&&line===intent.command){source.value='';inputChanged();}
      let option=[...byId('script-history').options].find(option=>option.value===row.id);if(!option){option=document.createElement('option');option.value=row.id;byId('script-history').append(option);}option.textContent=row.id+' · '+row.state;
      if(selected!==selectionAtSubmit)return;selected=row.id;scriptSelection=row.id;byId('script-history').value=row.id;await load(row.id,false);await poll();
    }catch(error){
      if(session?.account.id===actor&&intent&&scriptPendingStart===intent&&error.status>=400&&error.status<500)scriptPendingStart=null;
      if(current()&&!error.staleInteraction){status(error.message);retainScriptEntry({key:intent?.requestId||'local-'+crypto.randomUUID(),input:intent?.command||line,state:scriptPendingStart?'submission unconfirmed':'rejected',output:error.message,error:true});drawTranscript();}
    }finally{busy=false;if(current()){controls();if(document.activeElement===document.body||document.activeElement===runButton||document.activeElement===byId('script-retry'))source.focus({preventScroll:true});}}
  }
  form.onsubmit=e=>{e.preventDefault();submit();};
  byId('script-retry').onclick=()=>submit(true);
  source.onkeydown=e=>{
    if(e.isComposing)return;
    const plain=!e.ctrlKey&&!e.metaKey&&!e.altKey;
    if(e.key==='Enter'&&plain&&!e.shiftKey){e.preventDefault();if(!e.repeat)submit();return;}
    if((e.ctrlKey||e.metaKey)&&!e.altKey&&e.key.toLowerCase()==='l'){e.preventDefault();clearTranscript();return;}
    if(e.ctrlKey&&!e.metaKey&&!e.altKey&&e.key.toLowerCase()==='c'&&source.selectionStart===source.selectionEnd){e.preventDefault();if(currentRun&&scriptLive(currentRun.run.state))byId('script-stop').click();else{source.value='';inputChanged();}return;}
    if(plain&&!e.shiftKey&&['ArrowUp','ArrowDown'].includes(e.key)&&source.selectionStart===source.selectionEnd){
      const before=source.value.slice(0,source.selectionStart),after=source.value.slice(source.selectionEnd),up=e.key==='ArrowUp';
      if(up&&!before.includes('\n')||!up&&!after.includes('\n')){e.preventDefault();if(historyIndex===scriptCommands.length)historyDraft=source.value;historyIndex=Math.max(0,Math.min(scriptCommands.length,historyIndex+(up?-1:1)));source.value=scriptCommands[historyIndex]??historyDraft;inputChanged();source.selectionStart=source.selectionEnd=source.value.length;}return;
    }
    if(e.key==='Tab'&&plain&&!e.shiftKey){
      const prefix=source.value.slice(0,source.selectionStart),match=prefix.match(/^tk\s+([A-Za-z]*)$/)||prefix.match(/tk\(["']([A-Za-z]*)$/);
      if(match){const options=bindings.bindings.filter(binding=>binding.operation.startsWith(match[1]));if(options.length){e.preventDefault();let common=options[0].operation;for(const option of options)while(!option.operation.startsWith(common))common=common.slice(0,-1);source.setRangeText(common,source.selectionStart-match[1].length,source.selectionStart,'end');inputChanged();if(options.length>1)localResult('Tab',options.map(option=>option.operation).join('  '));}}
    }
  };
  on('script-stop','click',()=>{if(!currentRun)return;const id=currentRun.run.id;dialog.close();const confirmation=openDialog('Stop '+id,'<p>Stop the interpreter? Committed API effects remain recorded.</p>','Stop run'),completion=captureDialogCompletion(confirmation);byId('editor-form').onsubmit=async e=>{e.preventDefault();try{await api('stopScriptRun',{id,confirm:true});if(!completion.current())return;confirmation.close();await openScriptConsole(id);}catch(error){if(completion.current()&&!error.staleInteraction)showError(error);}};});
  dialog.addEventListener('close',()=>{if(epoch===scriptConsoleEpoch)closeScriptViewer();dialog.classList.remove('script-console');},{once:true});
  drawTranscript();if(selected&&!scriptPendingStart)await load(selected,false);await poll();controls();inputChanged();source.focus();
}
document.addEventListener('keydown',e=>{if(e.key!=='/'||!session||e.ctrlKey||e.metaKey||e.altKey||e.isComposing||foregroundOpen()||e.target.isContentEditable||['INPUT','TEXTAREA','SELECT'].includes(e.target.tagName))return;e.preventDefault();openScriptConsole().catch(showError);});
