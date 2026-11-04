const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  const events={},state={foreground:false};
  const context=vm.createContext({preferences:{navigationPosition:'left',navigationCollapsed:true},preferenceDraft:{},session:{account:{id:'one'}},view:'Home',byId:()=>null,foregroundOpen:()=>state.foreground,window:{addEventListener(){}},document:{documentElement:{dataset:{}},addEventListener:(name,fn)=>events[name]=fn},queueMicrotask,performance});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/navigation.js'),'utf8'),context);
  return {context,state,events,run:code=>vm.runInContext(code,context)};
}

test('reordering stable view IDs preserves hidden target views and the input order',()=>{
  const f=fixture();
  const value=f.run(`(()=>{const before=[...canonicalNavigation],after=moveNavigation(before,'Backlog','Graph',true);return {before,after};})()`);
  assert.equal(value.before[1],'Backlog');
  assert.equal(value.after[value.after.indexOf('Graph')+1],'Backlog');
  assert.equal(new Set(value.after).size,10);
  assert.ok(value.after.includes('Workflows')&&value.after.includes('Evolution'));
});

test('visible tab names are lowercase while persisted view identifiers remain stable',()=>{
  const f=fixture();f.run("view='Todos'");
  const html=f.run('navigationHTML()');
  const buttons=[...html.matchAll(/<button[^>]*data-view="([^"]+)"[^>]*aria-label="([^"]+)"[^>]*>(.*?)<\/button>/g)];
  assert.deepEqual(buttons.map(match=>[match[1],match[2]]),[
    ['Home','home'],['Backlog','backlog'],['Board','board'],['Todos','todo'],
    ['Calendar','calendar'],['Graph','graph'],['Workflows','workflows'],['Cost','cost / usage'],
  ]);
  for(const match of buttons)assert.ok(match[3].includes(`<span class="tab-label">${match[2]}</span>`));
  assert.match(html,/data-view="Todos"[^>]*aria-current="page"/);
});

test('Agents navigation is shown only to an installation administrator',()=>{
  const f=fixture();assert.equal(f.run('navigationHTML().includes(\'data-view="Agents"\')'),false);
  f.run('session.account.administrator=true');assert.equal(f.run('navigationHTML().includes(\'data-view="Agents"\')'),true);
});

test('remote layout changes defer for foreground ownership and reset without actor leakage',()=>{
  const f=fixture();f.state.foreground=true;f.run('applyNavigation()');
  assert.equal(f.context.document.documentElement.dataset.navigation,undefined);
  assert.equal(f.run('navigationDeferred'),true);
  f.state.foreground=false;f.run('applyNavigation()');
  assert.equal(f.context.document.documentElement.dataset.navigation,'left');
  assert.equal(f.context.document.documentElement.dataset.navigationCollapsed,'true');
  f.run('resetNavigation();session=null;applyNavigation()');
  assert.equal(f.context.document.documentElement.dataset.navigation,undefined);
  assert.equal(f.run('navigationDeferred'),false);
});

test('an explicit local layout preview may update behind its owning settings popup',()=>{
  const f=fixture();f.state.foreground=true;
  f.run("preferenceDraft={navigationPosition:'right'};applyNavigation()");
  assert.equal(f.context.document.documentElement.dataset.navigation,'right');
});

test('a local order draft does not pull a remote position change into a foreground editor',()=>{
  const f=fixture();f.run('applyNavigation()');f.state.foreground=true;
  f.run("preferences.navigationPosition='right';preferenceDraft={navigationOrder:[...canonicalNavigation].reverse()};applyNavigation()");
  assert.equal(f.context.document.documentElement.dataset.navigation,'left');
  assert.equal(f.run('navigationDeferred'),true);
  f.state.foreground=false;f.run('preferenceDraft={};applyNavigation()');
  assert.equal(f.context.document.documentElement.dataset.navigation,'right');
});
