const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  const pending=[],requests=[],controls=[{disabled:false,isConnected:true}],selector={},status={textContent:''};let pages=0;
  const dialog={open:true,querySelectorAll:()=>controls,querySelector:()=>null};
  const context=vm.createContext({session:{account:{id:'one'}},loadGeneration:1,projectId:1,flow:{id:'flow-a',draftRevision:2,published:false},flows:[],flowDirty:false,flowWritable:()=>true,flowKey:flow=>flow.id+'@draft',drawWorkflow(){},activateFlow(){},crypto:{randomUUID:()=>String(requests.length+1)},byId:id=>id==='workflow-editor'?dialog:id==='flow-select'?selector:status,flowMessage:text=>status.textContent=text,api:(op,input)=>{requests.push(input);return new Promise((resolve,reject)=>pending.push({resolve,reject}));},pages:async()=>{pages++;return [];}});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/workflow_publishing.js'),'utf8'),context);
  return {context,pending,requests,dialog,status,pages:()=>pages,run:source=>vm.runInContext(source,context)};
}

test('publication retry after a lost response retains its exact durable request identity',async()=>{
  const f=fixture(),first=f.run('publishFlow()');
  f.pending[0].reject(new Error('Response lost'));await first;
  const second=f.run('publishFlow()');
  assert.deepEqual(f.requests[0],f.requests[1]);
  f.pending[1].resolve({id:'flow-a',version:1,published:true,draftRevision:0});await second;
  assert.equal(f.context.flow.version,1);assert.match(f.status.textContent,/Published v-1/);
});

test('a late publication cannot replace a closed editor or another selected workflow',async()=>{
  const f=fixture(),request=f.run('publishFlow()');
  f.dialog.open=false;f.run("flow={id:'different',draftRevision:1,published:false}");
  f.pending[0].resolve({id:'flow-a',version:1,published:true});await request;
  assert.equal(f.context.flow.id,'different');assert.equal(f.pages(),0);
});
