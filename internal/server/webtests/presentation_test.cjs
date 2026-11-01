const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function fixture(source) {
  const handlers = {}, elements = {};
  let release;
  const context = vm.createContext({
    session: { account: { id: 'first' } }, loadGeneration: 1,
    projectId: 1, view: 'Settings', projects: [],
    graphCameras: new Map(), leaveGraph() {}, clearTimeout() {}, setTimeout() {}, Event,
    document: { documentElement: { dataset: {}, removeAttribute() {} }, dispatchEvent() {} },
    on(id, event, handler) { handlers[id] = handler; },
    byId(id) { return elements[id] ||= { value: id === 'appearance-style' ? 'paper' : id === 'appearance-mode' ? 'light' : id === 'preference-project' ? '0' : 'Home', disabled: false, textContent: '', classList: {toggle() {}} }; },
    api() { return new Promise(resolve => { release = resolve; }); },
    notice() {}, showError(error) { throw error; }, escapeHTML: String,
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../web', source), 'utf8'), context);
  return { context, handlers, elements, run: code => vm.runInContext(code, context), release: result => release(result) };
}

for (const [control, field, value] of [['appearance-style', 'style', 'instrument'], ['preference-view', 'defaultView', 'Board']]) {
  test(`${control} discards a delayed response after account switch`, async () => {
    const f = fixture('presentation.js');
    f.run("preferences={version:1,style:'paper',mode:'light',recent:[]};bindPresentation();");
    f.handlers[control]({target:{value}});
    const pending = f.run('flushPreferenceSave()');
    f.run("session={account:{id:'second'}};loadGeneration++;preferences={version:1,style:'instrument',mode:'dark',recent:[]};");
    f.release({ version: 2, style: 'paper', mode: 'light', recent: [{ projectId: 999 }] });
    await pending;
    assert.equal(f.run('preferences.style'), 'instrument');
    assert.equal(f.run('preferences.recent.length'), 0);
  });
}

test('autosave preserves a newer edit while the previous request is in flight', async () => {
  const f=fixture('presentation.js');
  f.run("preferencesActor='first';preferences={version:1,style:'paper',mode:'light',defaultView:'Home',defaultProject:0,recent:[]};bindPresentation();");
  f.handlers['appearance-mode']({target:{value:'dark'}});
  const pending=f.run('flushPreferenceSave()');
  f.handlers['appearance-mode']({target:{value:'light'}});
  f.release({version:2,style:'paper',mode:'dark',defaultView:'Home',defaultProject:0,recent:[]});
  await pending;
  assert.equal(f.run('preferenceDraft.mode'),'light');
  assert.equal(f.run('appearanceDirty'),true);
  assert.equal(f.context.document.documentElement.dataset.mode,'light');
  const next=f.run('flushPreferenceSave()');
  f.release({version:3,style:'paper',mode:'light',defaultView:'Home',defaultProject:0,recent:[]});
  await next;
  assert.equal(f.run('Object.keys(preferenceDraft).length'),0);
  assert.equal(f.run('appearanceDirty'),false);
  assert.equal(f.elements['preference-status'].textContent,'');
});

test('conflicting preferences retain the draft and require an explicit retry', async () => {
  const f=fixture('presentation.js');
  f.run("preferences={version:1,style:'paper',mode:'light'};bindPresentation();api=async()=>{const error=new Error('conflict');error.status=409;throw error;};");
  f.handlers['appearance-mode']({target:{value:'dark'}});
  await f.run('flushPreferenceSave()');
  assert.equal(f.run('preferenceDraft.mode'),'dark');
  assert.equal(f.run('preferenceConflict'),true);
  assert.equal(f.elements['preference-retry'].hidden,false);
  assert.match(f.elements['preference-status'].textContent,/elsewhere/);
});

test('a delayed graph save preserves edits made after submission', async () => {
  const f = fixture('graph.js');
  f.run("graphEpoch=3;graphData={canSave:true,layout:{version:0},nodes:[{id:'ticket:1',position:{x:0,y:0,z:0}}]};graphPending={'ticket:1':{x:10,y:0,z:0}};");
  const pending = f.run('saveGraphArrangement()');
  f.run("graphPending['ticket:1']={x:20,y:0,z:0};graphPending['ticket:2']={x:30,y:0,z:0};");
  f.release({ version: 1, positions: { 'ticket:1': { x: 10, y: 0, z: 0 } } });
  await pending;
  assert.equal(f.run("graphPending['ticket:1'].x"), 20);
  assert.equal(f.run("graphPending['ticket:2'].x"), 30);
  assert.equal(f.run('graphData.layout.version'), 1);
  assert.equal(f.elements['graph-save'].disabled, false);
});
