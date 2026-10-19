#!/usr/bin/env python3
"""Synthetic AXLR v2 dossiers for the public parity fixture.

Authored for AXLR, not taken from the private lab corpus. It imports the
pinned lab candidate only to place two dossiers exactly at and one byte
over the 24 KiB state bound. Usage:

    python3 scripts/jev/synthetic_v2_dossiers.py ../jev-judge-lab > /tmp/dossiers.jsonl
"""
import json, sys
sys.path.insert(0,sys.argv[1]+'/src')
from jev_judge.axlr_v2 import make_request
ok_exec=lambda out,code=0: json.dumps({"protocol_version":1,"tool":"local_exec","status":"completed","output":{"exit_code":code,"stdout":out,"stderr":""},"error":None},separators=(',',':'))
rows=[]
def add(id,family,dossier,**extra): rows.append(dict(id=id,family=family,dossier=dossier,**extra))
REDACTED_RESULT="Go redacts a JSON tool result value by value; the lab's text-wide KEY=value would swallow the fields after the secret"
tools=[{"name":"local_exec","arguments":{"program":"probe","args":["--html","<b>&</b>"],"env":{"API_KEY":"API_KEY=sk-abcdefghijklmnop","nested":{"TOKEN":"MY_TOKEN=abc123 next"}},"why":"check \"quotes\" \\ back"},"result":ok_exec("line1\nline2   sep   end; GITHUB_TOKEN=ghp_abcdefghijklmn tail",3)},
       {"name":"local_read","arguments":{"path":"notes/ñandú.md","limit":1.5,"offset":12345678901234567890},"result":"contenido \"citado\" con tab\t y control \u0001 y emoji 🚀"},
       {"name":"local_write","arguments":{"path":"x.txt","content":"","mode":"create"},"result":None},
       {"name":"local_edit","arguments":{"path":"y.txt"},"result":""}]
add("synthetic-final-escapes","final",{"prompt":"Revisa <script> & arregla el \"build\"\n\n[AXLR · recall] Memory of project:x: nada","answer":"Hecho: exit 3 en probe. Siguiente línea\\n literal. Bearer abcdefghijklmnop","tools":tools},go_divergence=REDACTED_RESULT)
clean=[dict(t,result=t["result"].replace("GITHUB_TOKEN=ghp_abcdefghijklmn ","")) if t["result"] else t for t in tools]
add("synthetic-final-escapes-clean-result","final",{"prompt":"Revisa <script> & arregla el \"build\"\n\n[AXLR · recall] Memory of project:x: nada","answer":"Hecho: exit 3 en probe. Siguiente línea\\n literal. Bearer abcdefghijklmnop","tools":clean})
add("synthetic-gate-escapes","gate",{"prompt":"Aplica el límite","answer":"Límite aplicado, PASSWORD=hunter2 queda fuera.","tools":clean})
add("synthetic-gate-no-tools","gate",{"prompt":"Hola","answer":"Hola.","tools":[]})
prefix="Las exportaciones del proyecto utilizan una codificación única para todos los ficheros generados por el servicio de informes, "
add("synthetic-sentence-shared-prefix","sentence",{"prompt":"Elige el formato","answer":"x","sentences":[prefix+"UTF-8 sin BOM",prefix+"UTF-16 con BOM","Los tests pasan"],"tools":clean[:1]})
add("synthetic-sentence-empty","sentence",{"prompt":"p","answer":"a","sentences":[],"tools":[]})
add("synthetic-sentence-fifteen","sentence",{"prompt":"p","answer":"a","sentences":[f"Frase candidata número {i} con texto suficiente" for i in range(15)],"tools":[]})
add("synthetic-sentence-sixteen","sentence",{"prompt":"p","answer":"a","sentences":[f"Frase candidata número {i} con texto suficiente" for i in range(16)],"tools":[]})
for origin,prompt in [("preference","Prefiero que los informes usen unidades métricas."),("feedback","No, te equivocaste: los informes usan unidades métricas."),("decision","Elegimos unidades métricas frente a imperiales para los informes.")]:
  add("synthetic-kind-"+origin,"kind",{"prompt":prompt,"answer":"Los informes usan unidades métricas"})
long_ref="project:fixture:entry:decision:"+"x"*220
mems=["project:fixture:entry:decision:bbb El servicio usa 6 workers.",long_ref+" Regla A y regla B independiente.","project:fixture:entry:constraint:aaa La documentación está en español e inglés."]
add("synthetic-supersede-long-ref","supersede",{"new_memory":"Regla A revisada y regla B conservada.","earlier_memories":mems})
add("synthetic-supersede-permuted","supersede",{"new_memory":"Regla A revisada y regla B conservada.","earlier_memories":[mems[2],mems[0],mems[1]]})
add("synthetic-supersede-kind-prefix","supersede",{"new_memory":"Las fechas se guardan en UTC.","earlier_memories":["project:x:entry:decision:old (decision): las fechas se guardan en hora local","project:x:entry:constraint:k (constraint): otra  con  dobles espacios "]})
add("synthetic-supersede-duplicate-ref","supersede",{"new_memory":"n","earlier_memories":["project:x:entry:decision:a uno","project:x:entry:decision:a dos"]},go_only_error="a repeated ref is ambiguous; the lab would offer it twice")
add("synthetic-supersede-no-text","supersede",{"new_memory":"n","earlier_memories":["project:x:entry:decision:a"]})
add("synthetic-supersede-none","supersede",{"new_memory":"n","earlier_memories":[]})
add("synthetic-supersede-fifteen","supersede",{"new_memory":"n","earlier_memories":[f"project:x:entry:decision:{i:02d} memoria {i}" for i in range(15)]})
add("synthetic-supersede-sixteen","supersede",{"new_memory":"n","earlier_memories":[f"project:x:entry:decision:{i:02d} memoria {i}" for i in range(16)]})
# state exactly at the 24 KiB bound and one byte over
base={"prompt":"Comprueba","answer":"","tools":[]}
q,_=make_request("gate",base); room=24576-len(q["State"].encode())
add("synthetic-gate-at-bound","gate",dict(base,answer="a"*room))
add("synthetic-gate-over-bound","gate",dict(base,answer="a"*(room+1)))
add("synthetic-gate-multibyte-over-bound","gate",dict(base,answer="ñ"*(room//2)+"a"))
for r in rows: print(json.dumps(r,ensure_ascii=False))
