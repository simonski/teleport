"use strict";
function captureDialogCompletion(dialog) {
  const form = dialog.querySelector('form'), actor = session?.account.id, generation = loadGeneration, epoch = detailEpoch;
  return {
    current: () => session?.account.id === actor && generation === loadGeneration && epoch === detailEpoch && dialog.open && dialog.querySelector('form') === form,
    follow: () => session?.account.id === actor && epoch === detailEpoch && !foregroundOpen()
  };
}
