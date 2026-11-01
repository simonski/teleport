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
