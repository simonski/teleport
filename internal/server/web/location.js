"use strict";
let locationEpoch=0,locationOwner='',locationKnown='',locationPending=null,locationInFlight=null,locationPromise=null;
function resetLocationWriter(){locationEpoch++;locationOwner='';locationKnown='';locationPending=null;locationInFlight=null;locationPromise=null;}
function locationCacheKey(account){return 'teleport-location:'+account.installationId+':'+account.account.id;}
function readLocationCache(account){if(typeof browserLocationAllowed!=='function'||!browserLocationAllowed())return null;try{return JSON.parse(localStorage.getItem(locationCacheKey(account))||'null');}catch{return null;}}
function writeLocationCache(account,value){if(typeof browserLocationAllowed!=='function'||!browserLocationAllowed())return;try{localStorage.setItem(locationCacheKey(account),JSON.stringify(value));}catch{/* Server persistence remains available when browser storage is disabled. */}}
function validLocation(context,available,account=session){return context&&supportedNavigation.includes(context.view)&&(context.view!=='Agents'||!!account?.account?.administrator)&&Number.isSafeInteger(context.projectId)&&context.projectId>=0&&(context.projectId===0||available.some(project=>project.id===context.projectId));}
function chooseInitialLocation(account,available){
  const cache=readLocationCache(account),fallback={view:account.preferences.defaultView,projectId:account.preferences.defaultProject};
  return [cache?.pending?cache:null,account.lastLocation,cache,fallback,{view:'Home',projectId:0}].map(context=>context?{...context,view:typeof canonicalView==='function'?canonicalView(context.view):context.view}:context).find(context=>validLocation(context,available,account));
}
function rememberCurrentLocation(force=false){
  if(!session||!validLocation({view,projectId},projects))return;
  const owner=locationCacheKey(session),value={view,projectId},signature=JSON.stringify(value);
  if(owner!==locationOwner){resetLocationWriter();locationOwner=owner;locationKnown=session.lastLocation?JSON.stringify({view:session.lastLocation.view,projectId:session.lastLocation.projectId}):'';}
  if(locationInFlight?.signature===signature){locationPending=null;return;}
  if(!force&&!locationInFlight&&!locationPending&&locationKnown===signature){writeLocationCache(session,{...value,pending:false});return;}
  writeLocationCache(session,{...value,pending:true});locationPending={...value,signature,owner,actor:session.account.id,installation:session.installationId};persistCurrentLocation();
}
function persistCurrentLocation(){
  if(locationPromise)return locationPromise;
  const epoch=locationEpoch,current=intent=>epoch===locationEpoch&&session?.account.id===intent.actor&&session?.installationId===intent.installation;
  locationPromise=(async()=>{
    while(locationPending){const intent=locationPending;locationPending=null;if(!current(intent))break;locationInFlight=intent;
      try{await api('saveCurrentLocation',{view:intent.view,projectId:intent.projectId});if(!current(intent))break;locationKnown=intent.signature;const cache=readLocationCache(session);if(cache?.view===intent.view&&cache.projectId===intent.projectId)writeLocationCache(session,{view:intent.view,projectId:intent.projectId,pending:false});}
      catch{if(current(intent)&&!locationPending)locationPending=intent;break;}
      finally{if(epoch===locationEpoch)locationInFlight=null;}
    }
  })().finally(()=>{if(epoch===locationEpoch)locationPromise=null;});return locationPromise;
}
async function flushCurrentLocation(){rememberCurrentLocation(true);await Promise.race([persistCurrentLocation(),new Promise(resolve=>setTimeout(resolve,1500))]);}
