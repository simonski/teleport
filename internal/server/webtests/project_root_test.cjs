const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

test('project settings show the root and grant it without a worktree input',async()=>{
  let body='',request;
  const form={};
  const context=vm.createContext({
    currentProject:()=>({id:7,version:2,name:'Flight',root:'~/code/flight'}),
    roleAllowsMaintain:()=>true,session:{account:{administrator:true}},escapeHTML:String,
    openDialog:(title,html)=>{body=html;return {close(){}}},
    byId:()=>form,FormData:class { *[Symbol.iterator](){yield ['repository','~/code/flight']} },
    api:async(op,data)=>{request={op,data}},workspace:async()=>{},notice(){},showError(e){throw e}
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/delivery.js'),'utf8'),context);
  const settings=vm.runInContext('repositoryHTML()',context);
  assert.match(settings,/Project root/);assert.match(settings,/~\/code\/flight/);assert.doesNotMatch(settings,/Worktrees|storageRoot/);
  vm.runInContext('bindingForm()',context);
  assert.match(body,/Project root/);assert.doesNotMatch(body,/worktree|storageRoot/i);
  await form.onsubmit({preventDefault(){},currentTarget:form});
  assert.equal(request.op,'configureLocalRepository');assert.equal(request.data.repository,'~/code/flight');assert.equal('storageRoot' in request.data,false);
});
