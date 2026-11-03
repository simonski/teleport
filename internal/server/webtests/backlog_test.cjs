const {test}=require('node:test'),assert=require('node:assert/strict'),vm=require('node:vm'),fs=require('node:fs'),path=require('node:path');
function fixture(){const context=vm.createContext({roleAllowsWrite:()=>true});vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/backlog.js'),'utf8'),context);return source=>vm.runInContext(source,context);}
test('backlog cards retain exact hierarchy, indent depth, personal sibling order and filtered ancestors',()=>{
  const run=fixture();run(`records=[{id:1,kind:'goal',parentId:0,projectId:1,title:'Goal'},{id:2,kind:'epic',parentId:1,projectId:1,title:'Epic'},{id:3,kind:'task',parentId:2,projectId:1,title:'First'},{id:4,kind:'task',parentId:2,projectId:1,title:'Second'}]`);
  assert.equal(run(`backlogRows(records,[],['ticket:4','ticket:3'])[2].ticket.id`),4);
  assert.equal(run(`backlogRows(records,[],[],'task','Second').map(row=>row.depth).join(',')`),'0,1,2');
  assert.equal(run(`backlogRows(records,[],[],'task','Second')[1].context`),true);
  assert.equal(run(`records[3].parentId`),2);
});
test('story drops adopt another story’s epic and never create nested tasks or cross-project relations',()=>{
  const run=fixture();run(`source={key:'ticket:3',kind:'task',projectId:1,parentId:2,ticket:{id:3,kind:'task',parentId:2}},target={key:'ticket:5',kind:'task',projectId:1,parentId:4,ticket:{id:5,kind:'task',parentId:4}}`);
  assert.equal(run(`backlogTarget(source,target,'inside').parentId`),4);
  run('target.projectId=2');assert.equal(run(`backlogTarget(source,target,'inside')`),null);
  run(`target={key:'ticket:1',projectId:1,ticket:{id:1,kind:'goal',parentId:0}}`);assert.equal(run(`backlogTarget(source,target,'inside')`),null);
});
test('locked work remains reorderable while its parent controls reject retained or active work',()=>{
  const run=fixture();for(const field of ["status:'Doing'","status:'Todo',active:true","status:'Todo',claim:'held'","status:'Todo',delivery:{}","status:'Todo',protected:true"]){assert.equal(run(`backlogCanMove({projectId:1,${field}})`),false);}
  assert.equal(run(`backlogCanMove({projectId:1,status:'Todo'})`),true);
  assert.equal(run(`backlogReorder(['a','b','c'],'c','a',false).join(',')`),'c,a,b');
});
