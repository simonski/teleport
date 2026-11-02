const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function fixture(){
  const handlers={},state={foreground:false,edited:null},body={};
  const context=vm.createContext({session:{account:{id:'one'}},loadGeneration:1,view:'Board',document:{body,activeElement:body,addEventListener:(name,fn)=>handlers[name]=fn},foregroundOpen:()=>state.foreground,editTicket:id=>state.edited=id});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/tickets.js'),'utf8'),context);
  return {context,state,handlers,run:source=>vm.runInContext(source,context)};
}

test('board arrows preserve row position, skip empty lanes and stop at bounds',()=>{
  const f=fixture();
  for(const [id,key,expected] of [['a','ArrowDown','b'],['b','ArrowDown','b'],['a','ArrowUp','a'],['b','ArrowRight','c'],['c','ArrowLeft','a'],['unknown','ArrowLeft','a']]){
    f.context.input={id,key};assert.equal(f.run("boardArrowTarget([['a','b'],[],['c']],input.id,input.key)"),expected);
  }
  assert.equal(f.run("boardArrowTarget([[],[]],null,'ArrowDown')"),null);
});

test('ticket defaults follow view and Board intent without changing the hierarchy',()=>{
  const f=fixture();
  for(const [view,lens,kind,human] of [['Goals','agent','goal',false],['Epics','agent','epic',false],['Todos','agent','task',true],['Board','human','task',true],['Board','both','task',false]]){
    f.context.input={view,lens};const result=f.run('ticketCreationContext(input.view,input.lens)');assert.equal(result.kind,kind);assert.equal(result.human,human);
  }
  assert.equal(f.run("ticketCreationContext('Tickets','agent','human').human"),true);
  assert.equal(f.run("ticketCreationContext('Tickets','human','agent').human"),false);
});

test('board edit respects foreground ownership, input, composing and modifier keys',()=>{
  const f=fixture(),button={dataset:{ticket:'7'}},target={closest:selector=>selector.startsWith('.board')?button:null};
  const key=overrides=>({key:'e',target,preventDefault(){this.defaultPrevented=true;},...overrides});
  for(const event of [key({isComposing:true}),key({ctrlKey:true}),key({metaKey:true}),key({altKey:true}),key({target:{closest:()=>({})}})])f.handlers.keydown(event);
  f.state.foreground=true;f.handlers.keydown(key());assert.equal(f.state.edited,null);
  f.state.foreground=false;const event=key();f.handlers.keydown(event);assert.equal(f.state.edited,7);assert.equal(event.defaultPrevented,true);
});
