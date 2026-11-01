"use strict";
let boardTicketDrag=null;
function boardRefineDrop(ticket,targetProject,key,phaseID){
  if(ticket.status!=='Todo'||!canQueueRefinement(ticket)||ticket.projectId!==targetProject)return '';
  const project=projects.find(project=>project.id===targetProject),workflow=workflowMap[key];
  if(!project||!workflow||key!==(ticket.agent?ticket.workflow:project.agentWorkflow))return '';
  const destination=workflow.phases.find(phase=>phase.id===phaseID);
  if(destination?.role!=='Refine'||!destination.agentic)return '';
  const initial=workflow.phases.filter(phase=>phase.role==='Todo');
  if(!ticket.agent&&initial.length!==1)return '';
  const source=ticket.agent?ticket.phase:initial[0].id;
  const routes=workflow.rules.filter(rule=>rule.source===source&&rule.outcome==='requestRefinement'&&!Object.keys(rule.conditions||{}).length);
  const priority=Math.max(...routes.map(rule=>rule.priority||0)),matches=routes.filter(rule=>(rule.priority||0)===priority);
  return matches.length===1&&matches[0].target===phaseID?'requestRefinement':'';
}
function cancelBoardTicketDrag(){
  boardTicketDrag=null;
  document.querySelectorAll('.board-drag-source,.board-drop-target,.board-drop-available,.board-drag-in-progress').forEach(node=>node.classList.remove('board-drag-source','board-drop-target','board-drop-available','board-drag-in-progress'));
}
function bindBoardDragging(){
  if(view!=='Board')return;
  const lanes=[...root.querySelectorAll('.board-group .lane')];
  const action=(ticket,lane)=>{const group=lane.closest('.board-group');return boardRefineDrop(ticket,Number(group.dataset.boardProject),group.dataset.boardWorkflow,lane.dataset.phaseId);};
  for(const card of root.querySelectorAll('[data-ticket-card]')){
    const ticket=tickets.find(ticket=>ticket.id===Number(card.dataset.ticketCard));
    if(!ticket||!roleAllowsWrite(ticket.projectId)||!lanes.some(lane=>action(ticket,lane)))continue;
    card.draggable=true;
    card.ondragstart=e=>{
      if(e.target.closest('input,textarea,.ticket-session,.ticket-description')){e.preventDefault();return;}
      boardTicketDrag={ticket,actor:session.account.id,generation:loadGeneration,scope:projectId};
      card.classList.add('board-drag-source');root.querySelector('.board-workspace').classList.add('board-drag-in-progress');
      for(const lane of lanes)if(action(ticket,lane))lane.classList.add('board-drop-available');
      e.dataTransfer.effectAllowed='move';e.dataTransfer.setData('application/x-teleport-board-ticket',String(ticket.id));
    };
    card.ondragend=cancelBoardTicketDrag;
  }
  for(const lane of lanes){
    const valid=()=>boardTicketDrag&&view==='Board'&&boardTicketDrag.actor===session?.account.id&&boardTicketDrag.generation===loadGeneration&&boardTicketDrag.scope===projectId&&action(boardTicketDrag.ticket,lane);
    lane.ondragenter=lane.ondragover=e=>{if(!valid())return;e.preventDefault();e.dataTransfer.dropEffect='move';lane.classList.add('board-drop-target');};
    lane.ondragleave=e=>{if(!lane.contains(e.relatedTarget))lane.classList.remove('board-drop-target');};
    lane.ondrop=e=>{const operation=valid(),ticket=boardTicketDrag?.ticket;e.preventDefault();cancelBoardTicketDrag();if(operation)changeRefinementQueue(ticket,operation);};
  }
}
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&boardTicketDrag){e.preventDefault();e.stopImmediatePropagation();cancelBoardTicketDrag();}},true);
window.addEventListener('blur',cancelBoardTicketDrag);
