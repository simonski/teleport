const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
function fixture(){
  const context=vm.createContext({document:{addEventListener(){}}});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/scripts.js'),'utf8'),context);
  return {context,run:source=>vm.runInContext(source,context),parse:line=>{context.line=line;return vm.runInContext('parseTkCommand(line)',context);}};
}
test('tk command arguments preserve quoted data without executing it',()=>{
  const f=fixture();
  const line=`tk api getTicket -data '{"id":3,"text":"globalThis.injected=true"}'`;
  assert.deepEqual(Array.from(f.parse(line).args),['tk','api','getTicket','-data','{"id":3,"text":"globalThis.injected=true"}']);
  assert.deepEqual(Array.from(f.parse('tk ticket ls -name "two words"').args),['tk','ticket','ls','-name','two words']);
  assert.equal(f.context.injected,undefined);
  assert.throws(()=>f.parse('tk ticket ls "unfinished'),/quoted argument/);
  assert.throws(()=>f.parse('globalThis.injected=true'),/Enter a tk command/);
  assert.equal(f.parse('clear').local,'clear');
  assert.equal(f.parse('exit').local,'exit');
  assert.equal(f.parse(' '),null);
});
test('results render as plain terminal output or formatted JSON',()=>{
  const f=fixture();
  assert.equal(f.run('formatTkResult({items:[]},false)'),'No items.');
  assert.equal(f.run('formatTkResult({b:2,a:"<script>"},false)'),'a="<script>"  b=2');
  assert.equal(f.run('formatTkResult({id:3},true)'),'{\n  "id": 3\n}');
});
test('command history is bounded and reset on account invalidation',()=>{
  const f=fixture();f.run('for(let i=0;i<100;i++)rememberScriptCommand("command "+i)');
  assert.equal(f.run('scriptCommands.length'),50);assert.equal(f.run('scriptCommands[0]'),'command 50');
  f.run('resetScriptConsole()');assert.equal(f.run('scriptCommands.length'),0);assert.equal(f.run('scriptEntries.length'),0);
});
