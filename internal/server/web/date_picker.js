"use strict";
function parseCalendarDate(value){
  if(!/^\d{4}-\d{2}-\d{2}$/.test(value))return null;
  const [year,month,day]=value.split('-').map(Number),date=new Date(0);date.setUTCHours(0,0,0,0);date.setUTCFullYear(year,month-1,day);
  return year>0&&date.getUTCFullYear()===year&&date.getUTCMonth()===month-1&&date.getUTCDate()===day?date:null;
}
function calendarDateISO(date){return `${String(date.getUTCFullYear()).padStart(4,'0')}-${String(date.getUTCMonth()+1).padStart(2,'0')}-${String(date.getUTCDate()).padStart(2,'0')}`;}
function enhanceDate(input){
  if(input.dataset.calendar)return;input.dataset.calendar='true';input.type='text';input.inputMode='numeric';input.pattern='[0-9]{4}-[0-9]{2}-[0-9]{2}';input.placeholder='YYYY-MM-DD';
  const name=input.labels?.[0]?.textContent.trim()||input.name||'Date',wrapper=document.createElement('span'),button=document.createElement('button');
  wrapper.className='date-field';input.before(wrapper);wrapper.append(input);button.type='button';button.className='date-trigger secondary';button.innerHTML='<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="5" width="18" height="16" rx="3"/><path d="M7 3v4m10-4v4M3 11h18"/></svg>';button.setAttribute('aria-label','Choose '+name.toLowerCase()+' date');button.setAttribute('aria-haspopup','dialog');button.setAttribute('aria-expanded','false');wrapper.append(button);
  const sync=()=>{button.disabled=input.disabled||input.readOnly||!!input.closest('fieldset:disabled');};sync();new MutationObserver(sync).observe(input,{attributes:true});
  const validate=()=>input.setCustomValidity(input.value&&!parseCalendarDate(input.value)?'Enter a valid date as YYYY-MM-DD.':'');input.addEventListener('input',validate);input.addEventListener('change',validate);
  button.onclick=()=>{
    if(openChoice?.button===button){dismissChoice(true);return;}dismissChoice();sync();if(button.disabled)return;
    const today=new Date(),utcToday=parseCalendarDate(`${today.getFullYear()}-${String(today.getMonth()+1).padStart(2,'0')}-${String(today.getDate()).padStart(2,'0')}`);
    let active=parseCalendarDate(input.value)||utcToday,month=new Date(active);month.setUTCDate(1);
    const list=document.createElement('div');list.className='choice-list date-picker';list.id='calendar-'+crypto.randomUUID();list.setAttribute('role','dialog');list.setAttribute('aria-label',name+' calendar');button.setAttribute('aria-controls',list.id);button.setAttribute('aria-expanded','true');(input.closest('tk-dialog[open]')||document.body).append(list);
    const allowed=date=>(!input.min||calendarDateISO(date)>=input.min)&&(!input.max||calendarDateISO(date)<=input.max);
    function position(){const rect=button.getBoundingClientRect(),width=Math.min(300,innerWidth-16);list.style.width=width+'px';list.style.left=Math.max(8,Math.min(rect.right-width,innerWidth-width-8))+'px';list.style.top=Math.max(8,Math.min(rect.bottom+4,innerHeight-list.offsetHeight-8))+'px';}
    function choose(date){input.value=date?calendarDateISO(date):'';input.dispatchEvent(new Event('input',{bubbles:true}));input.dispatchEvent(new Event('change',{bubbles:true}));dismissChoice(true);}
    function draw(focus=true){
      list.replaceChildren();const header=document.createElement('div');header.className='date-heading';
      for(const [text,label,offset] of [['‹','Previous month',-1],['›','Next month',1]]){const nav=document.createElement('button');nav.type='button';nav.className='secondary';nav.textContent=text;nav.setAttribute('aria-label',label);nav.disabled=offset<0&&month.getUTCFullYear()===1&&month.getUTCMonth()===0||offset>0&&month.getUTCFullYear()===9999&&month.getUTCMonth()===11;nav.onclick=()=>{month.setUTCMonth(month.getUTCMonth()+offset);active=new Date(month);draw();};header.append(nav);}
      const title=document.createElement('strong');title.textContent=month.toLocaleDateString(undefined,{month:'long',year:'numeric',timeZone:'UTC'});title.setAttribute('aria-live','polite');header.insertBefore(title,header.lastChild);list.append(header);
      const grid=document.createElement('div');grid.className='date-grid';grid.setAttribute('role','group');grid.setAttribute('aria-label','Days');
      for(const day of ['Mo','Tu','We','Th','Fr','Sa','Su']){const label=document.createElement('span');label.textContent=day;label.setAttribute('aria-hidden','true');grid.append(label);}
      const offset=(month.getUTCDay()+6)%7;for(let i=0;i<offset;i++)grid.append(document.createElement('span'));
      for(let day=1;day<=31;day++){const date=new Date(month);date.setUTCDate(day);if(date.getUTCMonth()!==month.getUTCMonth())break;
        const cell=document.createElement('button'),iso=calendarDateISO(date);cell.type='button';cell.textContent=day;cell.dataset.date=iso;cell.disabled=!allowed(date);cell.tabIndex=iso===calendarDateISO(active)?0:-1;cell.setAttribute('aria-label',date.toLocaleDateString(undefined,{dateStyle:'full',timeZone:'UTC'}));cell.setAttribute('aria-pressed',String(input.value===iso));if(iso===calendarDateISO(utcToday))cell.setAttribute('aria-current','date');cell.onclick=()=>choose(date);grid.append(cell);
      }list.append(grid);
      const footer=document.createElement('div');footer.className='date-footer';for(const [label,date] of [['Today',utcToday],['Clear',null]]){const action=document.createElement('button');action.type='button';action.className='secondary';action.textContent=label;action.disabled=date?!allowed(date):input.required;action.onclick=()=>choose(date);footer.append(action);}list.append(footer);position();if(focus)(list.querySelector('[tabindex="0"]:not(:disabled)')||list.querySelector('button:not(:disabled)'))?.focus();
    }
    openChoice={button,list};
    list.onkeydown=e=>{if(!e.target.matches('[data-date]')||e.ctrlKey||e.metaKey||e.altKey)return;const delta={ArrowLeft:-1,ArrowRight:1,ArrowUp:-7,ArrowDown:7,Home:-((active.getUTCDay()+6)%7),End:6-((active.getUTCDay()+6)%7)}[e.key];if(delta===undefined&&!['PageUp','PageDown'].includes(e.key))return;e.preventDefault();const next=new Date(active);if(delta!==undefined)next.setUTCDate(next.getUTCDate()+delta);else{const day=next.getUTCDate();next.setUTCDate(1);next.setUTCMonth(next.getUTCMonth()+(e.key==='PageUp'?-1:1)*(e.shiftKey?12:1));const end=new Date(next);end.setUTCMonth(end.getUTCMonth()+1);end.setUTCDate(0);next.setUTCDate(Math.min(day,end.getUTCDate()));}if(next.getUTCFullYear()<1||next.getUTCFullYear()>9999)return;active=next;month=new Date(next);month.setUTCDate(1);draw();};draw();
  };
  input.addEventListener('keydown',e=>{if(e.altKey&&e.key==='ArrowDown'){e.preventDefault();button.click();}});
}
