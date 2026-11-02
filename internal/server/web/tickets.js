"use strict";

function ticketCreationContext(currentView,currentLens,workflowFamily){
  return {kind:currentView==='Goals'?'goal':currentView==='Epics'?'epic':'task',human:currentView==='Todos'||currentView==='Board'&&currentLens==='human'||currentView==='Tickets'&&workflowFamily==='human'};
}

function boardArrowTarget(lanes,id,key){
  const nonempty=lanes.filter(lane=>lane.length);
  if(!nonempty.length)return null;
  const column=nonempty.findIndex(lane=>lane.includes(id));
  if(column<0)return nonempty[0][0];
  const row=nonempty[column].indexOf(id);
  if(key==='ArrowUp'||key==='ArrowDown')return nonempty[column][Math.max(0,Math.min(nonempty[column].length-1,row+(key==='ArrowDown'?1:-1)))];
  const destination=nonempty[Math.max(0,Math.min(nonempty.length-1,column+(key==='ArrowRight'?1:-1)))];
  return destination[Math.min(row,destination.length-1)];
}

function captureBoardFocus(){
  const button=document.activeElement?.closest('.board-workspace .ticket-open'),id=button?.dataset.ticket,actor=session?.account.id,generation=loadGeneration;
  return ()=>{if(id&&session?.account.id===actor&&loadGeneration===generation&&document.activeElement===document.body)root.querySelector(`.board-workspace .ticket-open[data-ticket="${CSS.escape(id)}"]`)?.focus({preventScroll:true});};
}

document.addEventListener('keydown',e=>{
  if(!session||!['Board','Todos'].includes(view)||e.defaultPrevented||e.ctrlKey||e.metaKey||e.altKey||e.shiftKey&&e.key.startsWith('Arrow')||e.isComposing||foregroundOpen())return;
  if(e.target.closest?.('input,textarea,select,[contenteditable]:not([contenteditable="false"])'))return;
  const button=e.target.closest?.('.board-workspace .ticket-open');
  if(!button&&e.target!==document.body&&!e.target.matches?.('.board-workspace'))return;
  if(['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key)){
    const lanes=[...root.querySelectorAll('.board .lane')].map(lane=>[...lane.querySelectorAll('.ticket-open')].map(node=>node.dataset.ticket));
    const target=boardArrowTarget(lanes,button?.dataset.ticket,e.key);
    if(target){e.preventDefault();root.querySelector(`.board-workspace .ticket-open[data-ticket="${CSS.escape(target)}"]`)?.focus();}
  }else if(e.key.toLowerCase()==='e'&&button){e.preventDefault();editTicket(Number(button.dataset.ticket));}
});
