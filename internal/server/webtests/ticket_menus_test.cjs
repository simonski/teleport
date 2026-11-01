const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  let release;const state={opened:null,notice:''};
  const context=vm.createContext({session:{account:{id:'one'}},loadGeneration:1,detailEpoch:1,view:'Board',graphData:null,document:{querySelector:()=>null,activeElement:null,addEventListener(){}},window:{addEventListener(){}},api:()=>new Promise(resolve=>release=resolve),pages:async()=>[],openTerminal:async id=>state.opened=id,notice:text=>state.notice=text});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/ticket_menus.js'),'utf8'),context);
  return {context,state,run:source=>vm.runInContext(source,context),release:result=>release(result)};
}

test('Teleport is offered only for the exact running ticket attachment',()=>{
  const f=fixture();f.run("ticket={id:7,projectId:2,execution:'local:H1'};rows=[{id:'H1',ticketId:7,projectId:2,state:'running',attention:'Native harness error'}]");
  assert.deepEqual(Array.from(f.run('ticketMenuEntries(ticket,rows).map(entry=>entry.id)')),['open','teleport']);
  for(const change of ["rows[0].state='unknown'","rows[0].state='exited'","rows[0].state='running';rows[0].projectId=3","rows[0].projectId=2;ticket.execution='local:H2'"]){f.run(change);assert.equal(f.run("ticketMenuEntries(ticket,rows).some(entry=>entry.id==='teleport')"),false);}
  assert.equal(f.run("ticketMenuEntries(ticket,rows,true).some(entry=>entry.id==='questions')"),true);
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
