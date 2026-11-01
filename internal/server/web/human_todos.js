"use strict";
let humanTicketDrag=null;
function humanMoves(ticket){
  if(ticket.agent||ticket.kind!=='task'||ticket.archived||ticket.deleted||ticket.active||ticket.claim||ticket.execution)return [];
  const workflow=workflowMap[ticket.workflow];if(!workflow)return [];
  return workflow.phases.filter(phase=>['Todo','Doing','Done'].includes(phase.role)&&phase.id!==ticket.phase&&workflow.rules.some(rule=>rule.source===ticket.phase&&rule.target===phase.id&&!Object.keys(rule.conditions||{}).length)).map(phase=>({target:phase.id,role:phase.role}));
}
function canQueueRefinement(ticket){return ticket.kind==='task'&&!ticket.protected&&!ticket.archived&&!ticket.deleted&&!ticket.active&&!ticket.claim&&!ticket.execution&&!ticket.delivery&&!(ticket.question&&!ticket.question.resolved&&!ticket.question.obsolete)&&!ticket.refinement?.acceptedBy;}
async function moveHumanTicket(ticket,target){
  const move=humanMoves(ticket).find(move=>move.target===target);if(!move)return;
  const dialog=byId('editor');if(!dialog.open)delete dialog.dataset.dirty;
  if(move.role==='Done'){completionForm(ticket);return;}
  try{await api('transitionTicket',{id:ticket.id,expectedVersion:ticket.version,target,confirm:false});await workspace();}catch(error){showError(error);}
}
async function changeRefinementQueue(ticket,operation){try{await api(operation,{id:ticket.id,expectedVersion:ticket.version});await workspace();}catch(error){showError(error);}}
function cancelHumanTicketDrag(){humanTicketDrag=null;document.querySelectorAll('.human-dragging,.human-drop-target').forEach(node=>node.classList.remove('human-dragging','human-drop-target'));}
function bindHumanTodos(){
  if(view!=='Todos')return;
  for(const card of root.querySelectorAll('[data-ticket-card]')){
    const ticket=tickets.find(ticket=>ticket.id===Number(card.dataset.ticketCard));if(!ticket||!roleAllowsWrite(ticket.projectId)||!humanMoves(ticket).length)continue;
    card.draggable=true;card.ondragstart=e=>{if(e.target.closest('input,textarea,[data-session]')){e.preventDefault();return;}humanTicketDrag={ticket,actor:session.account.id,generation:loadGeneration,scope:projectId};card.classList.add('human-dragging');e.dataTransfer.effectAllowed='move';e.dataTransfer.setData('application/x-teleport-human-ticket',String(ticket.id));};card.ondragend=cancelHumanTicketDrag;
  }
  for(const lane of root.querySelectorAll('.human-board .lane')){
    const role=lane.querySelector('h2').textContent;
    const valid=()=>humanTicketDrag&&view==='Todos'&&humanTicketDrag.actor===session?.account.id&&humanTicketDrag.generation===loadGeneration&&humanTicketDrag.scope===projectId&&humanMoves(humanTicketDrag.ticket).find(move=>move.role===role);
    lane.ondragover=e=>{if(!valid())return;e.preventDefault();e.dataTransfer.dropEffect='move';lane.classList.add('human-drop-target');};lane.ondragleave=e=>{if(!lane.contains(e.relatedTarget))lane.classList.remove('human-drop-target');};
    lane.ondrop=e=>{const move=valid(),ticket=humanTicketDrag?.ticket;e.preventDefault();cancelHumanTicketDrag();if(move)moveHumanTicket(ticket,move.target);};
  }
}
document.addEventListener('keydown',e=>{if(e.key==='Escape'&&humanTicketDrag){e.preventDefault();e.stopImmediatePropagation();cancelHumanTicketDrag();}},true);
window.addEventListener('blur',cancelHumanTicketDrag);
