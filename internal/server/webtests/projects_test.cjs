const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  const pending=[],drawn=[],status={isConnected:true,classList:{remove(){},add(){}}};
  const context=vm.createContext({session:{account:{id:'one'}},loadGeneration:1,view:'Projects',projects:[],byId:()=>status,pages:()=>new Promise(resolve=>pending.push(resolve)),escapeHTML:String});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/projects.js'),'utf8'),context);
  context.record=value=>drawn.push(value);
  vm.runInContext('drawProjectDirectory=()=>record(projectDirectory)',context);
  return {context,pending,drawn,run:source=>vm.runInContext(source,context)};
}

test('a delayed project directory cannot populate another account cache',async()=>{
  const f=fixture(),request=f.run('loadProjectDirectory()');
  f.run("resetProjectDirectory();session={account:{id:'two'}};loadGeneration++");
  f.pending[0]([{id:1,name:'Private old project'}]);await request;
  assert.equal(f.run('projectDirectory'),null);assert.equal(f.drawn.length,0);
});

test('a stale directory refresh cannot replace a newer summary',async()=>{
  const f=fixture(),first=f.run('loadProjectDirectory()'),second=f.run('loadProjectDirectory()');
  f.pending[1]([{id:1,name:'Current'}]);await second;
  f.pending[0]([{id:1,name:'Stale'}]);await first;
  assert.equal(f.run('projectDirectory[0].name'),'Current');assert.equal(f.drawn.length,1);
});

test('cached directory rows are filtered against current accessible project scope',()=>{
  const f=fixture();
  f.run("projects=[{id:2}];projectDirectory=[{id:1,name:'Revoked project'}]");
  assert.equal(f.run("projectDirectoryContent().includes('Revoked project')"),false);
});
