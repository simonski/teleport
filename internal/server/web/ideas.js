"use strict";
let ideas = [], ideaInboxOnly = false, ideaHidden = false, detailEpoch = 0;

function ideaCanWrite(idea) {
  return idea.placement === 'inbox' ? session.account.administrator || idea.creator === session.account.id : roleAllowsWrite(idea.projectId);
}
function ideaCanControl(idea) {
  return idea.placement === 'inbox' ? ideaCanWrite(idea) : roleAllowsMaintain(idea.projectId);
}
function ideasView() {
  const visible = ideas.filter(i => (!ideaInboxOnly || i.placement === 'inbox') && (ideaHidden || !i.archived && !i.deleted));
  return `<div class="page-heading view-actions"><button id="new-idea">＋ New idea</button></div>
    <div class="idea-filters">${!projectId ? `<label class="checkbox-label"><input id="idea-inbox" type="checkbox" ${ideaInboxOnly ? 'checked' : ''}>Private inbox only</label>` : ''}<label class="checkbox-label"><input id="idea-hidden" type="checkbox" ${ideaHidden ? 'checked' : ''}>Include archived and deleted</label></div>
    <div class="idea-grid">${visible.map(i => `<button class="idea-card" data-idea="${escapeHTML(i.id)}"><span class="eyebrow">${escapeHTML(i.id)} / ${i.placement === 'inbox' ? 'PRIVATE INBOX' : escapeHTML(projectName(i.projectId))}</span><h2>${escapeHTML(i.title)}</h2><p>${escapeHTML(i.text.slice(0, 240)) || 'No details'}</p><div class="idea-card-footer"><span>${i.acceptedCount ? `Accepted · ${i.acceptedCount} proposal${i.acceptedCount === 1 ? '' : 's'}` : 'Draft idea'}</span><span>${i.deleted ? 'Deleted' : i.archived ? 'Archived' : 'Open ↗'}</span></div></button>`).join('') || empty('No ideas yet.', '', 'first-idea', 'New idea')}</div>`;
}
function bindIdeas() {
  for (const id of ['new-idea', 'first-idea']) on(id, 'click', () => ideaForm());
  on('idea-inbox', 'change', e => { ideaInboxOnly = e.target.checked; render(); });
  on('idea-hidden', 'change', async e => { ideaHidden = e.target.checked; await workspace(); });
  root.querySelectorAll('[data-idea]').forEach(button => button.onclick = () => openIdea(button.dataset.idea));
}
function ideaForm(idea) {
  const requestId = crypto.randomUUID();
  const projectOptions = projects.filter(p => roleAllowsWrite(p.id)).map(p => `<option value="${p.id}" ${p.id === projectId ? 'selected' : ''}>${escapeHTML(p.name)}</option>`).join('');
  const dialog = openDialog(idea ? 'Edit idea' : 'New idea', `${!idea ? `<label>Placement<select name="projectId"><option value="0" ${!projectId ? 'selected' : ''}>Private inbox · creator and administrators</option>${projectOptions}</select></label><p class="form-help">Project placement shares the Idea and retained history with project members.</p>` : ''}<label>Title<input name="title" required maxlength="500" value="${escapeHTML(idea?.title || '')}"></label><label>Idea<textarea name="text" rows="7" maxlength="20000">${escapeHTML(idea?.text || '')}</textarea></label>`, idea ? 'Save idea' : 'Create idea');
  const completion=captureDialogCompletion(dialog);
  byId('editor-form').onsubmit = async e => {
    e.preventDefault(); const data = Object.fromEntries(new FormData(e.currentTarget));
    if (!idea) data.projectId = Number(data.projectId);
    try {
      const saved = await api('saveIdea', { ...data, ...(idea ? { id: idea.id, expectedVersion: idea.version } : { requestId }) });
      if(!completion.current())return;dialog.close(); await workspace();if(!completion.follow())return; await openIdea(saved.id); notice('Idea saved');
    } catch (error) { showError(error); }
  };
}
function proposalTargetsHTML(proposal) {
  return `<ol class="proposal-targets">${proposal.targets.map(target => target.unavailable ? '<li>Target unavailable under current permissions</li>' : target.create ? `<li><strong>Create ${escapeHTML(target.create.kind || 'task')}: ${escapeHTML(target.create.title)}</strong><span>${escapeHTML(projectName(target.create.projectId))} · Todo · ${target.create.agent === false ? 'Human' : 'Agent'}</span><p>${escapeHTML(target.create.description || '')}</p><small>${escapeHTML(target.create.criteria || '')}</small></li>` : `<li>Link existing ticket #${target.existingId} at v${target.existingVersion}; preserve its content and status.</li>`).join('')}</ol>`;
}
async function openIdea(id) {
  const epoch = ++detailEpoch, account = session?.account.id, scope = projectId;
  try {
    const [detail, history] = await Promise.all([api('getIdea', { id }), pages('getIdeaHistory', { id })]);
    if (epoch !== detailEpoch || session?.account.id !== account || scope !== projectId) return;
    const i = detail.idea, writable = ideaCanWrite(i), controls = ideaCanControl(i);
    const dialog = openDialog(`<span class="number">${escapeHTML(i.id)}</span> Idea`, `<div class="editor-meta"><span class="status-chip">${i.placement === 'inbox' ? 'Private inbox' : escapeHTML(projectName(i.projectId))}</span><span>v${i.version} · ${escapeHTML(i.creator)}</span>${i.acceptedCount ? '<span>Historically accepted</span>' : ''}</div><h3>${escapeHTML(i.title)}</h3><p class="idea-text">${escapeHTML(i.text)}</p><div class="status-actions">${writable ? '<button type="button" class="secondary" id="idea-edit">Edit source</button>' : ''}${writable && !i.archived && !i.deleted ? '<button type="button" id="idea-propose">Draft work proposal</button>' : ''}${controls ? `<button type="button" class="secondary" id="idea-place">Change placement</button><button type="button" class="secondary" id="idea-archive">${i.archived ? 'Restore archive' : 'Archive'}</button><button type="button" class="secondary" id="idea-delete">${i.deleted ? 'Restore deleted' : 'Delete'}</button>` : ''}</div><section class="idea-proposals"><h3>Proposed work</h3>${detail.proposals.map(p => {
      const acceptance = detail.acceptances.find(a => a.proposalId === p.id), stale = p.ideaVersion !== i.version;
      return `<article class="idea-proposal"><div class="eyebrow">${escapeHTML(p.actor)} / IDEA v${p.ideaVersion}</div>${proposalTargetsHTML(p)}${acceptance ? '<p class="accepted-label">Accepted · separate tickets retained below</p>' : writable && !i.archived && !i.deleted && !stale ? `<button type="button" data-accept-proposal="${escapeHTML(p.id)}">Review acceptance</button>` : `<p class="muted">${stale ? 'Source changed. Submit a fresh proposal.' : 'Human acceptance required.'}</p>`}</article>`;
    }).join('') || '<p class="muted">No proposal yet. Drafting creates no tickets.</p>'}</section><section><h3>Linked work</h3><div class="status-actions">${detail.links.map(link => `<button type="button" class="secondary" data-idea-ticket="${link.ticketId}">Ticket #${link.ticketId} ↗</button>`).join('') || '<p class="muted">No authorized linked work.</p>'}</div></section>${detail.hasOlder ? '<p class="form-help">Older proposals and links are retained through the paged history and API.</p>' : ''}<details class="history"><summary>History · ${history.length} events</summary>${history.map(h => `<div class="revision"><span>v${h.snapshot.version}</span><strong>${escapeHTML(h.action)}</strong><span>${escapeHTML(h.actor)}</span></div>`).join('')}</details>`, 'Close');
    byId('editor-form').onsubmit = e => { e.preventDefault(); dialog.close(); };
    on('idea-edit', 'click', () => { dialog.close(); ideaForm(i); });
    on('idea-propose', 'click', () => { dialog.close(); ideaProposalForm(i); });
    on('idea-place', 'click', () => { dialog.close(); ideaPlacementForm(i); });
    on('idea-archive', 'click', () => ideaVisibility(i, { archived: !i.archived }, dialog));
    on('idea-delete', 'click', () => ideaVisibility(i, { deleted: !i.deleted }, dialog));
    dialog.querySelectorAll('[data-accept-proposal]').forEach(button => button.onclick = () => { dialog.close(); ideaAcceptanceForm(i, detail.proposals.find(p => p.id === button.dataset.acceptProposal)); });
    dialog.querySelectorAll('[data-idea-ticket]').forEach(button => button.onclick = () => { dialog.close(); editTicket(Number(button.dataset.ideaTicket)); });
  } catch (error) { showError(error); }
}
function ideaProposalForm(idea) {
  const requestId = crypto.randomUUID();
  const writable = projects.filter(p => roleAllowsWrite(p.id));
  const dialog = openDialog('Draft a work proposal', `<p>Keep ${escapeHTML(idea.id)} unchanged. Human acceptance will create or link the exact proposed work.</p><label>Proposal type<select id="idea-target-kind"><option value="create">Create a new Todo ticket</option><option value="link">Link an existing ticket</option></select></label><div id="idea-create-fields"><label>Destination project<select name="projectId">${writable.map(p => `<option value="${p.id}" ${p.id === idea.projectId ? 'selected' : ''}>${escapeHTML(p.name)}</option>`).join('')}</select></label><div class="form-grid"><label>Type<select name="kind"><option value="task">Task</option><option value="epic">Epic</option><option value="goal">Goal</option></select></label><label>Intent<select name="intent"><option value="agent">Agent</option><option value="human">Human</option></select></label></div><label>Ticket title<input name="title" value="${escapeHTML(idea.title)}" maxlength="500"></label><label>Description<textarea name="description" rows="3">${escapeHTML(idea.text)}</textarea></label><label>Success criteria<textarea name="criteria" rows="2"></textarea></label><label>Existing parent ID (optional)<input name="parentId" type="number" min="1"></label></div><div id="idea-link-fields" hidden><label>Existing ticket ID<input id="idea-existing" type="number" min="1"></label></div>`, 'Save proposal');
  const completion=captureDialogCompletion(dialog);
  on('idea-target-kind', 'change', e => { byId('idea-create-fields').hidden = e.target.value !== 'create'; byId('idea-link-fields').hidden = e.target.value !== 'link'; });
  byId('editor-form').onsubmit = async e => {
    e.preventDefault();
    try {
      let target;
      if (byId('idea-target-kind').value === 'link') { const t = await api('getTicket', { id: Number(byId('idea-existing').value) }); target = { existingId: t.id, existingVersion: t.version }; }
      else { const data = Object.fromEntries(new FormData(e.currentTarget)); target = { create: { projectId: Number(data.projectId), parentId: Number(data.parentId) || 0, kind: data.kind, title: data.title, description: data.description, criteria: data.criteria, agent: data.intent === 'agent' } }; }
      await api('submitIdeaProposal', { id: idea.id, expectedVersion: idea.version, requestId, targets: [target] }); if(!completion.current())return;dialog.close(); await workspace();if(!completion.follow())return; await openIdea(idea.id); notice('Proposal saved; no work created');
    } catch (error) { showError(error); }
  };
}
function ideaAcceptanceForm(idea, proposal) {
  const dialog = openDialog('Accept this exact work proposal', `${proposalTargetsHTML(proposal)}<p>The Idea stays visible. New tickets start Todo; implementation readiness and code approval remain separate decisions.</p><label class="checkbox-label"><input id="idea-agree" type="checkbox" required>Create or link this exact proposed work</label>`, 'Accept proposal');
  const completion=captureDialogCompletion(dialog);
  byId('editor-form').onsubmit = async e => { e.preventDefault(); if (!byId('idea-agree').checked) return; try { await api('acceptIdeaProposal', { id: idea.id, expectedVersion: idea.version, proposalId: proposal.id }); if(!completion.current())return;dialog.close(); await workspace();if(!completion.follow())return; await openIdea(idea.id); notice('Proposal accepted; Idea retained'); } catch (error) { showError(error); } };
}
function ideaPlacementForm(idea) {
  const dialog = openDialog('Change Idea placement', `<label>Destination<select id="idea-placement"><option value="0">Creator's private inbox</option>${projects.filter(p => roleAllowsWrite(p.id)).map(p => `<option value="${p.id}" ${p.id === idea.projectId ? 'selected' : ''}>${escapeHTML(p.name)}</option>`).join('')}</select></label><p id="idea-publication-warning">Preview the access change before committing.</p><button type="button" class="secondary" id="idea-preview-placement">Preview access change</button><label class="checkbox-label"><input id="idea-publish-agree" type="checkbox" required disabled>Apply this publication/access change</label>`, 'Apply placement');
  let preview = null;
  on('idea-placement', 'change', () => { preview = null; byId('idea-publish-agree').checked = false; byId('idea-publish-agree').disabled = true; });
  on('idea-preview-placement', 'click', async () => { try { preview = await api('previewIdeaPlacement', { id: idea.id, expectedVersion: idea.version, projectId: Number(byId('idea-placement').value) }); byId('idea-publication-warning').textContent = preview.warning; byId('idea-publish-agree').disabled = false; } catch (error) { showError(error); } });
  byId('editor-form').onsubmit = async e => { e.preventDefault(); if (!preview || !byId('idea-publish-agree').checked) return; try { await api('setIdeaPlacement', { id: idea.id, expectedVersion: preview.version, projectId: preview.projectId, confirm: true }); dialog.close(); await workspace(); notice('Placement changed'); } catch (error) { showError(error); } };
}
async function ideaVisibility(idea, values, dialog) {
  try { await api('setIdeaVisibility', { id: idea.id, expectedVersion: idea.version, ...values }); dialog.close(); await workspace(); notice('Idea visibility changed; linked work retained'); } catch (error) { showError(error); }
}
