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

test('unknown sessions offer a recorded stop intent and ended sessions are grouped',async()=>{
  let html='';const form={},cancel={};
  const dialog={querySelectorAll:()=>[]};
  const context=vm.createContext({session:{account:{id:'one',administrator:false}},detailEpoch:1,foregroundOpen:()=>false,escapeHTML:String,document:{addEventListener(){}},setInterval(){},openDialog:(title,body)=>{html=body;return dialog},captureDialogCompletion:()=>({current:()=>true}),byId:id=>id==='cancel'?cancel:form,on(){}});
  vm.runInContext(source,context);
  vm.runInContext(`refreshSessionRoster=async()=>{};localSessions=[{id:'H1',harness:'codex',state:'exited'},{id:'H2',harness:'codex',state:'exited'},{id:'H3',harness:'codex',state:'unknown'}]`,context);
  await vm.runInContext('sessionSwitcher()',context);
  assert.match(html,/Ended sessions \(2\)/);assert.match(html,/Ownership unconfirmed/);assert.match(html,/data-stop-managed="H3" data-unknown="true"/);
});

test('stop failures are shown inside the confirmation instead of behind it',async()=>{
  const form={},status={textContent:''};let notices=0;
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},crypto:{randomUUID:()=> 'request'},openDialog:()=>({classList:{remove(){}}}),byId:id=>id==='editor-form'?form:status,captureDialogCompletion:()=>({current:()=>true}),api:async()=>{throw new Error('Ownership is unknown')},showError:()=>notices++});
  vm.runInContext(source,context);vm.runInContext("confirmTerminalAction('H3','Stop harness','Confirm')",context);
  await form.onsubmit({preventDefault(){}});
  assert.equal(status.textContent,'Ownership is unknown');assert.equal(notices,0);
});

test('non-Teleport stop failures remain visible in the confirmation',async()=>{
  const form={},status={textContent:''};let notices=0;
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},escapeHTML:String,openDialog:()=>({}),byId:id=>id==='editor-form'?form:status,captureDialogCompletion:()=>({current:()=>true}),api:async()=>{throw new Error('Process changed; refresh')},showError:()=>notices++});
  vm.runInContext(source,context);vm.runInContext("confirmHostHarnessStop({pid:12,harness:'codex',identity:'identity'})",context);
  await form.onsubmit({preventDefault(){}});
  assert.equal(status.textContent,'Process changed; refresh');assert.equal(notices,0);
});
