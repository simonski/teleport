const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const source=fs.readFileSync(path.join(__dirname,'../web/terminals.js'),'utf8');

test('agent control attaches its existing PTY or starts the default Codex terminal',async()=>{
  let rows=[{id:'H1',state:'running',ticketId:0,projectId:4,harness:'codex-pty'}],calls=[],opened=[];
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},session:{account:{id:'one'}},detailEpoch:1,projectId:4,projects:[{id:4}],roleAllowsWrite:()=>true,crypto:{randomUUID:()=> 'request'},pages:async()=>rows,api:async(op,input)=>{calls.push([op,input]);return op==='getLocalHarnesses'?{available:[{id:'codex'}]}:{id:'H2'};}});
  vm.runInContext(source,context);vm.runInContext('openTerminal=async id=>opened.push(id);refreshSessionRoster=async()=>{}',context);
  context.opened=opened;
  await vm.runInContext('chooseHarness()',context);assert.deepEqual(opened,['H1']);assert.equal(calls.length,0);
  rows=[];await vm.runInContext('chooseHarness()',context);assert.deepEqual(opened,['H1','H2']);
  assert.equal(calls[0][0],'openCodexTerminal');assert.equal(calls[0][1].projectId,4);assert.equal(calls[0][1].requestId,'request');assert.equal(calls[0][1].model,undefined);
});

for(const [kind,operation] of [['claude','openClaudeTerminal'],['copilot','openCopilotTerminal']])test(`agent control selects saved ${kind} default`,async()=>{
  let call;const opened=[];const context=vm.createContext({document:{addEventListener(){}},setInterval(){},session:{account:{id:'one'},welcomeSetup:{harness:kind}},detailEpoch:1,projectId:4,projects:[{id:4}],roleAllowsWrite:()=>true,crypto:{randomUUID:()=> 'request'},pages:async()=>[{id:'other',state:'running',projectId:5,harness:kind+'-pty'}],api:async(op,input)=>{call=[op,input];return {id:'new'}}});
  context.opened=opened;vm.runInContext(source,context);vm.runInContext('openTerminal=async id=>opened.push(id);refreshSessionRoster=async()=>{}',context);
  await vm.runInContext('chooseHarness()',context);assert.equal(call[0],operation);assert.equal(call[1].projectId,4);assert.deepEqual(opened,['new']);
});

test('shell control starts a distinct shell and opens its terminal',async()=>{
  const calls=[],opened=[];const context=vm.createContext({document:{addEventListener(){}},setInterval(){},session:{account:{id:'one'}},detailEpoch:1,projectId:4,projects:[{id:4}],roleAllowsWrite:()=>true,crypto:{randomUUID:()=> 'shell-request'},api:async(op,input)=>{calls.push([op,input]);return {id:'H4'}}});
  context.opened=opened;vm.runInContext(source,context);vm.runInContext('openTerminal=async id=>opened.push(id);refreshSessionRoster=async()=>{}',context);
  await vm.runInContext('startShell()',context);assert.equal(calls[0][0],'openShellSession');assert.equal(calls[0][1].projectId,4);assert.equal(calls[0][1].requestId,'shell-request');assert.deepEqual(opened,['H4']);
});

test('direct Codex control starts its own terminal',async()=>{
  const calls=[],opened=[];const context=vm.createContext({document:{addEventListener(){}},setInterval(){},session:{account:{id:'one'}},detailEpoch:1,projectId:4,projects:[{id:4}],roleAllowsWrite:()=>true,crypto:{randomUUID:()=> 'codex-request'},api:async(op,input)=>{calls.push([op,input]);return {id:'H5'}}});
  context.opened=opened;vm.runInContext(source,context);vm.runInContext('openTerminal=async id=>opened.push(id);refreshSessionRoster=async()=>{}',context);
  await vm.runInContext('startDirectCodex()',context);assert.equal(calls[0][0],'openCodexTerminal');assert.equal(calls[0][1].projectId,4);assert.equal(calls[0][1].requestId,'codex-request');assert.deepEqual(opened,['H5']);
});

test('structured Codex start rejects a direct-only discovery result',async()=>{
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},session:{account:{id:'one'}},detailEpoch:1,pages:async()=>[],api:async()=>({available:[{id:'codex',directOnly:true}]})});
  vm.runInContext(source,context);
  await assert.rejects(vm.runInContext('chooseHarness({id:7,projectId:4})',context),/direct Codex terminal/);
});

for(const [kind,operation] of [['claude','openClaudeTerminal'],['copilot','openCopilotTerminal']])test(`${kind} control starts its own terminal`,async()=>{
  const calls=[],opened=[];const context=vm.createContext({document:{addEventListener(){}},setInterval(){},session:{account:{id:'one'}},detailEpoch:1,projectId:4,projects:[{id:4}],roleAllowsWrite:()=>true,crypto:{randomUUID:()=> 'request'},api:async(op,input)=>{calls.push([op,input]);return {id:'H5'}}});
  context.opened=opened;vm.runInContext(source,context);vm.runInContext('openTerminal=async id=>opened.push(id);refreshSessionRoster=async()=>{}',context);
  await vm.runInContext(`startDirectHarness('${kind}')`,context);assert.equal(calls[0][0],operation);assert.equal(calls[0][1].projectId,4);assert.deepEqual(opened,['H5']);
});

test('only previously live direct terminals close their dialog on confirmed exit',()=>{
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){}});vm.runInContext(source,context);
  for(const harness of ['shell','codex-pty','claude-pty','copilot-pty'])assert.equal(vm.runInContext(`closeDirectTerminalAfterExit({harness:'${harness}',state:'running'},{state:'exited'})`,context),true);
  for(const test of ["closeDirectTerminalAfterExit({harness:'shell',state:'running'},{state:'unknown'})","closeDirectTerminalAfterExit({harness:'shell',state:'running'},{state:'stopping'})","closeDirectTerminalAfterExit({harness:'codex',state:'running'},{state:'exited'})","closeDirectTerminalAfterExit({harness:'shell',state:'exited'},{state:'exited'})"])assert.equal(vm.runInContext(test,context),false);
});

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
  const dialog={querySelectorAll:()=>[],addEventListener(){}};
  form.append=()=>{};const context=vm.createContext({session:{account:{id:'one',administrator:false}},detailEpoch:1,foregroundOpen:()=>false,escapeHTML:String,pages:async()=>[],document:{addEventListener(){},createElement:()=>({querySelectorAll:()=>[]})},setInterval(){},openDialog:(title,body)=>{html=body;return dialog},captureDialogCompletion:()=>({current:()=>true}),byId:id=>id==='cancel'?cancel:form,on(){}});
  vm.runInContext(source,context);
  vm.runInContext(`refreshSessionRoster=async()=>{};localSessions=[{id:'H1',harness:'codex',state:'exited',processId:42},{id:'H2',harness:'codex',state:'exited'},{id:'H3',harness:'codex',state:'unknown',processId:43}]`,context);
  await vm.runInContext('sessionSwitcher()',context);
  assert.match(html,/Ended sessions \(2\)/);assert.match(html,/Ownership unconfirmed/);assert.match(html,/data-stop-managed="H3" data-unknown="true"/);assert.match(html,/session PID 42/);assert.match(html,/session PID 43/);
});

test('recovery displays removed unresolved sessions without claiming exit',async()=>{
  let recoveryHTML='',requested;const form={append(section){recoveryHTML=section.innerHTML}};
  const context=vm.createContext({session:{account:{id:'one',administrator:false}},detailEpoch:1,foregroundOpen:()=>false,escapeHTML:String,pages:async(op,input)=>{requested=[op,input.includeHidden];return [{id:'H3',harness:'codex',projectId:4,state:'unknown',referenceRemoved:true,reference:'ref',errorCode:'native_exit_unverified'}]},document:{addEventListener(){},createElement:()=>({querySelectorAll:()=>[]})},setInterval(){},openDialog:()=>({querySelectorAll:()=>[],addEventListener(){}}),captureDialogCompletion:()=>({current:()=>true}),byId:id=>id==='cancel'?{}:form,on(){}});
  vm.runInContext(source,context);vm.runInContext('refreshSessionRoster=async()=>{}',context);await vm.runInContext('sessionSwitcher()',context);
  assert.deepEqual(requested,['listSessions',true]);assert.match(recoveryHTML,/H3/);assert.match(recoveryHTML,/native_exit_unverified/);assert.match(recoveryHTML,/Release reservation/);assert.match(recoveryHTML,/does not confirm/);
});

test('release confirmation submits the exact unresolved reference',async()=>{
  let html='',input;const form={},dialog={open:true,close(){this.open=false}};
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},escapeHTML:String,openDialog:(title,body)=>{html=body;return dialog},byId:()=>form,api:async(op,value)=>{assert.equal(op,'removeSessionReference');input=value},notice(){}});
  vm.runInContext(source,context);vm.runInContext('refreshSessionRoster=async()=>{};confirmReleaseSession({id:"H3",reference:"ref"})',context);
  assert.match(html,/cannot confirm/);await form.onsubmit({preventDefault(){}});assert.equal(input.id,'H3');assert.equal(input.expectedReference,'ref');assert.equal(input.releaseCapacity,true);assert.equal(input.confirm,true);
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

test('session-list stop returns to the list after the request completes',async()=>{
  const form={},events=[];let resolve;
  const context=vm.createContext({document:{addEventListener(){}},setInterval(){},crypto:{randomUUID:()=> 'request'},openDialog:()=>({classList:{remove(){}},close(){events.push('close')}}),byId:()=>form,captureDialogCompletion:()=>({follow:()=>true}),api:()=>new Promise(done=>resolve=done),notice:()=>events.push('notice')});
  vm.runInContext(source,context);context.events=events;vm.runInContext("sessionSwitcher=async()=>events.push('sessions');confirmTerminalAction('H1','Stop','Confirm',null,true)",context);
  const pending=form.onsubmit({preventDefault(){}});assert.deepEqual(events,['close']);resolve({state:'stopping'});await pending;assert.deepEqual(events,['close','notice','sessions']);
});

test('host stop returns to the session list without claiming the process exited',async()=>{
  const form={},events=[];const context=vm.createContext({document:{addEventListener(){}},setInterval(){},escapeHTML:String,openDialog:()=>({close(){events.push('close')}}),byId:()=>form,captureDialogCompletion:()=>({follow:()=>true}),api:async()=>({state:'signal_sent'}),notice:value=>events.push(value)});context.events=events;
  vm.runInContext(source,context);vm.runInContext("sessionSwitcher=async()=>events.push('sessions');confirmHostHarnessStop({pid:12,harness:'codex',identity:'identity'},false,true)",context);
  await form.onsubmit({preventDefault(){}});assert.deepEqual(events,['close','Stop requested; exit has not yet been confirmed.','sessions']);
});

test('open session list removes a session only after confirmed exit',async()=>{
  let tick,removed=false;const element={querySelector:()=>({dataset:{session:'H1'}}),remove(){removed=true}},list={innerHTML:'',querySelectorAll:()=>removed?[]:[element],querySelector:selector=>selector==='p'?null:removed?null:element};
  const form={append(){}},dialog={querySelectorAll:()=>[],querySelector:()=>list,addEventListener(){}};
  const context=vm.createContext({session:{account:{id:'one',administrator:false}},detailEpoch:1,foregroundOpen:()=>false,escapeHTML:String,pages:async()=>[],document:{addEventListener(){},createElement:()=>({querySelectorAll:()=>[]})},setInterval(fn){tick=fn;return 1},clearInterval(){},openDialog:()=>dialog,captureDialogCompletion:()=>({current:()=>true}),byId:id=>id==='cancel'?{}:form,on(){}});
  vm.runInContext(source,context);vm.runInContext("refreshSessionRoster=async()=>{};localSessions=[{id:'H1',state:'stopping',harness:'shell'}]",context);await vm.runInContext('sessionSwitcher()',context);
  await tick();assert.equal(removed,false);vm.runInContext("localSessions[0].state='exited'",context);await tick();assert.equal(removed,true);assert.match(list.innerHTML,/No active/);
});

test('host inspection returns to sessions on close without reopening a newer view',async()=>{
  for(const allowed of [true,false]){
    const form={},events=[];let onClose;const dialog={addEventListener(type,fn){if(type==='close')onClose=fn;},close(){events.push('close');onClose();}};
    const context=vm.createContext({document:{addEventListener(){}},setInterval(){},escapeHTML:String,openDialog:()=>dialog,byId:id=>id==='cancel'?{}:form,captureDialogCompletion:()=>({follow:()=>allowed}),showError(){}});context.events=events;
    vm.runInContext(source,context);vm.runInContext("sessionSwitcher=async()=>events.push('sessions');showHostHarnessDetails({pid:7,harness:'codex'})",context);
    form.onsubmit({preventDefault(){}});assert.deepEqual(events,allowed?['close','sessions']:['close']);
  }
});

test('unknown free consoles offer removal, with confirmation closing before the result',async()=>{
  let html='',resolve;const form={},events=[];
  form.append=()=>{};const context=vm.createContext({session:{account:{id:'one',administrator:false}},detailEpoch:1,foregroundOpen:()=>false,escapeHTML:String,pages:async()=>[],document:{addEventListener(){},createElement:()=>({querySelectorAll:()=>[]})},setInterval(){},openDialog:(title,body)=>{html=body;return {querySelectorAll:()=>[],addEventListener(){},close(){events.push('close')}}},captureDialogCompletion:()=>({current:()=>true,follow:()=>true}),byId:id=>id==='cancel'?{}:form,on(){},api:(op,input)=>{events.push([op,input.id,input.expectedReference,input.confirm]);return new Promise(done=>resolve=done)},notice:value=>events.push(value)});
  vm.runInContext(source,context);vm.runInContext("refreshSessionRoster=async()=>{};localSessions=[{id:'H3',harness:'codex',state:'unknown',reference:'identity',stopRequested:true}]",context);
  await vm.runInContext('sessionSwitcher()',context);assert.match(html,/data-remove-reference="H3"/);
  vm.runInContext("confirmRemoveSessionReference(localSessions[0])",context);assert.match(html,/No process will be stopped/);assert.match(html,/quarantined/);
  const pending=form.onsubmit({preventDefault(){}});assert.deepEqual(events,['close',['removeSessionReference','H3','identity',true]]);resolve({removed:true});await pending;
  assert.equal(vm.runInContext('localSessions.length',context),0);assert.equal(events[2],'Session reference removed; no process was stopped.');
});
