"use strict";
let calendarEpoch = 0, calendarDirty = false;
let calendarState = {date:'',period:'default',duePeriod:'',query:''};
function calendarBusy() {return view==='Calendar'&&calendarDirty;}
function calendarView() {
  if(!calendarState.date)calendarState.date=localCalendarDate();
  return `<form id="calendar-controls" class="calendar-controls"><label>Reference date<input name="date" type="date" required value="${calendarState.date}"></label><label>Period<select name="period">${['default','week','month','quarter','year','full'].map(value=>`<option value="${value}" ${calendarState.period===value?'selected':''}>${value[0].toUpperCase()+value.slice(1)}</option>`).join('')}</select></label><label>Due date<select name="duePeriod"><option value="">Any due date</option>${['today','week','month','quarter','year'].map(value=>`<option value="${value}" ${calendarState.duePeriod===value?'selected':''}>This ${value==='today'?'day':value}</option>`).join('')}</select></label><label>Search<input name="query" type="search" value="${escapeHTML(calendarState.query)}" placeholder="All query words"></label><button>Apply calendar scope</button></form><div id="calendar-status" class="form-help" role="status">Loading dated work…</div><div id="calendar-content" class="calendar-content"></div>`;
}
function calendarDay(value){const [year,month,day]=value.split('-').map(Number);const date=new Date(0);date.setUTCFullYear(year,month-1,day);date.setUTCHours(0,0,0,0);return date.getTime()/86400000;}
function calendarSVG(row,start,end) {
  const width=900,total=end-start+1;
  let span='';
  if(row.start&&row.end&&calendarDay(row.end)>=start&&calendarDay(row.start)<=end){const left=Math.max(0,(calendarDay(row.start)-start)/total*width),right=Math.min(width,(calendarDay(row.end)-start+1)/total*width),size=Math.max(1,right-left);span=`<rect class="calendar-bar ${escapeHTML(row.kind)}" x="${left}" y="7" width="${size}" height="20" rx="3"></rect><rect class="calendar-progress" x="${left}" y="22" width="${size*row.progress/100}" height="5" rx="1"></rect>`;}
  let due='';if(row.due){const offset=calendarDay(row.due)-start;if(offset>=0&&offset<=end-start){const x=(offset+.5)/total*width;due=`<path class="calendar-due" d="M${x} 2 l7 8 l-7 8 l-7 -8z"></path>`;}}
  return `<svg viewBox="0 0 900 34" role="img" aria-label="${escapeHTML(row.start?`${row.start} through ${row.end} inclusive, ${row.progress}% delivered${row.derived?', derived parent bounds':''}`:'No scheduled span')}"><line class="calendar-baseline" x1="0" x2="900" y1="32" y2="32"></line>${span}${due}</svg>`;
}
async function mountCalendar() {
  const epoch=++calendarEpoch,account=session.account.id,scope=projectId;
  on('calendar-controls','input',()=>calendarDirty=true);
  on('calendar-controls','change',()=>calendarDirty=true);
  on('calendar-controls','submit',e=>{e.preventDefault();calendarState=Object.fromEntries(new FormData(e.currentTarget));calendarDirty=false;render();});
  try{const data=await api('getCalendar',{projectId,...calendarState});if(epoch!==calendarEpoch||view!=='Calendar'||session?.account.id!==account||scope!==projectId)return;
    const start=calendarDay(data.start),end=calendarDay(data.end),days=end-start+1;
    const ticks=[];const interval=days<=31?1:days<=100?7:days<=400?30:Math.ceil(days/12);for(let day=0;day<days;day+=interval){const date=new Date((start+day)*86400000);ticks.push(`<text x="${day/days*900+3}" y="23">${escapeHTML(date.toLocaleDateString(undefined,{timeZone:'UTC',day:'numeric',month:'short'}))}</text>`);}
    byId('calendar-status').textContent=`${data.start} → ${data.end} · inclusive calendar dates · ${data.rows.length} rows · ${data.unscheduled} unscheduled entities`;
    byId('calendar-content').innerHTML=`<div class="calendar-sheet"><div class="calendar-ruler"><span>Goal / Epic / Task</span><svg viewBox="0 0 900 40" aria-hidden="true">${ticks.join('')}</svg></div>${data.rows.map(row=>`<button class="calendar-row ${row.context?'context':''}" data-calendar-ticket="${row.ticketId}"><span class="calendar-name ${escapeHTML(row.kind)}"><strong>#${row.ticketId} ${escapeHTML(row.title)}</strong><small>${escapeHTML(projectName(row.projectId))} · ${escapeHTML(row.kind)} · ${row.progress}% delivered${row.derived?' · derived bounds':''}${row.context?' · context':''}</small></span>${calendarSVG(row,start,end)}</button>`).join('')||'<div class="empty"><h2>No dated work in this scope.</h2><p>Set Start/ETA/Due on existing tickets. Unscheduled work stays in its ordinary views.</p></div>'}</div>`;
    byId('calendar-content').querySelectorAll('[data-calendar-ticket]').forEach(button=>button.onclick=()=>editTicket(Number(button.dataset.calendarTicket)));
  }catch(error){if(epoch===calendarEpoch&&view==='Calendar')byId('calendar-status').textContent=error.message;}
}

function finderQuery(value){
  const text=value.trim(),match=/^(goal|epic|task|story|ticket|action|actions)\s*:\s*(.*)$/i.exec(text);
  if(!match)return {query:text,kind:'',actions:true};
  const prefix=match[1].toLowerCase(),kind=({story:'task',ticket:'',action:'',actions:''})[prefix]??prefix;
  return {query:match[2].trim(),kind,actions:prefix==='action'||prefix==='actions',actionOnly:prefix==='action'||prefix==='actions'};
}
function finderActionRank(action,query){
  const tokens=query.toLocaleLowerCase().split(/\s+/u).filter(Boolean),label=action.title.toLocaleLowerCase(),haystack=(action.title+' '+action.keywords).toLocaleLowerCase();
  if(!tokens.length||!tokens.every(token=>haystack.includes(token)))return 0;
  const term=query.toLocaleLowerCase(),position=label.indexOf(term);
  let score=label===term?9000:position===0?7000:position>=0?5000+Math.max(0,200-position):tokens.every(token=>label.includes(token))?3000:1000;
  if(action.title.includes(query))score+=30;
  return score;
}
function finderActions(query){
  const actions=[
    {title:'Create ticket',keywords:'new task story work add',run:()=>newTicketForm('task')},
    {title:'Create epic',keywords:'new epic ticket work add',run:()=>newTicketForm('epic')},
    {title:'Create goal',keywords:'new goal ticket work add',run:()=>newTicketForm('goal')},
    {title:'Create project',keywords:'new project add',run:()=>projectForm()},
    {title:'Browse projects',keywords:'project directory manage list',run:()=>openProjectDirectory()},
    {title:'Harness sessions',keywords:'agent terminal session inspect list manage',run:()=>sessionSwitcher()},
    {title:'Start or attach default harness',keywords:'agent codex claude copilot terminal session run',run:()=>chooseHarness()},
    {title:'Open tk console',keywords:'command terminal cli',run:()=>openScriptConsole()},
    {title:'Settings',keywords:'account appearance harness preferences',run:()=>openPersonalSettings()},
    {title:'System settings',keywords:'execution agent harness mode',run:()=>openSystemSettings()},
    ...(projectId?[{title:'Project settings',keywords:'project harness execution agent configuration',run:()=>openProjectSettings()}]:[]),
    ...supportedNavigation.map(name=>({title:'Open '+name,keywords:name+' view navigate',run:()=>navigateView(name)})),
  ];
  return actions.map(action=>({...action,score:finderActionRank(action,query)})).filter(action=>action.score>0).sort((a,b)=>b.score-a.score||a.title.localeCompare(b.title));
}

function openFinder(explicit=false) {
  if(!session||document.querySelector('tk-dialog[open]')||!explicit&&foregroundOpen())return;
  const dialog=openDialog('Find work',`<label class="sr-only" for="finder-query">Search work or ticket ID</label><input id="finder-query" role="combobox" aria-autocomplete="list" aria-controls="finder-results" aria-expanded="false" autocomplete="off" maxlength="500" placeholder="Search work or #ID"><div id="finder-dropdown" class="finder-dropdown" hidden><div id="finder-results" role="listbox" aria-label="Matching work"></div><p id="finder-status" role="status" aria-live="polite"></p></div>`);
  dialog.classList.add('finder-dialog');
  const completion=captureDialogCompletion(dialog),input=byId('finder-query'),dropdown=byId('finder-dropdown');let generation=0,timer=null,items=[],tickets=[],actions=[],index=-1,cursor='',loading=false,selectionTouched=false;
  const valid=()=>completion.current();
  function select(n,scroll=true,touched=false){index=n;if(index<0||index>=items.length)return;if(touched)selectionTouched=true;input.setAttribute('aria-activedescendant','finder-option-'+index);const options=byId('finder-results').querySelectorAll('[role=option]');options.forEach((option,n)=>option.setAttribute('aria-selected',String(n===index)));if(scroll)options[index]?.scrollIntoView({block:'nearest'});}
  function draw(){const selected=items[index],key=selectionTouched?selected?.key:null;items=[...tickets.map(hit=>({key:'ticket:'+hit.ticket.id,ticket:hit.ticket})),...actions.map(action=>({key:'action:'+action.title,action}))];index=key?items.findIndex(item=>item.key===key):-1;if(index<0&&items.length)index=0;let section='';byId('finder-results').innerHTML=items.map((item,n)=>{const group=item.ticket?'Tickets':'Actions',heading=group!==section?`<div class="finder-section" role="presentation">${group}</div>`:'';section=group;return item.ticket?`${heading}<div role="option" id="finder-option-${n}" aria-selected="false" data-result="${n}"><span class="number">#${item.ticket.id}</span><strong>${escapeHTML(item.ticket.title)}</strong><small>${escapeHTML(projectName(item.ticket.projectId))} · ${escapeHTML(item.ticket.kind)}</small></div>`:`${heading}<div role="option" id="finder-option-${n}" aria-selected="false" data-result="${n}"><span class="finder-action-icon" aria-hidden="true">↗</span><strong>${escapeHTML(item.action.title)}</strong><small>Action</small></div>`;}).join('');byId('finder-results').querySelectorAll('[data-result]').forEach(option=>{option.onclick=()=>{select(Number(option.dataset.result),false,true);openSelected();};});select(index,false);}
  async function search(more=false,advance=false){if(more&&(!cursor||loading))return;const request=++generation,current=input.value.trim(),parsed=finderQuery(current),oldLength=tickets.length;if(!more){tickets=[];cursor='';actions=parsed.actions&&parsed.query?finderActions(parsed.query):[];draw();}if(!parsed.query){byId('finder-status').textContent='Enter a search term.';return;}if(parsed.actionOnly){byId('finder-status').textContent=items.length?'':'No matching actions.';return;}loading=true;byId('finder-status').textContent='Searching…';try{const page=await api('searchTickets',{projectId:0,query:parsed.query,...(parsed.kind?{kind:parsed.kind}:{}),limit:50,cursor:more?cursor:''});if(!valid()||request!==generation||current!==input.value.trim())return;tickets=more?[...tickets,...page.items]:page.items;cursor=page.nextCursor;draw();if(advance&&tickets.length>oldLength)select(oldLength,true,true);byId('finder-status').textContent=items.length?'':'No matching results.';}catch(error){if(valid()&&request===generation)byId('finder-status').textContent=error.message;}finally{if(request===generation)loading=false;}}
  function schedule(){generation++;loading=false;items=[];tickets=[];actions=[];index=-1;cursor='';selectionTouched=false;byId('finder-results').innerHTML='';input.removeAttribute('aria-activedescendant');clearTimeout(timer);const hasQuery=!!input.value.trim();dropdown.hidden=!hasQuery;input.setAttribute('aria-expanded',String(hasQuery));byId('finder-status').textContent=hasQuery?'Searching…':'';if(hasQuery)timer=setTimeout(()=>{if(valid())search();},160);}
  async function openSelected(){if(!valid()||index<0||!items[index])return;const item=items[index];dialog.classList.remove('finder-dialog');dialog.close();try{if(item.ticket)await editTicket(item.ticket.id);else await item.action.run();}catch(error){showError(error);}}
  on('finder-query','input',schedule);dropdown.onscroll=()=>{if(dropdown.scrollHeight-dropdown.scrollTop-dropdown.clientHeight<30)search(true);};
  input.onkeydown=e=>{if(e.isComposing)return;if(e.key==='Enter'){e.preventDefault();openSelected();return;}if(['ArrowDown','ArrowUp'].includes(e.key)){e.preventDefault();if(items.length){if(e.key==='ArrowDown'&&index===items.length-1&&cursor)search(true,true);else select((index+(e.key==='ArrowDown'?1:-1)+items.length)%items.length,true,true);}}};
  byId('editor-form').onsubmit=e=>{e.preventDefault();openSelected();};
  dialog.addEventListener('close',()=>{generation++;clearTimeout(timer);dialog.classList.remove('finder-dialog');},{once:true});input.focus();
}

let shiftDown=null,shiftFirstUp=0,shiftInvalid=false;
const resetShift=()=>{shiftDown=null;shiftFirstUp=0;shiftInvalid=false;};
document.addEventListener('keydown',e=>{if(e.key!=='Shift'){resetShift();return;}if(e.repeat||e.ctrlKey||e.altKey||e.metaKey||e.isComposing||document.hidden||foregroundOpen()||!session){resetShift();return;}if(shiftDown!==null){shiftInvalid=true;shiftFirstUp=0;return;}shiftDown={at:performance.now(),code:e.code};});
document.addEventListener('keyup',e=>{if(e.key!=='Shift')return;const now=performance.now();if(!shiftDown||shiftInvalid||e.code!==shiftDown.code||now-shiftDown.at>=400||e.ctrlKey||e.altKey||e.metaKey||e.isComposing||foregroundOpen()){resetShift();return;}shiftDown=null;if(shiftFirstUp&&now-shiftFirstUp<=400){resetShift();openFinder();}else shiftFirstUp=now;});
window.addEventListener('blur',resetShift);document.addEventListener('visibilitychange',resetShift);document.addEventListener('focusin',resetShift);document.addEventListener('compositionstart',resetShift);
