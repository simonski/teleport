// Pure presentation rules; arrangement never changes the ticket hierarchy.
export const CARD_WIDTH = 248, CARD_HEIGHT = 72;

export function layoutGraph2D(graph) {
  const nodes=new Map(graph.nodes.map(node=>[node.id,node])),children=new Map(),parents=new Set(),positions=new Map(),visited=new Set();
  for(const edge of graph.edges)if(edge.kind==='hierarchy'&&nodes.has(edge.source)&&nodes.has(edge.target)){
    if(!children.has(edge.source))children.set(edge.source,[]);
    children.get(edge.source).push(edge.target);parents.add(edge.target);
  }
  let row=0;
  function visit(id,depth){
    if(visited.has(id))return positions.get(id)?.y??0;
    visited.add(id);
    const descendants=(children.get(id)||[]).filter(child=>!visited.has(child));
    const rows=descendants.map(child=>visit(child,depth+1));
    const y=rows.length?(Math.min(...rows)+Math.max(...rows))/2:row++*96;
    positions.set(id,{x:depth*344,y,z:0});return y;
  }
  for(const node of graph.nodes)if(!parents.has(node.id))visit(node.id,0);
  for(const node of graph.nodes)if(!visited.has(node.id))visit(node.id,0);
  const middle=Math.max(0,row-1)*48;
  for(const [id,point] of positions)positions.set(id,graph.layout.positions[id]||{x:point.x,y:middle-point.y,z:0});
  return positions;
}

export function graphNeighborhood(graph,id){
  if(!id)return null;
  const related=new Set([id]);
  for(const edge of graph.edges){if(edge.source===id)related.add(edge.target);if(edge.target===id)related.add(edge.source);}
  return related;
}

export function graphCurvePoints(source,target){
  const start={x:source.x+CARD_WIDTH/2,y:source.y},end={x:target.x-CARD_WIDTH/2,y:target.y};
  const bend=Math.max(56,Math.abs(end.x-start.x)*.5);
  return [start,{x:start.x+bend,y:start.y},{x:end.x-bend,y:end.y},end];
}

export const graphNodeIcons={
  project:'M3 7h7l2 3h9v10H3ZM3 7V4h7l2 3',
  goal:'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18M12 7a5 5 0 1 0 0 10 5 5 0 0 0 0-10M12 11v2',
  epic:'m3 7 9-4 9 4-9 4ZM3 12l9 4 9-4M3 17l9 4 9-4',
  task:'m12 3 9 5v9l-9 5-9-5V8ZM3 8l9 5 9-5M12 13v9M7.5 5.5l9 5',
  agent:'M8 5h8v3h4v12H4V8h4ZM12 2v3M8 12v3M16 12v3M9 18h6',
  gate:'m12 3 8 3v5c0 5-4 8-8 10-4-2-8-5-8-10V6ZM8 12l3 3 5-6',
  human:'M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8M4 21v-3c0-6 16-6 16 0v3',
};

export function workflowGraph2D(definition,canSave=false,trace=false){
  const nodes=definition.phases.map((phase,index)=>{
    const point=definition.layout?.[phase.id]||{x:35+(index%4)*320,y:40+Math.floor(index/4)*175};
    const kind=['Approval','Clarify','Ready'].includes(phase.role)?'gate':phase.agentic?'agent':'human';
    return {id:phase.id,title:phase.title,kind,status:'',meta:`${phase.role} · ${phase.agentic?'Agent phase':'Human phase'}`,position:{x:point.x+115,y:-point.y-35,z:0}};
  });
  const ids=new Set(nodes.map(node=>node.id));
  const edges=definition.rules.filter(rule=>ids.has(rule.source)&&ids.has(rule.target)).map(rule=>({source:rule.source,target:rule.target,kind:'route',label:rule.outcome,directed:true,flowing:trace}));
  return {nodes,edges,canSave,layout:{version:definition.draftRevision,positions:Object.fromEntries(nodes.map(node=>[node.id,node.position]))}};
}
