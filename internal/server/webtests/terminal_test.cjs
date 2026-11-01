const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const source=fs.readFileSync(path.join(__dirname,'../web/terminals.js'),'utf8');

for(const change of ['new foreground form','sign out'])test(`delayed session switcher preserves ${change}`,async()=>{
  let release,opened=0;
  const context=vm.createContext({session:{account:{id:'first'}},detailEpoch:1,pages:()=>new Promise(resolve=>release=resolve),byId:()=>null,foregroundOpen:()=>false,document:{addEventListener(){},querySelectorAll:()=>[]},setInterval(){},openDialog(){opened++;}});
  vm.runInContext(source,context);
  const pending=vm.runInContext('sessionSwitcher()',context);
  vm.runInContext(change==='sign out'?'session=null;detailEpoch++;':'detailEpoch++;',context);
  release([{id:'H1',state:'running'}]);await pending;
  assert.equal(opened,0);
});
