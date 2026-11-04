"use strict";
const canonicalNavigation=['Home','Backlog','Board','Todos','Calendar','Graph','Workflows','Agents','Evolution','Cost'];
const supportedNavigation=['Home','Backlog','Board','Todos','Calendar','Graph','Workflows','Agents','Cost'];
function availableNavigation(name){return supportedNavigation.includes(name)&&(name!=='Agents'||!!session?.account?.administrator);}
function canonicalView(name){return ['Ideas','Goals','Epics','Tickets'].includes(name)?'Backlog':name==='Projects'?'Home':name;}
function viewLabel(name){return name==='Todos'?'todo':name.toLowerCase();}
let navigationGesture=null,navigationDeferred=false,navigationSuppressClick=0,navigationApplied=null;

function navigationValues(){return {...preferences,...preferenceDraft};}
function navigationOrder(values=navigationValues()){
  return [...new Set([...(values.navigationOrder||[]),...canonicalNavigation].map(canonicalView))].filter(name=>canonicalNavigation.includes(name));
}
function navigationIcon(name){
  if(name==='Backlog')return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 4h5v5H3ZM12 6h9M8 9v6h4M12 13h5v5h-5ZM8 15v6h4M20 15h1M16 21h5"/></svg>';
  if(name==='Tickets')return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 3h5v5H3ZM16 3h5v5h-5ZM16 16h5v5h-5ZM8 5h4q2 0 2 3v8q0 3 2 3M14 5h2"/></svg>';
  if(name==='Workflows')return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 3h6v5H4ZM14 16h6v5h-6ZM4 16h6v5H4ZM7 8v8M10 5h7v11M10 18h4"/></svg>';
  const paths={Home:'m3 11 9-8 9 8M6 10v11h12V10M10 21v-7h4v7',Ideas:'M9 18h6M10 21h4M8 14a7 7 0 1 1 8 0l-1 4H9Z',Goals:'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M12 7a5 5 0 1 0 0 10 5 5 0 0 0 0-10M12 11v2',Epics:'m3 7 9-4 9 4-9 4ZM3 12l9 4 9-4M3 17l9 4 9-4',Board:'M3 4h18v16H3ZM9 4v16M15 4v16M5 8h2M11 8h2M17 8h2',Todos:'M9 6h12M9 12h12M9 18h12M3 6l1 1 2-3M3 12l1 1 2-3M3 18l1 1 2-3',Calendar:'M3 5h18v16H3ZM7 3v4M17 3v4M3 10h18M7 14h2M13 14h2M7 18h2',Graph:'M5 5h4v4H5ZM16 15h4v4h-4ZM3 17h4v4H3ZM9 7l9 8M7 9l-2 8M7 19l9-2',Cost:'M4 7c0-5 16-5 16 0s-16 5-16 0ZM4 7v10c0 5 16 5 16 0V7M4 12c0 5 16 5 16 0',Projects:'M3 6h7l2 3h9v12H3ZM3 6V3h7l2 3'};
  return `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="${paths[name]||'M3 12h18M12 3v18'}"/></svg>`;
}
function navigationHTML(){
  const names=navigationOrder().filter(availableNavigation);
  return `<nav id="workspace-nav" class="tabs" aria-label="Workspace views"><div id="nav-views" class="nav-views">${names.map(name=>`<button type="button" data-view="${name}" class="tab ${name===view?'selected':''}" ${name===view?'aria-current="page"':''} aria-label="${viewLabel(name)}" title="${viewLabel(name)} · Alt+Shift+Arrow to reorder" aria-keyshortcuts="Alt+Shift+ArrowLeft Alt+Shift+ArrowRight Alt+Shift+ArrowUp Alt+Shift+ArrowDown"><span class="nav-grip" aria-hidden="true">⠿</span>${navigationIcon(name)}<span class="tab-label">${viewLabel(name)}</span></button>`).join('')}</div><span class="header-space"></span><div class="nav-save"><span id="nav-save-status" role="status"></span><button type="button" class="secondary" id="nav-retry" hidden>Retry</button></div><button type="button" class="secondary" id="nav-collapse" aria-controls="nav-views">${navigationIcon('Board')}<span class="tab-label">Collapse header</span></button></nav>`;
}
function navigationBusy(){return !!navigationGesture;}
function moveNavigation(order,source,target,after=false){
  const next=[...order];if(source===target||!next.includes(source)||!next.includes(target))return next;
  next.splice(next.indexOf(source),1);next.splice(next.indexOf(target)+(after?1:0),0,source);return next;
}
function showNavigationOrder(order,animate=false){
  const list=byId('nav-views');if(!list)return;
  const buttons=[...list.querySelectorAll('[data-view]')],focused=document.activeElement;
  if(!animate&&buttons.map(button=>button.dataset.view).join('|')===order.filter(name=>buttons.some(button=>button.dataset.view===name)).join('|'))return;
  const previous=new Map(buttons.map(button=>[button.dataset.view,button.getBoundingClientRect()]));
  for(const button of buttons)button.getAnimations?.().forEach(animation=>animation.cancel());
  for(const name of order){const button=buttons.find(row=>row.dataset.view===name);if(button)list.append(button);}
  if(list.contains(focused)&&document.activeElement!==focused)focused.focus({preventScroll:true});
  if(animate&&!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches){
    for(const button of buttons){const before=previous.get(button.dataset.view),after=button.getBoundingClientRect();if(button.animate&&(before.x!==after.x||before.y!==after.y))button.animate([{transform:`translate(${before.x-after.x}px,${before.y-after.y}px)`},{transform:'none'}],{duration:170,easing:'cubic-bezier(.2,.8,.2,1)'});}
  }
}
function applyNavigation(){
  if(!session)return;
  if(navigationGesture){navigationDeferred=true;return;}
  const values=navigationValues(),fields=['navigationOrder','navigationPosition','navigationCollapsed'],own=fields.some(field=>field in preferenceDraft),busy=foregroundOpen();
  if(!own&&busy){navigationDeferred=true;return;}
  if(busy&&navigationApplied)for(const field of fields)if(!(field in preferenceDraft))values[field]=navigationApplied[field];
  navigationDeferred=!!busy;
  document.documentElement.dataset.navigation=values.navigationPosition||'top';
  document.documentElement.dataset.navigationCollapsed=String(!!values.navigationCollapsed);
  showNavigationOrder(navigationOrder(values));
  navigationApplied={navigationOrder:navigationOrder(values),navigationPosition:values.navigationPosition||'top',navigationCollapsed:!!values.navigationCollapsed};
  const collapse=byId('nav-collapse');
  if(collapse){collapse.setAttribute('aria-expanded',String(!values.navigationCollapsed));collapse.setAttribute('aria-label',values.navigationCollapsed?'Expand header':'Collapse header');collapse.title=values.navigationCollapsed?'Expand header':'Collapse header';const label=collapse.querySelector('.tab-label');if(label)label.textContent=values.navigationCollapsed?'Expand header':'Collapse header';const path=collapse.querySelector('path');if(path)path.setAttribute('d',(values.navigationPosition==='left')===!!values.navigationCollapsed?'m9 5 7 7-7 7':'m15 5-7 7 7 7');}
}
function resetNavigation(){
  cancelNavigationGesture();navigationDeferred=false;navigationSuppressClick=0;navigationApplied=null;
  delete document.documentElement.dataset.navigation;delete document.documentElement.dataset.navigationCollapsed;
}
function cancelNavigationGesture(){
  const drag=navigationGesture;if(!drag)return;navigationGesture=null;
  drag.ghost?.remove();drag.button.classList.remove('nav-drag-source');
  if(drag.dragging){showNavigationOrder(drag.original,true);navigationSuppressClick=performance.now()+500;}
  if(drag.button.hasPointerCapture?.(drag.pointerId))drag.button.releasePointerCapture(drag.pointerId);
  if(drag.dragging&&navigationDeferred&&session&&!foregroundOpen())applyNavigation();
}
function bindNavigation(){
  const nav=byId('workspace-nav'),list=byId('nav-views');if(!nav||!list)return;
  applyNavigation();
  on('nav-collapse','click',()=>{queuePreferenceSave('navigationCollapsed',!navigationValues().navigationCollapsed);applyNavigation();});
  on('nav-retry','click',retryPreferenceSave);
  nav.addEventListener('click',e=>{if(performance.now()<navigationSuppressClick&&e.target.closest('[data-view]')){e.preventDefault();e.stopImmediatePropagation();navigationSuppressClick=0;}},true);
  list.addEventListener('keydown',e=>{
    const button=e.target.closest('[data-view]');if(!button||!e.altKey||!e.shiftKey||e.ctrlKey||e.metaKey||!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key))return;
    e.preventDefault();e.stopPropagation();
    const visible=[...list.querySelectorAll('[data-view]')].map(node=>node.dataset.view),index=visible.indexOf(button.dataset.view),direction=['ArrowLeft','ArrowUp'].includes(e.key)?-1:1,target=visible[index+direction];
    if(!target)return;
    const next=moveNavigation(navigationOrder(),button.dataset.view,target,direction>0);
    showNavigationOrder(next,true);queuePreferenceSave('navigationOrder',next);button.focus({preventScroll:true});
  });
  list.addEventListener('pointerdown',e=>{
    const button=e.target.closest('[data-view]');
    if(!button||e.button!==0||e.isPrimary===false||navigationGesture||e.pointerType==='touch'&&!e.target.closest('.nav-grip'))return;
    navigationGesture={button,pointerId:e.pointerId,startX:e.clientX,startY:e.clientY,original:navigationOrder(),order:navigationOrder(),dragging:false};
  });
}
document.addEventListener('pointermove',e=>{
  const drag=navigationGesture;if(!drag||drag.pointerId!==e.pointerId)return;
  if(!drag.button.isConnected){cancelNavigationGesture();return;}
  if(!drag.dragging){
    if(Math.hypot(e.clientX-drag.startX,e.clientY-drag.startY)<6)return;
    drag.dragging=true;drag.button.setPointerCapture(e.pointerId);drag.button.classList.add('nav-drag-source');
    const rect=drag.button.getBoundingClientRect();drag.ghost=drag.button.cloneNode(true);drag.ghost.removeAttribute('data-view');drag.ghost.className='nav-drag-ghost';drag.ghost.setAttribute('aria-hidden','true');drag.ghost.tabIndex=-1;drag.ghost.style.width=rect.width+'px';drag.ghost.style.height=rect.height+'px';document.body.append(drag.ghost);
  }
  e.preventDefault();drag.ghost.style.left=(e.clientX+12)+'px';drag.ghost.style.top=(e.clientY+10)+'px';
  const target=document.elementFromPoint(e.clientX,e.clientY)?.closest('#nav-views [data-view]');if(!target||target===drag.button)return;
  const rect=target.getBoundingClientRect(),vertical=document.documentElement.dataset.navigation!=='top',after=vertical?e.clientY>rect.y+rect.height/2:e.clientX>rect.x+rect.width/2;
  const next=moveNavigation(drag.order,drag.button.dataset.view,target.dataset.view,after);
  if(next.join('|')!==drag.order.join('|')){drag.order=next;showNavigationOrder(next,true);}
},{passive:false});
document.addEventListener('pointerup',e=>{
  const drag=navigationGesture;if(!drag||drag.pointerId!==e.pointerId)return;
  const order=drag.order,changed=drag.dragging&&order.join('|')!==drag.original.join('|');
  navigationGesture=null;drag.ghost?.remove();drag.button.classList.remove('nav-drag-source');
  if(drag.button.hasPointerCapture?.(e.pointerId))drag.button.releasePointerCapture(e.pointerId);
  if(drag.dragging){navigationSuppressClick=performance.now()+500;drag.button.focus({preventScroll:true});}
  if(changed)queuePreferenceSave('navigationOrder',order);
  if(drag.dragging&&navigationDeferred&&session&&!foregroundOpen())applyNavigation();
});
document.addEventListener('pointercancel',e=>{if(navigationGesture?.pointerId===e.pointerId)cancelNavigationGesture();});
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&navigationGesture){e.preventDefault();e.stopImmediatePropagation();cancelNavigationGesture();}},true);
window.addEventListener('resize',cancelNavigationGesture);
document.addEventListener('close',()=>{queueMicrotask(()=>{if(navigationDeferred&&session&&!foregroundOpen())applyNavigation();});},true);
