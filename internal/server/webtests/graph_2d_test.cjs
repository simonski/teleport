const {test}=require('node:test');
const assert=require('node:assert/strict');

const fs=require('node:fs');
const path=require('node:path');
const model=import('data:text/javascript;base64,'+fs.readFileSync(path.join(__dirname,'../web/graph_2d_model.js')).toString('base64'));
function fixture(){
  return {nodes:['project','goal','epic','one','two','three','other'].map(id=>({id})),edges:[['project','goal'],['goal','epic'],['epic','one'],['epic','two'],['epic','three']].map(([source,target])=>({source,target,kind:'hierarchy'})),layout:{positions:{}}};
}

test('hierarchy fan-out is deterministic, centered and separate from saved coordinates',async()=>{
  const {layoutGraph2D}=await model,graph=fixture(),positions=layoutGraph2D(graph);
  assert.equal(positions.size,graph.nodes.length);
  assert.ok(positions.get('project').x<positions.get('goal').x);
  assert.ok(positions.get('goal').x<positions.get('epic').x);
  assert.ok(positions.get('epic').x<positions.get('one').x);
  assert.equal(positions.get('epic').y,(positions.get('one').y+positions.get('three').y)/2);
  assert.deepEqual(positions,layoutGraph2D(graph));
  graph.layout.positions.one={x:123,y:456,z:0};
  assert.deepEqual(layoutGraph2D(graph).get('one'),{x:123,y:456,z:0});
  assert.equal(positions.get('one').z,0);
});

test('curves meet card sides with horizontal tangents, including reversed dependencies',async()=>{
  const {graphCurvePoints,CARD_WIDTH}=await model;
  for(const target of [{x:400,y:200},{x:-400,y:-200}]){
    const points=graphCurvePoints({x:0,y:0},target);
    assert.equal(points[0].x,CARD_WIDTH/2);assert.equal(points[3].x,target.x-CARD_WIDTH/2);
    assert.equal(points[0].y,points[1].y);assert.equal(points[2].y,points[3].y);
    assert.ok(points[1].x>points[0].x);assert.ok(points[2].x<points[3].x);
  }
});

test('hover neighborhood includes real incident relationships without inventing sibling edges',async()=>{
  const {graphNeighborhood}=await model,graph=fixture();
  graph.edges.push({source:'one',target:'other',kind:'dependency'});
  assert.deepEqual([...graphNeighborhood(graph,'one')],['one','epic','other']);
  assert.equal(graphNeighborhood(graph,''),null);
});

test('a maximum-sized fan-out has unique finite bounded coordinates',async()=>{
  const {layoutGraph2D}=await model;
  const nodes=Array.from({length:2000},(_,id)=>({id:String(id)}));
  const graph={nodes,edges:nodes.slice(1).map(node=>({source:'0',target:node.id,kind:'hierarchy'})),layout:{positions:{}}};
  const positions=layoutGraph2D(graph);assert.equal(positions.size,2000);
  for(const point of positions.values())assert.ok(Object.values(point).every(value=>Number.isFinite(value)&&Math.abs(value)<=100000));
});

test('workflow map retains backward routes, gate icons and stable phase coordinates',async()=>{
  const {workflowGraph2D}=await model;
  const definition={draftRevision:4,phases:[{id:'doing',title:'Implement',role:'Doing',agentic:true},{id:'approval',title:'Approve',role:'Approval',agentic:false}],rules:[{source:'doing',target:'approval',outcome:'success'},{source:'approval',target:'doing',outcome:'rework'},{source:'doing',action:'wait',outcome:'blocked'}],layout:{doing:{x:12,y:24}}};
  const before=JSON.stringify(definition),graph=workflowGraph2D(definition,false,true);
  assert.equal(graph.edges.length,2);assert.ok(graph.edges.some(edge=>edge.source==='approval'&&edge.target==='doing'));
  assert.equal(graph.nodes[1].kind,'gate');assert.equal(graph.canSave,false);
  assert.deepEqual(graph.nodes[0].position,{x:127,y:-59,z:0});
  assert.equal(JSON.stringify(definition),before);
});
