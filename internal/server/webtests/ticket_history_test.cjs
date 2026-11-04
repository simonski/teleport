const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

test('ticket history compares changed text and escapes both revisions',()=>{
  const context=vm.createContext({setInterval(){},document:{addEventListener(){}}});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/workspace.js'),'utf8'),context);
  context.history=[{version:1,action:'create',actor:'alice',snapshot:{title:'<old>',description:'before',status:'Todo'}},{version:2,action:'spellcheck',actor:'alice',snapshot:{title:'new',description:'<after>',status:'Todo'}},{version:3,action:'priority',actor:'alice',snapshot:{title:'new',description:'<after>',status:'Todo'}}];
  const html=vm.runInContext('ticketHistoryHTML(history)',context);
  assert.equal((html.match(/Compare text/g)||[]).length,1);
  assert.match(html,/&lt;old&gt;/);
  assert.match(html,/&lt;after&gt;/);
  assert.doesNotMatch(html,/<after>/);
});
