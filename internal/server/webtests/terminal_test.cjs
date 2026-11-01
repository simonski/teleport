const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const source=fs.readFileSync(path.join(__dirname,'../web/terminals.js'),'utf8');

test('all retained live and idle robots expose exact session and ticket identity',()=>{
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},escapeHTML:String});vm.runInContext(source,context);
  vm.runInContext(`localSessions=Array.from({length:5},(_,i)=>({id:'H'+i,state:i===4?'unknown':'running',ticketId:i?7:0,activity:i?'active':'idle'}))`,context);
  const html=vm.runInContext('sessionIcons()',context);assert.equal((html.match(/data-session=/g)||[]).length,5);assert.match(html,/#7/);assert.match(html,/Idle/);assert.match(html,/H4/);
});

for(const change of ['new foreground form','sign out'])test(`delayed session switcher preserves ${change}`,async()=>{
  let release,opened=0;
  const context=vm.createContext({session:{account:{id:'first'}},detailEpoch:1,pages:()=>new Promise(resolve=>release=resolve),byId:()=>null,foregroundOpen:()=>false,document:{addEventListener(){},querySelectorAll:()=>[]},setInterval(){},openDialog(){opened++;}});
  vm.runInContext(source,context);
  const pending=vm.runInContext('sessionSwitcher()',context);
  vm.runInContext(change==='sign out'?'session=null;detailEpoch++;':'detailEpoch++;',context);
  release([{id:'H1',state:'running'}]);await pending;
  assert.equal(opened,0);
});
