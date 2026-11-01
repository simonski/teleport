"use strict";
async function inspectNativeCapabilities() {
  const account = session?.account.id, generation = loadGeneration;
  try {
    const result = await api('getEffectivePermission', {projectId,adapter:'codex-structured',command:['git','status']});
    if (session?.account.id !== account || generation !== loadGeneration) return;
    const dialog = openDialog('Native execution readiness', `<p>Managed Codex execution is disabled. This inspection starts no process and grants no execution permission.</p><p class="eyebrow">LOCAL SCHEMA INSPECTION / ${escapeHTML(result.capabilities.schemaInspectedVersion)}</p><p>Required evidence:</p><ul>${result.capabilities.missingProofs.map(proof=>`<li>${escapeHTML(proof)}</li>`).join('')}</ul>`, 'Close');
    byId('editor-form').onsubmit = e => {e.preventDefault();dialog.close();};
  } catch (error) {showError(error);}
}
