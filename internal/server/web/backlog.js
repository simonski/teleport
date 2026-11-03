"use strict";
let backlogOrder={version:0,keys:[]},backlogScope='',backlogEpoch=0,backlogPending=false,backlogLoading=false,backlogLoaded=false,backlogDrag=null,backlogDispose=null,backlogKind='',backlogQuery='',backlogIcons=null;
const backlogCollapsed=new Set();
function backlogBusy(){return !!backlogDrag||backlogPending;}
function leaveBacklog(){backlogEpoch++;backlogDispose?.();backlogDispose=null;}
function resetBacklog(){leaveBacklog();backlogOrder={version:0,keys:[]};backlogScope='';backlogPending=false;backlogLoading=false;backlogKind='';backlogQuery='';backlogCollapsed.clear();}
function backlogCanMove(ticket){return !!ticket&&ticket.status==='Todo'&&!ticket.protected&&!ticket.archived&&!ticket.deleted&&!ticket.active&&!ticket.claim&&!ticket.execution&&!ticket.delivery&&roleAllowsWrite(ticket.projectId);}
function backlogRows(records,ideaRows,keys=[],kind='',query=''){
  const rank=new Map(keys.map((key,index)=>[key,index])),byID=new Map(records.map(ticket=>[ticket.id,ticket])),children=new Map(),roots=[];
  const sorted=rows=>rows.sort((a,b)=>(rank.get(a.key)??1e12)-(rank.get(b.key)??1e12)||String(a.key).localeCompare(String(b.key),undefined,{numeric:true}));
  const nodes=records.filter(ticket=>!ticket.archived&&!ticket.deleted).map(ticket=>({key:'ticket:'+ticket.id,kind:ticket.kind,ticket,projectId:ticket.projectId,parentId:ticket.parentId,title:ticket.title}));
  const nodeIDs=new Set(nodes.map(node=>node.ticket.id));
  for(const node of nodes){if(node.parentId&&nodeIDs.has(node.parentId)){if(!children.has(node.parentId))children.set(node.parentId,[]);children.get(node.parentId).push(node);}else roots.push(node);}
  roots.push(...ideaRows.filter(idea=>!idea.archived&&!idea.deleted).map(idea=>({key:idea.id,kind:'idea',idea,projectId:idea.projectId,parentId:0,title:idea.title})));
  const wanted=new Set(),match=node=>(!kind||node.kind===kind)&&(!query||`${node.title} ${node.key} ${node.ticket?.description||node.idea?.text||''}`.toLowerCase().includes(query.toLowerCase()));
  for(const node of nodes.concat(roots.filter(node=>node.idea)))if(match(node)&&(!node.ticket?.protected||(children.get(node.ticket.id)||[]).length)){
    wanted.add(node.key);let parent=byID.get(node.parentId);while(parent&&!wanted.has('ticket:'+parent.id)){wanted.add('ticket:'+parent.id);parent=byID.get(parent.parentId);}
  }
  const output=[],seen=new Set();
  function visit(node,depth){if(seen.has(node.key))return;seen.add(node.key);if(!wanted.has(node.key))return;const childRows=sorted(children.get(node.ticket?.id)||[]);output.push({...node,depth,children:childRows.filter(row=>wanted.has(row.key)).length,context:!match(node)||!!node.ticket?.protected});for(const child of childRows)visit(child,depth+1);}
  for(const node of sorted(roots))visit(node,node.ticket&&node.parentId&&!byID.has(node.parentId)?node.kind==='task'?2:1:0);
  return output;
}
function backlogHTML(){
  const scope=`${session.account.id}:${projectId}`;if(scope!==backlogScope){backlogScope=scope;backlogOrder={version:0,keys:[]};backlogCollapsed.clear();}
  return `<section class="backlog-view"><header class="summary-heading"><div><h2>Backlog</h2><p>Ideas, goals, epics and stories</p></div><label class="compact">Show<select id="backlog-kind">${[['','Everything'],['idea','Ideas'],['goal','Goals'],['epic','Epics'],['task','Stories']].map(([value,title])=>`<option value="${value}" ${value===backlogKind?'selected':''}>${title}</option>`).join('')}</select></label><label class="backlog-search"><span class="sr-only">Search backlog</span><input id="backlog-query" type="search" value="${escapeHTML(backlogQuery)}" placeholder="Search backlog"></label><button type="button" id="backlog-import" class="secondary" aria-keyshortcuts="I">Import <kbd>I</kbd></button><span class="shortcut-note"><kbd>N</kbd> Add ${backlogKind==='task'?'story':backlogKind||'story'}</span></header><p id="backlog-status" role="status" class="save-status">Loading saved order…</p><div id="backlog-list" role="list" aria-label="Backlog hierarchy">${backlogContent()}</div><div class="backlog-out" id="backlog-out">Drop here to move unlocked work to its project’s Unassigned parent</div></section>`;
}
function backlogImportForm(){
  const options=projects.filter(project=>roleAllowsWrite(project.id)).map(project=>`<option value="${project.id}" ${project.id===projectId?'selected':''}>${escapeHTML(project.name)}</option>`).join('');
  const dialog=openDialog('Import thoughts',`<p>Paste unstructured text or choose a UTF-8 text file. The text is saved as an Idea for later review.</p><label>Placement<select id="import-project"><option value="0" ${!projectId?'selected':''}>Private inbox · creator and administrators</option>${options}</select></label><p class="form-help">Project placement shares the Idea with project members. Automatic goal classification is not available yet.</p><label>Text file <input id="import-file" type="file" accept="text/*,.md,.txt"></label><label>Thoughts<textarea id="import-text" rows="10" required placeholder="Paste notes, plans or ideas…"></textarea></label><p class="form-help">Up to 20,000 UTF-8 bytes; the first nonblank line becomes the title.</p><p id="import-error" role="alert" class="save-status error"></p>`,'Save idea');
  const completion=captureDialogCompletion(dialog),input=byId('import-text'),file=byId('import-file'),error=byId('import-error'),form=byId('editor-form'),encoder=new TextEncoder();let requestId=crypto.randomUUID(),lastPayload='',pending=false;
  file.onchange=async()=>{const selected=file.files?.[0];if(!selected)return;const before=input.value;error.textContent='';if(selected.size>20000){error.textContent='The file exceeds 20,000 bytes.';return;}try{const value=new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(await selected.arrayBuffer());if(!completion.current()||file.files?.[0]!==selected||input.value!==before)return;input.value=value;input.focus();}catch{if(completion.current()&&file.files?.[0]===selected)error.textContent='Choose a valid UTF-8 text file.';}};
  form.onsubmit=async event=>{event.preventDefault();if(pending)return;error.textContent='';const text=input.value;if(!text.trim()){error.textContent='Enter some text to import.';return;}if(encoder.encode(text).length>20000){error.textContent='Text exceeds 20,000 UTF-8 bytes.';return;}let title='';for(const char of text.split(/\r?\n/).find(line=>line.trim())?.trim()||'Imported thoughts'){if(encoder.encode(title+char).length>500)break;title+=char;}const project=Number(byId('import-project').value),payload=JSON.stringify({title,text,projectId:project});if(lastPayload&&lastPayload!==payload)requestId=crypto.randomUUID();lastPayload=payload;pending=true;const submit=form.querySelector('[type=submit]');submit.disabled=true;try{const saved=await api('saveIdea',{title,text,projectId:project,requestId});if(!completion.current())return;if(input.value!==text||Number(byId('import-project').value)!==project){error.textContent='Earlier text was saved. Review your changes before saving again.';requestId=crypto.randomUUID();lastPayload='';return;}dialog.close();await workspace();if(!completion.follow())return;await openIdea(saved.id);notice('Idea saved; goal classification is not available yet.');}catch(failure){if(completion.current())error.textContent=failure.message||'Could not save the Idea.';}finally{pending=false;if(completion.current())submit.disabled=false;}};
  input.focus();
}
function backlogContent(){
  const rows=backlogRows(tickets,ideas,backlogOrder.keys,backlogKind,backlogQuery),hidden=new Set(),byID=new Map(tickets.map(ticket=>[ticket.id,ticket]));
  return rows.map(row=>{
    if(hidden.has(row.parentId)){if(row.ticket)hidden.add(row.ticket.id);return '';}
    if(row.ticket&&backlogCollapsed.has(row.key)&&!backlogQuery)hidden.add(row.ticket.id);
    const item=row.ticket||row.idea,story=row.kind==='task',label=story?'Story':row.kind,unlocked=backlogCanMove(row.ticket),parent=byID.get(row.parentId),icon=backlogIcons?.[row.kind]||'M3 4h18v16H3ZM7 9h10M7 14h6';
    const meta=row.idea?`${row.idea.placement==='inbox'?'Private inbox':projectName(row.projectId)} · ${row.idea.acceptedCount?'Accepted proposal':'Idea'}`:`${projectId?'':projectName(row.projectId)+' · '}#${item.id} · ${phaseTitle(item)}${row.parentId&&!parent?' · Hidden parent #'+row.parentId:''}`;
    return `<article class="backlog-row ${row.context?'backlog-context':''}" role="listitem" data-backlog-key="${escapeHTML(row.key)}" data-backlog-depth="${row.depth}" data-project-id="${row.projectId}"><button type="button" class="backlog-grip secondary" data-backlog-grip="${escapeHTML(row.key)}" aria-label="Reorder ${escapeHTML(row.title)}" aria-keyshortcuts="Alt+ArrowUp Alt+ArrowDown" title="Drag to reorder; drop at a card centre to change parent. Alt+Up/Down reorders.">⠿</button>${row.children?`<button type="button" class="backlog-fold secondary" data-backlog-fold="${escapeHTML(row.key)}" aria-label="${backlogCollapsed.has(row.key)?'Expand':'Collapse'} ${escapeHTML(row.title)}" aria-expanded="${!backlogCollapsed.has(row.key)}">${backlogCollapsed.has(row.key)?'›':'⌄'}</button>`:'<span class="backlog-fold-space"></span>'}<button type="button" class="graph2d-card backlog-card" data-backlog-open="${escapeHTML(row.key)}" data-kind="${row.kind}" data-status="${escapeHTML(row.ticket?.status||'')}"><span class="graph2d-icon" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="${icon}"/></svg></span><span class="graph2d-copy"><strong>${escapeHTML(row.title)}</strong><span class="graph2d-meta">${escapeHTML(meta)}</span></span><span class="backlog-type">${label}</span>${row.ticket?.attention?'<span class="backlog-attention" aria-label="Attention required">●</span>':''}<span class="backlog-lock" title="${unlocked?'Parent can change':'Parent is locked'}" aria-label="${unlocked?'Parent can change':'Parent is locked'}">${row.idea?'':unlocked?'':'▣'}</span></button>${row.ticket&&!row.ticket.protected&&row.kind!=='goal'?`<button type="button" class="secondary backlog-move" data-backlog-move="${item.id}" ${unlocked?'':'disabled'} aria-label="Change parent of ${escapeHTML(row.title)}">Move</button>`:'<span></span>'}</article>`;
  }).join('')||'<p class="quiet-state">No matching backlog items. Press N to add work.</p>';
}
function backlogReorder(keys,key,target,after=false){const result=keys.filter(value=>value!==key),index=result.indexOf(target);if(index<0)return [...keys];result.splice(index+(after?1:0),0,key);return result;}
function backlogTarget(source,target,zone='after'){
  if(!source||!target||source.key===target.key||source.projectId!==target.projectId)return null;
  if(source.idea||target.idea)return !source.parentId&&!target.parentId&&zone!=='inside'?{parentId:null,target:target.key,after:zone==='after'}:null;
  const ticket=source.ticket,destination=target.ticket;
  let parentId=ticket.parentId;
  if(zone==='inside'){
    if(ticket.kind==='task'&&destination.kind==='epic'||ticket.kind==='epic'&&destination.kind==='goal')parentId=destination.id;
    else if(ticket.kind===destination.kind)parentId=destination.parentId;else return null;
  }else{if(ticket.kind!==destination.kind)return null;parentId=destination.parentId;}
  return {parentId:parentId===ticket.parentId?null:parentId,target:target.key,after:zone!=='before'};
}
async function commitBacklog(keys,move=null){
  if(backlogPending||!backlogLoaded)return;const actor=session.account.id,scope=projectId,epoch=backlogEpoch,pending={};backlogPending=pending;
  const status=byId('backlog-status');status.textContent='Saving…';
  try{const result=await api('saveBacklogOrder',{projectId:scope,expectedVersion:backlogOrder.version,keys,...(move?{move}:{})});if(session?.account.id!==actor||projectId!==scope||view!=='Backlog'||epoch!==backlogEpoch)return;backlogOrder=result;backlogPending=false;await workspace().catch(error=>{if(!error.staleInteraction)throw error;});}
  catch(error){if(session?.account.id===actor&&status.isConnected){status.textContent=error.message||'Backlog changed; refresh before retrying.';status.classList.add('error');}}
  finally{if(backlogPending===pending)backlogPending=false;}
}
async function backlogMoveTo(ticket,parentId,keys){
  if(!backlogCanMove(ticket)){notice('Only unlocked Todo work can change parent.');return;}
  const parent=tickets.find(row=>row.id===parentId);if(parentId&&!parent){notice('Parent is unavailable. Refresh this backlog before moving.');return;}
  await commitBacklog(keys,{id:ticket.id,expectedVersion:ticket.version,parentId,expectedParentVersion:parent?.version||0});
}
function backlogMoveForm(id){
  const ticket=tickets.find(row=>row.id===id);if(!backlogCanMove(ticket))return;
  const parentKind=ticket.kind==='task'?'epic':ticket.kind==='epic'?'goal':null;if(!parentKind)return;
  const parents=tickets.filter(row=>row.projectId===ticket.projectId&&row.kind===parentKind&&!row.archived&&!row.deleted),dialog=openDialog('Change parent',`<label>${parentKind==='epic'?'Epic':'Goal'}<select id="backlog-parent">${parents.map(row=>`<option value="${row.id}" ${row.id===ticket.parentId?'selected':''}>#${row.id} · ${escapeHTML(row.title)}</option>`).join('')}</select></label>`,'Move');
  byId('editor-form').onsubmit=async event=>{event.preventDefault();const parentId=Number(byId('backlog-parent').value);dialog.close();await backlogMoveTo(ticket,parentId,backlogRows(tickets,ideas,backlogOrder.keys).map(row=>row.key));};
}
async function mountBacklog(focus){
  leaveBacklog();if(view!=='Backlog')return;const epoch=backlogEpoch,actor=session.account.id,scope=projectId,host=byId('backlog-list');backlogLoading=true;backlogLoaded=false;
  on('backlog-import','click',backlogImportForm);on('backlog-kind','change',event=>{backlogKind=event.target.value;host.innerHTML=backlogContent();});on('backlog-query','input',event=>{backlogQuery=event.target.value;host.innerHTML=backlogContent();});
  function current(){return epoch===backlogEpoch&&session?.account.id===actor&&projectId===scope&&view==='Backlog'&&host.isConnected;}
  function allRows(){return backlogRows(tickets,ideas,backlogOrder.keys);}
  function clear(){host.querySelectorAll('.backlog-before,.backlog-after,.backlog-inside,.backlog-dragging').forEach(row=>row.classList.remove('backlog-before','backlog-after','backlog-inside','backlog-dragging'));byId('backlog-out')?.classList.remove('active');}
  function cancel(){const drag=backlogDrag;if(!drag)return;backlogDrag=null;if(drag.frame)cancelAnimationFrame(drag.frame);drag.ghost?.remove();if(drag.grip.hasPointerCapture(drag.pointer))drag.grip.releasePointerCapture(drag.pointer);clear();}
  function down(event){const grip=event.target.closest('[data-backlog-grip]');if(!grip||event.button!==0||backlogPending||backlogLoading||!backlogLoaded)return;event.preventDefault();grip.focus({preventScroll:true});const rows=allRows(),byKey=new Map(rows.map(row=>[row.key,row])),source=byKey.get(grip.dataset.backlogGrip);backlogDrag={grip,pointer:event.pointerId,x:event.clientX,y:event.clientY,source,rows,byKey,frame:0};grip.setPointerCapture(event.pointerId);}
  function scrollDrag(){const drag=backlogDrag;if(!drag)return;drag.frame=0;const velocity=drag.lastY<65?-Math.min(18,(65-drag.lastY)/3):drag.lastY>innerHeight-65?Math.min(18,(drag.lastY-innerHeight+65)/3):0;if(!velocity)return;window.scrollBy({top:velocity,behavior:'instant'});move({pointerId:drag.pointer,clientX:drag.lastX,clientY:drag.lastY});}
  function move(event){const drag=backlogDrag;if(!drag||event.pointerId!==drag.pointer)return;if(!drag.ghost&&Math.hypot(event.clientX-drag.x,event.clientY-drag.y)<6)return;
    if(!drag.ghost){drag.ghost=drag.grip.closest('.backlog-row').querySelector('.backlog-card').cloneNode(true);drag.ghost.classList.add('backlog-ghost');drag.ghost.tabIndex=-1;drag.ghost.setAttribute('aria-hidden','true');document.body.append(drag.ghost);}
    drag.ghost.style.left=event.clientX+14+'px';drag.ghost.style.top=event.clientY-24+'px';clear();drag.grip.closest('.backlog-row').classList.add('backlog-dragging');
    drag.lastX=event.clientX;drag.lastY=event.clientY;if(!drag.frame&&(event.clientY<65||event.clientY>innerHeight-65))drag.frame=requestAnimationFrame(scrollDrag);
    const under=document.elementFromPoint(event.clientX,event.clientY),out=under?.closest('#backlog-out');drag.destination=null;drag.out=false;
    if(out&&drag.source.ticket&&['task','epic'].includes(drag.source.kind)&&backlogCanMove(drag.source.ticket)){drag.out=true;out.classList.add('active');return;}
    const targetElement=under?.closest('[data-backlog-key]');if(!targetElement||!host.contains(targetElement))return;const target=drag.byKey.get(targetElement.dataset.backlogKey),rect=targetElement.getBoundingClientRect(),ratio=(event.clientY-rect.top)/rect.height,zone=ratio<.25?'before':ratio>.75?'after':'inside';
    const destination=backlogTarget(drag.source,target,zone);if(!destination||destination.parentId!==null&&!backlogCanMove(drag.source.ticket))return;drag.destination=destination;targetElement.classList.add('backlog-'+zone);
  }
  function up(event){const drag=backlogDrag;if(!drag||event.pointerId!==drag.pointer)return;const moved=!!drag.ghost,destination=drag.destination,out=drag.out;cancel();if(!moved||!current())return;const keys=drag.rows.map(row=>row.key);
    if(out){const project=projects.find(row=>row.id===drag.source.projectId),id=drag.source.ticket.kind==='task'?project.unassignedEpic:drag.source.ticket.kind==='epic'?project.unassignedGoal:0;backlogMoveTo(drag.source.ticket,id,keys);return;}
    if(!destination)return;const next=backlogReorder(keys,drag.source.key,destination.target,destination.after);if(destination.parentId!==null)backlogMoveTo(drag.source.ticket,destination.parentId,next);else commitBacklog(next);
  }
  function key(event){if(event.key==='Escape'&&backlogDrag){event.preventDefault();event.stopPropagation();cancel();return;}const grip=event.target.closest('[data-backlog-grip]');if(!grip||!event.altKey||event.ctrlKey||event.metaKey||!['ArrowUp','ArrowDown'].includes(event.key)||backlogPending)return;
    event.preventDefault();const rows=allRows(),source=rows.find(row=>row.key===grip.dataset.backlogGrip),siblings=rows.filter(row=>row.projectId===source.projectId&&row.parentId===source.parentId),index=siblings.findIndex(row=>row.key===source.key),target=siblings[index+(event.key==='ArrowDown'?1:-1)];if(target)commitBacklog(backlogReorder(rows.map(row=>row.key),source.key,target.key,event.key==='ArrowDown'));
  }
  function click(event){const open=event.target.closest('[data-backlog-open]'),fold=event.target.closest('[data-backlog-fold]'),move=event.target.closest('[data-backlog-move]');if(backlogPending)return;if(open){const key=open.dataset.backlogOpen;if(key.startsWith('ticket:'))editTicket(Number(key.slice(7)));else openIdea(key);}else if(fold){const key=fold.dataset.backlogFold;if(backlogCollapsed.has(key))backlogCollapsed.delete(key);else backlogCollapsed.add(key);host.innerHTML=backlogContent();host.querySelector(`[data-backlog-fold="${CSS.escape(key)}"]`)?.focus();}else if(move)backlogMoveForm(Number(move.dataset.backlogMove));}
  host.addEventListener('click',click);host.addEventListener('pointerdown',down);host.addEventListener('pointermove',move);host.addEventListener('pointerup',up);host.addEventListener('pointercancel',cancel);document.addEventListener('keydown',key,true);window.addEventListener('blur',cancel);
  backlogDispose=()=>{cancel();host.removeEventListener('click',click);host.removeEventListener('pointerdown',down);host.removeEventListener('pointermove',move);host.removeEventListener('pointerup',up);host.removeEventListener('pointercancel',cancel);document.removeEventListener('keydown',key,true);window.removeEventListener('blur',cancel);};
  try{const [order,model]=await Promise.all([api('getBacklogOrder',{projectId:scope}),import('/graph_2d_model.js')]);if(!current())return;backlogOrder=order;backlogLoaded=true;backlogIcons=model.graphNodeIcons;host.innerHTML=backlogContent();byId('backlog-status').textContent='';if(focus&&document.activeElement===document.body)host.querySelector(`[data-backlog-${focus.kind}="${CSS.escape(focus.key)}"]`)?.focus({preventScroll:true});}
  catch(error){if(current())byId('backlog-status').textContent=error.message||'Saved order unavailable. Refresh before reordering.';}
  finally{if(current())backlogLoading=false;}
}
