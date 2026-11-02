const {test}=require('node:test'),assert=require('node:assert/strict'),vm=require('node:vm'),fs=require('node:fs'),path=require('node:path');
function fixture(file,values={}){const context=vm.createContext({console,...values});vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/'+file),'utf8'),context);return source=>vm.runInContext(source,context);}
test('workflow proximity creates only unambiguous, unused routes and preserves human gates',()=>{
  const run=fixture('workflow_inventory.js');
  run(`definition={family:'agent',phases:[{id:'a',role:'Doing',agentic:true},{id:'b',role:'Review',agentic:true}],layout:{a:{x:0,y:0},b:{x:320,y:0}},rules:[]}`);
  assert.equal(run(`workflowNearbyConnection(definition,'b').outcome`),'success');
  run(`definition.rules=[{source:'a',target:'b',outcome:'success',conditions:{result:'ok'}}]`);assert.equal(run(`workflowNearbyConnection(definition,'b')`),null);
  run(`definition.rules=[];definition.phases[1].role='Done'`);assert.equal(run(`workflowNearbyConnection(definition,'b')`),null);
  run(`definition.phases[0].role='Approval'`);assert.equal(run(`workflowNearbyConnection(definition,'b').outcome`),'approve');
  run(`definition.layout.b.y=300`);assert.equal(run(`workflowNearbyConnection(definition,'b')`),null);
  run(`definition.layout.b={x:0,y:130}`);assert.equal(run(`workflowNearbyConnection(definition,'b').outcome`),'approve');
  run(`definition={family:'human',phases:[{id:'a',role:'Doing'},{id:'b',role:'Done'},{id:'c',role:'Doing'}],layout:{a:{x:0,y:-10},b:{x:320,y:0},c:{x:0,y:10}},rules:[]}`);assert.equal(run(`workflowNearbyConnection(definition,'b')`),null);
});
test('Tickets groups each story under its own epic and pinned workflow without moving source data',()=>{
  const run=fixture('ticket_list.js');run(`rows=[{id:1,kind:'epic',projectId:1},{id:2,kind:'task',parentId:1,projectId:1,workflow:'w@1',status:'Todo'},{id:3,kind:'task',parentId:1,projectId:1,workflow:'w@2',status:'Todo'},{id:4,kind:'task',parentId:1,projectId:1,workflow:'w@1',status:'Cancelled'}]`);
  assert.equal(run(`ticketListGroups(rows,'w@1')[0].tasks.length`),1);assert.equal(run(`ticketListGroups(rows,'w@1')[0].epic.id`),1);assert.equal(run('rows.length'),4);
});
test('summary pies expose exact counts and do not divide by zero',()=>{
  const run=fixture('project_home.js',{escapeHTML:String});const empty=run(`homeDonut('Completion',[], '0%', 'No tasks')`);assert.ok(empty.includes('No tasks'));assert.ok(!empty.includes('NaN'));
  assert.ok(run(`homeDonut('Completion',[{label:'Done',count:2},{label:'Todo',count:3}],'40%','2 of 5')`).includes('Done 2, Todo 3'));
});
