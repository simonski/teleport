const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const source=fs.readFileSync(path.join(__dirname,'../web/welcome.js'),'utf8');

test('a delayed Welcome read cannot enter another account or replace its projects',async()=>{
  let release,opened=0;
  const context=vm.createContext({session:{account:{id:'first'},welcomeSetup:{completed:false}},projects:[{name:'second private project'}],api:()=>new Promise(resolve=>release=resolve),pages:async()=>[{name:'first private project'}],foregroundOpen:()=>false,openDialog:()=>opened++});
  vm.runInContext(source,context);
  const pending=vm.runInContext('openWelcome()',context);
  context.session={account:{id:'second'},welcomeSetup:{completed:false}};
  release({setup:{completed:false}});
  await pending;
  assert.equal(opened,0);
  assert.equal(context.projects[0].name,'second private project');
});

test('dismissed setup stays dismissed until explicitly resumed or a new sign-in',()=>{
  let opened=0;
  const context=vm.createContext({session:{account:{id:'first'},welcomeSetup:{completed:false}},foregroundOpen:()=>false,showError(){}});
  vm.runInContext(source,context);
  context.openWelcome=async()=>{opened++;};
  vm.runInContext('maybeWelcome();maybeWelcome()',context);
  assert.equal(opened,1);
  vm.runInContext('resetWelcome();maybeWelcome()',context);
  assert.equal(opened,2);
  context.session.welcomeSetup.completed=true;
  vm.runInContext('resetWelcome();maybeWelcome()',context);
  assert.equal(opened,2);
});

async function projectForm(step=1){
  const requests=[],timers=new Map(),listeners={},elements={};let timerId=0,close;
  for(const [name,value] of Object.entries({projectId:'0',projectName:'',projectKey:''}))elements[name]={name,value,disabled:false,isConnected:true,setCustomValidity(value){this.validation=value;}};
  const status={textContent:'',classList:{toggle(){},add(){}}},submit={},form={elements:{namedItem:name=>elements[name]},querySelector:()=>submit,querySelectorAll:()=>[],addEventListener:(name,fn)=>listeners[name]=fn};
  const nodes={'editor-form':form,'welcome-status':{classList:{toggle(){}}},'welcome-project-status':status,'welcome-new-project':{},cancel:{before(){}}};
  const context=vm.createContext({
    session:{account:{id:'one'}},projects:[],foregroundOpen:()=>false,escapeHTML:value=>value,
    api:async(op,input)=>op==='getWelcomeSetup'?{setup:{step,projectId:0,projectKey:'',version:0},harnesses:[]}:new Promise((resolve,reject)=>requests.push({op,input,resolve,reject})),
    pages:async()=>[],byId:id=>nodes[id],openDialog:()=>({addEventListener:(name,fn)=>close=fn,close:()=>close()}),
    projectId:0,workspace:async()=>{},
    document:{createElement:()=>({})},
    setTimeout:fn=>{timers.set(++timerId,fn);return timerId;},clearTimeout:id=>timers.delete(id)
  });
  vm.runInContext(source,context);await vm.runInContext('openWelcome()',context);
  async function type(name,value){elements[name].value=value;listeners.input({target:elements[name]});const [id,fn]=timers.entries().next().value;timers.delete(id);fn();await Promise.resolve();}
  async function answer(index,result){requests[index].resolve(result);await new Promise(resolve=>setImmediate(resolve));}
  return {elements,requests,status,type,answer,close:()=>close(),submit:()=>form.onsubmit({preventDefault(){}}),change:()=>listeners.change({target:{name:'attention'}}),timers};
}

test('a delayed project availability response cannot overwrite a newer name or edited prefix',async()=>{
  const h=await projectForm();
  await h.type('projectName','Teleport');
  await h.type('projectName','Rocket');
  await h.answer(1,{nameAvailable:false,keyAvailable:true,suggestedKey:'tele'});
  assert.equal(h.elements.projectKey.value,'');
  assert.notEqual(h.status.textContent,'Project name already exists');
  await h.answer(2,{nameAvailable:true,keyAvailable:true,suggestedKey:'rock'});
  assert.equal(h.elements.projectKey.value,'rock');
  await h.type('projectKey','custom');
  await h.answer(h.requests.findIndex((r,i)=>i>2&&r.op==='checkProjectAvailability'),{nameAvailable:true,keyAvailable:true,suggestedKey:'roc1'});
  assert.equal(h.elements.projectKey.value,'custom');
});

test('a dismissed project check cannot update or save the closed wizard',async()=>{
  const h=await projectForm();await h.type('projectName','Teleport');h.close();
  await h.answer(1,{nameAvailable:true,keyAvailable:true,suggestedKey:'tele'});
  assert.equal(h.elements.projectKey.value,'');
  assert.equal(h.requests.filter(r=>r.op==='saveWelcomeSetup').length,0);
});

test('lost completion retries without allowing delayed autosaves to rewrite the completed draft',async()=>{
  const h=await projectForm(5),first=h.submit();
  await new Promise(resolve=>setImmediate(resolve));
  h.change();
  h.requests[0].reject(new Error('Lost response'));await first;
  h.change();
  assert.equal(h.timers.size,0);
  const retry=h.submit();await new Promise(resolve=>setImmediate(resolve));
  assert.deepEqual(h.requests.map(r=>r.op),['completeWelcomeSetup','completeWelcomeSetup']);
  h.requests[1].resolve({completed:true,projectId:1});await retry;
});
