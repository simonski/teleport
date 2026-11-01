const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  const messages=[],draws=[],context=vm.createContext({flow:{phases:[{id:'a',role:'Todo'},{id:'b',role:'Doing'},{id:'c',role:'Done'}]},flowDirty:false,flowDrag:null,session:{account:{id:'one'}},loadGeneration:1,view:'Workflows',workflowMutation:null,workflowPublicationPending:null,workflowLoading:false,flowWritable:()=>true,drawWorkflow:()=>draws.push(true),flowMessage:text=>messages.push(text),byId:()=>null,document:{querySelector:()=>null,addEventListener(){},body:{hasPointerCapture:()=>false}},window:{addEventListener(){}}});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/workflow_order.js'),'utf8'),context);
  return {context,messages,draws,run:source=>vm.runInContext(source,context)};
}

test('lane reordering preserves stable records and refuses malformed permutations',()=>{
  const f=fixture();f.run('original=flow.phases;first=flow.phases[0];commitWorkflowLaneOrder(["c","a","b"],"a")');
  assert.deepEqual(Array.from(f.context.flow.phases,phase=>phase.id),['c','a','b']);
  assert.equal(f.run('flow.phases[1]===first'),true);assert.equal(f.context.flowDirty,true);
  assert.deepEqual(Array.from(f.context.original,phase=>phase.id),['a','b','c']);
  f.run('commitWorkflowLaneOrder(["a","a","c"],"a")');
  assert.equal(f.draws.length,1);
});

test('lane keyboard moves stop at bounds and cannot change a pending editor',()=>{
  const f=fixture();f.run('moveWorkflowLaneBy(0,-1)');assert.equal(f.context.flowDirty,false);
  f.run('moveWorkflowLaneBy(0,1)');assert.deepEqual(Array.from(f.context.flow.phases,phase=>phase.id),['b','a','c']);
  f.run('workflowMutation={};moveWorkflowLaneBy(1,1)');assert.equal(f.draws.length,1);
});

test('cancelling a preview restores rows and retains earlier unsaved edits',()=>{
  const f=fixture(),appended=[];
  const rows=[0,1,2].map(index=>({index,getBoundingClientRect:()=>({y:index*100}),getAnimations:()=>[],classList:{remove(){}}}));
  f.context.rows=rows;f.context.list={isConnected:true,append:row=>appended.push(row.index)};
  f.run('flowDirty=true;flowDrag=workflowLaneDrag={source:flow,actor:"one",generation:1,list,rows,original:["a","b","c"],order:["b","a","c"],row:rows[0],grip:{focus(){}},id:"a",dragging:true};cancelWorkflowLaneDrag()');
  assert.deepEqual(appended,[0,1,2]);assert.equal(f.context.flowDirty,true);
  assert.deepEqual(Array.from(f.context.flow.phases,phase=>phase.id),['a','b','c']);assert.equal(f.context.flowDrag,null);
});

test('a drag from a previous actor cannot commit or focus the replacement editor',()=>{
  const f=fixture();f.run('flowDrag=workflowLaneDrag={source:flow,actor:"previous",generation:1,list:{isConnected:true},row:{classList:{remove(){}}},grip:{focus(){throw Error("stale focus")}},order:["c","a","b"],id:"a",dragging:true};finishWorkflowLaneDrag(true)');
  assert.equal(f.draws.length,0);assert.equal(f.context.flowDirty,false);assert.equal(f.context.flowDrag,null);
});
