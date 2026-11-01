const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  let poll,release;
  const context=vm.createContext({document:{addEventListener(){},documentElement:{dataset:{}},dispatchEvent(){}},Event,
    setInterval(fn){poll=fn;},api(operation){return operation==='getSession'?new Promise(resolve=>release=resolve):Promise.resolve({items:[]});},
    graphInteracting:false,root:{querySelector:()=>null},ideaHidden:false,ideas:[],flowKey:()=>'',maybeWelcome(){}});
  for(const file of ['presentation.js','workspace.js'])vm.runInContext(fs.readFileSync(path.join(__dirname,'../web',file),'utf8'),context);
  const run=code=>vm.runInContext(code,context);
  run("session={account:{id:'one'},contentRevision:1};preferencesActor='one';preferences={version:1,style:'paper',mode:'light'};foregroundOpen=()=>false;render=()=>{};");
  return {run,poll:()=>poll(),release:version=>release({account:{id:'one'},contentRevision:1,preferences:{version,style:'paper',mode:'dark'}})};
}

for(const operation of ['poll','workspace'])test(`a delayed ${operation} snapshot cannot replace newer saved preferences`,async()=>{
  const f=fixture(),pending=operation==='poll'?f.poll():f.run('workspace()');
  f.run("preferences={version:3,style:'instrument',mode:'light'}");
  f.release(2);await pending;
  assert.equal(f.run('preferences.version'),3);
  assert.equal(f.run('preferences.style'),'instrument');
});

test('a newer remote preference snapshot still applies',async()=>{
  const f=fixture(),pending=f.poll();f.release(2);await pending;
  assert.equal(f.run('preferences.version'),2);
  assert.equal(f.run('preferences.mode'),'dark');
});
