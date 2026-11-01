const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function keyboard() {
  const handlers = {};
  let now = 0, foreground = false;
  const document = {hidden:false,addEventListener:(name,handler)=>handlers[name]=handler};
  const context = vm.createContext({document,window:{addEventListener:(name,handler)=>handlers[name]=handler},session:{},foregroundOpen:()=>foreground,performance:{now:()=>now},opened:0});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/planning.js'),'utf8'),context);
  vm.runInContext('openFinder=()=>opened++',context);
  function event(type,at,extra={}){now=at;handlers[type]({key:'Shift',code:'ShiftLeft',repeat:false,ctrlKey:false,altKey:false,metaKey:false,isComposing:false,...extra});}
  return {event,handlers,document,foreground:value=>foreground=value,count:()=>context.opened,tap:(down,up)=>{event('keydown',down);event('keyup',up);}};
}

test('Finder requires two completed short taps with keyups within 400ms',()=>{
  const k=keyboard();k.tap(0,20);assert.equal(k.count(),0);k.event('keydown',100);assert.equal(k.count(),0);k.event('keyup',120);assert.equal(k.count(),1);
  const held=keyboard();held.tap(0,400);held.tap(450,470);assert.equal(held.count(),0);
  const late=keyboard();late.tap(0,20);late.tap(440,460);assert.equal(late.count(),0);
});
test('Finder resets on intervening keys, overlapping Shifts and focus changes',()=>{
  const k=keyboard();k.tap(0,20);k.event('keydown',30,{key:'a',code:'KeyA'});k.tap(40,60);assert.equal(k.count(),0);
  const overlap=keyboard();overlap.event('keydown',0);overlap.event('keydown',10,{code:'ShiftRight'});overlap.event('keyup',20,{code:'ShiftRight'});overlap.event('keyup',30);overlap.tap(40,60);assert.equal(overlap.count(),0);
  const focus=keyboard();focus.tap(0,20);focus.handlers.focusin();focus.tap(40,60);assert.equal(focus.count(),0);
});
test('Finder never steals foreground, composing, hidden or repeated input',()=>{
  for(const reason of ['foreground','hidden','repeat','composition']){const k=keyboard();if(reason==='foreground')k.foreground(true);if(reason==='hidden')k.document.hidden=true;k.event('keydown',0,{repeat:reason==='repeat',isComposing:reason==='composition'});k.event('keyup',20,{isComposing:reason==='composition'});k.tap(40,60);assert.equal(k.count(),0,reason);}
});

test('an old planned span is omitted while its current deadline stays visible',()=>{
  const k=keyboard(); // Calendar renderer shares the same source fixture.
  const context=vm.createContext({document:{addEventListener(){}},window:{addEventListener(){}},escapeHTML:String});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/planning.js'),'utf8'),context);
  const svg=vm.runInContext("calendarSVG({kind:'task',progress:0,start:'2026-01-01',end:'2026-01-03',due:'2026-11-01'},calendarDay('2026-11-01'),calendarDay('2026-11-30'))",context);
  assert.equal(svg.includes('calendar-bar'),false);
  assert.equal(svg.includes('calendar-due'),true);
});
