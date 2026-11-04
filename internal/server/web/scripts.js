"use strict";
let scriptConsoleEpoch=0,scriptDraft='',scriptEntries=[],scriptCommands=[];
function resetScriptConsole(){scriptConsoleEpoch++;scriptDraft='';scriptEntries=[];scriptCommands=[];}
function closeScriptViewer(){scriptConsoleEpoch++;}
function rememberScriptCommand(line){if(scriptCommands.at(-1)!==line)scriptCommands.push(line);while(scriptCommands.length>50||scriptCommands.join('').length>65536)scriptCommands.shift();}
function retainScriptEntry(entry){const index=scriptEntries.findIndex(row=>row.key===entry.key);if(index<0)scriptEntries.push(entry);else scriptEntries[index]=entry;while(scriptEntries.length>32||scriptEntries.length>1&&scriptEntries.reduce((size,row)=>size+row.input.length+row.output.length,0)>262144)scriptEntries.shift();}

// Split shell-style quoting into data only. No shell, substitutions or source evaluation.
function parseTkCommand(line){
  const words=[];let word='',quote='',escaped=false,started=false;
  for(const char of line){
    if(escaped){word+=char;escaped=false;started=true;continue;}
    if(quote==="'"){if(char==="'")quote='';else word+=char;started=true;continue;}
    if(char==='\\'){escaped=true;started=true;continue;}
    if(quote==='"'){if(char==='"')quote='';else word+=char;started=true;continue;}
    if(char==='"'||char==="'"){quote=char;started=true;continue;}
    if(/\s/u.test(char)){if(started){words.push(word);word='';started=false;}continue;}
    word+=char;started=true;
  }
  if(escaped||quote)throw new Error('Complete the quoted argument before submitting.');
  if(started)words.push(word);
  if(!words.length)return null;
  if(words.length===1&&['clear','exit','help'].includes(words[0]))return {local:words[0]};
  if(words[0]!=='tk')throw new Error('Enter a tk command, such as tk ticket ls.');
  return {args:words};
}
function formatTkResult(value,machine){
  if(machine)return JSON.stringify(value,null,2);
  const line=item=>item&&typeof item==='object'&&!Array.isArray(item)?Object.keys(item).sort().map(key=>key+'='+JSON.stringify(item[key])).join('  '):JSON.stringify(item);
  if(value&&typeof value==='object'&&!Array.isArray(value)&&Array.isArray(value.items)){
    const rows=value.items.length?value.items.map(line):['No items.'];
    for(const [key,item] of Object.entries(value))if(key!=='items'&&item!==null&&item!=='')rows.push(key+': '+line(item));
    return rows.join('\n');
  }
  if(Array.isArray(value))return value.length?value.map(line).join('\n'):'No items.';
  return line(value);
}
async function openScriptConsole(){
  const actor=session?.account.id,initialDetail=detailEpoch,epoch=++scriptConsoleEpoch;
  if(!actor||foregroundOpen()||session?.account.id!==actor||detailEpoch!==initialDetail)return;
  const dialog=openDialog('tk console',`<p id="script-status" role="status">Enter a tk command. Type tk help for commands.</p><div id="script-result" tabindex="0" role="log" aria-label="Console output" aria-live="polite"></div><div class="script-prompt"><span class="script-prompt-marker" aria-hidden="true">$</span><textarea id="script-source" rows="1" spellcheck="false" autocomplete="off" autocapitalize="off" maxlength="1048576" aria-label="tk command" placeholder="tk ticket ls"></textarea></div>`,'Run');
  dialog.classList.add('script-console');byId('cancel').hidden=true;
  const viewerDetail=detailEpoch,source=byId('script-source'),form=byId('editor-form'),submitButton=form.querySelector('button[type=submit]'),transcript=byId('script-result');
  submitButton.id='script-submit';submitButton.setAttribute('aria-label','Run command');source.parentElement.append(submitButton);
  let busy=false,revision=0,historyIndex=scriptCommands.length,historyDraft='';
  const current=()=>epoch===scriptConsoleEpoch&&session?.account.id===actor&&detailEpoch===viewerDetail&&dialog.open&&dialog.querySelector('form')===form;
  function changed(){revision++;scriptDraft=source.value;source.style.height='auto';source.style.height=Math.min(200,Math.max(24,source.scrollHeight))+'px';}
  function draw(){if(!current())return;const atBottom=transcript.scrollHeight-transcript.scrollTop-transcript.clientHeight<40;transcript.replaceChildren();for(const entry of scriptEntries){const item=document.createElement('article'),command=document.createElement('pre'),state=document.createElement('small'),output=document.createElement('pre');item.className='script-entry'+(entry.error?' script-error':'');command.className='script-command';state.className='script-entry-state';output.className='script-entry-output';command.textContent='$ '+entry.input;state.textContent=entry.state;output.textContent=entry.output;item.append(command,state,output);transcript.append(item);}if(atBottom)transcript.scrollTop=transcript.scrollHeight;}
  source.value=scriptDraft;source.oninput=()=>{changed();historyIndex=scriptCommands.length;};
  form.onsubmit=async event=>{
    event.preventDefault();if(busy)return;const text=source.value,editRevision=revision;let parsed;
    try{parsed=parseTkCommand(text);if(!parsed)return;}catch(error){byId('script-status').textContent=error.message;return;}
    rememberScriptCommand(text);historyIndex=scriptCommands.length;
    if(parsed.local==='clear'){scriptEntries=[];source.value='';changed();draw();return;}
    if(parsed.local==='exit'){dialog.close();return;}
    const args=parsed.local==='help'?['tk','help']:parsed.args,key=crypto.randomUUID(),entry={key,input:text,state:'Resolving…',output:'',error:false};retainScriptEntry(entry);draw();busy=true;submitButton.disabled=true;byId('script-status').textContent='Running…';
    try{
      const resolved=await api('resolveConsoleCommand',{args});if(!current())return;
      entry.output=resolved.diagnostic||'';entry.state=resolved.operation?'Calling '+resolved.operation+'…':'';draw();
      if(resolved.operation==='logout'){await signOut();return;}
      const result=resolved.operation?await api(resolved.operation,resolved.data||{}):null;if(!current())return;
      entry.output+=(resolved.operation?formatTkResult(result,!!resolved.machine):resolved.data!==undefined?formatTkResult(resolved.data,!!resolved.machine):resolved.output||'');entry.state='';byId('script-status').textContent='Ready';
      if(source.value===text&&revision===editRevision){source.value='';changed();}
    }catch(error){if(!current()||error.staleInteraction)return;entry.error=true;entry.state='';entry.output+=(error.message||'Command failed')+(error.status===undefined?'\nOutcome may be unconfirmed. Check state before retrying; reuse the request ID shown above if one was generated.':'');byId('script-status').textContent='Command failed';}
    finally{busy=false;if(current()){submitButton.disabled=false;draw();source.focus({preventScroll:true});}}
  };
  source.onkeydown=e=>{if(e.isComposing)return;const plain=!e.ctrlKey&&!e.metaKey&&!e.altKey;if(e.key==='Enter'&&plain&&!e.shiftKey){e.preventDefault();if(!e.repeat)form.requestSubmit();return;}if((e.ctrlKey||e.metaKey)&&!e.altKey&&e.key.toLowerCase()==='l'){e.preventDefault();scriptEntries=[];draw();return;}if(plain&&!e.shiftKey&&['ArrowUp','ArrowDown'].includes(e.key)&&source.selectionStart===source.selectionEnd){const before=source.value.slice(0,source.selectionStart),after=source.value.slice(source.selectionEnd),up=e.key==='ArrowUp';if(up&&!before.includes('\n')||!up&&!after.includes('\n')){e.preventDefault();if(historyIndex===scriptCommands.length)historyDraft=source.value;historyIndex=Math.max(0,Math.min(scriptCommands.length,historyIndex+(up?-1:1)));source.value=scriptCommands[historyIndex]??historyDraft;changed();source.selectionStart=source.selectionEnd=source.value.length;}}};
  dialog.addEventListener('close',()=>{if(epoch===scriptConsoleEpoch)closeScriptViewer();dialog.classList.remove('script-console');},{once:true});
  draw();changed();source.focus();
}
document.addEventListener('keydown',e=>{if(e.key!=='/'||!session||e.ctrlKey||e.metaKey||e.altKey||e.isComposing||foregroundOpen()||e.target.isContentEditable||['INPUT','TEXTAREA','SELECT'].includes(e.target.tagName))return;e.preventDefault();openScriptConsole().catch(showError);});
