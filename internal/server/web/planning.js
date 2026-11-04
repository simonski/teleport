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

function openFinder(explicit=false) {
  if(!session||document.querySelector('tk-dialog[open]')||!explicit&&foregroundOpen())return;
  const dialog=openDialog('Find work',`<div class="finder-controls"><label>Search tickets or #ID<input id="finder-query" role="combobox" aria-autocomplete="list" aria-controls="finder-results" aria-expanded="true" autocomplete="off" maxlength="500" placeholder="Search all accessible work"></label></div><p id="finder-status" class="form-help" role="status">Type a ticket ID or all search words.</p><div class="finder-body"><div><div id="finder-results" role="listbox" aria-label="Matching tickets"></div><button type="button" class="secondary" id="finder-more" hidden>More matches</button></div><aside id="finder-preview" aria-label="Read-only ticket preview"><p>Choose a result to preview its current request.</p></aside></div>`,'Open selected ticket');
  dialog.classList.add('finder-dialog');
  const completion=captureDialogCompletion(dialog),input=byId('finder-query');let generation=0,timer=null,items=[],index=-1,cursor='';
  const valid=()=>completion.current();
  function select(n){index=n;if(index<0||index>=items.length)return;const t=items[index].ticket;input.setAttribute('aria-activedescendant','finder-option-'+index);byId('finder-results').querySelectorAll('[role=option]').forEach((button,n)=>button.setAttribute('aria-selected',String(n===index)));byId('finder-preview').innerHTML=`<span class="eyebrow">#${t.id} / ${escapeHTML(projectName(t.projectId))}</span><h3>${escapeHTML(t.title)}</h3><p>${escapeHTML(t.kind)} · ${escapeHTML(t.status)} · v${t.version}</p><h4>Request</h4><p class="idea-text">${escapeHTML(t.description||'No description')}</p><h4>Success criteria</h4><p class="idea-text">${escapeHTML(t.criteria||'No written criteria')}</p>`;}
  function draw(){byId('finder-results').innerHTML=items.map((hit,n)=>`<button type="button" role="option" tabindex="-1" id="finder-option-${n}" aria-selected="false" data-result="${n}"><span class="number">#${hit.ticket.id}</span><strong>${escapeHTML(hit.ticket.title)}</strong><small>${escapeHTML(projectName(hit.ticket.projectId))} · ${escapeHTML(hit.ticket.kind)}</small></button>`).join('');byId('finder-results').querySelectorAll('[data-result]').forEach(button=>{button.onclick=()=>{select(Number(button.dataset.result));input.focus({preventScroll:true});};button.ondblclick=()=>openSelected();});byId('finder-more').hidden=!cursor;select(index>=0&&index<items.length?index:items.length?0:-1);}
  async function search(more=false){const request=++generation,current=input.value.trim();if(!more){items=[];cursor='';}if(!current){byId('finder-results').innerHTML='';byId('finder-status').textContent='Type a ticket ID or all search words.';input.removeAttribute('aria-activedescendant');return;}byId('finder-status').textContent='Searching accessible work…';try{const page=await api('searchTickets',{projectId:0,query:current,limit:50,cursor:more?cursor:''});if(!valid()||request!==generation||current!==input.value.trim())return;items=more?[...items,...page.items]:page.items;cursor=page.nextCursor;draw();if(!items.length)byId('finder-preview').innerHTML='<p>No matching work to preview.</p>';byId('finder-status').textContent=items.length?`${items.length} ${items.length===1?'match':'matches'}${cursor?' · more available':''}`:'No accessible matches. Try fewer words.';}catch(error){if(valid()&&request===generation)byId('finder-status').textContent=error.message;}}
  function schedule(){generation++;items=[];index=-1;cursor='';byId('finder-results').innerHTML='';byId('finder-more').hidden=true;byId('finder-preview').innerHTML=input.value.trim()?'<p>Searching…</p>':'<p>Choose a result to preview its current request.</p>';input.removeAttribute('aria-activedescendant');clearTimeout(timer);timer=setTimeout(()=>{if(valid())search();},160);}
  async function openSelected(){if(!valid()||index<0||!items[index])return;const id=items[index].ticket.id;dialog.classList.remove('finder-dialog');dialog.close();await editTicket(id);}
  on('finder-query','input',schedule);on('finder-more','click',()=>search(true));
  input.onkeydown=e=>{if(['ArrowDown','ArrowUp'].includes(e.key)){e.preventDefault();if(items.length)select((index+(e.key==='ArrowDown'?1:-1)+items.length)%items.length);}};
  byId('editor-form').onsubmit=e=>{e.preventDefault();openSelected();};
  dialog.addEventListener('close',()=>{generation++;clearTimeout(timer);dialog.classList.remove('finder-dialog');},{once:true});input.focus();
}

let shiftDown=null,shiftFirstUp=0,shiftInvalid=false;
const resetShift=()=>{shiftDown=null;shiftFirstUp=0;shiftInvalid=false;};
document.addEventListener('keydown',e=>{if(e.key!=='Shift'){resetShift();return;}if(e.repeat||e.ctrlKey||e.altKey||e.metaKey||e.isComposing||document.hidden||foregroundOpen()||!session){resetShift();return;}if(shiftDown!==null){shiftInvalid=true;shiftFirstUp=0;return;}shiftDown={at:performance.now(),code:e.code};});
document.addEventListener('keyup',e=>{if(e.key!=='Shift')return;const now=performance.now();if(!shiftDown||shiftInvalid||e.code!==shiftDown.code||now-shiftDown.at>=400||e.ctrlKey||e.altKey||e.metaKey||e.isComposing||foregroundOpen()){resetShift();return;}shiftDown=null;if(shiftFirstUp&&now-shiftFirstUp<=400){resetShift();openFinder();}else shiftFirstUp=now;});
window.addEventListener('blur',resetShift);document.addEventListener('visibilitychange',resetShift);document.addEventListener('focusin',resetShift);document.addEventListener('compositionstart',resetShift);
