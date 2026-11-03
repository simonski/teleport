"use strict";
function positionAccountMenu(){
  const menu=byId('account-menu'),button=byId('account');if(!menu||menu.hidden||!button)return;
  const box=button.getBoundingClientRect(),mode=document.documentElement.dataset.navigation||'top',width=menu.offsetWidth,height=menu.offsetHeight,margin=12;
  const preferred=mode==='left'?box.right+12:mode==='right'?box.left-width-12:box.right-width;
  const left=Math.max(margin,Math.min(innerWidth-width-margin,preferred)),overlaps=left<box.right&&left+width>box.left;
  menu.style.left=`${left}px`;menu.style.right='auto';menu.style.top=`${Math.max(margin,Math.min(innerHeight-height-margin,mode==='top'||overlaps?box.bottom+8:box.top))}px`;
  menu.dataset.openSide=mode==='left'?'right':'left';
}
window.addEventListener('resize',positionAccountMenu);
