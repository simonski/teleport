const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  const handlers={},document={addEventListener:(type,fn)=>{(handlers[type]??=[]).push(fn);},documentElement:{},body:{children:[]}};
  const windowHandlers={};
  const context=vm.createContext({document,window:{addEventListener(type,fn){windowHandlers[type]=fn;}},HTMLElement:class {constructor(){this.handlers={};}addEventListener(type,fn){this.handlers[type]=fn;}},MutationObserver:class {observe(){}},customElements:{define(){}},crypto:{randomUUID:()=>"test"}});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/ui.js'),'utf8'),context);
  const items=[0,1].map(id=>({id,tabIndex:0,hidden:false,isConnected:true,matches:()=>false,closest:()=>null,getClientRects:()=>[{}],setAttribute(){},focus(){document.activeElement=this;}}));
  const dialog={querySelectorAll:()=>items,contains:target=>target===dialog||items.includes(target),focus(){document.activeElement=dialog;},close(){dialog.closed=true;}};
  context.dialog=dialog;context.items=items;document.activeElement=items[0];vm.runInContext('modalStack.push(dialog)',context);
  const key=(type,key,extra={})=>{const event={key,target:document.activeElement,preventDefault(){this.prevented=true;},stopImmediatePropagation(){this.stopped=true;},...extra};for(const fn of handlers[type]){fn(event);if(event.stopped)break;}return event;};
  return {context,document,dialog,items,key,handlers,windowHandlers,run:source=>vm.runInContext(source,context)};
}

test('modal containment stops application keys after local controls without preventing input',()=>{
  const f=fixture(),dialog=f.run('new TeleportDialog()');
  for(const type of ['keydown','keyup','keypress']){
    const event={stopped:false,stopPropagation(){this.stopped=true;},preventDefault(){throw Error('Local editing was prevented');}};
    dialog.handlers[type](event);assert.equal(event.stopped,true);
  }
  assert.equal(f.key('keydown','a').prevented,undefined);
});

test('modal Tab wraps in both directions and nested-selector Escape dismisses one layer',()=>{
  const f=fixture();f.document.activeElement=f.items[1];
  assert.equal(f.key('keydown','Tab').prevented,true);assert.equal(f.document.activeElement,f.items[0]);
  assert.equal(f.key('keydown','Tab',{shiftKey:true}).prevented,true);assert.equal(f.document.activeElement,f.items[1]);
  f.context.list={remove(){this.removed=true;}};f.run('openChoice={button:items[0],list}');
  const escape=f.key('keydown','Escape');assert.equal(escape.stopped,true);assert.equal(f.dialog.closed,undefined);assert.equal(f.context.list.removed,true);
  f.key('keydown','Escape');assert.equal(f.dialog.closed,true);
});

test('background keys and focus cannot bypass the foremost modal',()=>{
  const f=fixture(),outside={};
  for(const type of ['keydown','keyup','keypress']){
    const event=f.key(type,'n',{target:outside});assert.equal(event.prevented,true);assert.equal(event.stopped,true);assert.equal(f.document.activeElement,f.items[0]);
  }
  f.document.activeElement=outside;f.handlers.focusin[0]({target:outside});assert.equal(f.document.activeElement,f.items[0]);
});

test('disabled fieldset descendants are excluded from modal Tab stops',()=>{
  const f=fixture();f.items[0].matches=selector=>selector===':disabled';
  assert.equal(f.run('modalFocusables(dialog).length'),1);assert.equal(f.run('modalFocusables(dialog)[0]'),f.items[1]);
});

test('Escape still dismisses when a replaced form temporarily leaves focus on the body',()=>{
  const f=fixture(),event=f.key('keydown','Escape',{target:{}});
  assert.equal(event.prevented,true);assert.equal(event.stopped,true);assert.equal(f.dialog.closed,true);
});

test('queued scrolls preserve a newly positioned choice; moving its anchor dismisses it',()=>{
  const f=fixture(),rect={left:10,top:20};let removed=false;
  f.context.choiceButton={getBoundingClientRect:()=>rect,setAttribute(){}};
  f.context.choiceList={contains:()=>false,remove(){removed=true;}};
  f.run('openChoice={button:choiceButton,list:choiceList,anchor:{left:10,top:20}}');
  f.windowHandlers.scroll({target:{}});
  assert.equal(removed,false);
  rect.top=30;f.windowHandlers.scroll({target:{}});
  assert.equal(removed,true);
  assert.equal(f.run('openChoice'),null);
});
