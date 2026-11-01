const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  const reads=[],calls=[],drawn=[],selector={},host={isConnected:true,querySelectorAll:()=>[]};
  const context=vm.createContext({session:{account:{id:'one'}},loadGeneration:1,projectId:1,view:'Workflows',flow:null,flows:[],flowDirty:false,flowDrag:null,workflowPublicationPending:null,workflowEditor(){},flowMessage(){},saveFlow(){},copyFlowForm(){},activateFlow(){},drawWorkflow(){},flowKey:flow=>flow.id+'@'+(flow.published?flow.version:'draft'),currentProject:()=>({agentWorkflow:'agent@1'}),pages:()=>new Promise(resolve=>reads.push(resolve)),api:()=>new Promise(resolve=>calls.push(resolve)),byId:id=>id==='workflow-editor'?host:selector});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/workflow_view.js'),'utf8'),context);
  context.record=flow=>drawn.push(flow);vm.runInContext('drawWorkflow=()=>record(flow)',context);
  return {context,reads,calls,drawn,run:source=>vm.runInContext(source,context)};
}

test('a late Workflows load cannot populate another account',async()=>{
  const f=fixture(),request=f.run('mountWorkflows()');
  f.run("resetWorkflowView();session={account:{id:'two'}};loadGeneration++");
  f.reads[0]([{id:'private',published:false}]);await request;
  assert.equal(f.context.flow,null);assert.equal(f.drawn.length,0);assert.equal(f.run('workflowLoading'),false);
});

test('an older workflow load cannot replace a newer selected definition',async()=>{
  const f=fixture(),first=f.run("mountWorkflows('first@draft')"),second=f.run("mountWorkflows('second@draft')");
  f.reads[1]([{id:'second',published:false}]);await second;
  f.reads[0]([{id:'first',published:false}]);await first;
  assert.equal(f.context.flow.id,'second');assert.equal(f.drawn.length,1);
});

test('a delayed workflow mutation cannot replace another account editor',async()=>{
  const f=fixture();f.run("flow={id:'first',draftRevision:1,published:false}");
  const request=f.run("changeWorkflow('saveWorkflow',{},'Saved')");
  f.run("resetWorkflowView();session={account:{id:'two'}};loadGeneration++;flow={id:'second',draftRevision:1,published:false}");
  f.calls[0]({id:'first',draftRevision:2,published:false});await request;
  assert.equal(f.context.flow.id,'second');assert.equal(f.drawn.length,0);assert.equal(f.run('workflowMutation'),null);
});

test('feedback is bound to the exact workflow revision and reset with its actor',()=>{
  const f=fixture();f.run("flow={id:'first',draftRevision:2,published:false};flowMessage('Draft saved')");
  assert.equal(f.run('workflowFeedback.key'),'first@draft');assert.equal(f.run('workflowFeedback.revision'),2);
  f.run('resetWorkflowView()');assert.equal(f.run('workflowFeedback'),null);
});

test('a discard reload cannot overwrite edits made after that reload started',async()=>{
  const f=fixture();f.run("flow={id:'first',draftRevision:2,published:false,title:'Saved'}");
  const request=f.run("mountWorkflows('first@draft')");
  f.run("flow.title='Newer local input';flowDirty=true");
  f.reads[0]([{id:'first',draftRevision:2,published:false,title:'Saved'}]);await request;
  assert.equal(f.context.flow.title,'Newer local input');assert.equal(f.context.flowDirty,true);assert.equal(f.drawn.length,0);
});
