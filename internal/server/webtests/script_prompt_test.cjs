const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {TextEncoder}=require('node:util');
function fixture(){
  const context=vm.createContext({document:{addEventListener(){}},TextEncoder});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/scripts.js'),'utf8'),context);
  return {context,run:source=>vm.runInContext(source,context),parse:line=>{context.line=line;return vm.runInContext('parseScriptCommand(line,[{operation:"getTicket"},{operation:"listProjects"}])',context);}};
}
test('console parses tk JSON strictly as data and preserves JavaScript as explicit source',async()=>{
  const f=fixture(),argument=JSON.parse('{"id":3,"__proto__":{"literal":true}}');argument.text='\"); globalThis.injected=true; //';const line='tk getTicket --data '+JSON.stringify(argument);
  const parsed=f.parse(line);let captured;
  f.context.tk=async(op,args)=>{captured={op,args};return 3;};
  assert.equal(await f.run('(async()=>{'+parsed.source+'})()'),3);
  assert.equal(captured.op,'getTicket');assert.deepEqual(JSON.parse(JSON.stringify(captured.args)),JSON.parse(JSON.stringify(argument)));assert.equal(f.context.injected,undefined);
  assert.equal(Object.getPrototypeOf(captured.args).literal,undefined);assert.equal(Object.hasOwn(captured.args,'__proto__'),true);
  assert.equal(f.parse('1 + 2').mode,'console');assert.equal(f.parse('tk("listProjects", {})').mode,'console');
  assert.equal(f.parse("tk getTicket --data '{\"id\":3}'").mode,'script');
});
test('invalid command syntax and undeclared operations never produce executable source',()=>{
  const f=fixture();for(const line of ['tk getTicket --data','tk getTicket []','tk getTicket null','tk getTicket {"id":3};alert(1)','tk getTicket --other {}','tk logout','tk listProjects | sh'])assert.throws(()=>f.parse(line),undefined,line);
  assert.equal(f.parse('help').local,'help');assert.equal(f.parse('clear').local,'clear');assert.equal(f.parse('tk tools').local,'tools');assert.equal(f.parse(' '),null);
});
test('command history is bounded and reset on account invalidation',()=>{
  const f=fixture();f.run('for(let i=0;i<100;i++)rememberScriptCommand("command "+i)');
  assert.equal(f.run('scriptCommands.length'),50);assert.equal(f.run('scriptCommands[0]'),'command 50');
  f.context.clearTimeout=()=>{};f.run('resetScriptConsole()');assert.equal(f.run('scriptCommands.length'),0);assert.equal(f.run('scriptEntries.length'),0);
});
for(const change of ['actor','foreground'])test('a delayed console open preserves a newer '+change,async()=>{
  const f=fixture();let releaseBindings,releaseRuns,opened=0;
  Object.assign(f.context,{session:{account:{id:'one'}},detailEpoch:1,foregroundOpen:()=>false,api:()=>new Promise(resolve=>releaseBindings=resolve),pages:()=>new Promise(resolve=>releaseRuns=resolve),openDialog:()=>opened++});
  const pending=f.run('openScriptConsole()');
  if(change==='actor')f.run('session={account:{id:"two"}}');else f.run('detailEpoch++');
  releaseBindings({version:1,bindings:[]});releaseRuns([{id:'J1',creator:'one',createdAt:'now'}]);await pending;
  assert.equal(opened,0);
});
