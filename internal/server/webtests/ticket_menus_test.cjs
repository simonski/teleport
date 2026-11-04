const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  let release;const state={opened:null,notice:''},events={};
  const context=vm.createContext({session:{account:{id:'one'}},loadGeneration:1,detailEpoch:1,view:'Board',graphData:null,document:{querySelector:()=>null,activeElement:null,addEventListener(){}},window:{addEventListener(type,fn){events[type]=fn;}},api:()=>new Promise(resolve=>release=resolve),pages:async()=>[],openTerminal:async id=>state.opened=id,notice:text=>state.notice=text});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/ticket_menus.js'),'utf8'),context);
  return {context,state,events,run:source=>vm.runInContext(source,context),release:result=>release(result)};
}

test('Teleport is offered only for the exact running ticket attachment',()=>{
  const f=fixture();f.run("ticket={id:7,projectId:2,execution:'local:H1'};rows=[{id:'H1',ticketId:7,projectId:2,state:'running',attention:'Native harness error'}]");
  assert.deepEqual(Array.from(f.run('ticketMenuEntries(ticket,rows).map(entry=>entry.id)')),['open','teleport']);
  for(const change of ["rows[0].state='unknown'","rows[0].state='exited'","rows[0].state='running';rows[0].projectId=3","rows[0].projectId=2;ticket.execution='local:H2'"]){f.run(change);assert.equal(f.run("ticketMenuEntries(ticket,rows).some(entry=>entry.id==='teleport')"),false);}
  assert.equal(f.run("ticketMenuEntries(ticket,rows,true).some(entry=>entry.id==='questions')"),true);
});

test('Todos menu exposes adjacent valid moves, status, priority, open and guarded delete',()=>{
  const f=fixture();f.context.roleAllowsWrite=()=>true;f.context.roleAllowsMaintain=()=>true;
  f.context.humanMoves=()=>[{target:'lane:todo',role:'Todo'},{target:'lane:done',role:'Done'}];
  f.run("view='Todos';ticket={id:7,projectId:2,kind:'task',agent:false,status:'Doing',priority:'medium'}");
  assert.deepEqual(Array.from(f.run('ticketMenuEntries(ticket,[]).map(entry=>entry.id)')),['ahead','behind','status','priority','prompt','agents','proposals','open','delete']);
  assert.equal(f.run("ticketMenuEntries(ticket,[])[0].target"),'lane:done');
  assert.equal(f.run("ticketMenuEntries(ticket,[])[1].target"),'lane:todo');
  f.context.roleAllowsWrite=()=>false;f.context.roleAllowsMaintain=()=>false;
  assert.equal(f.run('ticketMenuEntries(ticket,[]).filter(entry=>entry.disabled).length'),5);
  assert.equal(f.run('ticketMenuEntries(ticket,[]).find(entry=>entry.id==="open").disabled'),undefined);
});

test('configured proposal actions appear directly by their configured name in the matching lane',()=>{
  const f=fixture();f.context.roleAllowsWrite=()=>true;f.context.roleAllowsMaintain=()=>true;f.context.humanMoves=()=>[];
  f.run("view='Todos';ticket={id:7,projectId:2,kind:'task',agent:false,status:'Todo',workflow:'human@1',phase:'todo'};agents=[{id:'planner'}];usages=[{id:'plan',agentId:'planner',name:'Idea Composer',kind:'todo-proposal',enabled:true,view:'Todos',workflow:'human@1',phase:'todo'},{id:'other',agentId:'planner',name:'Other',kind:'todo-proposal',enabled:true,view:'Todos',workflow:'human@1',phase:'doing'}]");
  assert.equal(f.run("ticketMenuEntries(ticket,[],false,agents,usages,true).find(entry=>entry.usageId==='plan').label"),'Idea Composer');
  assert.equal(f.run("ticketMenuEntries(ticket,[],false,agents,usages,true).some(entry=>entry.usageId==='other')"),false);
  assert.match(f.run("ticketMenuEntries(ticket,[],false,agents,usages,true).find(entry=>entry.id==='proposals').label"),/available/);
  f.run('agents=[]');assert.equal(f.run("ticketMenuEntries(ticket,[],false,agents,usages).find(entry=>entry.usageId==='plan').disabled"),true);
});

test('a late proposal review cannot replace a newer dialog',async()=>{
  const f=fixture();let resolveTicket,closed=false,form={};
  f.context.api=()=>new Promise(resolve=>{resolveTicket=resolve});f.context.pages=async()=>[];
  f.context.dialog={open:true,querySelector:()=>form,close(){closed=true;}};
  const pending=f.run('openTicketProposalReview(7,dialog)');form={};resolveTicket({id:7});await pending;
  assert.equal(closed,false);
});

test('a delayed menu read cannot replace a newer foreground interaction',async()=>{
  const f=fixture();f.context.source={isConnected:true};const request=f.run('showTicketMenu(7,source)');
  f.run('detailEpoch++');f.release({id:7});await request;
  assert.equal(f.run('ticketMenu'),null);assert.equal(f.run('ticketMenuPending'),false);
});

test('a delayed menu read cannot enter another account',async()=>{
  const f=fixture();f.context.source={isConnected:true};const request=f.run('showTicketMenu(7,source)');
  f.run("dismissTicketMenu();session={account:{id:'two'}};loadGeneration++");f.release({id:7});await request;
  assert.equal(f.run('ticketMenu'),null);assert.equal(f.run('ticketMenuPending'),false);
});

test('Teleport does not substitute another session after an attachment changes',async()=>{
  const f=fixture();const request=f.run("teleportTicketSession(7,'H1')");
  f.release({id:7,projectId:2,execution:'local:H2'});await request;
  assert.equal(f.state.opened,null);assert.match(f.state.notice,/no longer running/);
});

test('queued scroll and resize events preserve a menu positioned after those changes',()=>{
  const f=fixture(),rect={left:10,top:20};let removed=false;
  f.context.menuElement={contains:()=>false,remove(){removed=true;}};
  f.context.menuSource={getBoundingClientRect:()=>rect};
  f.run('innerWidth=390;innerHeight=844;ticketMenu={element:menuElement,source:menuSource,anchor:{left:10,top:20},width:390,height:844}');
  f.events.scroll({target:{}});f.events.resize();assert.equal(removed,false);
  rect.top=40;f.events.scroll({target:{}});assert.equal(removed,true);
  assert.equal(f.run('ticketMenu'),null);
});
