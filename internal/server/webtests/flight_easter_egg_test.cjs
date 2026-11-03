const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  const handlers={},windowHandlers={};let now=0;
  const context=vm.createContext({
    document:{addEventListener(type,fn){handlers[type]=fn;}},
    window:{addEventListener(type,fn){windowHandlers[type]=fn;}},
    session:{account:{id:'human'}},foregroundOpen:()=>false,performance:{now:()=>now},
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/flight_easter_egg.js'),'utf8'),context);
  vm.runInContext('let opened=0;showFlightEasterEgg=()=>opened++',context);
  const key=(key,extra={})=>{
    now+=100;
    const event={key,composedPath:()=>[],preventDefault(){this.prevented=true;},stopImmediatePropagation(){this.stopped=true;},...extra};
    handlers.keydown(event);return event;
  };
  return {context,handlers,windowHandlers,key,type:code=>[...code].map(letter=>key(letter)),run:source=>vm.runInContext(source,context),advance:amount=>now+=amount};
}

test('both case-insensitive codes activate once and contain Q before it can switch views',()=>{
  const f=fixture();
  for(const code of ['idkfa','IDDQD']){
    const events=f.type(code);
    assert.ok(events.every(event=>event.prevented&&event.stopped));
  }
  assert.equal(f.run('opened'),2);
  assert.equal(f.key('q').prevented,undefined);
});

test('noise, incomplete codes and slow typing never activate; a fresh overlapping prefix does',()=>{
  const f=fixture();f.type('idkfx');assert.equal(f.run('opened'),0);
  f.type('idd');f.advance(1801);f.type('qd');assert.equal(f.run('opened'),0);
  f.type('iidkfa');assert.equal(f.run('opened'),1);
});

test('editing, terminal and shadow-root input paths preserve typing and reset a partial code',()=>{
  for(const selector of ['input','textarea','select','[contenteditable]','[role="textbox"]','.xterm']){
    const f=fixture();f.type('id');
    const editing={matches:value=>value.includes(selector)};
    const event=f.key('k',{composedPath:()=>[{},editing]});
    assert.equal(event.prevented,undefined);f.type('fa');assert.equal(f.run('opened'),0);
  }
});

test('signed-out, composing, repeated, modified and modal keys never trigger the scene',()=>{
  for(const extra of [{ctrlKey:true},{metaKey:true},{altKey:true},{isComposing:true},{repeat:true},{defaultPrevented:true}]){
    const f=fixture();f.type('id');assert.equal(f.key('k',extra).prevented,undefined);f.type('fa');assert.equal(f.run('opened'),0);
  }
  for(const setup of ['session=null','foregroundOpen=()=>true']){
    const f=fixture();f.run(setup);assert.ok(f.type('iddqd').every(event=>!event.prevented));assert.equal(f.run('opened'),0);
  }
});

test('focus, pointer, visibility and window changes discard partial codes; Shift permits capitals',()=>{
  for(const name of ['focusin','pointerdown','visibilitychange','blur']){
    const f=fixture();f.type('id');(f.handlers[name]||f.windowHandlers[name])();f.type('kfa');assert.equal(f.run('opened'),0);
  }
  const f=fixture();f.type('ID');f.key('Shift');f.type('KFA');assert.equal(f.run('opened'),1);
});
