const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
test('a pending ticket editor owns foreground and cannot open for a replacement account',async()=>{
  let resolve,opened=0;const context=vm.createContext({document:{querySelector:()=>null,addEventListener(){}},setInterval(){},detailEpoch:0,api:operation=>operation==='getTicket'?new Promise(done=>resolve=done):Promise.resolve({items:[],nextCursor:''}),openDialog:()=>opened++,notice(){}});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/workspace.js'),'utf8'),context);
  vm.runInContext('session={account:{id:"one"}};projectId=1',context);
  const pending=vm.runInContext('editTicket(3)',context);
  assert.ok(vm.runInContext('foregroundOpen()',context));
  vm.runInContext('session={account:{id:"two"}}',context);resolve({id:3,agent:false});await pending;
  assert.equal(opened,0);assert.equal(vm.runInContext('ticketEditorPending',context),null);
});
