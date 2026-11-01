"use strict";
let graphRuntime = null, graphData = null, graphPending = {}, graphInteracting = false, graphQuery = '', graphKind = '', graphEpoch = 0, graphSelection = '';
const graphCameras = new Map();
function graphScopeKey() { return `${session?.account.id}:${projectId}`; }
function leaveGraph() { graphEpoch++; if (graphRuntime) { graphCameras.set(graphRuntime.scope, graphRuntime.scene.snapshot()); graphRuntime.scene.dispose(); graphRuntime = null; } graphInteracting = false; }
function graphBusy() { return view === 'Graph' && (graphInteracting || Object.keys(graphPending).length > 0); }
function graphView() {
  return `<div class="graph-heading"><div><p class="eyebrow">RELATIONSHIPS / ${escapeHTML(currentProject()?.name || 'ALL PROJECTS · PRIVATE ARRANGEMENT')}</p><h1>Graph<span>.</span></h1></div><form id="graph-filter"><label class="sr-only" for="graph-query">Search graph</label><input id="graph-query" type="search" placeholder="Find work in the graph" value="${escapeHTML(graphQuery)}"><label class="sr-only" for="graph-kind">Ticket type</label><select id="graph-kind"><option value="">All types</option>${['goal','epic','task'].map(kind=>`<option value="${kind}" ${graphKind===kind?'selected':''}>${kind}</option>`).join('')}</select><button class="secondary">Apply</button></form></div><div class="graph-toolbar"><button class="secondary" id="graph-fit">Fit</button><button class="secondary" id="graph-zoom-in" aria-label="Zoom graph in">＋</button><button class="secondary" id="graph-zoom-out" aria-label="Zoom graph out">−</button><button id="graph-save" disabled>Save arrangement</button><button class="secondary" id="graph-reload">Reload coordinates</button><span id="graph-state" role="status">Loading authorized graph…</span></div><div class="graph-workspace"><section id="graph-stage" aria-label="3D Graph"></section><aside class="graph-inspector"><p class="eyebrow">SELECT / INSPECT / ARRANGE</p><div id="graph-selection"><p>Select a node or its list entry.</p></div><h3>Entities</h3><div id="graph-list" class="graph-list" aria-label="Graph entities"></div></aside></div><p class="form-help">Drag to orbit; wheel to zoom; right-drag to pan. Shift-drag nodes to arrange. Arrows select, Enter opens, Alt-arrows arrange, Alt-Page Up/Down changes depth. Dashed nodes are required context. Arrangement never reparents work.</p>`;
}
async function mountGraph() {
  const epoch = ++graphEpoch, scope = graphScopeKey();
  graphPending = {};
  on('graph-filter', 'submit', e => { e.preventDefault(); graphQuery = byId('graph-query').value; graphKind = byId('graph-kind').value; render(); });
  on('graph-fit', 'click', () => graphRuntime?.scene.fit());
  on('graph-zoom-in', 'click', () => graphRuntime?.scene.zoom(0.8));
  on('graph-zoom-out', 'click', () => graphRuntime?.scene.zoom(1.25));
  on('graph-save', 'click', saveGraphArrangement);
  on('graph-reload', 'click', () => { graphPending = {}; render(); });
  try {
    const [data, module] = await Promise.all([api('getGraph', { projectId, query: graphQuery, kind: graphKind }), import('/graph_scene.js')]);
    if (epoch !== graphEpoch || view !== 'Graph' || scope !== graphScopeKey()) return;
    graphData = data; graphSelection = graphCameras.get(scope)?.selected || '';
    byId('graph-list').innerHTML = data.nodes.map(node=>`<button class="graph-entity secondary ${node.context?'context':''}" data-node="${escapeHTML(node.id)}"><span>${node.ticketId?'#'+node.ticketId: 'PROJECT'} / ${escapeHTML(node.kind)}</span><strong>${escapeHTML(node.title)}</strong></button>`).join('') || '<p>No matching entities. Narrowed searches retain required context.</p>';
    byId('graph-list').querySelectorAll('[data-node]').forEach(button=>button.onclick=()=>selectGraphNode(button.dataset.node));
    try { const scene = module.createGraphScene(byId('graph-stage'), data, { select: showGraphSelection, move: graphMoved, open: openGraphNode, interacting: value => { graphInteracting = value; } }, graphCameras.get(scope)); graphRuntime = { scope, scene }; }
    catch { byId('graph-stage').innerHTML = '<div class="graph-fallback"><h2>Explore through the entity list.</h2><p>WebGL is unavailable in this browser. Selection, editing and coordinate controls remain available beside the graph.</p></div>'; }
    byId('graph-state').textContent = `${data.nodes.length} entities · layout v${data.layout.version} · ${data.canSave ? projectId ? 'shared project arrangement' : 'private All projects arrangement' : 'Viewer · navigation only'}`;
    showGraphSelection(graphSelection);
  } catch (error) { if (epoch === graphEpoch && view === 'Graph') byId('graph-state').textContent = error.message; }
}
function selectGraphNode(id) { graphSelection = id; if (graphRuntime) graphRuntime.scene.select(id); else showGraphSelection(id); }
function showGraphSelection(id) {
  graphSelection = id;
  const node = graphData?.nodes.find(n=>n.id===id), panel = byId('graph-selection'); if (!panel) return;
  byId('graph-list')?.querySelectorAll('[data-node]').forEach(button=>button.setAttribute('aria-pressed', String(button.dataset.node===id)));
  if (!node) { panel.innerHTML = '<p>Select a node or its list entry.</p>'; return; }
  const point = graphPending[id] || node.position;
  panel.innerHTML = `<h3>${escapeHTML(node.title)}</h3><p>${escapeHTML(node.kind)} ${node.status?'· '+escapeHTML(node.status):''}${node.context?' · required context':''}</p><button class="secondary" id="graph-open">${node.ticketId?'Open ticket':'Open project'}</button>${graphData.canSave?`<fieldset class="graph-coordinates"><legend>World coordinates</legend>${['x','y','z'].map(axis=>`<label>${axis.toUpperCase()}<input data-coordinate="${axis}" type="number" min="-100000" max="100000" step="any" value="${Math.round(point[axis]*100)/100}"></label>`).join('')}<button class="secondary" id="graph-apply-position">Apply position</button></fieldset>`:'<p class="form-help">Project Viewers can navigate but cannot publish shared coordinates.</p>'}`;
  on('graph-open','click',()=>openGraphNode(id));
  on('graph-apply-position','click',()=>{const point={};panel.querySelectorAll('[data-coordinate]').forEach(input=>point[input.dataset.coordinate]=Number(input.value));if(Object.values(point).some(v=>!Number.isFinite(v)||Math.abs(v)>100000)){notice('Use finite coordinates within ±100,000.');return;}if(graphRuntime)graphRuntime.scene.move(id,point);else graphMoved(id,point);});
}
function graphMoved(id, point) { graphPending[id] = point; if (byId('graph-save')) byId('graph-save').disabled = false; if (byId('graph-state')) byId('graph-state').textContent = 'Unsaved arrangement · save explicitly; hierarchy unchanged'; if (graphSelection === id) showGraphSelection(id); }
async function saveGraphArrangement() {
  if (!graphData?.canSave || !Object.keys(graphPending).length) return;
  const epoch = graphEpoch, scope = graphScopeKey(), patch = {...graphPending};
  try { const result = await api('saveGraphLayout',{ projectId,expectedVersion:graphData.layout.version,positions:patch });if(epoch!==graphEpoch||scope!==graphScopeKey())return;for(const node of graphData.nodes)if(patch[node.id])node.position=patch[node.id];graphData.layout=result;for(const [id,point] of Object.entries(patch)){const pending=graphPending[id];if(pending&&['x','y','z'].every(axis=>pending[axis]===point[axis]))delete graphPending[id];}const dirty=Object.keys(graphPending).length>0;byId('graph-save').disabled=!dirty;byId('graph-state').textContent=dirty?'Earlier coordinates saved; newer local edits remain unsaved':`Arrangement saved · layout v${result.version}`; }
  catch(error){if(epoch===graphEpoch&&scope===graphScopeKey())byId('graph-state').textContent=error.message+'; local arrangement retained. Reload coordinates to discard it.';}
}
function openGraphNode(id) { const node=graphData?.nodes.find(n=>n.id===id);if(!node)return;if(node.ticketId)editTicket(node.ticketId);else{projectId=node.projectId;view='Board';workspace().catch(showError);} }
function costView() { return `<div class="page-heading"><div><p class="eyebrow">USAGE / ${escapeHTML(currentProject()?.name||'ALL AUTHORIZED PROJECTS')}</p><h1>Cost<span>.</span></h1></div></div><section class="cost-empty"><span class="eyebrow">NO OBSERVED PROVIDER USAGE</span><h2>No cost total is available.</h2><p>Provider metering has not been enabled. Demo stages make no model calls. External provider activity is unobserved.</p><p>Token counts, prices and totals will appear only when supported usage evidence is recorded.</p></section>`; }
