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
  const dialog=openDialog('Find work',`<label class="sr-only" for="finder-query">Search work or ticket ID</label><input id="finder-query" role="combobox" aria-autocomplete="list" aria-controls="finder-results" aria-expanded="false" autocomplete="off" maxlength="500" placeholder="Search work or #ID"><div id="finder-dropdown" class="finder-dropdown" hidden><div id="finder-results" role="listbox" aria-label="Matching work"></div><p id="finder-status" role="status" aria-live="polite"></p></div>`);
  dialog.classList.add('finder-dialog');
  const completion=captureDialogCompletion(dialog),input=byId('finder-query'),dropdown=byId('finder-dropdown');let generation=0,timer=null,items=[],index=-1,cursor='',loading=false;
  const valid=()=>completion.current();
  function select(n,scroll=true){index=n;if(index<0||index>=items.length)return;input.setAttribute('aria-activedescendant','finder-option-'+index);const options=byId('finder-results').querySelectorAll('[role=option]');options.forEach((option,n)=>option.setAttribute('aria-selected',String(n===index)));if(scroll)options[index]?.scrollIntoView({block:'nearest'});}
  function draw(){byId('finder-results').innerHTML=items.map((hit,n)=>`<div role="option" id="finder-option-${n}" aria-selected="false" data-result="${n}"><span class="number">#${hit.ticket.id}</span><strong>${escapeHTML(hit.ticket.title)}</strong><small>${escapeHTML(projectName(hit.ticket.projectId))} · ${escapeHTML(hit.ticket.kind)}</small></div>`).join('');byId('finder-results').querySelectorAll('[data-result]').forEach(option=>{option.onclick=()=>{select(Number(option.dataset.result),false);openSelected();};});select(index>=0&&index<items.length?index:items.length?0:-1,false);}
  async function search(more=false,advance=false){if(more&&(!cursor||loading))return;const request=++generation,current=input.value.trim(),oldLength=items.length;if(!more){items=[];cursor='';}if(!current)return;loading=true;byId('finder-status').textContent='Searching…';try{const page=await api('searchTickets',{projectId:0,query:current,limit:50,cursor:more?cursor:''});if(!valid()||request!==generation||current!==input.value.trim())return;items=more?[...items,...page.items]:page.items;cursor=page.nextCursor;draw();if(advance&&items.length>oldLength)select(oldLength);byId('finder-status').textContent=items.length?'':'No matching work.';}catch(error){if(valid()&&request===generation)byId('finder-status').textContent=error.message;}finally{if(request===generation)loading=false;}}
  function schedule(){generation++;loading=false;items=[];index=-1;cursor='';byId('finder-results').innerHTML='';input.removeAttribute('aria-activedescendant');clearTimeout(timer);const hasQuery=!!input.value.trim();dropdown.hidden=!hasQuery;input.setAttribute('aria-expanded',String(hasQuery));byId('finder-status').textContent=hasQuery?'Searching…':'';if(hasQuery)timer=setTimeout(()=>{if(valid())search();},160);}
  async function openSelected(){if(!valid()||index<0||!items[index])return;const id=items[index].ticket.id;dialog.classList.remove('finder-dialog');dialog.close();await editTicket(id);}
  on('finder-query','input',schedule);dropdown.onscroll=()=>{if(dropdown.scrollHeight-dropdown.scrollTop-dropdown.clientHeight<30)search(true);};
  input.onkeydown=e=>{if(e.isComposing)return;if(e.key==='Enter'){e.preventDefault();openSelected();return;}if(['ArrowDown','ArrowUp'].includes(e.key)){e.preventDefault();if(items.length){if(e.key==='ArrowDown'&&index===items.length-1&&cursor)search(true,true);else select((index+(e.key==='ArrowDown'?1:-1)+items.length)%items.length);}}};
  byId('editor-form').onsubmit=e=>{e.preventDefault();openSelected();};
  dialog.addEventListener('close',()=>{generation++;clearTimeout(timer);dialog.classList.remove('finder-dialog');},{once:true});input.focus();
}

let shiftDown=null,shiftFirstUp=0,shiftInvalid=false;
const resetShift=()=>{shiftDown=null;shiftFirstUp=0;shiftInvalid=false;};
document.addEventListener('keydown',e=>{if(e.key!=='Shift'){resetShift();return;}if(e.repeat||e.ctrlKey||e.altKey||e.metaKey||e.isComposing||document.hidden||foregroundOpen()||!session){resetShift();return;}if(shiftDown!==null){shiftInvalid=true;shiftFirstUp=0;return;}shiftDown={at:performance.now(),code:e.code};});
document.addEventListener('keyup',e=>{if(e.key!=='Shift')return;const now=performance.now();if(!shiftDown||shiftInvalid||e.code!==shiftDown.code||now-shiftDown.at>=400||e.ctrlKey||e.altKey||e.metaKey||e.isComposing||foregroundOpen()){resetShift();return;}shiftDown=null;if(shiftFirstUp&&now-shiftFirstUp<=400){resetShift();openFinder();}else shiftFirstUp=now;});
window.addEventListener('blur',resetShift);document.addEventListener('visibilitychange',resetShift);document.addEventListener('focusin',resetShift);document.addEventListener('compositionstart',resetShift);
