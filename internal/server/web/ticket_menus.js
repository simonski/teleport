"use strict";
let ticketMenu=null,ticketMenuEpoch=0,ticketMenuPending=false;
function ticketMenuBusy(){return !!ticketMenu||ticketMenuPending;}
function dismissTicketMenu(restore=false){
  ticketMenuEpoch++;ticketMenuPending=false;
  const previous=ticketMenu;ticketMenu=null;previous?.element.remove();
  if(restore&&previous?.source.isConnected)previous.source.focus({preventScroll:true});
}
function menuForeground(text=true){return document.querySelector('tk-dialog[open],.account-menu:not([hidden]),.choice-list')||text&&document.activeElement?.closest('input,textarea,select,[contenteditable]:not([contenteditable="false"])');}
function menuTicketSource(target){
  if(view==='Backlog'){
    const source=target.closest?.('[data-backlog-open]');if(!source)return null;
    const key=source.dataset.backlogOpen;return key.startsWith('ticket:')?{id:Number(key.slice(7)),source}:{ideaId:key,source};
  }
  if(!['Board','Todos','Graph'].includes(view))return null;
  const card=target.closest?.('.board-workspace [data-ticket-card]');
  if(card)return {id:Number(card.dataset.ticketCard),source:card.querySelector('.ticket-open')};
  const entry=target.closest?.('#graph-list [data-node]'),node=graphData?.nodes.find(row=>row.id===entry?.dataset.node);
  return node?.ticketId?{id:node.ticketId,source:entry}:null;
}
function deleteBacklogRecord(record,idea=false){
  const title=idea?'Delete idea':'Delete ticket',dialog=openDialog(title,`<p>${escapeHTML(record.title)}</p><p>This hides the selected record and retains its history.</p>`,title),completion=captureDialogCompletion(dialog);let pending=false;
  byId('editor-form').onsubmit=async event=>{event.preventDefault();if(pending)return;pending=true;try{await api(idea?'setIdeaVisibility':'setTicketVisibility',{id:record.id,expectedVersion:record.version,deleted:true});if(!completion.current())return;dialog.close();await workspace();if(completion.follow())notice(title+' saved');}catch(error){if(completion.current())showError(error);}finally{pending=false;}};
}
function showBacklogIdeaMenu(id,source,point=null){
  if(!session||!source?.isConnected||menuForeground())return;
  const idea=ideas.find(item=>item.id===id);if(!idea)return;
  dismissTicketMenu();const element=document.createElement('div');element.className='ticket-context-menu';element.setAttribute('role','menu');element.setAttribute('aria-label','Idea actions');
  const canDraft=ideaCanWrite(idea)&&!idea.archived&&!idea.deleted;
  element.innerHTML=`<div class="ticket-context-title">${escapeHTML(idea.id)} · Idea</div><button type="button" role="menuitem" data-idea-menu="open">Open idea</button>${canDraft?'<button type="button" role="menuitem" data-idea-menu="propose">Draft work proposal</button>':''}<button type="button" role="menuitem" disabled>Refine</button><button type="button" role="menuitem" disabled>Breakdown</button><button type="button" role="menuitem" disabled>Spellcheck</button><button type="button" role="menuitem" disabled>Assign to epic</button><button type="button" role="menuitem" data-idea-menu="delete" ${ideaCanControl(idea)&&!idea.deleted?'':'disabled'}>Delete</button><p class="ticket-context-note">Agent actions are not available yet. Ideas can link work through a proposal.</p>`;
  document.body.append(element);ticketMenu={element,source,anchor:source.getBoundingClientRect(),width:innerWidth,height:innerHeight};
  const anchor=source.getBoundingClientRect();element.style.left=Math.max(8,Math.min(point?.x??anchor.left,innerWidth-element.offsetWidth-8))+'px';element.style.top=Math.max(8,Math.min(point?.y??anchor.bottom,innerHeight-element.offsetHeight-8))+'px';
  const buttons=[...element.querySelectorAll('button:not(:disabled)')];buttons[0].focus({preventScroll:true});
  element.addEventListener('keydown',event=>{if(['ArrowDown','ArrowUp','Home','End'].includes(event.key)){event.preventDefault();const index=buttons.indexOf(document.activeElement),next=event.key==='Home'?0:event.key==='End'?buttons.length-1:(index+(event.key==='ArrowDown'?1:-1)+buttons.length)%buttons.length;buttons[next].focus();}else if(event.key==='Tab')dismissTicketMenu(true);});
  element.addEventListener('click',event=>{const action=event.target.closest('[data-idea-menu]')?.dataset.ideaMenu;if(!action)return;dismissTicketMenu(true);if(action==='open')openIdea(id);else if(action==='propose')ideaProposalForm(idea);else if(action==='delete')deleteBacklogRecord(idea,true);});
}
function attachedRunningSession(ticket,rows){return rows.find(row=>row.ticketId===ticket.id&&row.projectId===ticket.projectId&&row.state==='running'&&ticket.execution==='local:'+row.id);}
function ticketMenuEntries(ticket,rows,nativePending=false,projectAgents=[],projectUsages=[],hasProposals=false){
  if(view==='Todos'&&!ticket.agent){
    const canWrite=typeof roleAllowsWrite==='function'&&roleAllowsWrite(ticket.projectId),moves=canWrite?humanMoves(ticket):[],roles=['Todo','Doing','Done'],at=roles.indexOf(ticket.status);
    const relative=offset=>moves.find(move=>roles.indexOf(move.role)===at+offset);
    const ahead=relative(1),behind=relative(-1);
    return [
      {id:'ahead',label:'Move to '+(roles[at+1]||'next lane'),target:ahead?.target,disabled:!ahead},
      {id:'behind',label:'Move to '+(roles[at-1]||'previous lane'),target:behind?.target,disabled:!behind},
      {id:'status',label:'Status',submenu:true,disabled:!moves.length},
      {id:'priority',label:'Priority',submenu:true,disabled:!canWrite},
      {id:'projects',label:'Projects',submenu:true,disabled:!roleAllowsMaintain(ticket.projectId)},
      {id:'prompt',label:'Show Prompt'},
      {id:'agents',label:'Agents',submenu:true},
      ...(ticket.proposalAttention?[{id:'proposals',label:'Review proposals'}]:hasProposals?[{id:'proposals',label:'Proposal history'}]:[]),
      {id:'open',label:'Open'},
      {id:'delete',label:'Delete',disabled:ticket.protected||!roleAllowsMaintain(ticket.projectId)||ticket.archived||ticket.deleted||ticket.active||!!ticket.claim||!!ticket.execution},
    ];
  }
  const entries=[{id:'open',label:'Open ticket'}],attached=attachedRunningSession(ticket,rows);
  if(ticket.agent&&view!=='Todos')entries.push({id:'prompt',label:'Prompt',sessionId:attached?.id});
  if(typeof roleAllowsWrite==='function'&&roleAllowsWrite(ticket.projectId)){
    const epicRefine=view==='Backlog'&&ticket.kind==='epic'&&ticket.agent&&!ticket.protected&&!ticket.archived&&!ticket.deleted&&!ticket.active&&!ticket.claim&&!ticket.execution&&!ticket.delivery&&!(ticket.question&&!ticket.question.resolved&&!ticket.question.obsolete);
    if((canQueueRefinement(ticket)||epicRefine)&&ticket.status==='Todo')entries.push({id:'refine',label:view==='Backlog'?'Refine':'Send to refine'});
    if(ticket.agent&&ticket.status==='Refine'&&canQueueRefinement(ticket))entries.push({id:'return-todo',label:'Send back to todo'});
  }
  if(view==='Backlog'){
    if(!entries.some(entry=>entry.id==='refine'))entries.push({id:'refine-unavailable',label:'Refine',disabled:true});
    entries.push({id:'breakdown',label:'Breakdown',disabled:true},{id:'spellcheck',label:'Spellcheck',disabled:true});
    entries.push({id:'assign-epic',label:'Assign to epic',submenu:true,disabled:ticket.kind!=='task'||!backlogCanMove(ticket)||!backlogLoaded||backlogPending});
    entries.push({id:'delete',label:'Delete',disabled:ticket.protected||!roleAllowsMaintain(ticket.projectId)||ticket.archived||ticket.deleted||ticket.active||!!ticket.claim||!!ticket.execution});
  }
  if(projectAgents.length&&typeof roleAllowsWrite==='function'&&roleAllowsWrite(ticket.projectId))entries.push({id:'agents',label:'Agents',submenu:true});
  if(ticket.question&&!ticket.question.resolved&&!ticket.question.obsolete||nativePending)entries.push({id:'questions',label:'Questions'});
  if(attached&&view!=='Todos')entries.push({id:'teleport',label:'Teleport · '+attached.id,sessionId:attached.id});
  return entries;
}
async function setTicketPriority(ticket,priority){
  try{await api('updateTicket',{id:ticket.id,expectedVersion:ticket.version,priority});await workspace();}
  catch(error){showError(error);}
}
async function showTicketMenu(id,source,point=null){
  if(!session||!source?.isConnected||menuForeground())return;
  dismissTicketMenu();const epoch=++ticketMenuEpoch,actor=session.account.id,generation=loadGeneration,detail=++detailEpoch;ticketMenuPending=true;
  const current=()=>ticketMenuEpoch===epoch&&session?.account.id===actor&&loadGeneration===generation&&detailEpoch===detail&&source.isConnected&&!menuForeground();
  try{
    const [ticket,rows]=await Promise.all([api('getTicket',{id}),view==='Todos'?Promise.resolve([]):pages('listSessions')]);
    if(!current())return;
    const [projectAgents,projectUsages,marks,proposals]=await Promise.all([roleAllowsWrite(ticket.projectId)?api('listProjectAgents',{projectId:ticket.projectId}):Promise.resolve({items:[]}),roleAllowsWrite(ticket.projectId)?api('listAgentUsages',{projectId:ticket.projectId}):Promise.resolve({items:[]}),roleAllowsWrite(ticket.projectId)&&view==='Todos'?api('getTicketSpellchecks',{id:ticket.id}):Promise.resolve({items:[]}),view==='Todos'&&!ticket.agent?api('listTicketProposals',{ticketId:ticket.id,limit:1}):Promise.resolve({items:[]})]);
    if(!current())return;
    const attached=attachedRunningSession(ticket,rows),requests=attached?.harness==='codex'?[await api('getSessionRequests',{id:attached.id})]:[];
    if(!current())return;
    const element=document.createElement('div');element.className='ticket-context-menu';element.setAttribute('role','menu');element.setAttribute('aria-label','Ticket #'+id+' actions');
    const entries=ticketMenuEntries(ticket,rows,requests.some(group=>(group.items||[]).some(request=>request.state==='pending')),projectAgents.items,projectUsages.items,proposals.items.length>0);
    const epics=view==='Backlog'?tickets.filter(epic=>epic.kind==='epic'&&epic.projectId===ticket.projectId&&!epic.archived&&!epic.deleted).sort((a,b)=>a.title.localeCompare(b.title)||a.id-b.id):[];
    const moves=view==='Todos'?humanMoves(ticket):[];
    const destinations=projects.filter(project=>project.id!==ticket.projectId&&roleAllowsMaintain(project.id)).sort((a,b)=>a.name.localeCompare(b.name));
    function submenuContents(entry){
      if(entry.id==='assign-epic')return epics.map(epic=>`<button type="button" role="menuitem" data-ticket-epic="${epic.id}" ${epic.id===ticket.parentId?'disabled':''}>${escapeHTML(epic.title)}${epic.id===ticket.parentId?' · current':''}</button>`).join('');
      if(entry.id==='projects')return destinations.map(project=>`<button type="button" role="menuitem" data-ticket-project="${project.id}">${escapeHTML(project.name)}</button>`).join('')||'<p class="ticket-context-note">No other projects available to transfer.</p>';
      if(entry.id==='status')return ['Todo','Doing','Done'].map(role=>{const move=moves.find(item=>item.role===role);return `<button type="button" role="menuitem" data-ticket-status="${escapeHTML(move?.target||'')}" ${!move?'disabled':''}>${role}${role===ticket.status?' · current':''}</button>`;}).join('');
      if(entry.id==='agents')return projectUsages.items.filter(item=>item.enabled&&(item.view||'Todos')===view&&(!item.workflow||item.workflow===ticket.workflow&&item.phase===ticket.phase)).map(item=>{const active=projectAgents.items.some(agent=>agent.id===item.agentId),visible=ticket.kind==='task'&&(item.workflow||!ticket.agent)&&!ticket.protected&&!ticket.archived&&!ticket.deleted;if(item.kind==='todo-refine')return `<button type="button" role="menuitem" data-refine-usage="${escapeHTML(item.id)}" ${visible&&!ticket.agent&&ticket.status==='Todo'&&canQueueRefinement(ticket)?'':'disabled'}>${escapeHTML(item.name)}</button>`;if(item.kind==='todo-conversation'||item.kind==='todo-proposal')return `<button type="button" role="menuitem" data-agent-usage="${escapeHTML(item.id)}" ${active&&visible?'':'disabled'}>${escapeHTML(item.name)}</button>`;if(item.kind!=='todo-spellcheck')return '';const mark=marks.items.find(row=>row.usageId===item.id),eligible=active&&visible&&!ticket.agent&&ticket.status==='Todo'&&!ticket.active&&!ticket.claim&&!ticket.execution;return `<button type="button" role="menuitem" data-spellcheck-usage="${escapeHTML(item.id)}" ${eligible?'':'disabled'}>${mark?.current?'✓ ':''}${escapeHTML(item.name)}${mark&&!mark.current?(mark.reason==='configuration_changed'?' · checked before configuration changed':' · checked before text changed'):''}</button>`;}).join('')||'<p class="ticket-context-note">No enabled agent usages match this view and lane.</p>';
      return ['critical','high','medium','low'].map(priority=>`<button type="button" role="menuitem" data-ticket-priority="${priority}" ${priority===ticket.priority?'disabled':''}>${priority[0].toUpperCase()+priority.slice(1)}${priority===ticket.priority?' · current':''}</button>`).join('');
    }
    element.innerHTML=`<div class="ticket-context-title">#${id} · ${escapeHTML(phaseTitle(ticket))}</div>${entries.map(entry=>entry.submenu?`<button type="button" role="menuitem" data-ticket-menu="${entry.id}" aria-haspopup="menu" aria-expanded="false" ${entry.disabled||entry.id==='assign-epic'&&!epics.length?'disabled':''}>${escapeHTML(entry.label)} <span aria-hidden="true">›</span></button><div class="ticket-context-submenu" role="menu" aria-label="${escapeHTML(entry.label)}" hidden>${submenuContents(entry)}</div>`:`<button type="button" role="menuitem" data-ticket-menu="${entry.id}" ${entry.disabled?'disabled':''}>${escapeHTML(entry.label)}</button>`).join('')}${view==='Backlog'?'<p class="ticket-context-note">Breakdown and spellcheck are not available yet.</p>':''}`;
    document.body.append(element);ticketMenu={element,source,anchor:source.getBoundingClientRect(),width:innerWidth,height:innerHeight};ticketMenuPending=false;
    const anchor=source.getBoundingClientRect(),x=point?.x??anchor.left,y=point?.y??anchor.bottom;
    element.style.left=Math.max(8,Math.min(x,innerWidth-element.offsetWidth-8))+'px';element.style.top=Math.max(8,Math.min(y,innerHeight-element.offsetHeight-8))+'px';
    const buttons=[...element.querySelectorAll(':scope > button:not(:disabled)')];let submenu=null,trigger=null;buttons[0].focus({preventScroll:true});
    function openSubmenu(button,focus=false){const next=button?.nextElementSibling;if(!next?.classList.contains('ticket-context-submenu')||button.disabled)return;if(submenu&&submenu!==next){submenu.hidden=true;trigger.setAttribute('aria-expanded','false');}submenu=next;trigger=button;submenu.hidden=false;trigger.setAttribute('aria-expanded','true');const box=trigger.getBoundingClientRect();submenu.style.left=(box.right+submenu.offsetWidth+8>innerWidth?Math.max(8,box.left-submenu.offsetWidth):box.right)+'px';submenu.style.top=Math.max(8,Math.min(box.top,innerHeight-submenu.offsetHeight-8))+'px';if(focus)submenu.querySelector('button:not(:disabled)')?.focus();}
    element.querySelectorAll('[aria-haspopup=menu]').forEach(button=>button.addEventListener('pointerenter',()=>openSubmenu(button)));
    element.addEventListener('keydown',e=>{
      const subButtons=submenu&&!submenu.hidden?[...submenu.querySelectorAll('button:not(:disabled)')]:[],inSub=submenu?.contains(document.activeElement),items=inSub?subButtons:buttons;
      if(e.key==='ArrowRight'&&document.activeElement?.getAttribute('aria-haspopup')==='menu'){e.preventDefault();openSubmenu(document.activeElement,true);}
      else if(e.key==='ArrowLeft'&&inSub){e.preventDefault();submenu.hidden=true;trigger.setAttribute('aria-expanded','false');trigger.focus();}
      else if(['ArrowDown','ArrowUp','Home','End'].includes(e.key)&&items.length){e.preventDefault();const index=items.indexOf(document.activeElement),next=e.key==='Home'?0:e.key==='End'?items.length-1:(index+(e.key==='ArrowDown'?1:-1)+items.length)%items.length;items[next].focus();}
      else if(e.key==='Tab')dismissTicketMenu(true);
    });
    element.addEventListener('click',e=>{const refine=e.target.closest('[data-refine-usage]');if(!refine||refine.disabled)return;const usage=projectUsages.items.find(item=>item.id===refine.dataset.refineUsage);dismissTicketMenu(true);api('requestRefinement',{id:ticket.id,expectedVersion:ticket.version,usageId:usage.id,usageVersion:usage.version}).then(()=>workspace()).catch(showError);});
    element.addEventListener('click',e=>{const button=e.target.closest('[data-ticket-menu="proposals"]');if(!button)return;e.stopImmediatePropagation();dismissTicketMenu(true);openTicketProposalReview(ticket.id).catch(showError);});
    element.addEventListener('click',e=>{const button=e.target.closest('[data-ticket-project]');if(!button)return;e.stopImmediatePropagation();const destination=destinations.find(project=>project.id===Number(button.dataset.ticketProject));dismissTicketMenu(true);if(destination)confirmTicketProjectTransfer(ticket,destination).catch(showError);});
    element.addEventListener('click',e=>{const epic=e.target.closest('[data-ticket-epic]'),status=e.target.closest('[data-ticket-status]'),priority=e.target.closest('[data-ticket-priority]'),persona=e.target.closest('[data-ticket-persona]'),spell=e.target.closest('[data-spellcheck-usage]'),action=e.target.closest('[data-agent-usage]');if(action&&!action.disabled){const usage=projectUsages.items.find(item=>item.id===action.dataset.agentUsage),definition=projectAgents.items.find(item=>item.id===usage?.agentId);dismissTicketMenu(true);startAgentConversation(definition,ticket.projectId,ticket.id,usage).catch(showError);return;}if(spell&&!spell.disabled){const usage=projectUsages.items.find(item=>item.id===spell.dataset.spellcheckUsage);dismissTicketMenu(true);startTicketSpellcheck(ticket,usage).catch(showError);return;}if(epic){const parentId=Number(epic.dataset.ticketEpic);dismissTicketMenu(true);backlogMoveTo(ticket,parentId,backlogRows(tickets,ideas,backlogOrder.keys).map(row=>row.key));return;}if(status&&!status.disabled){const target=status.dataset.ticketStatus;dismissTicketMenu(true);moveHumanTicket(ticket,target);return;}if(priority&&!priority.disabled){const value=priority.dataset.ticketPriority;dismissTicketMenu(true);setTicketPriority(ticket,value);return;}if(persona){const definition=projectAgents.items.find(item=>item.id===persona.dataset.ticketPersona);dismissTicketMenu(true);startAgentConversation(definition,ticket.projectId,ticket.id).catch(showError);return;}const button=e.target.closest('[data-ticket-menu]'),entry=entries.find(item=>item.id===button?.dataset.ticketMenu);if(!entry||entry.disabled)return;if(entry.submenu){openSubmenu(button,true);return;}dismissTicketMenu(true);if(entry.target)moveHumanTicket(ticket,entry.target);else if(entry.id==='refine'||entry.id==='return-todo')changeRefinementQueue(ticket,entry.id==='refine'?'requestRefinement':'returnRefinementToTodo');else if(entry.id==='delete')deleteBacklogRecord(ticket);else if(entry.id==='teleport')teleportTicketSession(id,entry.sessionId).catch(showError);else if(entry.id==='prompt')showTicketPrompt(id,entry.sessionId).catch(showError);else editTicket(id);});
  }catch(error){if(current())showError(error);}
  finally{if(ticketMenuEpoch===epoch)ticketMenuPending=false;}
}
async function confirmTicketProjectTransfer(ticket,destination){
  const actor=session?.account.id,epoch=++detailEpoch,preview=await api('previewTicketTransfer',{id:ticket.id,destinationProjectId:destination.id});
  if(session?.account.id!==actor||detailEpoch!==epoch||foregroundOpen())return;
  const scope=preview.includesParent?`Move the parent <strong>${escapeHTML(preview.rootTitle)}</strong> and all its stories, including this one?`:`Move <strong>${escapeHTML(ticket.title)}</strong> to ${escapeHTML(destination.name)}?`;
  const dialog=openDialog('Move to '+escapeHTML(destination.name),`<p>${scope}</p><p>${preview.stories} ${preview.stories===1?'story':'stories'} and ${preview.total-preview.stories} parent ${preview.total-preview.stories===1?'ticket':'tickets'} will move${preview.hidden?`, including ${preview.hidden} archived or deleted`:''}.</p><p>Project access changes to the destination's existing members. No members are added.</p><p id="transfer-status" role="status"></p>`,'Move all'),guard=captureDialogCompletion(dialog);let busy=false;
  byId('editor-form').onsubmit=async event=>{event.preventDefault();if(busy)return;busy=true;try{await api('transferTicket',{id:preview.rootId,expectedVersion:preview.rootVersion,expectedRevision:preview.revision,destinationProjectId:destination.id});if(!guard.current())return;dialog.close();await workspace();notice('Moved to '+destination.name);}catch(error){if(guard.current())byId('transfer-status').textContent=error.message;}finally{busy=false;}};
}
async function openTicketProposalReview(ticketId,replaceDialog=null){
  const form=replaceDialog?.querySelector('form'),actor=session?.account.id,epoch=++detailEpoch,[ticket,rows]=await Promise.all([api('getTicket',{id:ticketId}),pages('listTicketProposals',{ticketId})]);
  if(actor!==session?.account.id||epoch!==detailEpoch)return;
  if(replaceDialog){if(!replaceDialog.open||replaceDialog.querySelector('form')!==form)return;replaceDialog.close();}
  else if(foregroundOpen())return;
  const latest=rows[rows.length-1],editable=roleAllowsWrite(ticket.projectId),discardable=rows.filter(proposal=>!proposal.acceptedBy&&!proposal.discardedBy);
  rows.forEach(proposal=>proposal.feedback.forEach(note=>{if(note.itemKey)note.text='['+(proposal.items.find(item=>item.key===note.itemKey)?.title||note.itemKey)+'] '+note.text;}));
  const dialog=closeOnlyDialog('Proposals · #'+ticketId,`${editable&&discardable.length?`<button type="button" class="secondary" id="discard-proposals">Discard proposals</button><p class="form-help">Start over with a fresh agent run.</p>`:''}${rows.length?rows.map((proposal,index)=>`<article class="idea-proposal"><h3>${escapeHTML(proposal.title)} · round ${index+1}</h3><p>Source v${proposal.ticketVersion}${proposal.discardedBy?' · Discarded':proposal.acceptedBy?' · Accepted':proposal.ticketVersion!==ticket.version?' · Source changed':''}</p><ol class="proposal-targets">${proposal.items.map(item=>`<li><strong>${escapeHTML(item.kind)}: ${escapeHTML(item.title)}</strong>${item.parentKey?`<span>Under ${escapeHTML(proposal.items.find(parent=>parent.key===item.parentKey)?.title||item.parentKey)}</span>`:''}<p>${escapeHTML(item.description||'')}</p><small>${escapeHTML(item.criteria||'')}</small></li>`).join('')}</ol>${proposal.feedback.map(note=>`<p><strong>${escapeHTML(note.actor)}:</strong> ${escapeHTML(note.text)}</p>`).join('')}${(proposal.createdIds||[]).map(id=>`<button type="button" class="secondary" data-proposal-ticket="${id}">Ticket #${id}</button>`).join('')}${editable&&proposal.id===latest.id&&!proposal.acceptedBy&&!proposal.discardedBy?`<button type="button" class="secondary" data-proposal-feedback="${escapeHTML(proposal.id)}">Add feedback</button> <button type="button" data-proposal-accept="${escapeHTML(proposal.id)}" ${proposal.ticketVersion!==ticket.version?'disabled':''}>Review and approve</button>`:''}</article>`).join(''):'<p>No proposal has been submitted for this ticket.</p>'}`);
  dialog.classList.add('proposal-review-dialog');dialog.addEventListener('close',()=>dialog.classList.remove('proposal-review-dialog'),{once:true});
  if(rows.length){const filter=document.createElement('label');filter.textContent='Find proposed work';const input=document.createElement('input');input.type='search';input.placeholder='Search titles, descriptions and criteria';filter.append(input);dialog.querySelector('.idea-proposal').before(filter);input.addEventListener('input',()=>{const query=input.value.trim().toLocaleLowerCase();dialog.querySelectorAll('.proposal-targets > li').forEach(item=>{item.hidden=!!query&&!item.textContent.toLocaleLowerCase().includes(query);});});}
  rows.forEach((proposal,index)=>{const article=dialog.querySelectorAll('.idea-proposal')[index];if(!proposal.ready&&!proposal.acceptedBy){const note=document.createElement('p');note.textContent='Agent run has not completed successfully; approval is unavailable.';article.append(note);article.querySelectorAll('[data-proposal-accept],[data-proposal-feedback]').forEach(button=>button.disabled=true);}});
  rows.forEach((proposal,index)=>{dialog.querySelectorAll('.idea-proposal')[index].querySelectorAll('.proposal-targets > li').forEach((item,i)=>{if(proposal.items[i].parentKey)item.classList.add('proposal-child');});});
  rows.forEach((proposal,index)=>{const previous=rows.find(row=>row.id===proposal.previousId);if(!previous)return;const article=dialog.querySelectorAll('.idea-proposal')[index],prior=new Map(previous.items.map(item=>[item.key,item])),current=new Set(proposal.items.map(item=>item.key));article.querySelectorAll('.proposal-targets > li').forEach((element,i)=>{const item=proposal.items[i],old=prior.get(item.key),changed=old&&['kind','parentKey','title','description','criteria'].some(field=>(old[field]||'')!==(item[field]||''));if(!old||changed){const label=document.createElement('span');label.textContent=old?'Changed':'Added';element.prepend(label);if(old){const detail=document.createElement('details'),summary=document.createElement('summary'),body=document.createElement('pre');summary.textContent='Previous version';body.textContent=[old.title,old.description||'',old.criteria||''].filter(Boolean).join('\n\n');detail.append(summary,body);element.append(detail);}}});const removed=previous.items.filter(item=>!current.has(item.key));if(removed.length){const note=document.createElement('p');note.textContent='Removed since previous round: '+removed.map(item=>item.title).join(', ');article.append(note);}});
  if(editable&&latest?.ready&&!latest.acceptedBy&&!latest.discardedBy){const article=dialog.querySelectorAll('.idea-proposal')[rows.length-1];article.querySelectorAll('.proposal-targets > li').forEach((element,index)=>{const button=document.createElement('button');button.type='button';button.className='secondary';button.textContent='Comment';button.onclick=()=>{dialog.close();ticketProposalFeedback(ticket,latest,latest.items[index].key);};element.append(button);});}
  if(editable&&latest?.ready&&!latest.acceptedBy&&!latest.discardedBy&&latest.usageId&&latest.ticketVersion===ticket.version){const button=document.createElement('button');button.type='button';button.className='secondary';button.textContent='Revise with feedback';dialog.querySelectorAll('.idea-proposal')[rows.length-1].append(button);button.onclick=async()=>{try{const [usages,agents]=await Promise.all([api('listAgentUsages',{projectId:ticket.projectId}),api('listProjectAgents',{projectId:ticket.projectId})]);const usage=usages.items.find(item=>item.id===latest.usageId&&item.enabled&&item.kind==='todo-proposal'),agent=agents.items.find(item=>item.id===usage?.agentId);if(!usage||!agent)throw new Error('The proposal agent usage is unavailable; check project settings.');dialog.close();await startAgentConversation(agent,ticket.projectId,ticket.id,usage,latest.id);}catch(error){showError(error);}};}
  dialog.querySelectorAll('[data-proposal-ticket]').forEach(button=>button.onclick=()=>{dialog.close();editTicket(Number(button.dataset.proposalTicket));});
  dialog.querySelectorAll('[data-proposal-feedback]').forEach(button=>button.onclick=()=>{const proposal=rows.find(row=>row.id===button.dataset.proposalFeedback);dialog.close();ticketProposalFeedback(ticket,proposal);});
  dialog.querySelectorAll('[data-proposal-accept]').forEach(button=>button.onclick=()=>{const proposal=rows.find(row=>row.id===button.dataset.proposalAccept);dialog.close();ticketProposalAcceptance(ticket,proposal);});
  if(rows.length>1){const articles=dialog.querySelectorAll('.idea-proposal');articles[0].before(articles[articles.length-1]);}
  if(editable&&discardable.length)on('discard-proposals','click',()=>{dialog.close();confirmDiscardTicketProposals(ticket,latest,discardable.length);});
  const archived=rows.filter(proposal=>proposal.discardedBy);if(archived.length){const details=document.createElement('details'),summary=document.createElement('summary');summary.textContent='Discarded proposals ('+archived.length+')';details.append(summary);const articles=[...dialog.querySelectorAll('.idea-proposal')];for(const article of articles){const heading=article.querySelector('h3')?.textContent,proposal=rows.find((p,index)=>heading===p.title+' · round '+(index+1));if(proposal?.discardedBy)details.append(article);}dialog.querySelector('form').insertBefore(details,dialog.querySelector('.dialog-actions'));}
}
function confirmDiscardTicketProposals(ticket,latest,count){
  const dialog=openDialog('Discard proposals',`<p>Discard ${count} unaccepted proposal${count===1?'':'s'} and start the next agent run fresh? Discarded proposals and feedback remain in history. Accepted work and created tickets are retained.</p><p id="proposal-status" role="status"></p>`,'Discard proposals'),guard=captureDialogCompletion(dialog),form=byId('editor-form'),requestId=crypto.randomUUID();let busy=false;
  form.onsubmit=async event=>{event.preventDefault();if(busy)return;busy=true;try{await api('discardTicketProposals',{ticketId:ticket.id,expectedTicketVersion:ticket.version,latestId:latest.id,latestVersion:latest.version,requestId});if(!guard.current())return;dialog.close();await workspace();await openTicketProposalReview(ticket.id);}catch(error){if(guard.current())byId('proposal-status').textContent=error.message;}finally{busy=false;}};
}
function ticketProposalFeedback(ticket,proposal,itemKey=''){
  const dialog=openDialog('Proposal feedback',`<p>Feedback is saved with this proposal for the next revision.</p><label>Feedback<textarea name="feedback" required maxlength="20000" rows="6"></textarea></label><p id="proposal-status" role="status"></p>`,'Save feedback'),guard=captureDialogCompletion(dialog),form=byId('editor-form'),requestId=crypto.randomUUID();let busy=false;
  form.onsubmit=async event=>{event.preventDefault();if(busy)return;busy=true;try{await api('commentTicketProposal',{ticketId:ticket.id,proposalId:proposal.id,expectedVersion:proposal.version,requestId,text:form.elements.feedback.value,...(itemKey?{itemKey}:{})});if(!guard.current())return;dialog.close();await openTicketProposalReview(ticket.id);}catch(error){if(guard.current())byId('proposal-status').textContent=error.message;}finally{busy=false;}};
}
function ticketProposalAcceptance(ticket,proposal){
  const dialog=openDialog('Approve proposed work',`<p>Create ${proposal.items.length} Todo tickets in this project from this exact proposal? The source ticket remains unchanged.</p><label class="checkbox-label"><input type="checkbox" required>Approve this proposal</label><p id="proposal-status" role="status"></p>`,'Create tickets'),guard=captureDialogCompletion(dialog),form=byId('editor-form');let busy=false;
  form.onsubmit=async event=>{event.preventDefault();if(busy)return;busy=true;try{await api('acceptTicketProposal',{ticketId:ticket.id,proposalId:proposal.id,expectedVersion:proposal.version,expectedTicketVersion:ticket.version});if(!guard.current())return;dialog.close();await workspace();await openTicketProposalReview(ticket.id);}catch(error){if(guard.current())byId('proposal-status').textContent=error.message;}finally{busy=false;}};
}
const spellcheckRequests=new Map();
async function startTicketSpellcheck(ticket,usage){
  const actor=session.account.id,key=[actor,ticket.id,ticket.version,usage.id,usage.version].join(':'),requestId=spellcheckRequests.get(key)||crypto.randomUUID();spellcheckRequests.set(key,requestId);
  let run;try{run=await api('startSpellcheck',{ticketId:ticket.id,expectedVersion:ticket.version,usageId:usage.id,usageVersion:usage.version,requestId});spellcheckRequests.delete(key);}catch(error){if(error.status&&error.status<500)spellcheckRequests.delete(key);throw error;}
  notice(usage.name+' started');
  async function poll(){if(session?.account.id!==actor)return;try{const status=await api('getSpellcheckRun',{id:run.id});if(session?.account.id!==actor)return;if(status.state==='running'){setTimeout(poll,1000);return;}notice(status.state==='completed'?usage.name+' completed':usage.name+' failed: '+(status.error||'inspect session'));if(status.state==='completed'){const fresh=await api('getTicket',{id:ticket.id});if(session?.account.id!==actor)return;tickets=tickets.map(row=>row.id===fresh.id?fresh:row);const title=document.querySelector(`.ticket-open[data-ticket="${fresh.id}"] h3`);if(title)title.textContent=fresh.title;}if(!foregroundOpen())await workspace(true);}catch{setTimeout(poll,3000);}}
  poll();
}
async function showTicketPrompt(id,sessionId){
  const actor=session?.account.id,generation=loadGeneration,detail=++detailEpoch;
  const input={id};if(sessionId)input.sessionId=sessionId;
  let result;
  try{result=await api('getPrompt',input);}catch(error){if(!sessionId)throw error;result=await api('getPrompt',{id});}
  if(session?.account.id!==actor||loadGeneration!==generation||detailEpoch!==detail||menuForeground())return;
  const dialog=openDialog('Prompt · #'+id,`<p>${result.saved?'Saved launch · '+escapeHTML(result.sessionId):result.hypothetical?'Refinement preview · this human ticket has not been sent to an agent':'Current composition · preview'}</p><details open><summary>Complete prompt</summary><pre class="criteria-box">${escapeHTML(result.prompt.text)}</pre></details>${result.prompt.contributions.map(part=>`<details><summary>${escapeHTML(part.source)} · ${escapeHTML(part.version)}</summary><pre class="criteria-box">${escapeHTML(part.text)}</pre></details>`).join('')}`,'Close');
  byId('cancel').hidden=true;byId('editor-form').onsubmit=e=>{e.preventDefault();dialog.close();};
}
async function teleportTicketSession(id,sessionId){
  const actor=session?.account.id,generation=loadGeneration,detail=++detailEpoch;
  const [ticket,rows]=await Promise.all([api('getTicket',{id}),pages('listSessions')]);
  if(session?.account.id!==actor||loadGeneration!==generation||detailEpoch!==detail||menuForeground())return;
  if(attachedRunningSession(ticket,rows)?.id!==sessionId){notice('The attached harness is no longer running. Open the ticket to inspect its current state.');return;}
  await openTerminal(sessionId);
}
document.addEventListener('contextmenu',e=>{
  const target=menuTicketSource(e.target);if(!target||menuForeground(false))return;
  e.preventDefault();target.source.focus({preventScroll:true});if(target.ideaId)showBacklogIdeaMenu(target.ideaId,target.source,{x:e.clientX,y:e.clientY});else showTicketMenu(target.id,target.source,{x:e.clientX,y:e.clientY});
});
document.addEventListener('keydown',e=>{
  if(e.key==='Escape'&&ticketMenuBusy()){e.preventDefault();e.stopImmediatePropagation();dismissTicketMenu(true);return;}
  if(e.key==='F10'&&e.shiftKey&&!e.ctrlKey&&!e.metaKey&&!e.altKey&&!e.isComposing){const target=menuTicketSource(e.target);if(target&&!menuForeground()){e.preventDefault();if(target.ideaId)showBacklogIdeaMenu(target.ideaId,target.source);else showTicketMenu(target.id,target.source);}}
},true);
document.addEventListener('pointerdown',e=>{if(ticketMenuPending||ticketMenu&&!ticketMenu.element.contains(e.target))dismissTicketMenu();},true);
window.addEventListener('resize',()=>{if(ticketMenu&&(ticketMenu.width!==innerWidth||ticketMenu.height!==innerHeight))dismissTicketMenu();});
window.addEventListener('scroll',e=>{if(ticketMenu&&!ticketMenu.element.contains(e.target)){const rect=ticketMenu.source.getBoundingClientRect(),anchor=ticketMenu.anchor;if(Math.abs(rect.left-anchor.left)>.5||Math.abs(rect.top-anchor.top)>.5)dismissTicketMenu();}},true);
