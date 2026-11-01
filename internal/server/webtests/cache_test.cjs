const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

test('account invalidation clears query, record, workflow and layout caches',()=>{
  let cleared=false;
  const context=vm.createContext({session:{account:{id:'first'}},root:{replaceChildren(){cleared=true;}},notice(){},clearTimeout(){},document:{documentElement:{removeAttribute(){}}}});
  vm.runInContext(`let graphData={secret:1},graphPending={secret:1},graphQuery='private graph query',graphKind='task',calendarState={query:'private calendar query'},calendarDirty=true,calendarEpoch=3,selected={title:'private ticket'},workflowMap={private:1},flow={instructions:'private draft'},flows=[{private:1}],flowDirty=true,flowDrag={},projects=[{}],tickets=[{}],ideas=[{}];const graphCameras=new Map([['first',{}]]);function leaveGraph(){};function resetShift(){}`,context);
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/presentation.js'),'utf8'),context);
  vm.runInContext('clearPresentation()',context);
  assert.equal(vm.runInContext('calendarState.query',context),'');
  assert.equal(vm.runInContext('graphQuery',context),'');
  assert.equal(vm.runInContext('calendarEpoch',context),4);
  assert.equal(vm.runInContext('selected',context),null);
  assert.equal(vm.runInContext('flow',context),null);
  assert.equal(vm.runInContext('flows.length+projects.length+tickets.length+ideas.length+graphCameras.size',context),0);
  assert.equal(cleared,true);
  assert.equal(context.session,null);
});
