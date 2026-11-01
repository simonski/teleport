const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = name => fs.readFileSync(path.join(__dirname, '../web', name), 'utf8');

test('dialog completion refuses replacement forms and newer foreground work', () => {
  let form = {}, foreground = false;
  const dialog = { open: true, querySelector: () => form };
  const context = vm.createContext({session:{account:{id:'first'}},loadGeneration:1,detailEpoch:4,foregroundOpen:()=>foreground,dialog});
  vm.runInContext(source('dialogs.js'), context);
  const guard = vm.runInContext('captureDialogCompletion(dialog)', context);
  assert.equal(guard.current(), true);
  form = {}; // Same shared dialog element, different unsaved form.
  assert.equal(guard.current(), false);
  foreground = true;
  assert.equal(guard.follow(), false);
  foreground = false;
  vm.runInContext('detailEpoch++', context);
  assert.equal(guard.follow(), false);
});

test('a late mutation response cannot close a replacement dialog or enter another account', async () => {
  let form = {}, release;
  const dialog = {open:true,querySelector:()=>form};
  const context = vm.createContext({
    session:{account:{id:'first'}},loadGeneration:1,
    document:{querySelector:selector=>selector==='dialog[open]'?dialog:{}},
    workspace:async()=>{},fetch:()=>new Promise(resolve=>release=resolve),
  });
  vm.runInContext(source('app.js'), context);
  const pending = vm.runInContext("api('configureGoalReview',{id:1})", context);
  form = {}; // Dismiss and open another form while the request is delayed.
  vm.runInContext("session={account:{id:'second'}};loadGeneration++",context);
  release({ok:true,json:async()=>({id:1,version:2})});
  await assert.rejects(pending, error => error.staleInteraction === true);
  assert.equal(dialog.open,true);
});

test('a late content read is rejected after actor or view invalidation',async()=>{
  let release;
  const context=vm.createContext({session:{account:{id:'first'}},loadGeneration:1,document:{querySelector:selector=>selector==='dialog[open]'?null:{}},workspace:async()=>{},fetch:()=>new Promise(resolve=>release=resolve)});
  vm.runInContext(source('app.js'),context);
  const pending=vm.runInContext("api('listWorkflows',{})",context);
  vm.runInContext("session={account:{id:'second'}};loadGeneration++",context);
  release({ok:true,json:async()=>({items:[{instructions:'first account private content'}]})});
  await assert.rejects(pending,error=>error.staleInteraction===true);
});
