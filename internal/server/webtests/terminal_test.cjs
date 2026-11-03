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
  vm.runInContext(`refreshSessionRoster=async()=>{};localSessions=[{id:'H1',harness:'codex',state:'exited',processId:42},{id:'H2',harness:'codex',state:'exited'},{id:'H3',harness:'codex',state:'unknown',processId:43}]`,context);
  await vm.runInContext('sessionSwitcher()',context);
  assert.match(html,/Ended sessions \(2\)/);assert.match(html,/Ownership unconfirmed/);assert.match(html,/data-stop-managed="H3" data-unknown="true"/);assert.match(html,/session PID 42/);assert.match(html,/session PID 43/);
});

test('stop confirmation closes immediately and reports the eventual error',async()=>{
  const form={},events=[];let reject;
  const dialog={classList:{remove(){}},close(){events.push('close')}};
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},crypto:{randomUUID:()=> 'request'},openDialog:()=>dialog,byId:()=>form,captureDialogCompletion:()=>({follow:()=>true}),api:()=>new Promise((resolve,fail)=>{reject=fail}),showError:error=>events.push(error.message)});
  vm.runInContext(source,context);vm.runInContext("confirmTerminalAction('H3','Stop harness','Confirm')",context);
  const pending=form.onsubmit({preventDefault(){}});assert.deepEqual(events,['close']);reject(new Error('Ownership is unknown'));await pending;
  assert.deepEqual(events,['close','Ownership is unknown']);
});

test('successful stop closes immediately and does not reopen the terminal',async()=>{
  const form={},events=[];let resolve;
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},crypto:{randomUUID:()=> 'request'},openDialog:()=>({classList:{remove(){}},close(){events.push('close')}}),byId:()=>form,captureDialogCompletion:()=>({follow:()=>true}),api:()=>new Promise(done=>{resolve=done}),notice:value=>events.push(value),openTerminal:()=>events.push('open')});
  vm.runInContext(source,context);vm.runInContext("confirmTerminalAction('H1','Stop harness','Confirm')",context);
  const pending=form.onsubmit({preventDefault(){}});assert.deepEqual(events,['close']);resolve({});await pending;
  assert.deepEqual(events,['close','Stop requested; exit has not yet been confirmed.']);
});

test('late stop results do not notify over a newer view',async()=>{
  const form={},events=[];const context=vm.createContext({document:{addEventListener(){}},setInterval(){},crypto:{randomUUID:()=> 'request'},openDialog:()=>({classList:{remove(){}},close(){events.push('close')}}),byId:()=>form,captureDialogCompletion:()=>({follow:()=>false}),api:async()=>{throw new Error('late')},showError:()=>events.push('toast')});
  vm.runInContext(source,context);vm.runInContext("confirmTerminalAction('H3','Stop harness','Confirm')",context);await form.onsubmit({preventDefault(){}});assert.deepEqual(events,['close']);
});

test('non-Teleport stop confirmation closes before an error is shown',async()=>{
  const form={},events=[];let reject;
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},escapeHTML:String,openDialog:()=>({close(){events.push('close')}}),byId:()=>form,captureDialogCompletion:()=>({follow:()=>true}),api:()=>new Promise((resolve,fail)=>{reject=fail}),showError:error=>events.push(error.message)});
  vm.runInContext(source,context);vm.runInContext("confirmHostHarnessStop({pid:12,harness:'codex',identity:'identity'})",context);
  const pending=form.onsubmit({preventDefault(){}});assert.deepEqual(events,['close']);reject(new Error('Process changed; refresh'));await pending;assert.deepEqual(events,['close','Process changed; refresh']);
});

test('unknown free consoles offer removal, with confirmation closing before the result',async()=>{
  let html='',resolve;const form={},events=[];
  const context=vm.createContext({session:{account:{id:'one',administrator:false}},detailEpoch:1,foregroundOpen:()=>false,escapeHTML:String,document:{addEventListener(){}},setInterval(){},openDialog:(title,body)=>{html=body;return {querySelectorAll:()=>[],close(){events.push('close')}}},captureDialogCompletion:()=>({current:()=>true,follow:()=>true}),byId:id=>id==='cancel'?{}:form,on(){},api:(op,input)=>{events.push([op,input.id,input.expectedReference,input.confirm]);return new Promise(done=>resolve=done)},notice:value=>events.push(value)});
  vm.runInContext(source,context);vm.runInContext("refreshSessionRoster=async()=>{};localSessions=[{id:'H3',harness:'codex',state:'unknown',reference:'identity',stopRequested:true}]",context);
  await vm.runInContext('sessionSwitcher()',context);assert.match(html,/data-remove-reference="H3"/);
  vm.runInContext("confirmRemoveSessionReference(localSessions[0])",context);assert.match(html,/No process will be stopped/);assert.match(html,/quarantined/);
  const pending=form.onsubmit({preventDefault(){}});assert.deepEqual(events,['close',['removeSessionReference','H3','identity',true]]);resolve({removed:true});await pending;
  assert.equal(vm.runInContext('localSessions.length',context),0);assert.equal(events[2],'Session reference removed; no process was stopped.');
});
