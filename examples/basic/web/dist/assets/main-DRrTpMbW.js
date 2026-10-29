const t=document.getElementById("vite-demo");let e=0;t&&(t.textContent="Vite bundle loaded — click me",t.addEventListener("click",()=>{e+=1,t.textContent=`Vite counted ${e} click${e===1?"":"s"}`}));
